package inspect

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

func TestRenderSummaryShowsThousandsSeparator(t *testing.T) {
	started := time.Date(2026, 9, 7, 22, 10, 0, 0, time.UTC)
	updated := started.Add(20 * time.Minute)
	reasoning := int64(0)
	session := model.Session{
		ID:        "sess-1",
		Source:    "codex",
		Model:     "gpt-5",
		StartedAt: &started,
		UpdatedAt: &updated,
		Usage: model.Usage{
			Input:       111249946,
			Cached:      16558343,
			Output:      45346,
			Reasoning:   &reasoning,
			Total:       168033860,
			Measurement: model.MeasurementMeasured,
			Confidence:  model.ConfidenceMeasured,
		},
	}
	var out bytes.Buffer
	if err := Render(&out, session, Options{}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, want := range []string{"111,249,946", "16,558,343", "168,033,860", "Source          codex", "Model calls"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output = %q, want %q", out.String(), want)
		}
	}
}

func TestRenderUnavailableFieldsUseDash(t *testing.T) {
	session := model.Session{
		ID:    "sess-2",
		Usage: model.Usage{Confidence: ""},
	}
	var out bytes.Buffer
	if err := Render(&out, session, Options{}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, want := range []string{"Input           -", "Cached          -", "Total           -"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output = %q, want unavailable -> dash for %q", out.String(), want)
		}
	}
}

func TestRenderExplicitZeroShowsZero(t *testing.T) {
	reasoning := int64(0)
	session := model.Session{
		ID: "sess-3",
		Usage: model.Usage{
			Input: 0, Cached: 0, Output: 0,
			Reasoning: &reasoning, Total: 0,
			Measurement: model.MeasurementMeasured,
			Confidence:  model.ConfidenceMeasured,
		},
	}
	var out bytes.Buffer
	if err := Render(&out, session, Options{}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out.String(), "0") {
		t.Fatalf("output = %q, want explicit zero rendered", out.String())
	}
	if strings.Contains(out.String(), "Input           -") {
		t.Fatalf("output = %q, want zero not dash when confidence is measured", out.String())
	}
}

func TestRenderTurnsInSequenceOrder(t *testing.T) {
	stamp := time.Date(2026, 9, 7, 22, 10, 0, 0, time.UTC)
	session := model.Session{
		ID:    "sess-seq",
		Usage: model.Usage{Input: 600, Total: 600, Confidence: model.ConfidenceMeasured, Measurement: model.MeasurementMeasured},
		Turns: []model.Turn{
			{ID: "sess-seq#1", Sequence: 1, Timestamp: &stamp, Model: "gpt-5",
				Usage:       model.Usage{Input: 100, Total: 100, Confidence: model.ConfidenceMeasured, Measurement: model.MeasurementMeasured},
				Measurement: model.MeasurementMeasured, Confidence: model.ConfidenceMeasured},
			{ID: "sess-seq#2", Sequence: 2, Timestamp: &stamp, Model: "gpt-5",
				Usage:       model.Usage{Input: 200, Total: 200, Confidence: model.ConfidenceMeasured, Measurement: model.MeasurementMeasured},
				Measurement: model.MeasurementMeasured, Confidence: model.ConfidenceMeasured},
			{ID: "sess-seq#3", Sequence: 3, Timestamp: &stamp, Model: "gpt-5",
				Usage:       model.Usage{Input: 300, Total: 300, Confidence: model.ConfidenceMeasured, Measurement: model.MeasurementMeasured},
				Measurement: model.MeasurementMeasured, Confidence: model.ConfidenceMeasured},
		},
	}
	var out bytes.Buffer
	if err := Render(&out, session, Options{AllTurns: true}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := out.String()
	idx1 := strings.Index(got, "1  22:10:00")
	idx2 := strings.Index(got, "2  22:10:00")
	idx3 := strings.Index(got, "3  22:10:00")
	if !(idx1 >= 0 && idx2 > idx1 && idx3 > idx2) {
		t.Fatalf("output = %q, want sequence 1/2/3 in row order", got)
	}
}

func TestRenderShowsMeasurementKind(t *testing.T) {
	stamp := time.Date(2026, 9, 7, 22, 10, 0, 0, time.UTC)
	session := model.Session{
		ID: "sess-meas",
		Turns: []model.Turn{
			{ID: "sess-meas#1", Sequence: 1, Timestamp: &stamp, Model: "gpt-5",
				Usage:       model.Usage{Input: 100, Total: 100, Confidence: model.ConfidenceMeasured, Measurement: model.MeasurementMeasured},
				Measurement: model.MeasurementMeasured, Confidence: model.ConfidenceMeasured},
		},
	}
	var out bytes.Buffer
	if err := Render(&out, session, Options{}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out.String(), "Turn measurement") || !strings.Contains(out.String(), "measured") {
		t.Fatalf("output = %q, want 'Turn measurement measured' summary line", out.String())
	}
}

func TestRenderShowsDerivedKindColumnWhenMixed(t *testing.T) {
	stamp := time.Date(2026, 9, 7, 22, 10, 0, 0, time.UTC)
	session := model.Session{
		ID:    "sess-mixed",
		Usage: model.Usage{Input: 300, Total: 300, Confidence: model.ConfidenceMeasured, Measurement: model.MeasurementMeasured},
		Turns: []model.Turn{
			{ID: "sess-mixed#1", Sequence: 1, Timestamp: &stamp, Model: "gpt-5",
				Usage:       model.Usage{Input: 100, Total: 100, Confidence: model.ConfidenceMeasured, Measurement: model.MeasurementMeasured},
				Measurement: model.MeasurementMeasured, Confidence: model.ConfidenceMeasured},
			{ID: "sess-mixed#2", Sequence: 2, Timestamp: &stamp, Model: "gpt-5",
				Usage:       model.Usage{Input: 200, Total: 200, Confidence: model.ConfidenceHigh, Measurement: model.MeasurementDerived},
				Measurement: model.MeasurementDerived, Confidence: model.ConfidenceHigh},
		},
	}
	var out bytes.Buffer
	if err := Render(&out, session, Options{AllTurns: true}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out.String(), "Kind") {
		t.Fatalf("output = %q, want Kind column when measurement kinds differ", out.String())
	}
}

func TestRenderTurnsMissingModelRendersDash(t *testing.T) {
	stamp := time.Date(2026, 9, 7, 22, 10, 0, 0, time.UTC)
	session := model.Session{
		ID:    "sess-nomodel",
		Usage: model.Usage{Input: 50, Total: 50, Confidence: model.ConfidenceMeasured, Measurement: model.MeasurementMeasured},
		Turns: []model.Turn{
			{ID: "sess-nomodel#1", Sequence: 1, Timestamp: &stamp, Model: "",
				Usage:       model.Usage{Input: 50, Total: 50, Confidence: model.ConfidenceMeasured, Measurement: model.MeasurementMeasured},
				Measurement: model.MeasurementMeasured, Confidence: model.ConfidenceMeasured},
		},
	}
	var out bytes.Buffer
	if err := Render(&out, session, Options{AllTurns: true}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	turnsSection := out.String()[strings.Index(out.String(), "Turns\n"):]
	if !strings.Contains(turnsSection, "-") {
		t.Fatalf("output = %q, want dash for missing turn model in turns section", out.String())
	}
}

func TestRenderEmptyTurnsShowsHint(t *testing.T) {
	session := model.Session{
		ID:    "sess-noTurns",
		Usage: model.Usage{Input: 100, Total: 100, Confidence: model.ConfidenceMeasured, Measurement: model.MeasurementMeasured},
	}
	var out bytes.Buffer
	if err := Render(&out, session, Options{}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out.String(), "No reliable Turn breakdown") {
		t.Fatalf("output = %q, want zero-turn hint", out.String())
	}
}

func TestRenderJSONKeepsNumbersNumeric(t *testing.T) {
	reasoning := int64(0)
	started := time.Date(2026, 9, 7, 22, 10, 0, 0, time.UTC)
	session := model.Session{
		ID:        "sess-json",
		Source:    "codex",
		StartedAt: &started,
		Usage: model.Usage{
			Input: 111249946, Cached: 100, Output: 50, Reasoning: &reasoning, Total: 111250096,
			Measurement: model.MeasurementMeasured, Confidence: model.ConfidenceMeasured,
		},
	}
	var out bytes.Buffer
	if err := RenderJSON(&out, session); err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	if strings.Contains(out.String(), `"total": "`) {
		t.Fatalf("json output has string total: %s", out.String())
	}
	var decoded InspectResult
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("decode json: %v\n%s", err, out.String())
	}
	if decoded.Session.ID != "sess-json" {
		t.Fatalf("session.id = %q, want sess-json", decoded.Session.ID)
	}
	if decoded.Session.Usage.Total != 111250096 {
		t.Fatalf("total = %d, want 111250096", decoded.Session.Usage.Total)
	}
}

func TestRenderDefaultLimitsOutput(t *testing.T) {
	stamp := time.Date(2026, 9, 7, 22, 10, 0, 0, time.UTC)
	manyTurns := make([]model.Turn, 50)
	for i := range manyTurns {
		manyTurns[i] = model.Turn{
			ID:          "t",
			Sequence:    i + 1,
			Timestamp:   &stamp,
			Model:       "m",
			Usage:       model.Usage{Input: int64(50 + i), Total: int64(50 + i), Measurement: model.MeasurementMeasured, Confidence: model.ConfidenceMeasured},
			Measurement: model.MeasurementMeasured, Confidence: model.ConfidenceMeasured,
		}
	}
	session := model.Session{
		ID:    "sess-many",
		Usage: model.Usage{Measurement: model.MeasurementMeasured, Confidence: model.ConfidenceMeasured},
		Turns: manyTurns,
	}
	var out bytes.Buffer
	if err := Render(&out, session, Options{}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Top expensive turns") {
		t.Fatalf("default output should show top section, got %q", got)
	}
	if strings.Count(got, "22:10:00") > 20 {
		t.Fatalf("default output should be bounded, got full table: %s", got)
	}
}

func TestRenderAllTurnsShowsFullTable(t *testing.T) {
	stamp := time.Date(2026, 9, 7, 22, 10, 0, 0, time.UTC)
	turns := make([]model.Turn, 12)
	for i := range turns {
		turns[i] = model.Turn{
			ID: "t", Sequence: i + 1, Timestamp: &stamp, Model: "m",
			Usage:       model.Usage{Input: int64(i + 1), Total: int64(i + 1), Measurement: model.MeasurementMeasured, Confidence: model.ConfidenceMeasured},
			Measurement: model.MeasurementMeasured, Confidence: model.ConfidenceMeasured,
		}
	}
	session := model.Session{
		ID:    "sess-all",
		Usage: model.Usage{Measurement: model.MeasurementMeasured, Confidence: model.ConfidenceMeasured},
		Turns: turns,
	}
	var out bytes.Buffer
	if err := Render(&out, session, Options{AllTurns: true}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := out.String()
	count := strings.Count(got, "22:10:00")
	if count != 12 {
		t.Fatalf("--all-turns should show all 12 turns; got %d rows", count)
	}
	if strings.Contains(got, "Top expensive") {
		t.Fatalf("--all-turns should not include top/recent sections")
	}
}

func TestRenderSingleTurnShowsOnlyThatOne(t *testing.T) {
	stamp := time.Date(2026, 9, 7, 22, 10, 0, 0, time.UTC)
	session := model.Session{
		ID:     "sess-single",
		Source: "claude", Model: "MiniMax-M3",
		Usage: model.Usage{Measurement: model.MeasurementMeasured, Confidence: model.ConfidenceMeasured},
		Turns: []model.Turn{
			{ID: "t1", Sequence: 1, Timestamp: &stamp, Model: "m", Usage: model.Usage{Input: 10, Total: 10, Measurement: model.MeasurementMeasured, Confidence: model.ConfidenceMeasured}, Measurement: model.MeasurementMeasured, Confidence: model.ConfidenceMeasured},
			{ID: "t2", Sequence: 2, Timestamp: &stamp, Model: "m", Usage: model.Usage{Input: 20, Total: 20, Measurement: model.MeasurementMeasured, Confidence: model.ConfidenceMeasured}, Measurement: model.MeasurementMeasured, Confidence: model.ConfidenceMeasured},
		},
	}
	var out bytes.Buffer
	if err := Render(&out, session, Options{Turn: 2}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, want := range []string{"Turn #2", "Input", "Cached", "Output", "Total", "Measurement   measured"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output = %q, want %q", out.String(), want)
		}
	}
	if strings.Contains(out.String(), "Top expensive") {
		t.Fatalf("single-turn rendering should not include top/recent sections")
	}
}

func TestRenderUnknownTurnSequenceError(t *testing.T) {
	session := model.Session{
		ID:    "sess-x",
		Usage: model.Usage{Measurement: model.MeasurementMeasured, Confidence: model.ConfidenceMeasured},
		Turns: []model.Turn{
			{ID: "t1", Sequence: 1, Usage: model.Usage{Input: 1, Total: 1, Measurement: model.MeasurementMeasured, Confidence: model.ConfidenceMeasured}, Measurement: model.MeasurementMeasured, Confidence: model.ConfidenceMeasured},
		},
	}
	var out bytes.Buffer
	if err := Render(&out, session, Options{Turn: 999}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out.String(), "999") || !strings.Contains(out.String(), "not found") {
		t.Fatalf("output = %q, want 'Turn 999 not found'", out.String())
	}
}

func TestRenderDropsKindColumnWhenUniform(t *testing.T) {
	stamp := time.Date(2026, 9, 7, 22, 10, 0, 0, time.UTC)
	session := model.Session{
		ID:    "sess-uniform",
		Usage: model.Usage{Measurement: model.MeasurementMeasured, Confidence: model.ConfidenceMeasured},
		Turns: []model.Turn{
			{ID: "t1", Sequence: 1, Timestamp: &stamp, Model: "m", Usage: model.Usage{Input: 10, Total: 10, Measurement: model.MeasurementMeasured, Confidence: model.ConfidenceMeasured}, Measurement: model.MeasurementMeasured, Confidence: model.ConfidenceMeasured},
			{ID: "t2", Sequence: 2, Timestamp: &stamp, Model: "m", Usage: model.Usage{Input: 20, Total: 20, Measurement: model.MeasurementMeasured, Confidence: model.ConfidenceMeasured}, Measurement: model.MeasurementMeasured, Confidence: model.ConfidenceMeasured},
		},
	}
	var out bytes.Buffer
	if err := Render(&out, session, Options{AllTurns: true}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(out.String(), "Top expensive") {
		t.Fatalf("all-turns should not include top/recent sections")
	}
}
