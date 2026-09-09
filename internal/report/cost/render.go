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
	if _, err := fmt.Fprintf(w, "Session %s\n", truncate(result.SessionID)); err != nil {
		return err
	}
	if result.Model != "" {
		if _, err := fmt.Fprintf(w, "Model %s\n", result.Model); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if result.Status != pricing.CostAvailable {
		if _, err := fmt.Fprintln(w, "API-equivalent cost unavailable"); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w, strings.Repeat("─", 60)); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "Reason        %s\n", result.Unavailable); err != nil {
			return err
		}
		if len(result.Missing) > 0 {
			if _, err := fmt.Fprintln(w, "Hint          Run `tok pricing update` or `tok pricing add <model>`"); err != nil {
				return err
			}
		}
		return nil
	}

	if _, err := fmt.Fprintln(w, "API-equivalent cost"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, strings.Repeat("─", 60)); err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintf(tw, "Fresh input\t%s\t%s\n", formatTokens(result.Usage.FreshInput), FormatMicros(result.Cost.FreshInputMicros)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(tw, "Cached input\t%s\t%s\n", formatTokens(result.Usage.CacheRead+result.Usage.CacheWrite), FormatMicros(result.Cost.CacheReadMicros+result.Cost.CacheWriteMicros)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(tw, "Output\t%s\t%s\n", formatTokens(result.Usage.Output), FormatMicros(result.Cost.OutputMicros)); err != nil {
		return err
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, strings.Repeat("─", 60)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%-27s %s\n", "Estimated total", FormatMicros(result.Cost.TotalMicros)); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if result.CacheHitPct != nil {
		if _, err := fmt.Fprintf(w, "%-27s %s\n", "Cache hit", formatBasisPoints(*result.CacheHitPct)); err != nil {
			return err
		}
	}
	if result.Pricing != nil {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w, "Pricing"); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w, strings.Repeat("─", 60)); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "%-14s %s\n", "Provider", result.Pricing.Provider); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "%-14s %s\n", "Model", result.Pricing.Model); err != nil {
			return err
		}
		if result.Pricing.Tier != "" {
			if _, err := fmt.Fprintf(w, "%-14s %s\n", "Tier", result.Pricing.Tier); err != nil {
				return err
			}
		}
		effective := "-"
		if result.Pricing.EffectiveFrom != nil {
			effective = result.Pricing.EffectiveFrom.Format("2006-01-02")
		}
		if _, err := fmt.Fprintf(w, "%-14s %s\n", "Effective", effective); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "%-14s %s\n", "Estimate", result.Provenance); err != nil {
			return err
		}
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
		if _, err := fmt.Fprintf(w, "Provider %s\n", result.Provider); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintln(w, "API-equivalent cost by provider/source"); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, strings.Repeat("─", 80)); err != nil {
		return err
	}
	if result.Overlap != "" {
		if _, err := fmt.Fprintf(w, "Overlap       %s\n\n", result.Overlap); err != nil {
			return err
		}
	}
	if len(result.Groups) == 0 {
		if _, err := fmt.Fprintln(w, result.Unavailable); err != nil {
			return err
		}
		return renderMissing(w, result.Missing)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "Provider\tSource\tSessions\tModel calls\tFresh input\tCached input\tOutput\tEstimated cost"); err != nil {
		return err
	}
	for _, group := range result.Groups {
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%s\t%s\t%s\t%s\n",
			group.Provider,
			orDash(group.Source),
			group.Sessions,
			group.ModelCalls,
			formatTokens(group.Usage.FreshInput),
			formatTokens(group.Usage.CacheRead+group.Usage.CacheWrite),
			formatTokens(group.Usage.Output),
			FormatMicros(group.Cost.TotalMicros),
		); err != nil {
			return err
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	return renderMissing(w, result.Missing)
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

func renderMissing(w io.Writer, missing []pricing.MissingProfile) error {
	if len(missing) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "Unresolved pricing"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, strings.Repeat("─", 80)); err != nil {
		return err
	}
	for _, miss := range missing {
		label := miss.Model
		if miss.Provider != "" {
			label = miss.Provider + "/" + miss.Model
		}
		if _, err := fmt.Fprintf(w, "pricing unavailable for %s (%d turns)\n", label, miss.Count); err != nil {
			return err
		}
	}
	return nil
}

func orDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
