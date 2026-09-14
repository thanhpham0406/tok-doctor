package reconcile

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/thanhpham0406/tok-doctor/internal/match"
	"github.com/thanhpham0406/tok-doctor/internal/model"
)

func reportOptions() match.Options {
	return match.Options{TimeWindow: 2 * time.Minute}
}

func reportTime(offset time.Duration) *time.Time {
	at := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC).Add(offset)
	return &at
}

func reportGateway(id string, usage model.ObservationUsage, identity model.ObservationIdentity) model.Observation {
	return model.Observation{
		SchemaVersion: model.ObservationSchemaVersion,
		ID:            id,
		Channel:       model.ObservationChannelGateway,
		Scope:         model.ObservationScopeRequest,
		Source:        "gateway",
		Identity:      identity,
		Usage:         usage,
		Outcome:       model.ObservationOutcomeSucceeded,
		Completeness:  model.ObservationCompletenessComplete,
	}
}

func reportTranscript(id string, usage model.ObservationUsage, identity model.ObservationIdentity) model.Observation {
	return model.Observation{
		SchemaVersion: model.ObservationSchemaVersion,
		ID:            id,
		Channel:       model.ObservationChannelSessionTranscript,
		Scope:         model.ObservationScopeTurn,
		Source:        "claude",
		Identity:      identity,
		Usage:         usage,
		Outcome:       model.ObservationOutcomeUnknown,
		Completeness:  model.ObservationCompletenessComplete,
	}
}

func at(observation model.Observation, offset time.Duration, modelName string) model.Observation {
	observation.StartedAt = reportTime(offset)
	observation.Model = modelName
	return observation
}

func mustBuildReport(t *testing.T, observations []model.Observation, options match.Options) Report {
	t.Helper()
	report, err := BuildReport(observations, options)
	if err != nil {
		t.Fatalf("BuildReport: %v", err)
	}
	if report.Summary.Matched != len(report.Reconciliations) {
		t.Fatalf("invariant: matched = %d, reconciliations = %d", report.Summary.Matched, len(report.Reconciliations))
	}
	if report.Summary.Matched != report.Summary.Equal+report.Summary.Different+report.Summary.Unavailable {
		t.Fatalf("invariant: matched breakdown = %+v", report.Summary)
	}
	if report.Summary.GatewayRequests != report.Summary.Matched+report.Summary.AmbiguousGateways+report.Summary.UnmatchedGateways {
		t.Fatalf("invariant: gateway accounting = %+v", report.Summary)
	}
	return report
}

func TestBuildReportEqualMatched(t *testing.T) {
	identity := model.ObservationIdentity{ExchangeID: "req-1"}
	observations := []model.Observation{
		reportGateway("gw-1", fullUsage(), identity),
		reportTranscript("tr-1", fullUsage(), identity),
	}

	report := mustBuildReport(t, observations, reportOptions())

	want := Summary{GatewayRequests: 1, TranscriptTurns: 1, Matched: 1, Equal: 1}
	if report.Summary != want {
		t.Fatalf("summary = %+v, want %+v", report.Summary, want)
	}
	if len(report.Matches) != 1 || len(report.Reconciliations) != 1 {
		t.Fatalf("matches/reconciliations = %d/%d, want 1/1", len(report.Matches), len(report.Reconciliations))
	}
	if report.Reconciliations[0].Status != StatusEqual {
		t.Fatalf("status = %q, want equal", report.Reconciliations[0].Status)
	}
}

func TestBuildReportDifferentMatched(t *testing.T) {
	identity := model.ObservationIdentity{ExchangeID: "req-1"}
	transcriptUsage := fullUsage()
	transcriptUsage.Output = measured(999)
	observations := []model.Observation{
		reportGateway("gw-1", fullUsage(), identity),
		reportTranscript("tr-1", transcriptUsage, identity),
	}

	report := mustBuildReport(t, observations, reportOptions())

	if report.Summary.Matched != 1 || report.Summary.Different != 1 {
		t.Fatalf("summary = %+v, want one different match", report.Summary)
	}
	if len(report.Reconciliations) != 1 || report.Reconciliations[0].Status != StatusDifferent {
		t.Fatalf("reconciliations = %+v, want one different", report.Reconciliations)
	}
}

func TestBuildReportUnavailableMatched(t *testing.T) {
	identity := model.ObservationIdentity{ExchangeID: "req-1"}
	observations := []model.Observation{
		reportGateway("gw-1", fullUsage(), identity),
		reportTranscript("tr-1", model.ObservationUsage{}, identity),
	}

	report := mustBuildReport(t, observations, reportOptions())

	if report.Summary.Matched != 1 || report.Summary.Unavailable != 1 {
		t.Fatalf("summary = %+v, want one unavailable match", report.Summary)
	}
	if report.Reconciliations[0].Status != StatusUnavailable {
		t.Fatalf("status = %q, want unavailable", report.Reconciliations[0].Status)
	}
}

func TestBuildReportGatewayOnly(t *testing.T) {
	gateway := at(reportGateway("gw-1", fullUsage(), model.ObservationIdentity{}), 0, "gpt-5")

	report := mustBuildReport(t, []model.Observation{gateway}, reportOptions())

	want := Summary{GatewayRequests: 1, UnmatchedGateways: 1}
	if report.Summary != want {
		t.Fatalf("summary = %+v, want %+v", report.Summary, want)
	}
	if len(report.Reconciliations) != 0 {
		t.Fatalf("reconciliations = %+v, want none", report.Reconciliations)
	}
}

func TestBuildReportTranscriptOnly(t *testing.T) {
	transcript := at(reportTranscript("tr-1", fullUsage(), model.ObservationIdentity{}), 0, "gpt-5")

	report := mustBuildReport(t, []model.Observation{transcript}, reportOptions())

	want := Summary{TranscriptTurns: 1, UnmatchedTranscripts: 1}
	if report.Summary != want {
		t.Fatalf("summary = %+v, want %+v", report.Summary, want)
	}
	if len(report.Reconciliations) != 0 {
		t.Fatalf("reconciliations = %+v, want none", report.Reconciliations)
	}
}

func TestBuildReportAmbiguousGateway(t *testing.T) {
	observations := []model.Observation{
		at(reportGateway("gw-1", fullUsage(), model.ObservationIdentity{}), 0, "gpt-5"),
		at(reportTranscript("tr-1", fullUsage(), model.ObservationIdentity{}), 10*time.Second, "gpt-5"),
		at(reportTranscript("tr-2", fullUsage(), model.ObservationIdentity{}), 10*time.Second, "gpt-5"),
	}

	report := mustBuildReport(t, observations, reportOptions())

	want := Summary{GatewayRequests: 1, TranscriptTurns: 2, AmbiguousGateways: 1}
	if report.Summary != want {
		t.Fatalf("summary = %+v, want %+v", report.Summary, want)
	}
	if report.Summary.UnmatchedTranscripts != 0 {
		t.Fatalf("unmatched transcripts = %d, ambiguity candidates must not be counted", report.Summary.UnmatchedTranscripts)
	}
	if len(report.Reconciliations) != 0 {
		t.Fatalf("reconciliations = %+v, want none", report.Reconciliations)
	}
	if len(report.Matches) != 1 || report.Matches[0].Status != match.StatusAmbiguous {
		t.Fatalf("matches = %+v, want one ambiguous gateway", report.Matches)
	}
	if !reflect.DeepEqual(report.Matches[0].CandidateTranscriptIDs, []string{"tr-1", "tr-2"}) {
		t.Fatalf("candidates = %+v, want [tr-1 tr-2]", report.Matches[0].CandidateTranscriptIDs)
	}
}

func TestBuildReportMixed(t *testing.T) {
	equalIdentity := model.ObservationIdentity{ExchangeID: "eq"}
	differentIdentity := model.ObservationIdentity{ExchangeID: "diff"}
	unavailableIdentity := model.ObservationIdentity{ExchangeID: "unav"}
	differentTranscriptUsage := fullUsage()
	differentTranscriptUsage.Output = measured(999)

	observations := []model.Observation{
		reportGateway("gw-eq", fullUsage(), equalIdentity),
		reportTranscript("tr-eq", fullUsage(), equalIdentity),
		reportGateway("gw-diff", fullUsage(), differentIdentity),
		reportTranscript("tr-diff", differentTranscriptUsage, differentIdentity),
		reportGateway("gw-unav", fullUsage(), unavailableIdentity),
		reportTranscript("tr-unav", model.ObservationUsage{}, unavailableIdentity),
		at(reportGateway("gw-lonely", fullUsage(), model.ObservationIdentity{}), 0, "gpt-5"),
		at(reportTranscript("tr-lonely", fullUsage(), model.ObservationIdentity{}), 2*time.Hour, "gpt-5"),
		at(reportGateway("gw-amb", fullUsage(), model.ObservationIdentity{}), time.Hour, "gpt-5"),
		at(reportTranscript("tr-amb-1", fullUsage(), model.ObservationIdentity{}), time.Hour+10*time.Second, "gpt-5"),
		at(reportTranscript("tr-amb-2", fullUsage(), model.ObservationIdentity{}), time.Hour+10*time.Second, "gpt-5"),
	}

	report := mustBuildReport(t, observations, reportOptions())

	want := Summary{
		GatewayRequests:      5,
		TranscriptTurns:      6,
		Matched:              3,
		Equal:                1,
		Different:            1,
		Unavailable:          1,
		AmbiguousGateways:    1,
		UnmatchedGateways:    1,
		UnmatchedTranscripts: 1,
	}
	if report.Summary != want {
		t.Fatalf("summary = %+v, want %+v", report.Summary, want)
	}
	if len(report.Reconciliations) != 3 {
		t.Fatalf("reconciliations = %d, want 3", len(report.Reconciliations))
	}

	order := []string{
		report.Reconciliations[0].GatewayObservationID,
		report.Reconciliations[1].GatewayObservationID,
		report.Reconciliations[2].GatewayObservationID,
	}
	if !reflect.DeepEqual(order, []string{"gw-diff", "gw-eq", "gw-unav"}) {
		t.Fatalf("reconciliation order = %+v, want matched-result order", order)
	}
}

func TestBuildReportIgnoresSessionScope(t *testing.T) {
	identity := model.ObservationIdentity{ExchangeID: "req-1"}
	session := reportTranscript("session-1", fullUsage(), identity)
	session.Scope = model.ObservationScopeSession

	observations := []model.Observation{
		reportGateway("gw-1", fullUsage(), identity),
		reportTranscript("tr-1", fullUsage(), identity),
		session,
	}

	report := mustBuildReport(t, observations, reportOptions())

	want := Summary{GatewayRequests: 1, TranscriptTurns: 1, Matched: 1, Equal: 1}
	if report.Summary != want {
		t.Fatalf("summary = %+v, want %+v", report.Summary, want)
	}
	if len(report.Reconciliations) != 1 {
		t.Fatalf("reconciliations = %d, want only the matched turn", len(report.Reconciliations))
	}
}

func TestBuildReportExplicitZeroIsEqual(t *testing.T) {
	identity := model.ObservationIdentity{ExchangeID: "req-1"}
	usage := model.ObservationUsage{FreshInput: measured(0)}
	observations := []model.Observation{
		reportGateway("gw-1", usage, identity),
		reportTranscript("tr-1", usage, identity),
	}

	report := mustBuildReport(t, observations, reportOptions())

	if report.Summary.Equal != 1 || report.Reconciliations[0].Status != StatusEqual {
		t.Fatalf("report = %+v, want explicit zeros to reconcile equal", report.Summary)
	}
}

func TestBuildReportMissingVersusZeroIsUnavailable(t *testing.T) {
	identity := model.ObservationIdentity{ExchangeID: "req-1"}
	observations := []model.Observation{
		reportGateway("gw-1", model.ObservationUsage{FreshInput: measured(0)}, identity),
		reportTranscript("tr-1", model.ObservationUsage{}, identity),
	}

	report := mustBuildReport(t, observations, reportOptions())

	if report.Summary.Unavailable != 1 {
		t.Fatalf("summary = %+v, want unavailable", report.Summary)
	}
	fresh := field(t, report.Reconciliations[0], "freshInput")
	if fresh.Status != StatusUnavailable || fresh.Delta != nil {
		t.Fatalf("freshInput = %+v, want unavailable with nil delta", fresh)
	}
}

func TestBuildReportIsInputOrderIndependent(t *testing.T) {
	differentTranscriptUsage := fullUsage()
	differentTranscriptUsage.Output = measured(999)

	ordered := []model.Observation{
		reportGateway("gw-eq", fullUsage(), model.ObservationIdentity{ExchangeID: "eq"}),
		reportTranscript("tr-eq", fullUsage(), model.ObservationIdentity{ExchangeID: "eq"}),
		reportGateway("gw-diff", fullUsage(), model.ObservationIdentity{ExchangeID: "diff"}),
		reportTranscript("tr-diff", differentTranscriptUsage, model.ObservationIdentity{ExchangeID: "diff"}),
		at(reportTranscript("tr-lonely", fullUsage(), model.ObservationIdentity{}), 2*time.Hour, "gpt-5"),
		at(reportGateway("gw-amb", fullUsage(), model.ObservationIdentity{}), time.Hour, "gpt-5"),
		at(reportTranscript("tr-amb-1", fullUsage(), model.ObservationIdentity{}), time.Hour+10*time.Second, "gpt-5"),
		at(reportTranscript("tr-amb-2", fullUsage(), model.ObservationIdentity{}), time.Hour+10*time.Second, "gpt-5"),
	}
	shuffled := []model.Observation{
		ordered[6], ordered[2], ordered[0], ordered[7],
		ordered[4], ordered[5], ordered[1], ordered[3],
	}
	reversed := make([]model.Observation, 0, len(ordered))
	for i := len(ordered) - 1; i >= 0; i-- {
		reversed = append(reversed, ordered[i])
	}

	baseline := mustBuildReport(t, ordered, reportOptions())
	if got := mustBuildReport(t, shuffled, reportOptions()); !reflect.DeepEqual(baseline, got) {
		t.Fatalf("shuffled report differs:\nfirst  = %+v\nsecond = %+v", baseline, got)
	}
	if got := mustBuildReport(t, reversed, reportOptions()); !reflect.DeepEqual(baseline, got) {
		t.Fatalf("reversed report differs:\nfirst  = %+v\nsecond = %+v", baseline, got)
	}
}

func TestBuildReportMutationSafety(t *testing.T) {
	identity := model.ObservationIdentity{ExchangeID: "req-1"}
	observations := []model.Observation{
		reportGateway("gw-1", fullUsage(), identity),
		reportTranscript("tr-1", fullUsage(), identity),
	}
	before := append([]model.Observation(nil), observations...)

	first := mustBuildReport(t, observations, reportOptions())
	second := mustBuildReport(t, observations, reportOptions())
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("independent builds differ:\nfirst  = %+v\nsecond = %+v", first, second)
	}

	if first.Reconciliations[0].Fields[0].Left.Value != nil {
		*first.Reconciliations[0].Fields[0].Left.Value = 9999
	}
	if !reflect.DeepEqual(observations, before) {
		t.Fatalf("observations mutated:\n got = %+v\nwant = %+v", observations, before)
	}

	third := mustBuildReport(t, observations, reportOptions())
	if !reflect.DeepEqual(third, second) {
		t.Fatalf("mutating a report affected a later build:\n got = %+v\nwant = %+v", third, second)
	}
}

func TestBuildReportMatcherErrorPropagated(t *testing.T) {
	observations := []model.Observation{
		reportGateway("gw-1", fullUsage(), model.ObservationIdentity{ExchangeID: "req-1"}),
		reportTranscript("tr-1", fullUsage(), model.ObservationIdentity{ExchangeID: "req-1"}),
	}

	_, err := BuildReport(observations, match.Options{})
	if err == nil || !strings.Contains(err.Error(), "build reconciliation report") || !strings.Contains(err.Error(), "invalid time window") {
		t.Fatalf("error = %v, want contextual invalid time window", err)
	}

	invalid := reportGateway("gw-bad", fullUsage(), model.ObservationIdentity{})
	invalid.Source = ""
	_, err = BuildReport([]model.Observation{invalid}, reportOptions())
	if err == nil || !strings.Contains(err.Error(), "gw-bad") {
		t.Fatalf("error = %v, want invalid observation id", err)
	}

	duplicate := reportTranscript("dup", fullUsage(), model.ObservationIdentity{})
	_, err = BuildReport([]model.Observation{
		reportGateway("dup", fullUsage(), model.ObservationIdentity{}),
		duplicate,
	}, reportOptions())
	if err == nil || !strings.Contains(err.Error(), "duplicate observation id") {
		t.Fatalf("error = %v, want duplicate observation id", err)
	}
}

func TestReportFromMatchesRejectsInvalidMatches(t *testing.T) {
	validObservations := func() []model.Observation {
		return []model.Observation{
			reportGateway("gw-1", fullUsage(), model.ObservationIdentity{}),
			reportTranscript("tr-1", fullUsage(), model.ObservationIdentity{}),
		}
	}
	matched := func() match.ObservationMatch {
		return match.ObservationMatch{
			ID:                      "match:gw-1=tr-1",
			GatewayObservationID:    "gw-1",
			TranscriptObservationID: "tr-1",
			Status:                  match.StatusMatched,
			Confidence:              match.ConfidenceHigh,
			Reasons:                 []match.Reason{match.ReasonSharedIdentity},
		}
	}
	wrongGatewayScope := func() []model.Observation {
		observations := validObservations()
		observations[0].Scope = model.ObservationScopeTurn
		return observations
	}
	wrongTranscriptScope := func() []model.Observation {
		observations := validObservations()
		observations[1].Scope = model.ObservationScopeSession
		return observations
	}

	cases := []struct {
		name         string
		observations []model.Observation
		match        match.ObservationMatch
		want         string
	}{
		{
			name:         "missing transcript id",
			observations: validObservations(),
			match:        func() match.ObservationMatch { m := matched(); m.TranscriptObservationID = ""; return m }(),
			want:         "missing observation id",
		},
		{
			name:         "unknown gateway",
			observations: validObservations(),
			match:        func() match.ObservationMatch { m := matched(); m.GatewayObservationID = "ghost"; return m }(),
			want:         "not found",
		},
		{
			name:         "unknown transcript",
			observations: validObservations(),
			match:        func() match.ObservationMatch { m := matched(); m.TranscriptObservationID = "ghost"; return m }(),
			want:         "not found",
		},
		{
			name:         "gateway wrong scope",
			observations: wrongGatewayScope(),
			match:        matched(),
			want:         "not a gateway request",
		},
		{
			name:         "transcript wrong scope",
			observations: wrongTranscriptScope(),
			match:        matched(),
			want:         "not a transcript turn",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := reportFromMatches(tc.observations, []match.ObservationMatch{tc.match})
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want substring %q", err, tc.want)
			}
		})
	}
}

func TestBuildReportSyntheticIntegration(t *testing.T) {
	gatewayClaude := at(reportGateway("gateway:claude:req-1", model.ObservationUsage{
		FreshInput:         measured(1000),
		CachedInput:        measured(200),
		CacheCreationInput: measured(50),
		TotalInput:         measured(1250),
		Output:             measured(300),
		Total:              measured(1550),
	}, model.ObservationIdentity{}), 0, "claude-sonnet-4")

	transcriptClaude := at(reportTranscript("claude:turn:turn-1", model.ObservationUsage{
		FreshInput:         measured(1000),
		CachedInput:        measured(180),
		CacheCreationInput: measured(50),
		TotalInput:         measured(1230),
		Output:             measured(320),
		Total:              measured(1550),
	}, model.ObservationIdentity{}), 2*time.Second, "claude-sonnet-4")

	gatewayCodex := at(reportGateway("gateway:codex:req-2", fullUsage(), model.ObservationIdentity{}), 30*time.Minute, "gpt-5")
	gatewayCodex.Source = "gateway"

	transcriptCodex := at(reportTranscript("codex:turn:turn-2", fullUsage(), model.ObservationIdentity{}), 90*time.Minute, "gpt-5")
	transcriptCodex.Source = "codex"

	observations := []model.Observation{gatewayClaude, transcriptClaude, gatewayCodex, transcriptCodex}
	report := mustBuildReport(t, observations, reportOptions())

	want := Summary{
		GatewayRequests:      2,
		TranscriptTurns:      2,
		Matched:              1,
		Different:            1,
		UnmatchedGateways:    1,
		UnmatchedTranscripts: 1,
	}
	if report.Summary != want {
		t.Fatalf("summary = %+v, want %+v", report.Summary, want)
	}

	result := report.Reconciliations[0]
	if result.GatewayObservationID != "gateway:claude:req-1" || result.TranscriptObservationID != "claude:turn:turn-1" {
		t.Fatalf("reconciled ids = %q/%q, want claude pair", result.GatewayObservationID, result.TranscriptObservationID)
	}
	if result.Status != StatusDifferent {
		t.Fatalf("status = %q, want different", result.Status)
	}

	wantDeltas := map[string]*int64{
		"freshInput":         model.Int64(0),
		"cachedInput":        model.Int64(-20),
		"cacheCreationInput": model.Int64(0),
		"totalInput":         model.Int64(-20),
		"output":             model.Int64(20),
		"reasoningOutput":    nil,
		"total":              model.Int64(0),
	}
	for name, wantDelta := range wantDeltas {
		got := field(t, result, name).Delta
		if wantDelta == nil {
			if got != nil {
				t.Fatalf("%s delta = %v, want nil", name, *got)
			}
			continue
		}
		if got == nil || *got != *wantDelta {
			t.Fatalf("%s delta = %v, want %d", name, got, *wantDelta)
		}
	}
}

func TestBuildReportEmpty(t *testing.T) {
	cases := map[string][]model.Observation{"nil": nil, "empty": {}}
	for name, observations := range cases {
		t.Run(name, func(t *testing.T) {
			report := mustBuildReport(t, observations, reportOptions())
			if report.Summary != (Summary{}) {
				t.Fatalf("summary = %+v, want zero", report.Summary)
			}
			if len(report.Matches) != 0 || len(report.Reconciliations) != 0 {
				t.Fatalf("matches/reconciliations = %d/%d, want 0/0", len(report.Matches), len(report.Reconciliations))
			}
		})
	}
}
