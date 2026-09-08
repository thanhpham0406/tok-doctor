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
		Usage:     model.MeasuredUsage(111249946, 16558343, 45346, &reasoning, 168033860),
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
	session := model.Session{ID: "sess-2"}
	var out bytes.Buffer
	if err := Render(&out, session, Options{}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, want := range []string{"Input           -", "Cached          -", "Reasoning       -", "Total           -"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output = %q, want unavailable -> dash for %q", out.String(), want)
		}
	}
}

func TestRenderExplicitZeroShowsZero(t *testing.T) {
	reasoning := int64(0)
	session := model.Session{
		ID:    "sess-3",
		Usage: model.MeasuredUsage(0, 0, 0, &reasoning, 0),
	}
	var out bytes.Buffer
	if err := Render(&out, session, Options{}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out.String(), "Input           0") {
		t.Fatalf("output = %q, want explicit zero rendered", out.String())
	}
	if strings.Contains(out.String(), "Input           -") {
		t.Fatalf("output = %q, want zero not dash", out.String())
	}
}

func TestRenderTurnsInSequenceOrder(t *testing.T) {
	stamp := time.Date(2026, 9, 7, 22, 10, 0, 0, time.UTC)
	session := model.Session{
		ID:    "sess-seq",
		Usage: model.MeasuredUsage(600, 0, 0, nil, 600),
		Turns: []model.Turn{
			{ID: "sess-seq#1", Sequence: 1, Timestamp: &stamp, Model: "gpt-5", Usage: model.MeasuredUsage(100, 0, 0, nil, 100)},
			{ID: "sess-seq#2", Sequence: 2, Timestamp: &stamp, Model: "gpt-5", Usage: model.MeasuredUsage(200, 0, 0, nil, 200)},
			{ID: "sess-seq#3", Sequence: 3, Timestamp: &stamp, Model: "gpt-5", Usage: model.MeasuredUsage(300, 0, 0, nil, 300)},
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

func TestRenderSingleTurnShowsPerMetricMeasurementKind(t *testing.T) {
	stamp := time.Date(2026, 9, 7, 22, 10, 0, 0, time.UTC)
	session := model.Session{
		ID:     "sess-single",
		Source: "claude",
		Model:  "MiniMax-M3",
		Usage:  model.DerivedUsage(30, 0, 0, nil, 30),
		Turns: []model.Turn{
			{ID: "t1", Sequence: 1, Timestamp: &stamp, Model: "m", Usage: model.MeasuredUsage(10, 0, 0, nil, 10)},
			{ID: "t2", Sequence: 2, Timestamp: &stamp, Model: "m", Usage: model.MeasuredUsage(20, 0, 0, nil, 20)},
		},
	}
	var out bytes.Buffer
	if err := Render(&out, session, Options{Turn: 2}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, want := range []string{"Turn #2", "Input", "Cached", "Output", "Total", "measured"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output = %q, want %q", out.String(), want)
		}
	}
	if strings.Contains(out.String(), "Top expensive") {
		t.Fatalf("single-turn rendering should not include top/recent sections")
	}
}

func TestRenderShowsKindColumnWhenTurnKindsDiffer(t *testing.T) {
	stamp := time.Date(2026, 9, 7, 22, 10, 0, 0, time.UTC)
	session := model.Session{
		ID:    "sess-mixed",
		Usage: model.DerivedUsage(300, 0, 0, nil, 300),
		Turns: []model.Turn{
			{ID: "sess-mixed#1", Sequence: 1, Timestamp: &stamp, Model: "gpt-5", Usage: model.MeasuredUsage(100, 0, 0, nil, 100)},
			{ID: "sess-mixed#2", Sequence: 2, Timestamp: &stamp, Model: "gpt-5", Usage: model.DerivedUsage(200, 0, 0, nil, 200)},
		},
	}
	var out bytes.Buffer
	if err := Render(&out, session, Options{AllTurns: true}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out.String(), "Kind") {
		t.Fatalf("output = %q, want Kind column when turn metric kinds differ", out.String())
	}
}

func TestRenderTurnsMissingModelRendersDash(t *testing.T) {
	stamp := time.Date(2026, 9, 7, 22, 10, 0, 0, time.UTC)
	session := model.Session{
		ID:    "sess-nomodel",
		Usage: model.MeasuredUsage(50, 0, 0, nil, 50),
		Turns: []model.Turn{
			{ID: "sess-nomodel#1", Sequence: 1, Timestamp: &stamp, Model: "", Usage: model.MeasuredUsage(50, 0, 0, nil, 50)},
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
		Usage: model.MeasuredUsage(100, 0, 0, nil, 100),
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
		Usage:     model.MeasuredUsage(111249946, 100, 50, &reasoning, 111250096),
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
	if decoded.Session.Usage.Total.ValueOrZero() != 111250096 {
		t.Fatalf("total = %d, want 111250096", decoded.Session.Usage.Total.ValueOrZero())
	}
}

func TestRenderDefaultLimitsOutput(t *testing.T) {
	stamp := time.Date(2026, 9, 7, 22, 10, 0, 0, time.UTC)
	manyTurns := make([]model.Turn, 50)
	for i := range manyTurns {
		manyTurns[i] = model.Turn{
			ID:        "t",
			Sequence:  i + 1,
			Timestamp: &stamp,
			Model:     "m",
			Usage:     model.MeasuredUsage(int64(50+i), 0, 0, nil, int64(50+i)),
		}
	}
	session := model.Session{ID: "sess-many", Usage: model.MeasuredUsage(0, 0, 0, nil, 0), Turns: manyTurns}
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
			Usage: model.MeasuredUsage(int64(i+1), 0, 0, nil, int64(i+1)),
		}
	}
	session := model.Session{ID: "sess-all", Usage: model.MeasuredUsage(0, 0, 0, nil, 0), Turns: turns}
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

func TestRenderUnknownTurnSequenceError(t *testing.T) {
	session := model.Session{
		ID:    "sess-x",
		Usage: model.MeasuredUsage(1, 0, 0, nil, 1),
		Turns: []model.Turn{
			{ID: "t1", Sequence: 1, Usage: model.MeasuredUsage(1, 0, 0, nil, 1)},
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
		Usage: model.MeasuredUsage(30, 0, 0, nil, 30),
		Turns: []model.Turn{
			{ID: "t1", Sequence: 1, Timestamp: &stamp, Model: "m", Usage: model.MeasuredUsage(10, 0, 0, nil, 10)},
			{ID: "t2", Sequence: 2, Timestamp: &stamp, Model: "m", Usage: model.MeasuredUsage(20, 0, 0, nil, 20)},
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

func TestRenderEvidenceSectionHiddenByDefault(t *testing.T) {
	stamp := time.Date(2026, 9, 7, 22, 10, 0, 0, time.UTC)
	ev := model.Evidence{Kind: model.EvidenceSourceValue, Source: "codex_rollout", Record: "snap:1", Field: "total_token_usage.input_tokens"}
	turnUsage := model.MeasuredUsage(10, 0, 0, nil, 10)
	turnUsage.Input.Evidence = []model.Evidence{ev}
	session := model.Session{
		ID:    "sess-ev-default",
		Usage: model.MeasuredUsage(10, 0, 0, nil, 10),
		Turns: []model.Turn{
			{ID: "snap:1", Sequence: 1, Timestamp: &stamp, Usage: turnUsage},
		},
	}
	var out bytes.Buffer
	if err := Render(&out, session, Options{Turn: 1}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(out.String(), "Evidence") {
		t.Fatalf("evidence section should be hidden without --evidence, got %q", out.String())
	}
}

func TestRenderEvidenceSectionShowsWithFlag(t *testing.T) {
	stamp := time.Date(2026, 9, 7, 22, 10, 0, 0, time.UTC)
	ev := model.Evidence{Kind: model.EvidenceSourceValue, Source: "codex_rollout", Record: "snap:1", Field: "total_token_usage.input_tokens"}
	turnUsage := model.MeasuredUsage(10, 0, 0, nil, 10)
	turnUsage.Input.Evidence = []model.Evidence{ev}
	turnUsage.Total.Evidence = []model.Evidence{ev}
	session := model.Session{
		ID:    "sess-ev",
		Usage: model.MeasuredUsage(10, 0, 0, nil, 10),
		Turns: []model.Turn{
			{ID: "snap:1", Sequence: 1, Timestamp: &stamp, Usage: turnUsage},
		},
	}
	var out bytes.Buffer
	if err := Render(&out, session, Options{Turn: 1, ShowEvidence: true}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, want := range []string{"Evidence", "Method", "source value", "Source", "codex_rollout", "Field", "Record", "snap:1"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q: %s", want, out.String())
		}
	}
}

func TestRenderEvidenceRendersCumulativeDeltaFields(t *testing.T) {
	stamp := time.Date(2026, 9, 7, 22, 10, 0, 0, time.UTC)
	ev := model.Evidence{Kind: model.EvidenceCumulativeDelta, Source: "codex_rollout", Field: "input_tokens", Previous: "snap:1", Current: "snap:2"}
	turnUsage := model.DerivedUsage(10, 0, 0, nil, 10)
	turnUsage.Input.Evidence = []model.Evidence{ev}
	turnUsage.Total.Evidence = []model.Evidence{ev}
	session := model.Session{
		ID:    "sess-ev-delta",
		Usage: model.DerivedUsage(10, 0, 0, nil, 10),
		Turns: []model.Turn{
			{ID: "delta", Sequence: 1, Timestamp: &stamp, Usage: turnUsage},
		},
	}
	var out bytes.Buffer
	if err := Render(&out, session, Options{Turn: 1, ShowEvidence: true}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, want := range []string{"cumulative delta", "Previous", "Current", "snap:1", "snap:2", "input_tokens"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q: %s", want, out.String())
		}
	}
}

func TestRenderEvidenceRendersAggregateFields(t *testing.T) {
	agg := model.Evidence{Kind: model.EvidenceAggregate, Source: "turns", Operation: model.AggregateSum, Count: 1}
	usage := model.DerivedUsage(10, 0, 0, nil, 10)
	usage.Input.Evidence = []model.Evidence{agg}
	usage.Total.Evidence = []model.Evidence{agg}
	session := model.Session{
		ID:       "sess-ev-agg",
		Usage:    usage,
		Turns:    []model.Turn{{ID: "t", Sequence: 1, Usage: model.MeasuredUsage(10, 0, 0, nil, 10)}},
		Evidence: []model.Evidence{agg},
	}
	var out bytes.Buffer
	if err := Render(&out, session, Options{ShowEvidence: true}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, want := range []string{"aggregate", "Operation", "sum", "Count"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q: %s", want, out.String())
		}
	}
}

func TestRenderEvidenceSkippedWhenNoEvidence(t *testing.T) {
	session := model.Session{
		ID:    "sess-no-ev",
		Usage: model.DerivedUsage(10, 0, 0, nil, 10),
	}
	var out bytes.Buffer
	if err := Render(&out, session, Options{ShowEvidence: true}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(out.String(), "Evidence") {
		t.Fatalf("output should not include empty Evidence section, got %q", out.String())
	}
}
