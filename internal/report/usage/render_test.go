package usage

import (
	"bytes"
	"math"
	"strings"
	"testing"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

func TestFormatTokenCount(t *testing.T) {
	tests := map[int64]string{
		0:             "0",
		12:            "12",
		1234:          "1,234",
		1234567:       "1,234,567",
		-1234:         "-1,234",
		math.MinInt64: "-9,223,372,036,854,775,808",
	}

	for input, want := range tests {
		if got := FormatTokenCount(input); got != want {
			t.Fatalf("FormatTokenCount(%d) = %q, want %q", input, got, want)
		}
	}
}

func TestRenderUsesThousandsSeparators(t *testing.T) {
	var out bytes.Buffer
	result := Single("codex", model.Usage{
		Input:      1200,
		Cached:     300,
		Output:     450,
		Reasoning:  0,
		Total:      1950,
		Confidence: model.ConfidenceMeasured,
	})

	if err := Render(&out, result); err != nil {
		t.Fatalf("Render: %v", err)
	}

	got := out.String()
	for _, want := range []string{"1,200", "1,950"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want %q", got, want)
		}
	}
}

func TestRenderAllNoData(t *testing.T) {
	var out bytes.Buffer
	result := model.UsageResult{Sources: []model.UsageEntry{
		{Source: "codex"},
		{Source: "claude"},
	}}

	if err := Render(&out, result); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "codex: no usage data available") ||
		!strings.Contains(got, "claude: no usage data available") {
		t.Fatalf("output = %q, want no-data entries", got)
	}
}
