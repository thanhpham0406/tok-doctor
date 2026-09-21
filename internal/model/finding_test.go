package model

import (
	"encoding/json"
	"testing"
)

func TestFindingEvidenceOutputBytesDistinguishesMissingFromZero(t *testing.T) {
	tests := []struct {
		name        string
		outputBytes *int64
		wantJSON    string
	}{
		{name: "missing", outputBytes: nil, wantJSON: ""},
		{name: "observed zero", outputBytes: Int64(0), wantJSON: "0"},
		{name: "positive", outputBytes: Int64(4096), wantJSON: "4096"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := json.Marshal(FindingEvidence{TurnSequence: 1, OutputBytes: tt.outputBytes})
			if err != nil {
				t.Fatalf("marshal evidence: %v", err)
			}

			var decoded map[string]json.RawMessage
			if err := json.Unmarshal(raw, &decoded); err != nil {
				t.Fatalf("unmarshal evidence: %v", err)
			}
			value, ok := decoded["outputBytes"]
			if tt.outputBytes == nil {
				if ok {
					t.Fatalf("outputBytes = %s, want the field absent", value)
				}
				return
			}
			if !ok {
				t.Fatalf("encoded = %s, want an outputBytes field", raw)
			}
			if string(value) != tt.wantJSON {
				t.Fatalf("outputBytes = %s, want %s", value, tt.wantJSON)
			}
		})
	}
}
