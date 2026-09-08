package pricing

import (
	"bytes"
	"strings"
	"testing"

	corepricing "github.com/thanhpham0406/tok-doctor/internal/pricing"
)

func TestRenderListShowsTieredProfiles(t *testing.T) {
	active := corepricing.ActiveCatalog{Source: corepricing.SourceEmbedded, Catalog: corepricing.Catalog{
		Version: "test",
		Profiles: []corepricing.PricingProfile{{
			Provider: "openai",
			SKU:      "tiered",
			Model:    "tiered",
			Currency: "USD",
			Rates:    corepricing.Rates{},
			Tiers: []corepricing.PricingTier{
				{Name: "base", UpToInputTokens: int64Ptr(272_000), Rates: corepricing.Rates{InputMicrosPerMillion: 1}},
				{Name: "long-context", Rates: corepricing.Rates{InputMicrosPerMillion: 2}},
			},
		}},
	}}
	var out bytes.Buffer
	if err := RenderList(&out, active); err != nil {
		t.Fatalf("RenderList: %v", err)
	}
	for _, want := range []string{"Tiers", "base, long-context"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output = %q, want %q", out.String(), want)
		}
	}
}

func TestRenderShowShowsTierThresholdsAndRates(t *testing.T) {
	profile := corepricing.PricingProfile{
		Provider: "openai",
		SKU:      "tiered",
		Model:    "tiered",
		Currency: "USD",
		Rates:    corepricing.Rates{},
		Tiers: []corepricing.PricingTier{
			{Name: "base", UpToInputTokens: int64Ptr(272_000), Rates: corepricing.Rates{InputMicrosPerMillion: 200_000}},
			{Name: "long-context", Rates: corepricing.Rates{InputMicrosPerMillion: 400_000}},
		},
	}
	var out bytes.Buffer
	if err := RenderShow(&out, corepricing.ActiveCatalog{Source: corepricing.SourceEmbedded, Catalog: corepricing.Catalog{Version: "test"}}, "tiered", profile, true); err != nil {
		t.Fatalf("RenderShow: %v", err)
	}
	for _, want := range []string{"base (input <= 272,000)", "long-context (above prior tier)", "Input          $0.20", "Input          $0.40"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output = %q, want %q", out.String(), want)
		}
	}
}

func int64Ptr(v int64) *int64 {
	return &v
}
