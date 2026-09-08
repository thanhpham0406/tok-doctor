package cost

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/thanhpham0406/tok-doctor/internal/model"
	"github.com/thanhpham0406/tok-doctor/internal/pricing"
	reportusage "github.com/thanhpham0406/tok-doctor/internal/report/usage"
)

func Render(w io.Writer, result pricing.CostResult) error {
	fmt.Fprintf(w, "Session %s\n", truncate(result.SessionID))
	if result.Model != "" {
		fmt.Fprintf(w, "Model %s\n", result.Model)
	}
	fmt.Fprintln(w)
	if result.Status != pricing.CostAvailable {
		fmt.Fprintln(w, "API-equivalent cost unavailable")
		fmt.Fprintln(w, strings.Repeat("─", 60))
		fmt.Fprintf(w, "Reason        %s\n", result.Unavailable)
		if len(result.Missing) > 0 {
			fmt.Fprintln(w, "Hint          Run `tok pricing update` or `tok pricing add <model>`")
		}
		return nil
	}

	fmt.Fprintln(w, "API-equivalent cost")
	fmt.Fprintln(w, strings.Repeat("─", 60))
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Fresh input\t%s\t%s\n", formatTokens(result.Usage.FreshInput), FormatMicros(result.Cost.FreshInputMicros))
	fmt.Fprintf(tw, "Cached input\t%s\t%s\n", formatTokens(result.Usage.CacheRead+result.Usage.CacheWrite), FormatMicros(result.Cost.CacheReadMicros+result.Cost.CacheWriteMicros))
	fmt.Fprintf(tw, "Output\t%s\t%s\n", formatTokens(result.Usage.Output), FormatMicros(result.Cost.OutputMicros))
	tw.Flush()
	fmt.Fprintln(w, strings.Repeat("─", 60))
	fmt.Fprintf(w, "%-27s %s\n", "Estimated total", FormatMicros(result.Cost.TotalMicros))
	fmt.Fprintln(w)
	if result.CacheHitPct != nil {
		fmt.Fprintf(w, "%-27s %s\n", "Cache hit", formatBasisPoints(*result.CacheHitPct))
	}
	if result.Pricing != nil {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Pricing")
		fmt.Fprintln(w, strings.Repeat("─", 60))
		fmt.Fprintf(w, "%-14s %s\n", "Provider", result.Pricing.Provider)
		fmt.Fprintf(w, "%-14s %s\n", "Model", result.Pricing.Model)
		if result.Pricing.Tier != "" {
			fmt.Fprintf(w, "%-14s %s\n", "Tier", result.Pricing.Tier)
		}
		effective := "-"
		if result.Pricing.EffectiveFrom != nil {
			effective = result.Pricing.EffectiveFrom.Format("2006-01-02")
		}
		fmt.Fprintf(w, "%-14s %s\n", "Effective", effective)
		fmt.Fprintf(w, "%-14s %s\n", "Estimate", result.Provenance)
	}
	return nil
}

func RenderJSON(w io.Writer, result pricing.CostResult) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

func RenderCollection(w io.Writer, result pricing.CostCollection) error {
	if result.Provider != "" {
		fmt.Fprintf(w, "Provider %s\n", result.Provider)
	} else {
		fmt.Fprintln(w, "API-equivalent cost by provider/source")
	}
	fmt.Fprintln(w, strings.Repeat("─", 80))
	if result.Overlap != "" {
		fmt.Fprintf(w, "Overlap       %s\n\n", result.Overlap)
	}
	if len(result.Groups) == 0 {
		fmt.Fprintln(w, result.Unavailable)
		renderMissing(w, result.Missing)
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Provider\tSource\tSessions\tModel calls\tFresh input\tCached input\tOutput\tEstimated cost")
	for _, group := range result.Groups {
		fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%s\t%s\t%s\t%s\n",
			group.Provider,
			orDash(group.Source),
			group.Sessions,
			group.ModelCalls,
			formatTokens(group.Usage.FreshInput),
			formatTokens(group.Usage.CacheRead+group.Usage.CacheWrite),
			formatTokens(group.Usage.Output),
			FormatMicros(group.Cost.TotalMicros),
		)
	}
	tw.Flush()
	renderMissing(w, result.Missing)
	return nil
}

func RenderCollectionJSON(w io.Writer, result pricing.CostCollection) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

func FormatMicros(micros int64) string {
	negative := micros < 0
	if negative {
		micros = -micros
	}
	dollars := micros / 1_000_000
	cents := (micros % 1_000_000) / 10_000
	out := "$" + formatInt(dollars) + "." + twoDigits(cents)
	if negative {
		return "-" + out
	}
	return out
}

func truncate(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}

func formatTokens(n int64) string {
	return reportusage.FormatTokenCount(model.NewMeasurement(n, model.MeasurementDerived))
}

func formatInt(n int64) string {
	return reportusage.FormatTokenCount(model.NewMeasurement(n, model.MeasurementDerived))
}

func twoDigits(n int64) string {
	s := strconv.FormatInt(n, 10)
	if len(s) == 1 {
		return "0" + s
	}
	return s
}

func formatBasisPoints(bp pricing.BasisPoints) string {
	whole := int64(bp) / 100
	frac := int64(bp) % 100
	return fmt.Sprintf("%d.%02d%%", whole, frac)
}

func renderMissing(w io.Writer, missing []pricing.MissingProfile) {
	if len(missing) == 0 {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Unresolved pricing")
	fmt.Fprintln(w, strings.Repeat("─", 80))
	for _, miss := range missing {
		label := miss.Model
		if miss.Provider != "" {
			label = miss.Provider + "/" + miss.Model
		}
		fmt.Fprintf(w, "pricing unavailable for %s (%d turns)\n", label, miss.Count)
	}
}

func orDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
