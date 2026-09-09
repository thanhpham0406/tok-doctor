package gateway

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

func RenderRequests(w io.Writer, summaries []RequestSummary) error {
	if len(summaries) == 0 {
		_, err := fmt.Fprintln(w, "No captured gateway requests for this profile.")
		return err
	}
	header := fmt.Sprintf("%-4s  %-8s  %-15s  %12s  %10s  %s", "#", "Time", "Model", "Context", "Payload", "Exchange")
	if _, err := fmt.Fprintln(w, header); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, strings.Repeat("-", len(header))); err != nil {
		return err
	}
	for i, s := range summaries {
		short := truncateString(s.ExchangeID, 12)
		model := truncateString(s.Model, 15)
		context := formatMeasurement(s.Attributed.Value)
		payload := humaniseBytes(s.PayloadBytes)
		t := formatTime(s.StartedAt)
		if _, err := fmt.Fprintf(w, "%-4d  %-8s  %-15s  %12s  %10s  %s\n",
			i+1, t, model, context, payload, short,
		); err != nil {
			return err
		}
	}
	return nil
}

func RenderInspect(w io.Writer, s RequestSummary) error {
	short := truncateString(s.ExchangeID, 10)
	if _, err := fmt.Fprintf(w, "Gateway request %s\n\n", short); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Profile    %s\n", s.Profile); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Model      %s\n", s.Model); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Endpoint   %s\n", s.Endpoint); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Payload    %s\n", humaniseBytes(s.PayloadBytes)); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if err := renderCategoryTable(w, s.Categories); err != nil {
		return err
	}
	if err := renderAttributedTotal(w, s.Attributed.Value); err != nil {
		return err
	}
	if s.Provider != nil {
		if err := renderProviderUsage(w, s.Provider); err != nil {
			return err
		}
		if err := renderAttribution(w, s.Attributed.Value, s.Provider); err != nil {
			return err
		}
	}
	if len(s.Files) > 0 {
		if err := renderFileTable(w, s.Files); err != nil {
			return err
		}
	}
	return nil
}

func RenderChains(w io.Writer, chains []ChainSummary) error {
	if len(chains) == 0 {
		_, err := fmt.Fprintln(w, "No correlated gateway chains for this profile.")
		return err
	}
	header := fmt.Sprintf("%-18s  %-8s  %5s  %15s  %12s  %10s  %s", "Chain", "Start", "Calls", "Total Context", "Peak Context", "Coverage", "Model")
	if _, err := fmt.Fprintln(w, header); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, strings.Repeat("-", len(header))); err != nil {
		return err
	}
	for _, chain := range chains {
		if _, err := fmt.Fprintf(w, "%-18s  %-8s  %5d  %15s  %12s  %10s  %s\n",
			chain.ID,
			formatTime(chain.StartedAt),
			chain.ModelCalls,
			formatMeasurement(chain.TotalContextSent),
			formatMeasurement(chain.PeakContext),
			formatChainListCoverage(chain),
			truncateString(chain.Model, 32),
		); err != nil {
			return err
		}
	}
	return nil
}

func formatChainListCoverage(chain ChainSummary) string {
	if chain.AttributionCoverage == nil || chain.AttributionCoverage.Percent == nil {
		return "—"
	}
	return formatCoverage(chain.AttributionCoverage, chain.TotalContextSent.Kind)
}

func RenderChain(w io.Writer, chain ChainSummary) error {
	if _, err := fmt.Fprintf(w, "Chain %s\n\n", chain.ID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Profile        %s\n", chain.Profile); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Protocol       %s\n", chain.Protocol); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Model          %s\n", chain.Model); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Started        %s\n", formatTime(chain.StartedAt)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Model calls    %d\n\n", chain.ModelCalls); err != nil {
		return err
	}
	header := fmt.Sprintf("%-3s  %-8s  %12s  %11s  %10s  %12s  %s", "#", "Time", "Context", "Δ Context", "Tools", "Tool results", "Exchange")
	if _, err := fmt.Fprintln(w, header); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, strings.Repeat("-", len(header))); err != nil {
		return err
	}
	for _, call := range chain.Calls {
		if _, err := fmt.Fprintf(w, "%-3d  %-8s  %12s  %11s  %10s  %12s  %s\n",
			call.Index,
			formatTime(call.StartedAt),
			formatMeasurement(call.Context),
			formatDelta(call.ContextDelta),
			formatMeasurement(call.Categories.ToolDefinition),
			formatMeasurement(call.Categories.ToolResult),
			truncateString(call.ExchangeID, 16),
		); err != nil {
			return err
		}
	}
	if hasAnyCallProviderUsage(chain.Calls) {
		if err := renderChainProviderUsageTable(w, chain.Calls); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "Summary"); err != nil {
		return err
	}
	rows := []struct {
		label string
		value string
	}{
		{"Total context sent", formatMeasurement(chain.TotalContextSent)},
		{"Initial context", formatMeasurement(chain.InitialContext)},
		{"Peak context", formatMeasurement(chain.PeakContext)},
		{"Final context", formatMeasurement(chain.FinalContext)},
		{"Context growth", formatDelta(chain.ContextGrowth)},
		{"Largest growth", formatLargestGrowth(chain.LargestGrowth, chain.LargestGrowthCall)},
		{"Tool-result growth", formatDelta(chain.ToolResultsGrowth)},
		{"History growth", formatDelta(chain.HistoryGrowth)},
		{"Tools growth", formatDelta(chain.ToolsGrowth)},
	}
	for _, row := range rows {
		if _, err := fmt.Fprintf(w, "%-22s%12s\n", row.label, row.value); err != nil {
			return err
		}
	}
	if hasChainProviderAggregate(chain) {
		if err := renderChainProviderSummary(w, chain); err != nil {
			return err
		}
	}
	return nil
}

func hasAnyCallProviderUsage(calls []ChainCallSummary) bool {
	for _, c := range calls {
		if c.ProviderUsage != nil {
			return true
		}
	}
	return false
}

func hasChainProviderAggregate(chain ChainSummary) bool {
	return chain.ProviderInput != nil || chain.ProviderOutput != nil || chain.ProviderCachedInput != nil || chain.AttributionCoverage != nil
}

func renderChainProviderUsageTable(w io.Writer, calls []ChainCallSummary) error {
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "Provider usage"); err != nil {
		return err
	}
	header := fmt.Sprintf("%-3s  %10s  %10s  %10s  %10s  %10s", "#", "Input", "Cached", "Fresh", "Output", "Coverage")
	if _, err := fmt.Fprintln(w, header); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, strings.Repeat("-", len(header))); err != nil {
		return err
	}
	for _, call := range calls {
		if call.ProviderUsage == nil {
			if _, err := fmt.Fprintf(w, "%-3d  %10s  %10s  %10s  %10s  %10s\n",
				call.Index, "—", "—", "—", "—", "—"); err != nil {
				return err
			}
			continue
		}
		coverageStr := "—"
		if call.AttributionCoverage != nil {
			coverageStr = formatCoverage(call.AttributionCoverage, call.Context.Kind)
		}
		if _, err := fmt.Fprintf(w, "%-3d  %10s  %10s  %10s  %10s  %10s\n",
			call.Index,
			providerInt64OrDash(call.ProviderUsage.Input),
			providerInt64OrDash(call.ProviderUsage.CachedInput),
			providerInt64OrDash(call.ProviderUsage.FreshInput),
			providerInt64OrDash(call.ProviderUsage.Output),
			coverageStr,
		); err != nil {
			return err
		}
	}
	return nil
}

func providerInt64OrDash(p *int64) string {
	if p == nil {
		return "—"
	}
	return formatInt(*p)
}

func renderChainProviderSummary(w io.Writer, chain ChainSummary) error {
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	rows := []struct {
		label string
		value string
	}{
		{"Provider input", formatOptionalMeasurement(chain.ProviderInput)},
		{"Cached input", formatOptionalMeasurement(chain.ProviderCachedInput)},
		{"Fresh input", formatOptionalMeasurement(chain.ProviderFreshInput)},
		{"Output", formatOptionalMeasurement(chain.ProviderOutput)},
	}
	for _, row := range rows {
		if _, err := fmt.Fprintf(w, "%-22s%12s\n", row.label, row.value); err != nil {
			return err
		}
	}
	if chain.UsageObservedCalls != nil && chain.UsageTotalCalls != nil {
		label := fmt.Sprintf("Usage observed calls  %d/%d", *chain.UsageObservedCalls, *chain.UsageTotalCalls)
		if _, err := fmt.Fprintln(w, label); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if chain.AttributionGap != nil {
		if _, err := fmt.Fprintf(w, "Attributed context  %s\n", formatMeasurement(chain.TotalContextSent)); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "Attribution gap      %s\n", formatDelta(*chain.AttributionGap)); err != nil {
			return err
		}
	}
	if chain.AttributionCoverage != nil {
		coverageStr := formatCoverage(chain.AttributionCoverage, chain.TotalContextSent.Kind)
		if _, err := fmt.Fprintf(w, "Coverage             %s\n", coverageStr); err != nil {
			return err
		}
	}
	return nil
}

func formatOptionalMeasurement(m *model.Measurement) string {
	if m == nil {
		return "—"
	}
	return formatMeasurement(*m)
}

func renderCategoryTable(w io.Writer, c CategoryBreakdown) error {
	if _, err := fmt.Fprintln(w, "Context breakdown"); err != nil {
		return err
	}
	rows := []struct {
		label string
		value model.Measurement
	}{
		{"Instructions", c.Instructions},
		{"History", c.History},
		{"User prompt", c.UserPrompt},
		{"Tool results", c.ToolResult},
		{"Tools", c.ToolDefinition},
		{"Files", c.File},
		{"Other", c.Other},
	}
	for _, r := range rows {
		if _, err := fmt.Fprintf(w, "%-15s%12s   %s\n", r.label, formatMeasurement(r.value), measurementKindLabel(r.value)); err != nil {
			return err
		}
	}
	return nil
}

func renderAttributedTotal(w io.Writer, total model.Measurement) error {
	if _, err := fmt.Fprintln(w, strings.Repeat("-", 43)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%-15s%12s   %s\n", "Attributed", formatMeasurement(total), measurementKindLabel(total)); err != nil {
		return err
	}
	return nil
}

func renderFileTable(w io.Writer, files []FileAttribution) error {
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "Files"); err != nil {
		return err
	}
	for _, f := range files {
		path := truncateString(f.Path, 40)
		if _, err := fmt.Fprintf(w, "%-40s  %s\n", path, formatInt(f.Measurement.ValueOrZero())); err != nil {
			return err
		}
	}
	return nil
}

func renderProviderUsage(w io.Writer, p *RequestProviderSummary) error {
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "Provider usage"); err != nil {
		return err
	}
	rows := []struct {
		label string
		value string
	}{
		{"Input", providerTokenLabel(p.Input)},
		{"Cached", providerTokenLabel(p.CachedInput)},
		{"Fresh", providerTokenLabel(p.FreshInput)},
		{"Output", providerTokenLabel(p.Output)},
		{"Reasoning", providerTokenLabel(p.ReasoningOutput)},
	}
	for _, row := range rows {
		if _, err := fmt.Fprintf(w, "%-15s%12s\n", row.label, row.value); err != nil {
			return err
		}
	}
	return nil
}

func renderAttribution(w io.Writer, attributed model.Measurement, p *RequestProviderSummary) error {
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "Attribution"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%-15s%12s\n", "Attributed", formatMeasurement(attributed)); err != nil {
		return err
	}
	gapLabel := "Gap"
	var gapMeasurement model.Measurement
	if p.Gap != nil {
		gapMeasurement = *p.Gap
	} else {
		gapMeasurement = model.Measurement{Kind: model.MeasurementUnknown}
	}
	if _, err := fmt.Fprintf(w, "%-15s%12s\n", gapLabel, formatDelta(gapMeasurement)); err != nil {
		return err
	}
	coverageLabel := "Coverage"
	if _, err := fmt.Fprintf(w, "%-15s%12s\n", coverageLabel, formatCoverage(p.Coverage, attributed.Kind)); err != nil {
		return err
	}
	return nil
}

func providerTokenLabel(p *int64) string {
	if p == nil {
		return "          —"
	}
	return fmt.Sprintf("%10s", formatInt(*p))
}

func formatCoverage(c *AttributionCoverageValue, attributionKind model.MeasurementKind) string {
	if c == nil || c.Percent == nil {
		return "          —"
	}
	suffix := ""
	if c.Kind == "estimated" || attributionKind == model.MeasurementEstimated {
		suffix = "*"
	}
	return fmt.Sprintf("%9s%%", formatFloat(*c.Percent)) + suffix
}

func formatFloat(f float64) string {
	if f == 0 {
		return "0"
	}
	return fmt.Sprintf("%.1f", f)
}

func formatMeasurement(m model.Measurement) string {
	if !m.Available() {
		return "          —"
	}
	suffix := ""
	if m.Kind == model.MeasurementEstimated {
		suffix = "*"
	}
	return fmt.Sprintf("%10s", formatInt(m.ValueOrZero())+suffix)
}

func formatDelta(m model.Measurement) string {
	if !m.Available() {
		return "          —"
	}
	value := m.ValueOrZero()
	prefix := ""
	if value > 0 {
		prefix = "+"
	}
	suffix := ""
	if m.Kind == model.MeasurementEstimated {
		suffix = "*"
	}
	return fmt.Sprintf("%10s", prefix+formatInt(value)+suffix)
}

func formatLargestGrowth(m model.Measurement, call int) string {
	formatted := formatDelta(m)
	if !m.Available() || call == 0 {
		return formatted
	}
	return strings.TrimSpace(formatted) + " at call #" + fmt.Sprintf("%d", call)
}

func formatInt(v int64) string {
	if v < 0 {
		return "-" + formatInt(-v)
	}
	s := fmt.Sprintf("%d", v)
	if len(s) <= 3 {
		return s
	}
	out := make([]byte, 0, len(s)+len(s)/3)
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, byte(c))
	}
	return string(out)
}

func measurementKindLabel(m model.Measurement) string {
	if !m.Available() {
		return "unknown"
	}
	switch m.Kind {
	case model.MeasurementEstimated:
		return "estimated"
	case model.MeasurementCounted:
		return "counted"
	case model.MeasurementDerived:
		return "derived"
	case model.MeasurementMeasured:
		return "measured"
	}
	return "unknown"
}

func formatTime(rfc3339 string) string {
	if rfc3339 == "" {
		return "—"
	}
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		return "—"
	}
	return t.Format("15:04:05")
}

func humaniseBytes(v int64) string {
	if v < 0 {
		v = 0
	}
	switch {
	case v < 1024:
		return fmt.Sprintf("%d B", v)
	case v < 1024*1024:
		return fmt.Sprintf("%d KB", v/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(v)/(1024*1024))
	}
}

func truncateString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}
