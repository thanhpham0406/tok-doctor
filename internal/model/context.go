package model

type ContextComponentKind string

const (
	ContextInstructions ContextComponentKind = "instructions"
	ContextUserPrompt   ContextComponentKind = "user_prompt"
	ContextHistory      ContextComponentKind = "history"
	ContextFile         ContextComponentKind = "file"
	ContextToolResult   ContextComponentKind = "tool_result"
	ContextOther        ContextComponentKind = "other"
	ContextUnknown      ContextComponentKind = "unknown"
)

type ContextObservationScope string

const (
	ContextObservedByAgent   ContextObservationScope = "agent"
	ContextObservedInRequest ContextObservationScope = "request"
	ContextPayloadUnknown    ContextObservationScope = "unknown"
)

type ContextAttribution struct {
	Components     []ContextComponent    `json:"components,omitempty"`
	Input          InputAccounting       `json:"input,omitempty"`
	Reconciliation ContextReconciliation `json:"reconciliation,omitempty"`
}

type ContextComponent struct {
	Kind        ContextComponentKind    `json:"kind"`
	Source      string                  `json:"source,omitempty"`
	Record      string                  `json:"record,omitempty"`
	Path        string                  `json:"path,omitempty"`
	ContentHash string                  `json:"contentHash,omitempty"`
	Observation ContextObservationScope `json:"observation"`
	Measurement Measurement             `json:"measurement"`
	Evidence    []Evidence              `json:"evidence,omitempty"`
}

type ContextReconciliation struct {
	FreshInput          Measurement `json:"freshInput"`
	FreshAttributed     Measurement `json:"freshAttributed"`
	FreshUnknown        Measurement `json:"freshUnknown"`
	FreshCoverage       *float64    `json:"freshCoverage,omitempty"`
	CachedContext       Measurement `json:"cachedContext"`
	CachedAttributed    Measurement `json:"cachedAttributed"`
	CachedCoverage      *float64    `json:"cachedCoverage,omitempty"`
	FullPayloadCoverage *float64    `json:"fullPayloadCoverage,omitempty"`
	Conflict            bool        `json:"conflict,omitempty"`
}

type InputAccounting struct {
	Fresh  Measurement `json:"fresh"`
	Cached Measurement `json:"cached"`
}

func CodexInputAccounting(usage Usage) InputAccounting {
	return InputAccounting{
		Fresh:  subtractMeasurements(usage.Input, usage.Cached),
		Cached: usage.Cached,
	}
}

func SeparateInputAccounting(usage Usage) InputAccounting {
	return InputAccounting{
		Fresh:  usage.Input,
		Cached: usage.Cached,
	}
}

func ReconcileContext(turn Turn) ContextReconciliation {
	input := turn.ContextAttribution.Input
	if !input.Fresh.Available() && !input.Cached.Available() {
		input = SeparateInputAccounting(turn.Usage)
	}
	if !input.Fresh.Available() || !input.Fresh.HasAuthoritativeUsage() {
		return ContextReconciliation{
			FreshInput:       input.Fresh,
			FreshAttributed:  Measurement{Kind: MeasurementUnknown},
			FreshUnknown:     Measurement{Kind: MeasurementUnknown},
			CachedContext:    input.Cached,
			CachedAttributed: Measurement{Kind: MeasurementUnknown},
		}
	}

	attributed, seen := SumAttributionMeasurements(turn.ContextAttribution.Components)
	if !seen {
		return ContextReconciliation{
			FreshInput:       input.Fresh,
			FreshAttributed:  Measurement{Kind: MeasurementUnknown},
			FreshUnknown:     Measurement{Kind: MeasurementUnknown},
			CachedContext:    input.Cached,
			CachedAttributed: Measurement{Kind: MeasurementUnknown},
		}
	}

	measuredValue := input.Fresh.ValueOrZero()
	attributedValue := attributed.ValueOrZero()
	unknownValue := measuredValue - attributedValue
	conflict := unknownValue < 0
	if conflict {
		unknownValue = 0
	}
	unknown := NewMeasurement(unknownValue, attributed.Kind)
	coverage := float64(0)
	if measuredValue > 0 {
		coverage = float64(attributedValue) / float64(measuredValue)
	}
	return ContextReconciliation{
		FreshInput:       input.Fresh,
		FreshAttributed:  attributed,
		FreshUnknown:     unknown,
		FreshCoverage:    &coverage,
		CachedContext:    input.Cached,
		CachedAttributed: Measurement{Kind: MeasurementUnknown},
		Conflict:         conflict,
	}
}

func subtractMeasurements(left, right Measurement) Measurement {
	if !left.Available() || !right.Available() {
		return Measurement{Kind: MeasurementUnknown}
	}
	return Measurement{
		Value: Int64(left.ValueOrZero() - right.ValueOrZero()),
		Kind:  weakerKind(left.Kind, right.Kind),
	}
}

func weakerKind(a, b MeasurementKind) MeasurementKind {
	if a == MeasurementEstimated || b == MeasurementEstimated {
		return MeasurementEstimated
	}
	if a == MeasurementUnknown || b == MeasurementUnknown {
		return MeasurementUnknown
	}
	if a == MeasurementDerived || b == MeasurementDerived {
		return MeasurementDerived
	}
	if a == MeasurementCounted || b == MeasurementCounted {
		return MeasurementCounted
	}
	return MeasurementMeasured
}

func SumAttributionMeasurements(components []ContextComponent) (Measurement, bool) {
	var total int64
	seen := false
	kind := MeasurementCounted
	for _, component := range components {
		m := component.Measurement
		if !m.Available() || m.Kind == MeasurementMeasured || m.Kind == MeasurementDerived {
			continue
		}
		seen = true
		total += m.ValueOrZero()
		if m.Kind == MeasurementEstimated {
			kind = MeasurementEstimated
		}
	}
	if !seen {
		return Measurement{}, false
	}
	return NewMeasurement(total, kind), true
}
