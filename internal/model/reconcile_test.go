package model

import "testing"

func TestReconcileUsageMatchingTurns(t *testing.T) {
	session := Session{
		Usage: Usage{Input: 100, Cached: 50, Output: 30, Total: 180, Confidence: ConfidenceMeasured},
		Turns: []Turn{
			{Usage: Usage{Input: 60, Cached: 20, Output: 20, Total: 100, Confidence: ConfidenceMeasured}},
			{Usage: Usage{Input: 40, Cached: 30, Output: 10, Total: 80, Confidence: ConfidenceMeasured}},
		},
	}
	if c := ReconcileUsage(session); !c.Matches() {
		t.Fatalf("comparison = %+v, want match", c)
	}
}

func TestReconcileUsageDetectsMismatchedTotal(t *testing.T) {
	session := Session{
		Usage: Usage{Input: 100, Cached: 0, Output: 0, Total: 200, Confidence: ConfidenceMeasured},
		Turns: []Turn{
			{Usage: Usage{Input: 60, Total: 60, Confidence: ConfidenceMeasured}},
			{Usage: Usage{Input: 40, Total: 40, Confidence: ConfidenceMeasured}},
		},
	}
	if c := ReconcileUsage(session); c.Matches() {
		t.Fatalf("comparison matched unexpectedly: %+v", c)
	}
	if c := ReconcileUsage(session); c.TotalDelta != -100 {
		t.Fatalf("total delta = %d, want -100", c.TotalDelta)
	}
}

func TestReconcileUsageReasoning(t *testing.T) {
	r10, r6, r4 := int64(10), int64(6), int64(4)
	session := Session{
		Usage: Usage{Input: 100, Total: 100, Reasoning: &r10, Confidence: ConfidenceMeasured},
		Turns: []Turn{
			{Usage: Usage{Input: 60, Total: 60, Reasoning: &r6, Confidence: ConfidenceMeasured}},
			{Usage: Usage{Input: 40, Total: 40, Reasoning: &r4, Confidence: ConfidenceMeasured}},
		},
	}
	if c := ReconcileUsage(session); !c.Matches() {
		t.Fatalf("comparison = %+v, want match", c)
	}
}

func TestReconcileUsageMissingReasoning(t *testing.T) {
	r10 := int64(10)
	session := Session{
		Usage: Usage{Input: 100, Total: 100, Reasoning: &r10, Confidence: ConfidenceMeasured},
		Turns: []Turn{
			{Usage: Usage{Input: 100, Total: 100, Confidence: ConfidenceMeasured}},
		},
	}
	if c := ReconcileUsage(session); c.Matches() {
		t.Fatal("comparison matched when turn has no reasoning but session has 10")
	}
}

func TestSessionHasAuthoritativeUsageDistinguishesMissingFromExplicitZero(t *testing.T) {
	missing := Session{}
	if missing.HasAuthoritativeUsage() {
		t.Fatal("missing usage should not be authoritative")
	}

	explicitZero := Session{Usage: Usage{Measurement: MeasurementMeasured, Confidence: ConfidenceMeasured}}
	if !explicitZero.HasAuthoritativeUsage() {
		t.Fatal("explicit zero measured usage should be authoritative")
	}
}

func TestSessionHasAuthoritativeUsageDoesNotRequireTotal(t *testing.T) {
	session := Session{Usage: Usage{Input: 12, Measurement: MeasurementMeasured, Confidence: ConfidenceMeasured}}
	if !session.HasAuthoritativeUsage() {
		t.Fatal("measured component usage without total should be authoritative")
	}
}
