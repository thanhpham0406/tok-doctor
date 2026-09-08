package cost

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/thanhpham0406/tok-doctor/internal/pricing"
)

func TestRenderCostShowsConciseBreakdown(t *testing.T) {
	effective := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	result := pricing.CostResult{
		SessionID:  "sess-123456789",
		Model:      "gpt-test",
		Status:     pricing.CostAvailable,
		Provenance: pricing.CostEstimatedAPI,
		Currency:   "USD",
		Usage:      &pricing.BillableTokens{FreshInput: 750_000, CacheRead: 250_000, Output: 500_000},
		Cost:       &pricing.CostBreakdown{FreshInputMicros: 1_500_000, CacheReadMicros: 125_000, OutputMicros: 4_000_000, TotalMicros: 5_625_000},
		Pricing:    &pricing.ProfileRef{Provider: "OpenAI", Model: "gpt-test", Currency: "USD", EffectiveFrom: &effective},
	}
	cacheHit := pricing.BasisPoints(2500)
	result.CacheHitPct = &cacheHit

	var out bytes.Buffer
	if err := Render(&out, result); err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, want := range []string{"API-equivalent cost", "750,000", "$5.62", "Cache hit", "25.00%", "estimated_api"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output = %q, want %q", out.String(), want)
		}
	}
}

func TestRenderJSONKeepsNumbersNumeric(t *testing.T) {
	result := pricing.CostResult{
		SessionID:  "sess",
		Status:     pricing.CostAvailable,
		Provenance: pricing.CostEstimatedAPI,
		Usage:      &pricing.BillableTokens{FreshInput: 1},
		Cost:       &pricing.CostBreakdown{TotalMicros: 2},
	}
	var out bytes.Buffer
	if err := RenderJSON(&out, result); err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	if strings.Contains(out.String(), `"total_micros": "`) {
		t.Fatalf("json cost should stay numeric: %s", out.String())
	}
	var decoded pricing.CostResult
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Cost.TotalMicros != 2 {
		t.Fatalf("total = %d, want 2", decoded.Cost.TotalMicros)
	}
}
