package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

func TestOversizedOutput(t *testing.T) {
	tests := []struct {
		name      string
		component model.ContextComponent
		want      int
		severity  model.Severity
	}{
		{
			name:      "above threshold",
			component: outputComponent(DefaultMaxOutputBytes+1, model.ContextCompletenessComplete),
			want:      1, severity: model.SeverityMedium,
		},
		{
			name:      "high severity",
			component: outputComponent(highSeverityBytes, model.ContextCompletenessComplete),
			want:      1, severity: model.SeverityHigh,
		},
		{
			name:      "threshold boundary",
			component: outputComponent(DefaultMaxOutputBytes, model.ContextCompletenessComplete),
		},
		{
			name:      "truncated output",
			component: outputComponent(DefaultMaxOutputBytes+1, model.ContextCompletenessTruncated),
		},
		{
			name:      "missing byte size",
			component: model.ContextComponent{Kind: model.ContextToolResult, Completeness: model.ContextCompletenessComplete},
		},
		{
			name:      "non-tool component",
			component: model.ContextComponent{Kind: model.ContextHistory, ContentBytes: model.Int64(DefaultMaxOutputBytes + 1), Completeness: model.ContextCompletenessComplete},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := sessionWithComponent(tt.component)
			got := NewOversizedOutput(0).Analyze(context.Background(), session)
			if len(got) != tt.want {
				t.Fatalf("findings = %d, want %d", len(got), tt.want)
			}
			if tt.want == 0 {
				return
			}
			if got[0].RuleID != OversizedOutputRuleID || got[0].Severity != tt.severity || got[0].Confidence != model.ConfidenceHigh {
				t.Fatalf("finding = %+v", got[0])
			}
			evidence := got[0].Evidence[0]
			if evidence.OutputBytes == nil || *evidence.OutputBytes != *tt.component.ContentBytes {
				t.Fatalf("evidence = %+v, want the observed byte size", evidence)
			}
		})
	}
}

func TestOversizedOutputIgnoresTrailingContext(t *testing.T) {
	session := sessionWithComponent(model.ContextComponent{})
	session.TrailingContext = []model.ContextComponent{outputComponent(DefaultMaxOutputBytes+1, model.ContextCompletenessComplete)}
	if got := NewOversizedOutput(0).Analyze(context.Background(), session); len(got) != 0 {
		t.Fatalf("findings = %+v, want none for trailing context", got)
	}
}

func TestOversizedOutputDoesNotExposeContentMetadata(t *testing.T) {
	component := outputComponent(DefaultMaxOutputBytes+1, model.ContextCompletenessComplete)
	component.ContentHash = "secret-hash"
	component.Path = "/secret/path"
	component.Record = "private-record"
	got := NewOversizedOutput(0).Analyze(context.Background(), sessionWithComponent(component))
	if len(got) != 1 {
		t.Fatalf("findings = %d, want 1", len(got))
	}
	raw, err := json.Marshal(got[0])
	if err != nil {
		t.Fatalf("marshal finding: %v", err)
	}
	for _, secret := range []string{"secret-hash", "/secret/path", "private-record"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("finding exposed private component metadata %q", secret)
		}
	}
}

func outputComponent(bytes int64, completeness model.ContextCompleteness) model.ContextComponent {
	return model.ContextComponent{
		Kind:         model.ContextToolResult,
		ToolCallID:   "call-1",
		ToolName:     "exec_command",
		ContentBytes: model.Int64(bytes),
		Completeness: completeness,
		Measurement:  model.NewMeasurement(bytes/4, model.MeasurementEstimated),
	}
}

func sessionWithComponent(component model.ContextComponent) model.Session {
	return model.Session{Turns: []model.Turn{{
		ID:       "turn-1",
		Sequence: 1,
		ContextAttribution: model.ContextAttribution{
			Components: []model.ContextComponent{component},
		},
	}}}
}
