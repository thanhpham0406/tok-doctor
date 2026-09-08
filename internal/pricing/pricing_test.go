package pricing

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

func TestEmbeddedCatalogContainsRealProfiles(t *testing.T) {
	catalog, err := EmbeddedCatalog()
	if err != nil {
		t.Fatalf("EmbeddedCatalog: %v", err)
	}
	if len(catalog.Profiles) == 0 {
		t.Fatal("embedded catalog has no pricing profiles")
	}
}

func TestGPT55ResolvesFromEmbeddedCatalog(t *testing.T) {
	catalog, err := EmbeddedCatalog()
	if err != nil {
		t.Fatalf("EmbeddedCatalog: %v", err)
	}
	profile, ok := catalog.Resolve("gpt-5.5", nil)
	if !ok {
		t.Fatal("gpt-5.5 did not resolve")
	}
	if profile.Provider != "openai" || profile.SKU != "gpt-5.5" {
		t.Fatalf("profile = %s/%s, want openai/gpt-5.5", profile.Provider, profile.SKU)
	}
}

func TestEmbeddedCatalogExactAliasResolution(t *testing.T) {
	catalog, err := EmbeddedCatalog()
	if err != nil {
		t.Fatalf("EmbeddedCatalog: %v", err)
	}
	profile, ok := catalog.Resolve("GPT-5.5", nil)
	if !ok {
		t.Fatal("GPT-5.5 alias did not resolve")
	}
	if profile.SKU != "gpt-5.5" {
		t.Fatalf("sku = %s, want gpt-5.5", profile.SKU)
	}
}

func TestEmbeddedCatalogDoesNotGuessUnknownModels(t *testing.T) {
	catalog, err := EmbeddedCatalog()
	if err != nil {
		t.Fatalf("EmbeddedCatalog: %v", err)
	}
	for _, modelName := range []string{"gpt-5.5-preview-extra", "MiniMax-M3"} {
		if _, ok := catalog.Resolve(modelName, nil); ok {
			t.Fatalf("unexpected pricing match for %s", modelName)
		}
	}
}

func TestPublicCatalogMatchesEmbeddedFallback(t *testing.T) {
	publicFile, err := os.Open(filepath.Join("..", "..", "pricing", "catalog.json"))
	if err != nil {
		t.Fatalf("open public catalog: %v", err)
	}
	defer publicFile.Close()

	publicCatalog, err := DecodeCatalog(publicFile)
	if err != nil {
		t.Fatalf("decode public catalog: %v", err)
	}
	embedded, err := EmbeddedCatalog()
	if err != nil {
		t.Fatalf("EmbeddedCatalog: %v", err)
	}
	if !reflect.DeepEqual(publicCatalog, embedded) {
		t.Fatalf("embedded fallback differs from public pricing/catalog.json")
	}
}

func TestGPT55CostUsesEmbeddedCatalogRates(t *testing.T) {
	catalog, err := EmbeddedCatalog()
	if err != nil {
		t.Fatalf("EmbeddedCatalog: %v", err)
	}
	result := EstimateSession(sessionWithTurn("gpt-5.5", usageWithBillable(1_000_000, 250_000, nil, 500_000)), active(catalog))
	if result.Status != CostAvailable {
		t.Fatalf("status = %s: %s", result.Status, result.Unavailable)
	}
	if result.Turns[0].Pricing.Provider != "openai" || result.Turns[0].Pricing.SKU != "gpt-5.5" {
		t.Fatalf("pricing = %+v, want openai/gpt-5.5", result.Turns[0].Pricing)
	}
	if result.Cost.TotalMicros != 18_875_000 {
		t.Fatalf("total micros = %d, want 18875000", result.Cost.TotalMicros)
	}
}

func TestSessionSourceDoesNotOverrideCatalogProvider(t *testing.T) {
	catalog, err := EmbeddedCatalog()
	if err != nil {
		t.Fatalf("EmbeddedCatalog: %v", err)
	}
	session := sessionWithTurn("gpt-5.5", usageWithBillable(1_000_000, 0, nil, 0))
	session.Source = "claude"
	result := EstimateSession(session, active(catalog))
	if result.Status != CostAvailable {
		t.Fatalf("status = %s: %s", result.Status, result.Unavailable)
	}
	if result.Turns[0].Pricing.Provider != "openai" {
		t.Fatalf("provider = %s, want openai", result.Turns[0].Pricing.Provider)
	}
}

func TestEstimateSubtractsCachedFromFreshInput(t *testing.T) {
	result := EstimateSession(sessionWithTurn("gpt-test", usageWithBillable(1_000_000, 250_000, nil, 500_000)), testActiveCatalog())
	if result.Status != CostAvailable {
		t.Fatalf("status = %s: %s", result.Status, result.Unavailable)
	}
	if result.Usage.FreshInput != 750_000 {
		t.Fatalf("fresh input = %d, want 750000", result.Usage.FreshInput)
	}
}

func TestEstimatePricesCachedInputAndOutputSeparately(t *testing.T) {
	result := EstimateSession(sessionWithTurn("gpt-test", usageWithBillable(1_000_000, 250_000, nil, 500_000)), testActiveCatalog())
	if result.Cost.FreshInputMicros != 1_500_000 {
		t.Fatalf("fresh cost = %d, want 1500000", result.Cost.FreshInputMicros)
	}
	if result.Cost.CacheReadMicros != 125_000 {
		t.Fatalf("cache cost = %d, want 125000", result.Cost.CacheReadMicros)
	}
	if result.Cost.OutputMicros != 4_000_000 {
		t.Fatalf("output cost = %d, want 4000000", result.Cost.OutputMicros)
	}
	if result.Cost.TotalMicros != 5_625_000 {
		t.Fatalf("total cost = %d, want 5625000", result.Cost.TotalMicros)
	}
}

func TestEstimateDoesNotDoubleCountReasoning(t *testing.T) {
	reasoning := int64(300_000)
	usage := usageWithBillable(1_000_000, 0, nil, 500_000)
	usage.Reasoning = model.NewMeasurement(reasoning, model.MeasurementMeasured)
	result := EstimateSession(sessionWithTurn("gpt-test", usage), testActiveCatalog())
	if result.Cost.OutputMicros != 4_000_000 || result.Cost.TotalMicros != 6_000_000 {
		t.Fatalf("cost = %+v, want output-only reasoning accounting", result.Cost)
	}
}

func TestEstimateMissingIsNotExplicitZero(t *testing.T) {
	usage := model.MeasuredUsage(10, 0, 0, nil, 10)
	result := EstimateSession(sessionWithTurn("gpt-test", usage), testActiveCatalog())
	if result.Status != CostUnavailable {
		t.Fatalf("status = %s, want unavailable", result.Status)
	}
}

func TestEstimateUsesIntegerMoneyMath(t *testing.T) {
	effective := testTime(2026, 1, 1)
	catalog := Catalog{Version: "test", Profiles: []PricingProfile{{
		Provider: "OpenAI", SKU: "tiny", Model: "tiny", Currency: "USD", EffectiveFrom: &effective,
		Rates: Rates{InputMicrosPerMillion: 333_333, CachedInputMicrosPerMillion: 111_111, OutputMicrosPerMillion: 777_777},
	}}}
	result := EstimateSession(sessionWithTurn("tiny", usageWithBillable(3, 1, nil, 2)), active(catalog))
	if result.Cost.TotalMicros != 1 {
		t.Fatalf("total micros = %d, want deterministic integer truncation to 1", result.Cost.TotalMicros)
	}
}

func TestUnknownPricingProfileIsUnavailableNotZero(t *testing.T) {
	result := EstimateSession(sessionWithTurn("missing", usageWithBillable(1000, 0, nil, 1000)), testActiveCatalog())
	if result.Status != CostUnavailable {
		t.Fatalf("status = %s, want unavailable", result.Status)
	}
	if result.Cost != nil || result.Unavailable == "" {
		t.Fatalf("result = %+v, want unavailable explanation not zero cost", result)
	}
}

func TestMixedModelTurnsResolveDifferentProfilesAndAggregate(t *testing.T) {
	session := model.Session{
		ID: "sess",
		Turns: []model.Turn{
			{ID: "t1", Sequence: 1, Model: "gpt-test", Usage: usageWithBillable(1_000_000, 0, nil, 0)},
			{ID: "t2", Sequence: 2, Model: "other-test", Usage: usageWithBillable(1_000_000, 0, int64Ptr(0), 0)},
		},
	}
	result := EstimateSession(session, testActiveCatalog())
	if result.Status != CostAvailable {
		t.Fatalf("status = %s: %s", result.Status, result.Unavailable)
	}
	if result.Cost.TotalMicros != 5_000_000 {
		t.Fatalf("total = %d, want 5000000", result.Cost.TotalMicros)
	}
	if result.Turns[0].Pricing.Provider != "OpenAI" || result.Turns[1].Pricing.Provider != "MiniMax" {
		t.Fatalf("providers = %s/%s, want OpenAI/MiniMax", result.Turns[0].Pricing.Provider, result.Turns[1].Pricing.Provider)
	}
}

func TestCacheReadWriteDistinctionIsPreserved(t *testing.T) {
	write := int64(200_000)
	result := EstimateSession(sessionWithTurn("other-test", usageWithBillable(1_000_000, 300_000, &write, 0)), testActiveCatalog())
	if result.Usage.FreshInput != 500_000 || result.Usage.CacheRead != 300_000 || result.Usage.CacheWrite != 200_000 {
		t.Fatalf("usage = %+v, want read/write/fresh distinction", result.Usage)
	}
	if result.Cost.CacheReadMicros != 300_000 || result.Cost.CacheWriteMicros != 600_000 {
		t.Fatalf("cost = %+v, want separate read/write rates", result.Cost)
	}
}

func TestClaudeMiniMaxIsNotAnthropicBySourceName(t *testing.T) {
	session := sessionWithTurn("MiniMax-M3", usageWithBillable(1_000_000, 0, nil, 0))
	session.Source = "claude"
	effective := testTime(2026, 1, 1)
	result := EstimateSession(session, active(Catalog{Version: "test", Profiles: []PricingProfile{
		{Provider: "Anthropic", SKU: "claude-sonnet", Model: "claude-sonnet", Currency: "USD", EffectiveFrom: &effective, Rates: Rates{InputMicrosPerMillion: 9_000_000}},
		{Provider: "MiniMax", SKU: "MiniMax-M3", Model: "MiniMax-M3", Currency: "USD", EffectiveFrom: &effective, Rates: Rates{InputMicrosPerMillion: 1_000_000}},
	}}))
	if result.Status != CostAvailable {
		t.Fatalf("status = %s: %s", result.Status, result.Unavailable)
	}
	if result.Turns[0].Pricing.Provider != "MiniMax" {
		t.Fatalf("provider = %s, want MiniMax", result.Turns[0].Pricing.Provider)
	}
}

func TestExactAliasResolution(t *testing.T) {
	profile, ok := testCatalog().Resolve("gpt-test-latest", nil)
	if !ok {
		t.Fatal("alias did not resolve")
	}
	if profile.SKU != "gpt-test" {
		t.Fatalf("sku = %s, want gpt-test", profile.SKU)
	}
}

func TestUnknownModelIsNotGuessed(t *testing.T) {
	if _, ok := testCatalog().Resolve("gpt-test-preview-extra", nil); ok {
		t.Fatal("unexpected broad alias match")
	}
}

func TestHistoricalProfileResolutionUsesTurnTime(t *testing.T) {
	oldDate := testTime(2026, 1, 1)
	newDate := testTime(2026, 6, 1)
	turnDate := testTime(2026, 3, 1)
	catalog := Catalog{Version: "test", Profiles: []PricingProfile{
		{Provider: "OpenAI", SKU: "hist", Model: "hist", Currency: "USD", EffectiveFrom: &oldDate, Rates: Rates{InputMicrosPerMillion: 1_000_000}},
		{Provider: "OpenAI", SKU: "hist", Model: "hist", Currency: "USD", EffectiveFrom: &newDate, Rates: Rates{InputMicrosPerMillion: 9_000_000}},
	}}
	session := sessionWithTurn("hist", usageWithBillable(1_000_000, 0, nil, 0))
	session.Turns[0].Timestamp = &turnDate
	result := EstimateSession(session, active(catalog))
	if result.Cost.TotalMicros != 1_000_000 {
		t.Fatalf("total = %d, want old price", result.Cost.TotalMicros)
	}
}

func TestCatalogValidationAllowsHistoricalProfilesForSameSKU(t *testing.T) {
	oldDate := testTime(2026, 1, 1)
	newDate := testTime(2026, 6, 1)
	catalog := Catalog{Version: "test", Profiles: []PricingProfile{
		{Provider: "OpenAI", SKU: "hist", Model: "hist", Currency: "USD", EffectiveFrom: &oldDate, Rates: Rates{InputMicrosPerMillion: 1_000_000}},
		{Provider: "OpenAI", SKU: "hist", Model: "hist", Currency: "USD", EffectiveFrom: &newDate, Rates: Rates{InputMicrosPerMillion: 9_000_000}},
	}}
	if err := catalog.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestMixedProviderSessionWorks(t *testing.T) {
	session := model.Session{ID: "sess", Turns: []model.Turn{
		{Sequence: 1, Model: "gpt-test", Usage: usageWithBillable(1_000_000, 0, nil, 0)},
		{Sequence: 2, Model: "other-test", Usage: usageWithBillable(1_000_000, 0, int64Ptr(0), 0)},
	}}
	result := EstimateSession(session, testActiveCatalog())
	if result.Status != CostAvailable {
		t.Fatalf("result = %+v", result)
	}
	if result.Turns[0].Pricing.Provider == result.Turns[1].Pricing.Provider {
		t.Fatalf("providers = %s/%s, want mixed", result.Turns[0].Pricing.Provider, result.Turns[1].Pricing.Provider)
	}
}

func TestRepeatedMissingProfilesAreDeduplicated(t *testing.T) {
	session := model.Session{ID: "sess", Turns: []model.Turn{
		{Sequence: 1, Model: "missing", Usage: usageWithBillable(1, 0, nil, 0)},
		{Sequence: 2, Model: "missing", Usage: usageWithBillable(1, 0, nil, 0)},
	}}
	result := EstimateSession(session, testActiveCatalog())
	if len(result.Missing) != 1 || result.Missing[0].Count != 2 {
		t.Fatalf("missing = %+v, want one grouped missing profile", result.Missing)
	}
}

func TestProviderAggregation(t *testing.T) {
	sessions := []model.Session{
		{ID: "s1", Source: "codex", Turns: []model.Turn{{Sequence: 1, Model: "gpt-test", Usage: usageWithBillable(1_000_000, 0, nil, 0)}}},
		{ID: "s2", Source: "claude", Turns: []model.Turn{{Sequence: 1, Model: "other-test", Usage: usageWithBillable(1_000_000, 0, nil, 0)}}},
	}
	result := EstimateCollection(sessions, testActiveCatalog(), "OpenAI")
	if len(result.Groups) != 1 || result.Groups[0].Provider != "OpenAI" || result.Groups[0].Sessions != 1 {
		t.Fatalf("groups = %+v, want one OpenAI group", result.Groups)
	}
}

func TestAllOverlapGuardOmitsGrandTotal(t *testing.T) {
	sessions := []model.Session{{ID: "s1", Source: "codex", Turns: []model.Turn{{Sequence: 1, Model: "gpt-test", Usage: usageWithBillable(1_000_000, 0, nil, 0)}}}}
	result := EstimateCollection(sessions, testActiveCatalog(), "")
	if result.Overlap == "" {
		t.Fatalf("overlap = empty, want possible overlap warning")
	}
}

func sessionWithTurn(modelName string, usage model.Usage) model.Session {
	return model.Session{ID: "sess", Model: modelName, Turns: []model.Turn{{ID: "t1", Sequence: 1, Model: modelName, Usage: usage}}}
}

func usageWithBillable(input, cacheRead int64, cacheWrite *int64, output int64) model.Usage {
	usage := model.MeasuredUsage(input, cacheRead, output, nil, input+output)
	if cacheWrite != nil {
		usage.Cached = model.NewMeasurement(cacheRead+*cacheWrite, model.MeasurementMeasured)
	}
	usage.Billable = model.NewBillableUsage(input, cacheRead, output, cacheWrite, model.MeasurementMeasured)
	return usage
}

func testCatalog() Catalog {
	effective := testTime(2026, 1, 1)
	return Catalog{Version: "test", Profiles: []PricingProfile{
		{Provider: "OpenAI", SKU: "gpt-test", Model: "gpt-test", Aliases: []string{"gpt-test-latest"}, Currency: "USD", EffectiveFrom: &effective, Rates: Rates{InputMicrosPerMillion: 2_000_000, CachedInputMicrosPerMillion: 500_000, OutputMicrosPerMillion: 8_000_000}},
		{Provider: "MiniMax", SKU: "other-test", Model: "other-test", Currency: "USD", EffectiveFrom: &effective, Rates: Rates{InputMicrosPerMillion: 3_000_000, CacheReadMicrosPerMillion: 1_000_000, CacheWriteMicrosPerMillion: 3_000_000, OutputMicrosPerMillion: 9_000_000}},
	}}
}

func testActiveCatalog() ActiveCatalog {
	return active(testCatalog())
}

func active(catalog Catalog) ActiveCatalog {
	return ActiveCatalog{Catalog: catalog, Source: SourceEmbedded, Meta: CatalogMeta{Version: catalog.Version}}
}

func testTime(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func int64Ptr(v int64) *int64 {
	return &v
}
