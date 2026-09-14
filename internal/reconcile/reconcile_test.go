package reconcile

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/thanhpham0406/tok-doctor/internal/match"
	"github.com/thanhpham0406/tok-doctor/internal/model"
)

func measured(value int64) model.Measurement {
	return model.NewMeasurement(value, model.MeasurementMeasured)
}

func fullUsage() model.ObservationUsage {
	return model.ObservationUsage{
		FreshInput:         measured(100),
		CachedInput:        measured(40),
		CacheCreationInput: measured(10),
		TotalInput:         measured(150),
		Output:             measured(20),
		ReasoningOutput:    measured(5),
		Total:              measured(170),
	}
}

func gatewayObservation(id string, usage model.ObservationUsage) model.Observation {
	return model.Observation{
		SchemaVersion: model.ObservationSchemaVersion,
		ID:            id,
		Channel:       model.ObservationChannelGateway,
		Scope:         model.ObservationScopeRequest,
		Source:        "gateway",
		Usage:         usage,
		Outcome:       model.ObservationOutcomeSucceeded,
		Completeness:  model.ObservationCompletenessComplete,
	}
}

func transcriptObservation(id string, usage model.ObservationUsage) model.Observation {
	return model.Observation{
		SchemaVersion: model.ObservationSchemaVersion,
		ID:            id,
		Channel:       model.ObservationChannelSessionTranscript,
		Scope:         model.ObservationScopeTurn,
		Source:        "claude",
		Usage:         usage,
		Outcome:       model.ObservationOutcomeUnknown,
		Completeness:  model.ObservationCompletenessComplete,
	}
}

func matchedPair(gatewayUsage, transcriptUsage model.ObservationUsage) Pair {
	return Pair{
		Gateway:    gatewayObservation("gw-1", gatewayUsage),
		Transcript: transcriptObservation("tr-1", transcriptUsage),
		Match: match.ObservationMatch{
			ID:                      "match:gw-1=tr-1",
			GatewayObservationID:    "gw-1",
			TranscriptObservationID: "tr-1",
			Status:                  match.StatusMatched,
			Confidence:              match.ConfidenceMedium,
			Reasons:                 []match.Reason{match.ReasonTime},
		},
	}
}

func mustReconcile(t *testing.T, pair Pair) Result {
	t.Helper()
	result, err := Reconcile(pair)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if len(result.Fields) != 7 {
		t.Fatalf("fields = %d, want 7", len(result.Fields))
	}
	return result
}

func field(t *testing.T, result Result, name string) FieldComparison {
	t.Helper()
	for _, comparison := range result.Fields {
		if comparison.Field == name {
			return comparison
		}
	}
	t.Fatalf("field %q not found in %+v", name, result.Fields)
	return FieldComparison{}
}

var usageFieldSetters = []struct {
	name string
	set  func(*model.ObservationUsage, int64)
}{
	{"freshInput", func(u *model.ObservationUsage, v int64) { u.FreshInput = measured(v) }},
	{"cachedInput", func(u *model.ObservationUsage, v int64) { u.CachedInput = measured(v) }},
	{"cacheCreationInput", func(u *model.ObservationUsage, v int64) { u.CacheCreationInput = measured(v) }},
	{"totalInput", func(u *model.ObservationUsage, v int64) { u.TotalInput = measured(v) }},
	{"output", func(u *model.ObservationUsage, v int64) { u.Output = measured(v) }},
	{"reasoningOutput", func(u *model.ObservationUsage, v int64) { u.ReasoningOutput = measured(v) }},
	{"total", func(u *model.ObservationUsage, v int64) { u.Total = measured(v) }},
}

func TestReconcileAllFieldsEqual(t *testing.T) {
	result := mustReconcile(t, matchedPair(fullUsage(), fullUsage()))
	if result.Status != StatusEqual {
		t.Fatalf("status = %q, want equal", result.Status)
	}
	for _, comparison := range result.Fields {
		if comparison.Status != StatusEqual {
			t.Fatalf("%s status = %q, want equal", comparison.Field, comparison.Status)
		}
		if comparison.Delta == nil || *comparison.Delta != 0 {
			t.Fatalf("%s delta = %v, want zero", comparison.Field, comparison.Delta)
		}
	}
}

func TestReconcileEachFieldDifference(t *testing.T) {
	for _, setter := range usageFieldSetters {
		t.Run(setter.name, func(t *testing.T) {
			transcriptUsage := fullUsage()
			setter.set(&transcriptUsage, 0)
			result := mustReconcile(t, matchedPair(fullUsage(), transcriptUsage))
			if result.Status != StatusDifferent {
				t.Fatalf("status = %q, want different", result.Status)
			}
			changed := field(t, result, setter.name)
			if changed.Status != StatusDifferent {
				t.Fatalf("%s status = %q, want different", setter.name, changed.Status)
			}
			for _, comparison := range result.Fields {
				if comparison.Field == setter.name {
					continue
				}
				if comparison.Status != StatusEqual {
					t.Fatalf("%s status = %q, want equal", comparison.Field, comparison.Status)
				}
			}
		})
	}
}

func TestReconcileDeltaIsTranscriptMinusGateway(t *testing.T) {
	gatewayUsage := model.ObservationUsage{
		FreshInput: measured(50),
		Output:     measured(100),
	}
	transcriptUsage := model.ObservationUsage{
		FreshInput: measured(20),
		Output:     measured(130),
	}
	result := mustReconcile(t, matchedPair(gatewayUsage, transcriptUsage))

	fresh := field(t, result, "freshInput")
	if fresh.Delta == nil || *fresh.Delta != -30 {
		t.Fatalf("freshInput delta = %v, want -30", fresh.Delta)
	}
	output := field(t, result, "output")
	if output.Delta == nil || *output.Delta != 30 {
		t.Fatalf("output delta = %v, want 30", output.Delta)
	}
	if !reflect.DeepEqual(fresh.Left, gatewayUsage.FreshInput) {
		t.Fatalf("left = %+v, want gateway measurement", fresh.Left)
	}
	if !reflect.DeepEqual(fresh.Right, transcriptUsage.FreshInput) {
		t.Fatalf("right = %+v, want transcript measurement", fresh.Right)
	}
}

func TestReconcileMissingOneSideIsUnavailable(t *testing.T) {
	gatewayUsage := model.ObservationUsage{FreshInput: measured(100)}
	transcriptUsage := model.ObservationUsage{}
	result := mustReconcile(t, matchedPair(gatewayUsage, transcriptUsage))

	fresh := field(t, result, "freshInput")
	if fresh.Status != StatusUnavailable {
		t.Fatalf("freshInput status = %q, want unavailable", fresh.Status)
	}
	if fresh.Delta != nil {
		t.Fatalf("freshInput delta = %v, want nil", fresh.Delta)
	}
	for _, comparison := range result.Fields {
		if comparison.Status != StatusUnavailable {
			t.Fatalf("%s status = %q, want unavailable", comparison.Field, comparison.Status)
		}
	}
}

func TestReconcileExplicitZeroVersusZeroIsEqual(t *testing.T) {
	usage := model.ObservationUsage{FreshInput: measured(0)}
	result := mustReconcile(t, matchedPair(usage, usage))

	fresh := field(t, result, "freshInput")
	if fresh.Status != StatusEqual {
		t.Fatalf("freshInput status = %q, want equal", fresh.Status)
	}
	if fresh.Delta == nil || *fresh.Delta != 0 {
		t.Fatalf("freshInput delta = %v, want zero", fresh.Delta)
	}
}

func TestReconcileExplicitZeroVersusMissingIsUnavailable(t *testing.T) {
	gatewayUsage := model.ObservationUsage{FreshInput: measured(0)}
	transcriptUsage := model.ObservationUsage{}
	result := mustReconcile(t, matchedPair(gatewayUsage, transcriptUsage))

	fresh := field(t, result, "freshInput")
	if fresh.Status != StatusUnavailable {
		t.Fatalf("freshInput status = %q, want unavailable", fresh.Status)
	}
	if fresh.Delta != nil {
		t.Fatalf("freshInput delta = %v, want nil", fresh.Delta)
	}
}

func TestReconcileMixedEqualAndDifferent(t *testing.T) {
	gatewayUsage := model.ObservationUsage{
		FreshInput: measured(100),
		Output:     measured(20),
	}
	transcriptUsage := model.ObservationUsage{
		FreshInput: measured(100),
		Output:     measured(50),
	}
	result := mustReconcile(t, matchedPair(gatewayUsage, transcriptUsage))

	if result.Status != StatusDifferent {
		t.Fatalf("status = %q, want different", result.Status)
	}
	if field(t, result, "freshInput").Status != StatusEqual {
		t.Fatalf("freshInput should be equal")
	}
	if field(t, result, "output").Status != StatusDifferent {
		t.Fatalf("output should be different")
	}
}

func TestReconcileNoComparableFieldIsUnavailable(t *testing.T) {
	result := mustReconcile(t, matchedPair(model.ObservationUsage{}, model.ObservationUsage{}))
	if result.Status != StatusUnavailable {
		t.Fatalf("status = %q, want unavailable", result.Status)
	}
}

func TestReconcileDoesNotMutateInputs(t *testing.T) {
	pair := matchedPair(fullUsage(), fullUsage())
	gatewayBefore := pair.Gateway
	transcriptBefore := pair.Transcript
	matchBefore := pair.Match

	result := mustReconcile(t, pair)
	if result.Fields[0].Left.Value != nil {
		*result.Fields[0].Left.Value = 9999
	}
	if result.Fields[0].Left.Evidence != nil {
		result.Fields[0].Left.Evidence[0].Field = "mutated"
	}

	if !reflect.DeepEqual(pair.Gateway, gatewayBefore) {
		t.Fatalf("gateway mutated:\n got = %+v\nwant = %+v", pair.Gateway, gatewayBefore)
	}
	if !reflect.DeepEqual(pair.Transcript, transcriptBefore) {
		t.Fatalf("transcript mutated:\n got = %+v\nwant = %+v", pair.Transcript, transcriptBefore)
	}
	if !reflect.DeepEqual(pair.Match, matchBefore) {
		t.Fatalf("match mutated:\n got = %+v\nwant = %+v", pair.Match, matchBefore)
	}
}

func TestReconcileRejectsInvalidPair(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Pair)
		want   string
	}{
		{
			name:   "unmatched status",
			mutate: func(p *Pair) { p.Match.Status = match.StatusAmbiguous },
			want:   "not matched",
		},
		{
			name: "transcript-only unmatched result",
			mutate: func(p *Pair) {
				p.Match = match.ObservationMatch{
					ID:                      "match:transcript:tr-1",
					TranscriptObservationID: "tr-1",
					Status:                  match.StatusUnmatched,
					Side:                    match.SideTranscript,
				}
			},
			want: "not matched",
		},
		{
			name: "gateway-only unmatched result",
			mutate: func(p *Pair) {
				p.Match = match.ObservationMatch{
					ID:                   "match:gateway:gw-1",
					GatewayObservationID: "gw-1",
					Status:               match.StatusUnmatched,
					Side:                 match.SideGateway,
				}
			},
			want: "not matched",
		},
		{
			name: "matched result missing transcript id",
			mutate: func(p *Pair) {
				p.Match.TranscriptObservationID = ""
			},
			want: "must reference both",
		},
		{
			name: "matched result with candidate transcript ids",
			mutate: func(p *Pair) {
				p.Match.CandidateTranscriptIDs = []string{"tr-2"}
			},
			want: "candidate transcript ids",
		},
		{
			name:   "same observation both sides",
			mutate: func(p *Pair) { p.Transcript.ID = p.Gateway.ID; p.Match.TranscriptObservationID = p.Gateway.ID },
			want:   "same observation on both sides",
		},
		{
			name:   "wrong gateway scope",
			mutate: func(p *Pair) { p.Gateway.Scope = model.ObservationScopeSession },
			want:   "gateway side",
		},
		{
			name:   "wrong transcript channel",
			mutate: func(p *Pair) { p.Transcript.Channel = model.ObservationChannelGateway },
			want:   "transcript side",
		},
		{
			name:   "match gateway id mismatch",
			mutate: func(p *Pair) { p.Match.GatewayObservationID = "other" },
			want:   "match gateway id",
		},
		{
			name:   "match transcript id mismatch",
			mutate: func(p *Pair) { p.Match.TranscriptObservationID = "other" },
			want:   "match transcript id",
		},
		{
			name:   "invalid observation",
			mutate: func(p *Pair) { p.Gateway.Source = "" },
			want:   "gw-1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pair := matchedPair(fullUsage(), fullUsage())
			tc.mutate(&pair)
			_, err := Reconcile(pair)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want substring %q", err, tc.want)
			}
		})
	}
}

func TestIntegrationGatewayTranscriptReconciliation(t *testing.T) {
	at := time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC)
	gateway := gatewayObservation("gateway:claude:req-1", model.ObservationUsage{
		FreshInput:         measured(1000),
		CachedInput:        measured(200),
		CacheCreationInput: measured(50),
		TotalInput:         measured(1250),
		Output:             measured(300),
		Total:              measured(1550),
	})
	gateway.StartedAt = &at
	gateway.Model = "claude-sonnet-4"

	transcript := transcriptObservation("claude:turn:turn-1", model.ObservationUsage{
		FreshInput:         measured(1000),
		CachedInput:        measured(180),
		CacheCreationInput: measured(50),
		TotalInput:         measured(1230),
		Output:             measured(320),
		Total:              measured(1550),
	})
	started := at.Add(2 * time.Second)
	transcript.StartedAt = &started
	transcript.Model = "claude-sonnet-4"

	matches, err := match.Match([]model.Observation{gateway, transcript}, match.Options{TimeWindow: time.Minute})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if len(matches) != 1 || matches[0].Status != match.StatusMatched {
		t.Fatalf("matches = %+v, want a single matched pair", matches)
	}

	result, err := Reconcile(Pair{Gateway: gateway, Transcript: transcript, Match: matches[0]})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
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
	for name, want := range wantDeltas {
		got := field(t, result, name).Delta
		if want == nil {
			if got != nil {
				t.Fatalf("%s delta = %v, want nil", name, *got)
			}
			continue
		}
		if got == nil || *got != *want {
			t.Fatalf("%s delta = %v, want %d", name, got, *want)
		}
	}
}
