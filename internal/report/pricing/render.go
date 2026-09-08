package pricing

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/thanhpham0406/tok-doctor/internal/pricing"
	reportcost "github.com/thanhpham0406/tok-doctor/internal/report/cost"
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
	fmt.Fprintln(tw, "Provider\tSKU\tModel\tAliases\tCurrency")
	for _, profile := range active.Catalog.Profiles {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			profile.Provider,
			profile.SKU,
			orDash(profile.Model),
			orDash(strings.Join(profile.Aliases, ", ")),
			profile.Currency,
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
	fmt.Fprintf(w, "%-14s %s/%s\n", "Catalog", active.Source, active.Catalog.Version)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Rates per 1M tokens")
	fmt.Fprintln(w, strings.Repeat("─", 60))
	renderRate(w, "Input", profile.Rates.InputMicrosPerMillion)
	renderRate(w, "Cached input", profile.Rates.CachedInputMicrosPerMillion)
	renderRate(w, "Cache read", profile.Rates.CacheReadMicrosPerMillion)
	renderRate(w, "Cache write", profile.Rates.CacheWriteMicrosPerMillion)
	renderRate(w, "Output", profile.Rates.OutputMicrosPerMillion)
	return nil
}

func renderRate(w io.Writer, name string, micros int64) {
	if micros == 0 {
		return
	}
	fmt.Fprintf(w, "%-14s %s\n", name, reportcost.FormatMicros(micros))
}

func orDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
