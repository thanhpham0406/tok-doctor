package model

import "testing"

func TestReconcileUsageMatchingTurns(t *testing.T) {
	session := Session{
		Usage: MeasuredUsage(100, 50, 30, nil, 180),
		Turns: []Turn{
			{Usage: MeasuredUsage(60, 20, 20, nil, 100)},
			{Usage: MeasuredUsage(40, 30, 10, nil, 80)},
		},
	}
	if c := ReconcileUsage(session); !c.Matches() {
		t.Fatalf("comparison = %+v, want match", c)
	}
}

func TestReconcileUsageDetectsMismatchedTotal(t *testing.T) {
	session := Session{
		Usage: MeasuredUsage(100, 0, 0, nil, 200),
		Turns: []Turn{
			{Usage: MeasuredUsage(60, 0, 0, nil, 60)},
			{Usage: MeasuredUsage(40, 0, 0, nil, 40)},
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
		Usage: MeasuredUsage(100, 0, 0, &r10, 100),
		Turns: []Turn{
			{Usage: MeasuredUsage(60, 0, 0, &r6, 60)},
			{Usage: MeasuredUsage(40, 0, 0, &r4, 40)},
		},
	}
	if c := ReconcileUsage(session); !c.Matches() {
		t.Fatalf("comparison = %+v, want match", c)
	}
}

func TestReconcileUsageMissingReasoning(t *testing.T) {
	r10 := int64(10)
	session := Session{
		Usage: MeasuredUsage(100, 0, 0, &r10, 100),
		Turns: []Turn{
			{Usage: MeasuredUsage(100, 0, 0, nil, 100)},
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

	explicitZero := Session{Usage: MeasuredUsage(0, 0, 0, nil, 0)}
	if !explicitZero.HasAuthoritativeUsage() {
		t.Fatal("explicit zero measured usage should be authoritative")
	}
}

func TestSessionHasAuthoritativeUsageDoesNotRequireTotal(t *testing.T) {
	session := Session{Usage: Usage{Input: NewMeasurement(12, MeasurementMeasured)}}
	if !session.HasAuthoritativeUsage() {
		t.Fatal("measured component usage without total should be authoritative")
	}
}

func TestMeasurementKinds(t *testing.T) {
	for _, kind := range []MeasurementKind{
		MeasurementMeasured,
		MeasurementDerived,
		MeasurementCounted,
		MeasurementEstimated,
		MeasurementUnknown,
	} {
		if err := RequireValidMeasurementKind(kind); err != nil {
			t.Fatalf("kind %q should be valid: %v", kind, err)
		}
	}
	if err := RequireValidMeasurementKind("surprise"); err == nil {
		t.Fatal("expected invalid measurement kind to be rejected")
	}
}

func TestSumUsageMakesAggregatesDerived(t *testing.T) {
	sum := SumUsage([]Usage{
		MeasuredUsage(1, 2, 3, nil, 4),
		MeasuredUsage(5, 6, 7, nil, 8),
	})
	if sum.Total.ValueOrZero() != 12 {
		t.Fatalf("total = %d, want 12", sum.Total.ValueOrZero())
	}
	if sum.Total.Kind != MeasurementDerived {
		t.Fatalf("total kind = %q, want derived", sum.Total.Kind)
	}
}

func TestSumUsageAllMissingStaysUnavailable(t *testing.T) {
	sum := SumUsage([]Usage{{}, {}})
	if sum.HasUsage() {
		t.Fatalf("sum = %+v, want unavailable", sum)
	}
}

func TestMeasurementWithEvidenceAttaches(t *testing.T) {
	m := MeasurementWithEvidence(10, MeasurementMeasured,
		Evidence{Kind: EvidenceSourceValue, Source: "claude_session", Record: "msg_1"})
	if m.ValueOrZero() != 10 {
		t.Fatalf("value = %d, want 10", m.ValueOrZero())
	}
	if m.Kind != MeasurementMeasured {
		t.Fatalf("kind = %q, want measured", m.Kind)
	}
	if len(m.Evidence) != 1 {
		t.Fatalf("evidence = %d, want 1", len(m.Evidence))
	}
}

func TestValidEvidenceKindAcceptsKnownKinds(t *testing.T) {
	for _, kind := range []EvidenceKind{
		EvidenceSourceValue,
		EvidenceCumulativeDelta,
		EvidenceAggregate,
		EvidenceProvenance,
	} {
		if !ValidEvidenceKind(kind) {
			t.Fatalf("evidence kind %q should be valid", kind)
		}
	}
	if ValidEvidenceKind("surprise") {
		t.Fatal("unknown evidence kind should be rejected")
	}
}

func TestReconcileContextCoverageLeavesUnknownRemainder(t *testing.T) {
	turn := Turn{
		Usage: Usage{
			Input:  NewMeasurement(100, MeasurementMeasured),
			Cached: NewMeasurement(20, MeasurementMeasured),
		},
		ContextAttribution: ContextAttribution{Components: []ContextComponent{
			{Kind: ContextUserPrompt, Measurement: NewMeasurement(30, MeasurementEstimated), Observation: ContextObservedByAgent},
			{Kind: ContextToolResult, Measurement: NewMeasurement(50, MeasurementEstimated), Observation: ContextObservedByAgent},
			{Kind: ContextFile, Measurement: Measurement{Kind: MeasurementUnknown}, Observation: ContextObservedByAgent},
		}},
	}
	rec := ReconcileContext(turn)
	if rec.FreshAttributed.ValueOrZero() != 80 || rec.FreshUnknown.ValueOrZero() != 20 {
		t.Fatalf("reconciliation = %+v, want attributed 80 unknown 20", rec)
	}
	if rec.FreshCoverage == nil || *rec.FreshCoverage != 0.8 {
		t.Fatalf("coverage = %v, want 0.8", rec.FreshCoverage)
	}
	if rec.CachedContext.ValueOrZero() != 20 || rec.CachedAttributed.DisplayKind() != MeasurementUnknown {
		t.Fatalf("cached reconciliation = %+v, want cached 20 unknown attribution", rec)
	}
	if rec.Conflict {
		t.Fatal("conflict = true, want false")
	}
}

func TestReconcileContextConflictDoesNotClampAttributed(t *testing.T) {
	turn := Turn{
		Usage: Usage{Input: NewMeasurement(50, MeasurementMeasured)},
		ContextAttribution: ContextAttribution{Components: []ContextComponent{
			{Kind: ContextUserPrompt, Measurement: NewMeasurement(60, MeasurementCounted), Observation: ContextObservedByAgent},
		}},
	}
	rec := ReconcileContext(turn)
	if rec.FreshAttributed.ValueOrZero() != 60 {
		t.Fatalf("attributed = %d, want 60", rec.FreshAttributed.ValueOrZero())
	}
	if rec.FreshUnknown.ValueOrZero() != 0 {
		t.Fatalf("unknown = %d, want 0 after conflict", rec.FreshUnknown.ValueOrZero())
	}
	if !rec.Conflict {
		t.Fatal("conflict = false, want true")
	}
}

func TestReconcileContextUsesCodexFreshInputAccounting(t *testing.T) {
	turn := Turn{
		Usage: Usage{
			Input:  NewMeasurement(161135, MeasurementDerived),
			Cached: NewMeasurement(160128, MeasurementDerived),
		},
		ContextAttribution: ContextAttribution{
			Components: []ContextComponent{
				{Kind: ContextToolResult, Measurement: NewMeasurement(693, MeasurementEstimated), Observation: ContextObservedByAgent},
			},
		},
	}
	turn.ContextAttribution.Input = CodexInputAccounting(turn.Usage)
	rec := ReconcileContext(turn)
	if rec.FreshInput.ValueOrZero() != 1007 {
		t.Fatalf("fresh input = %d, want 1007", rec.FreshInput.ValueOrZero())
	}
	if rec.FreshUnknown.ValueOrZero() != 314 {
		t.Fatalf("fresh unknown = %d, want 314", rec.FreshUnknown.ValueOrZero())
	}
	if rec.FreshCoverage == nil || *rec.FreshCoverage < 0.688 || *rec.FreshCoverage > 0.689 {
		t.Fatalf("fresh coverage = %v, want about 0.688", rec.FreshCoverage)
	}
	if rec.CachedContext.ValueOrZero() != 160128 || rec.FullPayloadCoverage != nil {
		t.Fatalf("reconciliation = %+v, want cached context and unavailable full coverage", rec)
	}
}

func TestReconcileContextUsesSeparateFreshInputAccounting(t *testing.T) {
	turn := Turn{
		Usage: Usage{
			Input:  NewMeasurement(920, MeasurementMeasured),
			Cached: NewMeasurement(299106, MeasurementMeasured),
		},
		ContextAttribution: ContextAttribution{
			Components: []ContextComponent{
				{Kind: ContextUserPrompt, Measurement: NewMeasurement(47, MeasurementEstimated), Observation: ContextObservedByAgent},
			},
		},
	}
	turn.ContextAttribution.Input = SeparateInputAccounting(turn.Usage)
	rec := ReconcileContext(turn)
	if rec.FreshInput.ValueOrZero() != 920 || rec.FreshUnknown.ValueOrZero() != 873 {
		t.Fatalf("reconciliation = %+v, want fresh 920 unknown 873", rec)
	}
	if rec.CachedContext.ValueOrZero() != 299106 || rec.CachedAttributed.DisplayKind() != MeasurementUnknown {
		t.Fatalf("cached reconciliation = %+v, want cached 299106 unknown attribution", rec)
	}
}

func TestSumAttributionMeasurementsDowngradesMixedToEstimated(t *testing.T) {
	m, ok := SumAttributionMeasurements([]ContextComponent{
		{Measurement: NewMeasurement(10, MeasurementCounted)},
		{Measurement: NewMeasurement(15, MeasurementEstimated)},
		{Measurement: Measurement{Kind: MeasurementUnknown}},
	})
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if m.ValueOrZero() != 25 || m.Kind != MeasurementEstimated {
		t.Fatalf("measurement = %+v, want 25 estimated", m)
	}
}

func TestEvidenceConsistentWithKind(t *testing.T) {
	measuredValue := Evidence{Kind: EvidenceSourceValue, Source: "x", Record: "y"}
	if !measuredValue.ConsistentWithKind(MeasurementMeasured) {
		t.Fatal("source_value should be consistent with measured")
	}
	if measuredValue.ConsistentWithKind(MeasurementDerived) {
		t.Fatal("source_value should not be consistent with derived")
	}

	delta := Evidence{Kind: EvidenceCumulativeDelta, Source: "x", Previous: "a", Current: "b"}
	if !delta.ConsistentWithKind(MeasurementDerived) {
		t.Fatal("cumulative_delta should be consistent with derived")
	}
	if delta.ConsistentWithKind(MeasurementMeasured) {
		t.Fatal("cumulative_delta should not be consistent with measured")
	}

	agg := Evidence{Kind: EvidenceAggregate, Source: "turns", Operation: AggregateSum, Count: 3}
	if !agg.ConsistentWithKind(MeasurementDerived) {
		t.Fatal("aggregate should be consistent with derived")
	}
	if agg.ConsistentWithKind(MeasurementMeasured) {
		t.Fatal("aggregate should not be consistent with measured")
	}
}

func TestEvidenceConsistencyRejectsUnknownOrMissing(t *testing.T) {
	bad := Evidence{Kind: "unknown_kind"}
	if bad.ConsistentWithKind(MeasurementMeasured) {
		t.Fatal("unknown evidence kind should not be consistent")
	}
	none := Evidence{Kind: EvidenceSourceValue, Source: "x", Record: "y"}
	if none.ConsistentWithKind(MeasurementUnknown) {
		t.Fatal("missing measurement should not be consistent with any evidence")
	}
	if none.ConsistentWithKind("") {
		t.Fatal("empty measurement kind should not be consistent")
	}
}

func TestMeasurementHasEvidenceReportsPresence(t *testing.T) {
	if (Measurement{}).HasEvidence() {
		t.Fatal("empty measurement should not have evidence")
	}
	withEv := MeasurementWithEvidence(0, MeasurementMeasured, Evidence{Kind: EvidenceSourceValue, Source: "x"})
	if !withEv.HasEvidence() {
		t.Fatal("measurement with evidence should report true")
	}
}
