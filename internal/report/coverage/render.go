package coverage

import (
	"encoding/json"
	"fmt"
	"io"

	corecoverage "github.com/thanhpham0406/tok-doctor/internal/coverage"
	"github.com/thanhpham0406/tok-doctor/internal/model"
	reportusage "github.com/thanhpham0406/tok-doctor/internal/report/usage"
)

const labelWidth = 24

func Render(w io.Writer, result corecoverage.Result) error {
	if _, err := fmt.Fprintln(w, title(result)); err != nil {
		return err
	}
	switch result.Scope {
	case corecoverage.ScopeAll:
		for _, source := range result.Sources {
			if _, err := fmt.Fprintf(w, "\n%s\n", source.Source); err != nil {
				return err
			}
			if err := renderSummary(w, "  ", source.Summary); err != nil {
				return err
			}
		}
		if len(result.Sources) > 1 {
			if _, err := fmt.Fprintln(w, "\nOverall"); err != nil {
				return err
			}
			if err := renderSummary(w, "  ", result.Overall); err != nil {
				return err
			}
		}
	default:
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		if err := renderSummary(w, "", result.Overall); err != nil {
			return err
		}
	}
	if err := renderUnsupportedSources(w, result.UnsupportedSources); err != nil {
		return err
	}
	return renderGateway(w, result.Gateway)
}

func RenderJSON(w io.Writer, result corecoverage.Result) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

func title(result corecoverage.Result) string {
	switch result.Scope {
	case corecoverage.ScopeSession:
		return fmt.Sprintf("Tool Output Coverage — %s (session %s)", result.Source, result.SessionID)
	case corecoverage.ScopeSource:
		return fmt.Sprintf("Tool Output Coverage — %s", result.Source)
	default:
		return "Tool Output Coverage — all sources"
	}
}

func renderSummary(w io.Writer, indent string, summary corecoverage.Summary) error {
	rows := []struct {
		label string
		value string
	}{
		{"Sessions scanned", fmt.Sprintf("%d", summary.Sessions)},
		{"Turns scanned", fmt.Sprintf("%d", summary.Turns)},
		{"Tool outputs", fmt.Sprintf("%d", summary.ToolOutputs)},
		{"With byte size", formatRatio(summary.WithContentBytes)},
		{"With estimated tokens", formatRatio(summary.WithEstimatedTokens)},
		{"With call ID", formatRatio(summary.WithToolCallID)},
		{"With tool name", formatRatio(summary.WithToolName)},
		{"In trailing context", fmt.Sprintf("%d", summary.TrailingContext)},
		{"Linked to fresh input", formatRatio(summary.WithFreshInput)},
	}
	for _, row := range rows {
		if _, err := fmt.Fprintf(w, "%s%-*s%s\n", indent, labelWidth, row.label+":", row.value); err != nil {
			return err
		}
	}
	if err := renderCompleteness(w, indent, summary); err != nil {
		return err
	}
	if err := renderUnreadableRecords(w, indent, summary); err != nil {
		return err
	}
	if err := renderBytes(w, indent, summary.ContentBytes); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "\n%sOutput tokens (estimated): %s\n", indent, formatEstimatedTokens(summary.EstimatedTokens))
	return err
}

// renderCompleteness prints the completeness counts of recognized tool outputs.
func renderCompleteness(w io.Writer, indent string, summary corecoverage.Summary) error {
	if _, err := fmt.Fprintf(w, "\n%sRecognized tool outputs:\n", indent); err != nil {
		return err
	}
	completeness := summary.RecognizedToolOutputCompleteness
	rows := []struct {
		label string
		value int
	}{
		{"Complete", completeness.Complete},
		{"Truncated", completeness.Truncated},
		{"Unavailable", completeness.Unavailable},
		{"Unknown", completeness.Unknown},
	}
	for _, row := range rows {
		if _, err := fmt.Fprintf(w, "%s  %-*s%d\n", indent, labelWidth-2, row.label+":", row.value); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w, "%s  counted over %s only\n", indent, plural(summary.ToolOutputs, "recognized tool output"))
	return err
}

// renderUnreadableRecords prints the records TokDoctor could not read in full,
// apart from the recognized tool outputs.
func renderUnreadableRecords(w io.Writer, indent string, summary corecoverage.Summary) error {
	if _, err := fmt.Fprintf(w, "\n%s%-*s%d (%s)\n", indent, labelWidth, "Unreadable records:",
		summary.UnreadableRecords, plural(summary.UnreadableSessions, "session")); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "%s  records read partially or not at all; a record whose kind stays unknown\n", indent)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s  is not counted as tool output and stays outside the counts above\n", indent)
	return err
}

func renderBytes(w io.Writer, indent string, bytes corecoverage.Bytes) error {
	if _, err := fmt.Fprintf(w, "\n%sOutput bytes:\n", indent); err != nil {
		return err
	}
	if bytes.Samples == 0 {
		_, err := fmt.Fprintf(w, "%s  no byte size reported\n", indent)
		return err
	}
	rows := []struct {
		label string
		value string
	}{
		{"total", formatBytes(bytes.Total)},
		{"min", formatOptionalBytes(bytes.Min)},
		{"p50", formatOptionalBytes(bytes.P50)},
		{"p90", formatOptionalBytes(bytes.P90)},
		{"p95", formatOptionalBytes(bytes.P95)},
		{"max", formatOptionalBytes(bytes.Max)},
	}
	for _, row := range rows {
		if _, err := fmt.Fprintf(w, "%s  %-6s%s\n", indent, row.label, row.value); err != nil {
			return err
		}
	}
	return nil
}

func renderUnsupportedSources(w io.Writer, unsupported []corecoverage.Unsupported) error {
	if len(unsupported) == 0 {
		return nil
	}
	if _, err := fmt.Fprint(w, "\nUnsupported sources:\n"); err != nil {
		return err
	}
	for _, entry := range unsupported {
		if _, err := fmt.Fprintf(w, "  %s: %s\n", entry.Source, entry.Reason); err != nil {
			return err
		}
	}
	return nil
}

func renderGateway(w io.Writer, gateway corecoverage.GatewayCoverage) error {
	if gateway.Supported {
		return nil
	}
	_, err := fmt.Fprintf(w, "\nGateway coverage: not supported in this iteration (%s)\n", gateway.Reason)
	return err
}

func formatRatio(ratio corecoverage.Ratio) string {
	if ratio.Value == nil {
		return fmt.Sprintf("%d (n/a)", ratio.Count)
	}
	return fmt.Sprintf("%d (%.1f%%)", ratio.Count, *ratio.Value*100)
}

func formatOptionalBytes(value *int64) string {
	if value == nil {
		return "n/a"
	}
	return formatBytes(*value)
}

func formatBytes(bytes int64) string {
	switch {
	case bytes < 1024:
		return fmt.Sprintf("%d B", bytes)
	case bytes < 1024*1024:
		return fmt.Sprintf("%.1f KiB", float64(bytes)/1024)
	case bytes < 1024*1024*1024:
		return fmt.Sprintf("%.1f MiB", float64(bytes)/(1024*1024))
	default:
		return fmt.Sprintf("%.1f GiB", float64(bytes)/(1024*1024*1024))
	}
}

func formatEstimatedTokens(measurement model.Measurement) string {
	if !measurement.Available() {
		return "n/a"
	}
	return reportusage.FormatTokenCount(measurement)
}

func plural(count int, noun string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, noun)
	}
	return fmt.Sprintf("%d %ss", count, noun)
}
