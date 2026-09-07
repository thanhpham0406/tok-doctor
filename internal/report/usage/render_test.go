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
		1:             "1",
		12:            "12",
		123:           "123",
		999:           "999",
		1000:          "1,000",
		1234:          "1,234",
		12345:         "12,345",
		123456:        "123,456",
		1234567:       "1,234,567",
		111249946:     "111,249,946",
		1000000000:    "1,000,000,000",
		-1234:         "-1,234",
		math.MinInt64: "-9,223,372,036,854,775,808",
	}

	for input, want := range tests {
		if got := FormatTokenCount(input); got != want {
			t.Fatalf("FormatTokenCount(%d) = %q, want %q", input, got, want)
		}
	}
}

func TestFormatReasoning(t *testing.T) {
	zero := int64(0)
	many := int64(1234)
	if got := FormatReasoning(nil); got != "-" {
		t.Fatalf("FormatReasoning(nil) = %q, want %q", got, "-")
	}
	if got := FormatReasoning(&zero); got != "0" {
		t.Fatalf("FormatReasoning(&0) = %q, want %q", got, "0")
	}
	if got := FormatReasoning(&many); got != "1,234" {
		t.Fatalf("FormatReasoning(&1234) = %q, want %q", got, "1,234")
	}
}

func TestRenderUsesThousandsSeparators(t *testing.T) {
	var out bytes.Buffer
	reasoning := int64(0)
	result := Single("codex", model.Usage{
		Input:      1200,
		Cached:     300,
		Output:     450,
		Reasoning:  &reasoning,
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

func TestRenderRendersReasoningStates(t *testing.T) {
	cases := []struct {
		name string
		r    *int64
		want string
	}{
		{"nil", nil, "reasoning:  -"},
		{"zero", ptr(int64(0)), "reasoning:  0"},
		{"nonzero", ptr(int64(1234)), "reasoning:  1,234"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var out bytes.Buffer
			result := Single("codex", model.Usage{
				Input:      100,
				Output:     50,
				Reasoning:  c.r,
				Total:      150,
				Confidence: model.ConfidenceMeasured,
			})
			if err := Render(&out, result); err != nil {
				t.Fatalf("Render: %v", err)
			}
			if got := out.String(); !strings.Contains(got, c.want) {
				t.Fatalf("output = %q, want %q", got, c.want)
			}
		})
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

func ptr(v int64) *int64 { return &v }
