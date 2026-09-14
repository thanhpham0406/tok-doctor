package app

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/thanhpham0406/tok-doctor/internal/config"
	"github.com/thanhpham0406/tok-doctor/internal/gateway"
	"github.com/thanhpham0406/tok-doctor/internal/match"
	"github.com/thanhpham0406/tok-doctor/internal/model"
	"github.com/thanhpham0406/tok-doctor/internal/reconcile"
)

func newReconcileApp(t *testing.T) (*App, string) {
	t.Helper()
	dir := t.TempDir()
	gatewayDir := filepath.Join(dir, "gateway")
	t.Setenv("TOKDOCTOR_GATEWAY_DIR", gatewayDir)
	return NewWithStore(config.NewStoreAt(filepath.Join(dir, "config.toml"))), gatewayDir
}

func seedGatewayExchanges(t *testing.T, dir string, exchanges ...gateway.Exchange) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir gateway: %v", err)
	}
	recorder, err := gateway.NewFileRecorder(dir)
	if err != nil {
		t.Fatalf("new recorder: %v", err)
	}
	t.Cleanup(func() { _ = recorder.Close() })
	for _, exchange := range exchanges {
		if err := recorder.Record(exchange); err != nil {
			t.Fatalf("record %s: %v", exchange.ID, err)
		}
	}
}

func codexUsage() model.Usage {
	return model.MeasuredUsage(1200, 300, 450, model.Int64(0), 1950)
}

func syntheticCodexSession(at time.Time, usages ...model.Usage) model.Session {
	if len(usages) == 0 {
		usages = []model.Usage{codexUsage()}
	}
	turns := make([]model.Turn, 0, len(usages))
	for i, usage := range usages {
		stamp := at
		turns = append(turns, model.Turn{
			ID:        "sess-1#" + strconv.Itoa(i+1),
			Sequence:  i + 1,
			Timestamp: &stamp,
			Model:     "gpt-5",
			Usage:     usage,
		})
	}
	started := at
	updated := at
	return model.Session{
		ID:        "sess-1",
		Source:    "codex",
		Agent:     model.AgentCodex,
		Model:     "gpt-5",
		StartedAt: &started,
		UpdatedAt: &updated,
		Usage:     codexUsage(),
		Turns:     turns,
	}
}

func claudeUsage(fresh, cacheRead, cacheWrite, output int64) model.Usage {
	totalInput := fresh + cacheRead + cacheWrite
	return model.Usage{
		Input:    model.NewMeasurement(fresh, model.MeasurementMeasured),
		Cached:   model.NewMeasurement(cacheRead, model.MeasurementMeasured),
		Output:   model.NewMeasurement(output, model.MeasurementMeasured),
		Total:    model.NewMeasurement(totalInput+output, model.MeasurementDerived),
		Billable: model.NewBillableUsage(fresh, cacheRead, output, model.Int64(cacheWrite), model.MeasurementMeasured),
	}
}

func syntheticClaudeSession(at time.Time, usages ...model.Usage) model.Session {
	if len(usages) == 0 {
		usages = []model.Usage{claudeUsage(1000, 200, 50, 300)}
	}
	turns := make([]model.Turn, 0, len(usages))
	for i, usage := range usages {
		stamp := at
		turns = append(turns, model.Turn{
			ID:        "turn-" + strconv.Itoa(i+1),
			Sequence:  i + 1,
			Timestamp: &stamp,
			Model:     "claude-sonnet-4",
			Usage:     usage,
		})
	}
	started := at
	updated := at
	return model.Session{
		ID:        "claude-sess",
		Source:    "claude",
		Agent:     model.AgentClaude,
		Model:     "claude-sonnet-4",
		StartedAt: &started,
		UpdatedAt: &updated,
		Usage:     model.SumUsage(usages),
		Turns:     turns,
	}
}

func openAIExchange(id string, at time.Time, input, cached, output, total int64) gateway.Exchange {
	return gateway.Exchange{
		ID:         id,
		Profile:    "codex",
		SourceHint: "codex",
		Protocol:   gateway.ProtocolOpenAIResponses,
		StartedAt:  at,
		Kind:       gateway.RequestKindModel,
		Outcome:    gateway.OutcomeUpstreamOK,
		Model:      "gpt-5",
		Response: gateway.ExchangeResponse{
			Status: 200,
			Model:  "gpt-5",
			ProviderUsage: &gateway.ProviderUsage{
				Source:                string(gateway.ProtocolOpenAIResponses),
				InputTokens:           model.Int64(input),
				CacheReadInputTokens:  model.Int64(cached),
				OutputTokens:          model.Int64(output),
				ReasoningOutputTokens: model.Int64(0),
				TotalTokens:           model.Int64(total),
			},
		},
	}
}

func anthropicExchange(id string, at time.Time, fresh, cacheRead, cacheWrite, output int64) gateway.Exchange {
	return gateway.Exchange{
		ID:         id,
		Profile:    "claude",
		SourceHint: "claude",
		Protocol:   gateway.ProtocolAnthropicMessages,
		StartedAt:  at,
		Kind:       gateway.RequestKindModel,
		Outcome:    gateway.OutcomeUpstreamOK,
		Model:      "claude-sonnet-4",
		Response: gateway.ExchangeResponse{
			Status: 200,
			Model:  "claude-sonnet-4",
			ProviderUsage: &gateway.ProviderUsage{
				Source:                   string(gateway.ProtocolAnthropicMessages),
				InputTokens:              model.Int64(fresh),
				CacheReadInputTokens:     model.Int64(cacheRead),
				CacheCreationInputTokens: model.Int64(cacheWrite),
				OutputTokens:             model.Int64(output),
			},
		},
	}
}

func TestReconcileSessionCodexEqual(t *testing.T) {
	a, dir := newReconcileApp(t)
	at := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	seedGatewayExchanges(t, dir, openAIExchange("req-1", at.Add(2*time.Second), 1200, 300, 450, 1950))

	report, reason, turns, err := a.reconcileSession(syntheticCodexSession(at))
	if err != nil {
		t.Fatalf("reconcileSession: %v", err)
	}
	if reason != "" {
		t.Fatalf("reason = %q, want report", reason)
	}
	if report.Summary.GatewayRequests != 1 || report.Summary.TranscriptTurns != 1 {
		t.Fatalf("summary counts = %+v", report.Summary)
	}
	if report.Summary.Matched != 1 || report.Summary.Equal != 1 {
		t.Fatalf("summary = %+v, want one equal match", report.Summary)
	}
	if len(report.Reconciliations) != 1 || report.Reconciliations[0].Status != reconcile.StatusEqual {
		t.Fatalf("reconciliations = %+v, want equal", report.Reconciliations)
	}
	turnMatch := turns["sess-1#1"]
	if turnMatch.Status != match.StatusMatched || turnMatch.Result == nil {
		t.Fatalf("turn reconciliation = %+v, want matched result", turnMatch)
	}
}

func TestReconcileSessionClaudeEqual(t *testing.T) {
	a, dir := newReconcileApp(t)
	at := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	seedGatewayExchanges(t, dir, anthropicExchange("req-1", at.Add(2*time.Second), 1000, 200, 50, 300))

	report, reason, _, err := a.reconcileSession(syntheticClaudeSession(at))
	if err != nil {
		t.Fatalf("reconcileSession: %v", err)
	}
	if reason != "" {
		t.Fatalf("reason = %q, want report", reason)
	}
	if report.Summary.Matched != 1 || report.Summary.Equal != 1 {
		t.Fatalf("summary = %+v, want one equal match", report.Summary)
	}
}

func TestReconcileSessionDifferentMatch(t *testing.T) {
	a, dir := newReconcileApp(t)
	at := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	seedGatewayExchanges(t, dir, openAIExchange("req-1", at.Add(2*time.Second), 1200, 300, 500, 2000))

	report, reason, _, err := a.reconcileSession(syntheticCodexSession(at))
	if err != nil {
		t.Fatalf("reconcileSession: %v", err)
	}
	if reason != "" {
		t.Fatalf("reason = %q, want report", reason)
	}
	if report.Summary.Matched != 1 || report.Summary.Different != 1 {
		t.Fatalf("summary = %+v, want one different match", report.Summary)
	}
}

func TestReconcileSessionUnmatchedBothSides(t *testing.T) {
	a, dir := newReconcileApp(t)
	at := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	exchange := openAIExchange("req-1", at.Add(2*time.Second), 11, 2, 3, 14)
	exchange.Model = "other-model"
	exchange.Response.Model = "other-model"
	seedGatewayExchanges(t, dir, exchange)

	report, reason, turns, err := a.reconcileSession(syntheticCodexSession(at))
	if err != nil {
		t.Fatalf("reconcileSession: %v", err)
	}
	if reason != "" {
		t.Fatalf("reason = %q, want report", reason)
	}
	if report.Summary.UnmatchedGateways != 1 || report.Summary.UnmatchedTranscripts != 1 {
		t.Fatalf("summary = %+v, want unmatched both sides", report.Summary)
	}
	if turns["sess-1#1"].Status != match.StatusUnmatched {
		t.Fatalf("turn reconciliation = %+v, want unmatched", turns["sess-1#1"])
	}
}

func TestReconcileSessionAmbiguousGateway(t *testing.T) {
	a, dir := newReconcileApp(t)
	at := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	seedGatewayExchanges(t, dir, openAIExchange("req-1", at.Add(2*time.Second), 1200, 300, 450, 1950))

	report, reason, turns, err := a.reconcileSession(syntheticCodexSession(at, codexUsage(), codexUsage()))
	if err != nil {
		t.Fatalf("reconcileSession: %v", err)
	}
	if reason != "" {
		t.Fatalf("reason = %q, want report", reason)
	}
	if report.Summary.AmbiguousGateways != 1 || report.Summary.Matched != 0 {
		t.Fatalf("summary = %+v, want one ambiguous gateway", report.Summary)
	}
	for _, turnID := range []string{"sess-1#1", "sess-1#2"} {
		if turns[turnID].Status != match.StatusAmbiguous {
			t.Fatalf("turn %s reconciliation = %+v, want ambiguous", turnID, turns[turnID])
		}
	}
}

func TestReconcileSessionGatewayUnavailable(t *testing.T) {
	a, _ := newReconcileApp(t)
	at := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

	report, reason, _, err := a.reconcileSession(syntheticCodexSession(at))
	if err != nil {
		t.Fatalf("reconcileSession: %v", err)
	}
	if reason != "gateway capture not available" {
		t.Fatalf("reason = %q, want gateway capture not available", reason)
	}
	if report.Summary != (reconcile.Summary{}) {
		t.Fatalf("report summary = %+v, want zero", report.Summary)
	}
}

func TestReconcileSessionMissingTimestampIsUnavailable(t *testing.T) {
	a, _ := newReconcileApp(t)
	at := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	session := syntheticCodexSession(at)
	session.StartedAt = nil
	session.UpdatedAt = nil
	session.Turns[0].Timestamp = nil

	_, reason, _, err := a.reconcileSession(session)
	if err != nil {
		t.Fatalf("reconcileSession: %v", err)
	}
	if !strings.Contains(reason, "no reliable timestamp") {
		t.Fatalf("reason = %q, want timestamp reason", reason)
	}
}

func TestReconcileSessionUnsupportedSourceIsUnavailable(t *testing.T) {
	a, _ := newReconcileApp(t)
	at := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	session := syntheticCodexSession(at)
	session.Source = "router9"
	session.Agent = model.AgentRouter9

	_, reason, _, err := a.reconcileSession(session)
	if err != nil {
		t.Fatalf("reconcileSession: %v", err)
	}
	if !strings.Contains(reason, "not supported") {
		t.Fatalf("reason = %q, want unsupported source", reason)
	}
}

func TestReconcileSessionProjectorErrorHasContext(t *testing.T) {
	a, _ := newReconcileApp(t)
	at := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	session := syntheticCodexSession(at)
	session.Turns[0].ID = ""

	_, _, _, err := a.reconcileSession(session)
	if err == nil {
		t.Fatal("expected projector error")
	}
	if !strings.Contains(err.Error(), "project session sess-1 observations") || !strings.Contains(err.Error(), "turn id is required") {
		t.Fatalf("error = %v, want wrapped projector context", err)
	}
}

func TestReconcileSessionCorruptionIsUnavailableWithoutFailing(t *testing.T) {
	a, dir := newReconcileApp(t)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir gateway: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "codex.jsonl"), []byte("not-json\n"), 0o600); err != nil {
		t.Fatalf("write corrupt capture: %v", err)
	}
	at := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

	_, reason, _, err := a.reconcileSession(syntheticCodexSession(at))
	if err != nil {
		t.Fatalf("reconcileSession: %v", err)
	}
	if !strings.Contains(reason, "could not be read") {
		t.Fatalf("reason = %q, want read failure", reason)
	}
	if strings.Contains(reason, dir) {
		t.Fatalf("reason leaks private path: %q", reason)
	}
}

func TestReconcileSessionLimitsGatewayWindow(t *testing.T) {
	a, dir := newReconcileApp(t)
	at := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	seedGatewayExchanges(t, dir,
		openAIExchange("req-in-window", at.Add(2*time.Second), 1200, 300, 450, 1950),
		openAIExchange("req-out-of-window", at.Add(10*time.Hour), 1200, 300, 450, 1950),
	)

	session := syntheticCodexSession(at)
	report, reason, _, err := a.reconcileSession(session)
	if err != nil {
		t.Fatalf("reconcileSession: %v", err)
	}
	if reason != "" {
		t.Fatalf("reason = %q, want report", reason)
	}
	if report.Summary.GatewayRequests != 1 || report.Summary.Matched != 1 {
		t.Fatalf("summary = %+v, want only the in-window request", report.Summary)
	}
	if report.Summary.TranscriptTurns != len(session.Turns) {
		t.Fatalf("transcript turns = %d, want selected session turns %d", report.Summary.TranscriptTurns, len(session.Turns))
	}
}

func TestInspectReportReturnsSessionWhenGatewayUnavailable(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".codex", "sessions"), 0o755); err != nil {
		t.Fatalf("mkdir codex: %v", err)
	}
	copyTestFixture(t, filepath.Join("..", "..", "fixtures", "codex", "basic-session.jsonl"),
		filepath.Join(home, ".codex", "sessions", "basic-session.jsonl"))
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("TOKDOCTOR_GATEWAY_DIR", filepath.Join(t.TempDir(), "gateway"))

	app := NewWithStore(config.NewStoreAt(filepath.Join(t.TempDir(), "config.toml")))
	result, err := app.InspectReport(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("InspectReport: %v", err)
	}
	if result.Session.ID != "sess-1" {
		t.Fatalf("session = %q, want sess-1", result.Session.ID)
	}
	if result.Reconciliation != nil {
		t.Fatalf("reconciliation = %+v, want unavailable", result.Reconciliation)
	}
	if result.UnavailableReason == "" {
		t.Fatal("expected unavailable reason")
	}
}
