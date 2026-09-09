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
	header := fmt.Sprintf("%-18s  %-8s  %5s  %15s  %12s  %s", "Chain", "Start", "Calls", "Total Context", "Peak Context", "Model")
	if _, err := fmt.Fprintln(w, header); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, strings.Repeat("-", len(header))); err != nil {
		return err
	}
	for _, chain := range chains {
		if _, err := fmt.Fprintf(w, "%-18s  %-8s  %5d  %15s  %12s  %s\n",
			chain.ID,
			formatTime(chain.StartedAt),
			chain.ModelCalls,
			formatMeasurement(chain.TotalContextSent),
			formatMeasurement(chain.PeakContext),
			truncateString(chain.Model, 32),
		); err != nil {
			return err
		}
	}
	return nil
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
	return nil
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
