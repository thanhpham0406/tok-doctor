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
	if _, err := fmt.Fprintln(w, "Pricing catalog"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, strings.Repeat("─", 60)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%-12s %s\n", "Source", status.Source); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%-12s %s\n", "Version", orDash(status.Version)); err != nil {
		return err
	}
	if status.GeneratedAt != nil {
		if _, err := fmt.Fprintf(w, "%-12s %s\n", "Generated", status.GeneratedAt.Format("2006-01-02 15:04")); err != nil {
			return err
		}
	}
	if status.FetchedAt != nil {
		if _, err := fmt.Fprintf(w, "%-12s %s\n", "Fetched", status.FetchedAt.Format("2006-01-02 15:04")); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "%-12s %d\n", "Profiles", status.Profiles); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%-12s %s\n", "Remote", status.RemoteURL); err != nil {
		return err
	}
	return nil
}

func RenderUpdate(w io.Writer, result pricing.UpdateResult, updateErr error) error {
	if updateErr != nil {
		if _, err := fmt.Fprintln(w, "Update failed"); err != nil {
			return err
		}
		if result.UsingCache {
			if _, err := fmt.Fprintf(w, "Using cached catalog %s\n", result.Version); err != nil {
				return err
			}
		}
		return nil
	}
	if result.Updated {
		if _, err := fmt.Fprintln(w, "Pricing catalog updated"); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintln(w, "Pricing catalog unchanged"); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "%-10s %s\n", "Version", result.Version); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%-10s %d\n", "Profiles", result.Profiles); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%-10s %s\n", "Source", result.Source); err != nil {
		return err
	}
	return nil
}

func RenderList(w io.Writer, active pricing.ActiveCatalog) error {
	if _, err := fmt.Fprintln(w, "Pricing profiles"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, strings.Repeat("─", 80)); err != nil {
		return err
	}
	if len(active.Catalog.Profiles) == 0 {
		_, err := fmt.Fprintln(w, "No pricing profiles available")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "Provider\tSKU\tModel\tAliases\tCurrency\tTiers"); err != nil {
		return err
	}
	for _, profile := range active.Catalog.Profiles {
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			profile.Provider,
			profile.SKU,
			orDash(profile.Model),
			orDash(strings.Join(profile.Aliases, ", ")),
			profile.Currency,
			tierSummary(profile.Tiers),
		); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func RenderShow(w io.Writer, active pricing.ActiveCatalog, modelName string, profile pricing.PricingProfile, ok bool) error {
	if _, err := fmt.Fprintf(w, "Pricing %s\n", modelName); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, strings.Repeat("─", 60)); err != nil {
		return err
	}
	if !ok {
		_, err := fmt.Fprintln(w, "Pricing profile unavailable")
		return err
	}
	if _, err := fmt.Fprintf(w, "%-14s %s\n", "Provider", profile.Provider); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%-14s %s\n", "SKU", profile.SKU); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%-14s %s\n", "Currency", profile.Currency); err != nil {
		return err
	}
	if profile.EffectiveFrom != nil {
		if _, err := fmt.Fprintf(w, "%-14s %s\n", "Effective", profile.EffectiveFrom.Format("2006-01-02")); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintf(w, "%-14s %s\n", "Effective", "-"); err != nil {
			return err
		}
	}
	source := profile.CatalogSource
	if source == "" {
		source = active.Source
	}
	catalogLabel := string(source)
	if source == active.Source && active.Catalog.Version != "" {
		catalogLabel += "/" + active.Catalog.Version
	}
	if _, err := fmt.Fprintf(w, "%-14s %s\n", "Catalog", catalogLabel); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "Rates per 1M tokens"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, strings.Repeat("─", 60)); err != nil {
		return err
	}
	if len(profile.Tiers) == 0 {
		return renderRates(w, profile.Rates)
	}
	for _, tier := range profile.Tiers {
		if _, err := fmt.Fprintf(w, "%s (%s)\n", tier.Name, inputRange(tier.UpToInputTokens)); err != nil {
			return err
		}
		if err := renderRates(w, tier.Rates); err != nil {
			return err
		}
	}
	return nil
}

func RenderMissing(w io.Writer, missing []pricing.MissingProfile) error {
	if _, err := fmt.Fprintln(w, "Missing pricing"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, strings.Repeat("─", 80)); err != nil {
		return err
	}
	if len(missing) == 0 {
		_, err := fmt.Fprintln(w, "No unresolved pricing profiles found")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "Provider\tModel\tSessions\tTurns"); err != nil {
		return err
	}
	for _, miss := range missing {
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%d\t%d\n", orDash(miss.Provider), miss.Model, miss.Sessions, miss.Turns); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func RenderAdd(w io.Writer, profile pricing.PricingProfile) error {
	if _, err := fmt.Fprintln(w, "Pricing override saved"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%-10s %s\n", "Provider", profile.Provider); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%-10s %s\n", "SKU", profile.SKU); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%-10s %s\n", "Currency", profile.Currency); err != nil {
		return err
	}
	return nil
}

func renderRate(w io.Writer, name string, micros int64) error {
	if micros == 0 {
		return nil
	}
	_, err := fmt.Fprintf(w, "%-14s %s\n", name, reportcost.FormatMicros(micros))
	return err
}

func renderRates(w io.Writer, rates pricing.Rates) error {
	if err := renderRate(w, "Input", rates.InputMicrosPerMillion); err != nil {
		return err
	}
	if err := renderRate(w, "Cached input", rates.CachedInputMicrosPerMillion); err != nil {
		return err
	}
	if err := renderRate(w, "Cache read", rates.CacheReadMicrosPerMillion); err != nil {
		return err
	}
	if err := renderRate(w, "Cache write", rates.CacheWriteMicrosPerMillion); err != nil {
		return err
	}
	return renderRate(w, "Output", rates.OutputMicrosPerMillion)
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
