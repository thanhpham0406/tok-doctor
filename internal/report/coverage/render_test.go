package coverage

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	corecoverage "github.com/thanhpham0406/tok-doctor/internal/coverage"
	"github.com/thanhpham0406/tok-doctor/internal/model"
)

const (
	secretContentHash = "sha256:6f1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c"
	secretRecord      = "record-secret-1"
	secretPath        = "/home/synthetic/private/AGENTS.md"
	secretToolOutput  = "SECRET_TOOL_OUTPUT_DO_NOT_RENDER"
)

func completeToolOutput(bytes int64, tokens int64) model.ContextComponent {
	return model.ContextComponent{
		Kind:         model.ContextToolResult,
		Source:       "codex_rollout",
		Record:       secretRecord,
		Path:         secretPath,
		ContentHash:  secretContentHash,
		ToolCallID:   "call-1",
		ToolName:     "exec_command",
		ContentBytes: model.Int64(bytes),
		Measurement:  model.NewMeasurement(tokens, model.MeasurementEstimated),
		Completeness: model.ContextCompletenessComplete,
	}
}

func measuredTurn(sequence int, components ...model.ContextComponent) model.Turn {
	usage := model.MeasuredUsage(1000, 400, 150, model.Int64(0), 1150)
	return model.Turn{
		ID:       "sess-1#" + string(rune('0'+sequence)),
		Sequence: sequence,
		Usage:    usage,
		Evidence: []model.Evidence{{Kind: model.EvidenceSourceValue, Source: "codex_rollout", Record: secretRecord}},
		ContextAttribution: model.ContextAttribution{
			Components: components,
			Input:      model.CodexInputAccounting(usage),
		},
	}
}

func sessionResult(sessions ...model.Session) corecoverage.Result {
	return corecoverage.Result{
		Scope:   corecoverage.ScopeSession,
		Source:  "codex",
		Overall: corecoverage.Scan(sessions),
		Gateway: corecoverage.UnsupportedGateway(),
	}
}

func lineSet(out string) map[string]bool {
	lines := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		lines[strings.Join(strings.Fields(line), " ")] = true
	}
	return lines
}

func TestRenderTerminalSessionSummary(t *testing.T) {
	result := sessionResult(model.Session{
		ID: "sess-1",
		Turns: []model.Turn{measuredTurn(1,
			completeToolOutput(2048, 512),
			model.ContextComponent{
				Kind:         model.ContextToolResult,
				Measurement:  model.NewMeasurement(10, model.MeasurementEstimated),
				Completeness: model.ContextCompletenessTruncated,
			},
		)},
	})
	result.SessionID = "sess-1"

	var out bytes.Buffer
	if err := Render(&out, result); err != nil {
		t.Fatalf("render: %v", err)
	}
	got := out.String()

	if !strings.Contains(got, "Tool Output Coverage — codex (session sess-1)") {
		t.Fatalf("missing title: %q", got)
	}
	if !strings.Contains(got, "Sessions scanned:       1") {
		t.Fatalf("missing aligned session count: %q", got)
	}
	if !strings.Contains(got, "Tool outputs:           2") {
		t.Fatalf("missing aligned tool output count: %q", got)
	}

	lines := lineSet(got)
	for _, want := range []string{
		"Turns scanned: 1",
		"With byte size: 1 (50.0%)",
		"With estimated tokens: 2 (100.0%)",
		"With call ID: 1 (50.0%)",
		"With tool name: 1 (50.0%)",
		"Complete: 1",
		"Truncated: 1",
		"Unavailable: 0",
		"Unknown: 0",
		"In trailing context: 0",
		"Linked to fresh input: 2 (100.0%)",
		"Recognized tool outputs:",
		"counted over 2 recognized tool outputs only",
		"Unreadable records: 1 (1 session)",
		"records read partially or not at all; a record whose kind stays unknown",
		"is not counted as tool output and stays outside the counts above",
		"Output bytes:",
		"total 2.0 KiB",
		"min 2.0 KiB",
		"p50 2.0 KiB",
		"p90 2.0 KiB",
		"p95 2.0 KiB",
		"max 2.0 KiB",
		"Output tokens (estimated): 522",
	} {
		if !lines[want] {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
	if !strings.Contains(got, "Gateway coverage: not supported in this iteration") {
		t.Fatalf("missing gateway note: %q", got)
	}
}

// TestRenderTerminalSeparatesRecognizedCompletenessFromUnreadableRecords covers
// the case the flat list used to blur: every recognized tool output is
// complete, while records the source could not read stay outside that
// denominator instead of reading as more complete output.
func TestRenderTerminalSeparatesRecognizedCompletenessFromUnreadableRecords(t *testing.T) {
	result := sessionResult(model.Session{
		ID: "sess-1",
		Turns: []model.Turn{measuredTurn(1,
			completeToolOutput(2048, 512),
		)},
		TrailingContext: []model.ContextComponent{
			{Kind: model.ContextUnknown, Completeness: model.ContextCompletenessUnavailable},
			{Kind: model.ContextUnknown, Completeness: model.ContextCompletenessTruncated},
		},
	})
	result.SessionID = "sess-1"

	var out bytes.Buffer
	if err := Render(&out, result); err != nil {
		t.Fatalf("render: %v", err)
	}
	got := out.String()

	if !strings.Contains(got, "Recognized tool outputs:\n  Complete:") {
		t.Fatalf("completeness must sit under its own header: %q", got)
	}
	if strings.Index(got, "Unreadable records:") < strings.Index(got, "Recognized tool outputs:") {
		t.Fatalf("unreadable records must be reported apart from the recognized counts: %q", got)
	}
	lines := lineSet(got)
	for _, want := range []string{
		"Tool outputs: 1",
		"Complete: 1",
		"Truncated: 0",
		"Unavailable: 0",
		"Unknown: 0",
		"counted over 1 recognized tool output only",
		"Unreadable records: 2 (1 session)",
	} {
		if !lines[want] {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
}

func TestRenderTerminalWithoutToolOutputs(t *testing.T) {
	result := sessionResult(model.Session{
		ID:    "sess-1",
		Turns: []model.Turn{measuredTurn(1, model.ContextComponent{Kind: model.ContextUserPrompt})},
	})

	var out bytes.Buffer
	if err := Render(&out, result); err != nil {
		t.Fatalf("render: %v", err)
	}
	got := out.String()

	lines := lineSet(got)
	for _, want := range []string{
		"Tool outputs: 0",
		"With byte size: 0 (n/a)",
		"With estimated tokens: 0 (n/a)",
		"With call ID: 0 (n/a)",
		"With tool name: 0 (n/a)",
		"Linked to fresh input: 0 (n/a)",
		"no byte size reported",
		"Output tokens (estimated): n/a",
	} {
		if !lines[want] {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
	if strings.Contains(got, "NaN") || strings.Contains(got, "+Inf") {
		t.Fatalf("output has a non-finite ratio: %q", got)
	}
}

func TestRenderTerminalAllSources(t *testing.T) {
	result := corecoverage.Result{
		Scope: corecoverage.ScopeAll,
		Overall: corecoverage.Scan([]model.Session{
			{ID: "sess-1", Turns: []model.Turn{measuredTurn(1, completeToolOutput(1024, 256))}},
		}),
		Sources: []corecoverage.SourceSummary{
			{Source: "claude", Summary: corecoverage.Scan(nil)},
			{Source: "codex", Summary: corecoverage.Scan([]model.Session{
				{ID: "sess-1", Turns: []model.Turn{measuredTurn(1, completeToolOutput(1024, 256))}},
			})},
		},
		UnsupportedSources: []corecoverage.Unsupported{
			{Source: "router9", Reason: "tool output coverage not supported for router9: source exposes no sessions"},
		},
		Gateway: corecoverage.UnsupportedGateway(),
	}

	var out bytes.Buffer
	if err := Render(&out, result); err != nil {
		t.Fatalf("render: %v", err)
	}
	got := out.String()

	for _, want := range []string{
		"Tool Output Coverage — all sources",
		"\nclaude\n",
		"\ncodex\n",
		"\nOverall\n",
		"Unsupported sources:",
		"router9: tool output coverage not supported for router9: source exposes no sessions",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
	if strings.Contains(got, "(session ") {
		t.Fatalf("aggregate output must not name a session: %q", got)
	}
}

func TestRenderJSONKeepsNumbersUnformatted(t *testing.T) {
	result := sessionResult(model.Session{
		ID: "sess-1",
		Turns: []model.Turn{measuredTurn(1,
			completeToolOutput(2048, 512),
			model.ContextComponent{
				Kind:         model.ContextToolResult,
				Measurement:  model.NewMeasurement(10, model.MeasurementEstimated),
				Completeness: model.ContextCompletenessTruncated,
			},
		)},
	})
	result.SessionID = "sess-1"

	var out bytes.Buffer
	if err := RenderJSON(&out, result); err != nil {
		t.Fatalf("render json: %v", err)
	}

	var decoded struct {
		Scope     string `json:"scope"`
		SessionID string `json:"sessionId"`
		Overall   struct {
			Sessions                         int   `json:"sessions"`
			Turns                            int   `json:"turns"`
			ToolOutputs                      int   `json:"toolOutputs"`
			TrailingContext                  int   `json:"trailingContext"`
			UnreadableRecords                int   `json:"unreadableRecords"`
			UnreadableSessions               int   `json:"unreadableSessions"`
			WithContentBytes                 ratio `json:"withContentBytes"`
			WithEstimatedTokens              ratio `json:"withEstimatedTokens"`
			WithToolCallID                   ratio `json:"withToolCallId"`
			WithToolName                     ratio `json:"withToolName"`
			WithFreshInput                   ratio `json:"withFreshInput"`
			RecognizedToolOutputCompleteness struct {
				Complete    int `json:"complete"`
				Truncated   int `json:"truncated"`
				Unavailable int `json:"unavailable"`
				Unknown     int `json:"unknown"`
			} `json:"recognizedToolOutputCompleteness"`
			ContentBytes struct {
				Total   int64  `json:"total"`
				Samples int    `json:"samples"`
				Min     *int64 `json:"min"`
				P50     *int64 `json:"p50"`
				P90     *int64 `json:"p90"`
				P95     *int64 `json:"p95"`
				Max     *int64 `json:"max"`
			} `json:"contentBytes"`
			EstimatedTokens model.Measurement `json:"estimatedTokens"`
		} `json:"overall"`
		Gateway struct {
			Supported bool   `json:"supported"`
			Reason    string `json:"reason"`
		} `json:"gateway"`
	}
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("decode json: %v\n%s", err, out.String())
	}

	if decoded.Scope != "session" || decoded.SessionID != "sess-1" {
		t.Fatalf("scope/session = %q/%q, want session/sess-1", decoded.Scope, decoded.SessionID)
	}
	if decoded.Overall.ToolOutputs != 2 || decoded.Overall.Turns != 1 {
		t.Fatalf("counts = %+v, want 2 tool outputs over 1 turn", decoded.Overall)
	}
	if decoded.Overall.WithContentBytes.Count != 1 || decoded.Overall.WithContentBytes.Total != 2 {
		t.Fatalf("with byte size = %+v, want 1 of 2", decoded.Overall.WithContentBytes)
	}
	if value := decoded.Overall.WithContentBytes.Value; value == nil || *value != 0.5 {
		t.Fatalf("byte size value = %v, want 0.5", value)
	}
	completeness := decoded.Overall.RecognizedToolOutputCompleteness
	if completeness.Complete != 1 || completeness.Truncated != 1 {
		t.Fatalf("completeness = %+v, want one complete and one truncated", completeness)
	}
	distribution := decoded.Overall.ContentBytes
	if distribution.Total != 2048 || distribution.Samples != 1 ||
		distribution.P50 == nil || *distribution.P50 != 2048 ||
		distribution.Max == nil || *distribution.Max != 2048 {
		t.Fatalf("content bytes = %+v, want 2048 across every percentile", distribution)
	}
	if decoded.Overall.EstimatedTokens.Kind != model.MeasurementEstimated || decoded.Overall.EstimatedTokens.ValueOrZero() != 522 {
		t.Fatalf("estimated tokens = %+v, want 522 estimated", decoded.Overall.EstimatedTokens)
	}
	if decoded.Overall.UnreadableRecords != 1 || decoded.Overall.UnreadableSessions != 1 {
		t.Fatalf("unreadable = %d records, %d sessions, want 1 and 1",
			decoded.Overall.UnreadableRecords, decoded.Overall.UnreadableSessions)
	}
	if decoded.Gateway.Supported || decoded.Gateway.Reason == "" {
		t.Fatalf("gateway = %+v, want unsupported with a reason", decoded.Gateway)
	}
	for _, unwanted := range []string{"KiB", "MiB", "97.5%", "%"} {
		if strings.Contains(out.String(), unwanted) {
			t.Fatalf("json output contains formatted value %q: %s", unwanted, out.String())
		}
	}
}

func TestRenderJSONKeepsUndefinedRatioNull(t *testing.T) {
	result := sessionResult(model.Session{ID: "sess-1"})

	var out bytes.Buffer
	if err := RenderJSON(&out, result); err != nil {
		t.Fatalf("render json: %v", err)
	}
	if !strings.Contains(out.String(), `"value": null`) {
		t.Fatalf("json must state undefined ratios explicitly: %s", out.String())
	}
	if !strings.Contains(out.String(), `"min": null`) {
		t.Fatalf("json must keep absent byte samples null: %s", out.String())
	}
}

// TestRenderJSONNamesBothPopulations keeps the two groups distinguishable for
// machine readers: completeness is named for the recognized tool outputs it
// counts, and the unreadable records that stay outside that denominator are
// reported under their own keys.
func TestRenderJSONNamesBothPopulations(t *testing.T) {
	result := sessionResult(model.Session{
		ID: "sess-1",
		Turns: []model.Turn{measuredTurn(1,
			completeToolOutput(2048, 512),
		)},
		TrailingContext: []model.ContextComponent{
			{Kind: model.ContextUnknown, Completeness: model.ContextCompletenessUnavailable},
		},
	})
	result.SessionID = "sess-1"

	var out bytes.Buffer
	if err := RenderJSON(&out, result); err != nil {
		t.Fatalf("render json: %v", err)
	}
	var decoded struct {
		Overall map[string]json.RawMessage `json:"overall"`
	}
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("decode json: %v\n%s", err, out.String())
	}

	raw, ok := decoded.Overall["recognizedToolOutputCompleteness"]
	if !ok {
		t.Fatalf("json must name the recognized completeness denominator: %s", out.String())
	}
	var recognized struct {
		Complete int `json:"complete"`
	}
	if err := json.Unmarshal(raw, &recognized); err != nil {
		t.Fatalf("decode completeness: %v", err)
	}
	if recognized.Complete != 1 {
		t.Fatalf("recognized complete = %d, want 1", recognized.Complete)
	}
	if _, ok := decoded.Overall["completeness"]; ok {
		t.Fatalf("json kept the ambiguous completeness key: %s", out.String())
	}
	unreadable, ok := decoded.Overall["unreadableRecords"]
	if !ok || string(unreadable) != "1" {
		t.Fatalf("unreadable records = %s (present %t), want 1 reported apart", unreadable, ok)
	}
}

func TestRenderNeverLeaksContentHashRecordOrPath(t *testing.T) {
	result := sessionResult(model.Session{
		ID:    "sess-1",
		Turns: []model.Turn{measuredTurn(1, completeToolOutput(2048, 512))},
	})
	result.SessionID = "sess-1"

	var terminal, jsonOut bytes.Buffer
	if err := Render(&terminal, result); err != nil {
		t.Fatalf("render terminal: %v", err)
	}
	if err := RenderJSON(&jsonOut, result); err != nil {
		t.Fatalf("render json: %v", err)
	}

	for name, out := range map[string]string{"terminal": terminal.String(), "json": jsonOut.String()} {
		for _, unwanted := range []string{secretContentHash, secretRecord, secretPath, secretToolOutput, "sha256:"} {
			if strings.Contains(out, unwanted) {
				t.Fatalf("%s output leaks %q: %s", name, unwanted, out)
			}
		}
	}
	if !strings.Contains(terminal.String(), "Tool Output Coverage — codex (session sess-1)") {
		t.Fatalf("single session output should name the session: %q", terminal.String())
	}
}

type ratio struct {
	Count int      `json:"count"`
	Total int      `json:"total"`
	Value *float64 `json:"value"`
}
