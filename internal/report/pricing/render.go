package pricing

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/thanhpham0406/tok-doctor/internal/model"
	"github.com/thanhpham0406/tok-doctor/internal/pricing"
	reportcost "github.com/thanhpham0406/tok-doctor/internal/report/cost"
	reportusage "github.com/thanhpham0406/tok-doctor/internal/report/usage"
)

func RenderStatus(w io.Writer, status pricing.Status) error {
	fmt.Fprintln(w, "Pricing catalog")
	fmt.Fprintln(w, strings.Repeat("─", 60))
	fmt.Fprintf(w, "%-12s %s\n", "Source", status.Source)
	fmt.Fprintf(w, "%-12s %s\n", "Version", orDash(status.Version))
	if status.GeneratedAt != nil {
		fmt.Fprintf(w, "%-12s %s\n", "Generated", status.GeneratedAt.Format("2006-01-02 15:04"))
	}
	if status.FetchedAt != nil {
		fmt.Fprintf(w, "%-12s %s\n", "Fetched", status.FetchedAt.Format("2006-01-02 15:04"))
	}
	fmt.Fprintf(w, "%-12s %d\n", "Profiles", status.Profiles)
	fmt.Fprintf(w, "%-12s %s\n", "Remote", status.RemoteURL)
	return nil
}

func RenderUpdate(w io.Writer, result pricing.UpdateResult, updateErr error) error {
	if updateErr != nil {
		fmt.Fprintln(w, "Update failed")
		if result.UsingCache {
			fmt.Fprintf(w, "Using cached catalog %s\n", result.Version)
		}
		return nil
	}
	if result.Updated {
		fmt.Fprintln(w, "Pricing catalog updated")
	} else {
		fmt.Fprintln(w, "Pricing catalog unchanged")
	}
	fmt.Fprintf(w, "%-10s %s\n", "Version", result.Version)
	fmt.Fprintf(w, "%-10s %d\n", "Profiles", result.Profiles)
	fmt.Fprintf(w, "%-10s %s\n", "Source", result.Source)
	return nil
}

func RenderList(w io.Writer, active pricing.ActiveCatalog) error {
	fmt.Fprintln(w, "Pricing profiles")
	fmt.Fprintln(w, strings.Repeat("─", 80))
	if len(active.Catalog.Profiles) == 0 {
		fmt.Fprintln(w, "No pricing profiles available")
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Provider\tSKU\tModel\tAliases\tCurrency\tTiers")
	for _, profile := range active.Catalog.Profiles {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			profile.Provider,
			profile.SKU,
			orDash(profile.Model),
			orDash(strings.Join(profile.Aliases, ", ")),
			profile.Currency,
			tierSummary(profile.Tiers),
		)
	}
	return tw.Flush()
}

func RenderShow(w io.Writer, active pricing.ActiveCatalog, modelName string, profile pricing.PricingProfile, ok bool) error {
	fmt.Fprintf(w, "Pricing %s\n", modelName)
	fmt.Fprintln(w, strings.Repeat("─", 60))
	if !ok {
		fmt.Fprintln(w, "Pricing profile unavailable")
		return nil
	}
	fmt.Fprintf(w, "%-14s %s\n", "Provider", profile.Provider)
	fmt.Fprintf(w, "%-14s %s\n", "SKU", profile.SKU)
	fmt.Fprintf(w, "%-14s %s\n", "Currency", profile.Currency)
	if profile.EffectiveFrom != nil {
		fmt.Fprintf(w, "%-14s %s\n", "Effective", profile.EffectiveFrom.Format("2006-01-02"))
	} else {
		fmt.Fprintf(w, "%-14s %s\n", "Effective", "-")
	}
	source := profile.CatalogSource
	if source == "" {
		source = active.Source
	}
	catalogLabel := string(source)
	if source == active.Source && active.Catalog.Version != "" {
		catalogLabel += "/" + active.Catalog.Version
	}
	fmt.Fprintf(w, "%-14s %s\n", "Catalog", catalogLabel)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Rates per 1M tokens")
	fmt.Fprintln(w, strings.Repeat("─", 60))
	if len(profile.Tiers) == 0 {
		renderRates(w, profile.Rates)
		return nil
	}
	for _, tier := range profile.Tiers {
		fmt.Fprintf(w, "%s (%s)\n", tier.Name, inputRange(tier.UpToInputTokens))
		renderRates(w, tier.Rates)
	}
	return nil
}

func RenderMissing(w io.Writer, missing []pricing.MissingProfile) error {
	fmt.Fprintln(w, "Missing pricing")
	fmt.Fprintln(w, strings.Repeat("─", 80))
	if len(missing) == 0 {
		fmt.Fprintln(w, "No unresolved pricing profiles found")
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Provider\tModel\tSessions\tTurns")
	for _, miss := range missing {
		fmt.Fprintf(tw, "%s\t%s\t%d\t%d\n", orDash(miss.Provider), miss.Model, miss.Sessions, miss.Turns)
	}
	return tw.Flush()
}

func RenderAdd(w io.Writer, profile pricing.PricingProfile) error {
	fmt.Fprintln(w, "Pricing override saved")
	fmt.Fprintf(w, "%-10s %s\n", "Provider", profile.Provider)
	fmt.Fprintf(w, "%-10s %s\n", "SKU", profile.SKU)
	fmt.Fprintf(w, "%-10s %s\n", "Currency", profile.Currency)
	return nil
}

func renderRate(w io.Writer, name string, micros int64) {
	if micros == 0 {
		return
	}
	fmt.Fprintf(w, "%-14s %s\n", name, reportcost.FormatMicros(micros))
}

func renderRates(w io.Writer, rates pricing.Rates) {
	renderRate(w, "Input", rates.InputMicrosPerMillion)
	renderRate(w, "Cached input", rates.CachedInputMicrosPerMillion)
	renderRate(w, "Cache read", rates.CacheReadMicrosPerMillion)
	renderRate(w, "Cache write", rates.CacheWriteMicrosPerMillion)
	renderRate(w, "Output", rates.OutputMicrosPerMillion)
}

func tierSummary(tiers []pricing.PricingTier) string {
	if len(tiers) == 0 {
		return "-"
	}
	names := make([]string, 0, len(tiers))
	for _, tier := range tiers {
		names = append(names, tier.Name)
	}
	return strings.Join(names, ", ")
}

func inputRange(limit *int64) string {
	if limit == nil {
		return "above prior tier"
	}
	return fmt.Sprintf("input <= %s", formatInt(*limit))
}

func formatInt(n int64) string {
	return reportusage.FormatTokenCount(model.NewMeasurement(n, model.MeasurementDerived))
}

func orDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
