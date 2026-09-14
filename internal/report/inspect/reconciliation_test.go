package inspect

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/thanhpham0406/tok-doctor/internal/app"
	"github.com/thanhpham0406/tok-doctor/internal/match"
	"github.com/thanhpham0406/tok-doctor/internal/model"
	"github.com/thanhpham0406/tok-doctor/internal/reconcile"
)

func measured(value int64) model.Measurement {
	return model.NewMeasurement(value, model.MeasurementMeasured)
}

func reconciliationSession() model.Session {
	stamp := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	return model.Session{
		ID:        "sess-1",
		Source:    "codex",
		Model:     "gpt-5",
		StartedAt: &stamp,
		Usage:     model.MeasuredUsage(1250, 200, 300, nil, 1550),
		Turns: []model.Turn{{
			ID:        "sess-1#1",
			Sequence:  1,
			Timestamp: &stamp,
			Model:     "gpt-5",
			Usage:     model.MeasuredUsage(1250, 200, 300, nil, 1550),
		}},
	}
}

func matchedResult() reconcile.Result {
	return reconcile.Result{
		GatewayObservationID:    "gateway:codex:req-1",
		TranscriptObservationID: "codex:turn:sess-1#1",
		Confidence:              match.ConfidenceMedium,
		Reasons:                 []match.Reason{match.ReasonTime, match.ReasonModel},
		Status:                  reconcile.StatusDifferent,
		Fields: []reconcile.FieldComparison{
			{Field: "freshInput", Left: measured(1000), Right: measured(1000), Delta: model.Int64(0), Status: reconcile.StatusEqual},
			{Field: "cachedInput", Left: measured(200), Right: measured(180), Delta: model.Int64(-20), Status: reconcile.StatusDifferent},
			{Field: "cacheCreationInput", Left: measured(50), Right: measured(50), Delta: model.Int64(0), Status: reconcile.StatusEqual},
			{Field: "totalInput", Left: measured(1250), Right: measured(1230), Delta: model.Int64(-20), Status: reconcile.StatusDifferent},
			{Field: "output", Left: measured(300), Right: measured(320), Delta: model.Int64(20), Status: reconcile.StatusDifferent},
			{Field: "reasoningOutput", Status: reconcile.StatusUnavailable},
			{Field: "total", Left: measured(1550), Right: measured(1550), Delta: model.Int64(0), Status: reconcile.StatusEqual},
		},
	}
}

func TestRenderResultReconciliationSummaryCounters(t *testing.T) {
	result := app.InspectResult{
		Session: reconciliationSession(),
		Reconciliation: &reconcile.Report{Summary: reconcile.Summary{
			GatewayRequests:      12,
			TranscriptTurns:      13,
			Matched:              11,
			Equal:                9,
			Different:            2,
			Unavailable:          0,
			AmbiguousGateways:    0,
			UnmatchedGateways:    1,
			UnmatchedTranscripts: 2,
		}},
	}
	var out bytes.Buffer
	if err := RenderResult(&out, result, Options{}); err != nil {
		t.Fatalf("RenderResult: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"Reconciliation", "Gateway requests", "Transcript turns", "Matched",
		"Equal", "Different", "Unavailable", "Ambiguous",
		"Unmatched gateway", "Unmatched transcript",
		"12", "13", "11", "9", "2", "1",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestRenderResultUnavailableMessage(t *testing.T) {
	result := app.InspectResult{
		Session:           reconciliationSession(),
		UnavailableReason: "gateway capture not available",
	}
	var out bytes.Buffer
	if err := RenderResult(&out, result, Options{}); err != nil {
		t.Fatalf("RenderResult: %v", err)
	}
	if !strings.Contains(out.String(), "Unavailable: gateway capture not available") {
		t.Fatalf("output = %q, want unavailable message", out.String())
	}
}

func TestRenderResultTurnMatchedComparison(t *testing.T) {
	matched := matchedResult()
	result := app.InspectResult{
		Session:        reconciliationSession(),
		Reconciliation: &reconcile.Report{Summary: reconcile.Summary{GatewayRequests: 1, TranscriptTurns: 1, Matched: 1, Different: 1}},
		TurnReconciliations: map[string]app.TurnReconciliation{
			"sess-1#1": {Status: match.StatusMatched, Result: &matched},
		},
	}
	var out bytes.Buffer
	if err := RenderResult(&out, result, Options{Turn: 1}); err != nil {
		t.Fatalf("RenderResult: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"Gateway match", "Confidence", "medium", "Reasons", "time, model",
		"Gateway", "gateway:codex:req-1", "Status", "different",
		"Token comparison", "Field", "Gateway", "Agent", "Delta",
		"Fresh input", "Cached input", "Cache creation", "Total input",
		"Output", "Reasoning", "Total", "-20", "+20", "—",
		"Delta:", "transcript - gateway",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestRenderResultTurnUnmatchedAndAmbiguous(t *testing.T) {
	cases := map[string]app.TurnReconciliation{
		"unmatched": {Status: match.StatusUnmatched},
		"ambiguous": {Status: match.StatusAmbiguous},
	}
	for want, reconciliation := range cases {
		result := app.InspectResult{
			Session:             reconciliationSession(),
			Reconciliation:      &reconcile.Report{},
			TurnReconciliations: map[string]app.TurnReconciliation{"sess-1#1": reconciliation},
		}
		var out bytes.Buffer
		if err := RenderResult(&out, result, Options{Turn: 1}); err != nil {
			t.Fatalf("RenderResult: %v", err)
		}
		got := out.String()
		if !strings.Contains(got, "Gateway match") || !strings.Contains(got, want) {
			t.Fatalf("output = %q, want gateway %s status", got, want)
		}
		if strings.Contains(got, "Token comparison") {
			t.Fatalf("output = %q, want no comparison for %s", got, want)
		}
	}
}

func TestRenderResultWithoutReconciliationHasNoSection(t *testing.T) {
	var out bytes.Buffer
	if err := Render(&out, reconciliationSession(), Options{}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(out.String(), "Gateway requests") {
		t.Fatalf("output = %q, want no reconciliation section", out.String())
	}
}

func TestFormatComparisonValueMissingVersusZero(t *testing.T) {
	if got := formatComparisonValue(model.Measurement{}); got != missingValue {
		t.Fatalf("missing = %q, want %q", got, missingValue)
	}
	if got := formatComparisonValue(model.Measurement{Kind: model.MeasurementUnknown}); got != missingValue {
		t.Fatalf("unknown = %q, want %q", got, missingValue)
	}
	if got := formatComparisonValue(measured(0)); got != "0" {
		t.Fatalf("explicit zero = %q, want 0", got)
	}
}

func TestFormatComparisonDeltaSign(t *testing.T) {
	cases := []struct {
		name  string
		delta *int64
		want  string
	}{
		{"missing", nil, missingValue},
		{"zero", model.Int64(0), "0"},
		{"positive", model.Int64(20), "+20"},
		{"positive separated", model.Int64(1500), "+1,500"},
		{"negative", model.Int64(-20), "-20"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatComparisonDelta(tc.delta); got != tc.want {
				t.Fatalf("delta = %q, want %q", got, tc.want)
			}
		})
	}
}

func availableInspectResult() app.InspectResult {
	matched := matchedResult()
	return app.InspectResult{
		Session: reconciliationSession(),
		Reconciliation: &reconcile.Report{
			Summary:         reconcile.Summary{GatewayRequests: 1, TranscriptTurns: 1, Matched: 1, Different: 1},
			Matches:         []match.ObservationMatch{{ID: "match:gw=tr", Status: match.StatusMatched, Confidence: match.ConfidenceMedium, Reasons: []match.Reason{match.ReasonTime}}},
			Reconciliations: []reconcile.Result{matched},
		},
	}
}

func TestRenderJSONResultAvailableSchema(t *testing.T) {
	var out bytes.Buffer
	if err := RenderJSONResult(&out, availableInspectResult(), Options{}); err != nil {
		t.Fatalf("RenderJSONResult: %v", err)
	}
	var decoded struct {
		Session        model.Session `json:"session"`
		Reconciliation struct {
			Available       bool              `json:"available"`
			Summary         map[string]int    `json:"summary"`
			Matches         []json.RawMessage `json:"matches"`
			Reconciliations []json.RawMessage `json:"reconciliations"`
		} `json:"reconciliation"`
	}
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("decode json: %v\n%s", err, out.String())
	}
	if !decoded.Reconciliation.Available {
		t.Fatalf("available = false, want true")
	}
	if decoded.Reconciliation.Summary["gatewayRequests"] != 1 || decoded.Reconciliation.Summary["matched"] != 1 {
		t.Fatalf("summary = %+v", decoded.Reconciliation.Summary)
	}
	if len(decoded.Reconciliation.Matches) != 1 || len(decoded.Reconciliation.Reconciliations) != 1 {
		t.Fatalf("matches/reconciliations = %d/%d, want 1/1", len(decoded.Reconciliation.Matches), len(decoded.Reconciliation.Reconciliations))
	}
	if decoded.Session.ID != "sess-1" {
		t.Fatalf("session id = %q, want sess-1", decoded.Session.ID)
	}
}

func TestRenderJSONResultUnavailableSchema(t *testing.T) {
	result := app.InspectResult{Session: reconciliationSession(), UnavailableReason: "gateway capture not available"}
	var out bytes.Buffer
	if err := RenderJSONResult(&out, result, Options{}); err != nil {
		t.Fatalf("RenderJSONResult: %v", err)
	}
	var decoded struct {
		Reconciliation map[string]json.RawMessage `json:"reconciliation"`
	}
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("decode json: %v\n%s", err, out.String())
	}
	if string(decoded.Reconciliation["available"]) != "false" {
		t.Fatalf("available = %s, want false", decoded.Reconciliation["available"])
	}
	var reason string
	if err := json.Unmarshal(decoded.Reconciliation["reason"], &reason); err != nil || reason != "gateway capture not available" {
		t.Fatalf("reason = %q (err %v)", reason, err)
	}
	if _, ok := decoded.Reconciliation["summary"]; ok {
		t.Fatalf("unavailable schema must not include summary: %s", out.String())
	}
}

func TestRenderJSONResultMissingVersusZero(t *testing.T) {
	matched := matchedResult()
	matched.Fields = []reconcile.FieldComparison{
		{Field: "freshInput", Left: measured(1000), Right: measured(1000), Delta: model.Int64(0), Status: reconcile.StatusEqual},
		{Field: "reasoningOutput", Status: reconcile.StatusUnavailable},
	}
	result := availableInspectResult()
	result.Reconciliation.Reconciliations = []reconcile.Result{matched}

	var out bytes.Buffer
	if err := RenderJSONResult(&out, result, Options{}); err != nil {
		t.Fatalf("RenderJSONResult: %v", err)
	}
	var decoded struct {
		Reconciliation struct {
			Reconciliations []struct {
				Fields []struct {
					Field   string `json:"field"`
					Gateway struct {
						Value *int64 `json:"value"`
						Kind  string `json:"kind"`
					} `json:"gateway"`
					Delta *int64 `json:"delta"`
				} `json:"fields"`
			} `json:"reconciliations"`
		} `json:"reconciliation"`
	}
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("decode json: %v\n%s", err, out.String())
	}
	fields := decoded.Reconciliation.Reconciliations[0].Fields
	if fields[0].Gateway.Value == nil || *fields[0].Gateway.Value != 1000 {
		t.Fatalf("present measurement = %+v, want numeric 1000", fields[0].Gateway)
	}
	if fields[0].Delta == nil || *fields[0].Delta != 0 {
		t.Fatalf("zero delta = %v, want explicit 0", fields[0].Delta)
	}
	if fields[1].Gateway.Value != nil {
		t.Fatalf("missing measurement = %+v, want nil value", fields[1].Gateway)
	}
	if fields[1].Delta != nil {
		t.Fatalf("missing delta = %v, want omitted", fields[1].Delta)
	}
}

func TestRenderJSONResultDeterministic(t *testing.T) {
	result := availableInspectResult()
	var first, second bytes.Buffer
	if err := RenderJSONResult(&first, result, Options{}); err != nil {
		t.Fatalf("RenderJSONResult: %v", err)
	}
	if err := RenderJSONResult(&second, result, Options{}); err != nil {
		t.Fatalf("RenderJSONResult: %v", err)
	}
	if first.String() != second.String() {
		t.Fatalf("output not deterministic:\n%s\n%s", first.String(), second.String())
	}
}

func TestRenderJSONResultKeepsTurnContext(t *testing.T) {
	session := reconciliationSession()
	session.Turns[0].ContextAttribution = model.ContextAttribution{Components: []model.ContextComponent{{
		Kind:        model.ContextUserPrompt,
		Source:      "codex_rollout",
		Record:      "u1",
		Observation: model.ContextObservedByAgent,
		Measurement: model.NewMeasurement(4, model.MeasurementEstimated),
	}}}
	result := availableInspectResult()
	result.Session = session

	var out bytes.Buffer
	if err := RenderJSONResult(&out, result, Options{Turn: 1, ShowContext: true}); err != nil {
		t.Fatalf("RenderJSONResult: %v", err)
	}
	var decoded struct {
		Turn *model.Turn `json:"turn"`
	}
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("decode json: %v\n%s", err, out.String())
	}
	if decoded.Turn == nil || len(decoded.Turn.ContextAttribution.Components) != 1 {
		t.Fatalf("turn context = %+v, want one component", decoded.Turn)
	}
}

func TestReconciliationJSONReasonOmittedWhenEmpty(t *testing.T) {
	encoded, err := json.Marshal(reconciliationJSON{available: false})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !reflect.DeepEqual(string(encoded), `{"available":false}`) {
		t.Fatalf("encoded = %s, want available only", encoded)
	}
}
