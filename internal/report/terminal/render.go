package terminal

import (
	"fmt"
	"io"

	"github.com/thanhpham0406/tok-doctor/internal/analyze"
)

func Render(w io.Writer, result analyze.Result) error {
	if _, err := fmt.Fprintln(w, "TokDoctor"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Source: %s\n", result.Source); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Session: %s\n", result.Session.ID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Status: %s\n", result.Summary.Status); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%s\n", result.Summary.Message); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Findings: %d\n", len(result.Findings)); err != nil {
		return err
	}
	for _, finding := range result.Findings {
		if _, err := fmt.Fprintf(w, "\n[%s] %s %s\n", finding.Severity, finding.RuleID, finding.Title); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "%s\n", finding.Description); err != nil {
			return err
		}
		for _, evidence := range finding.Evidence {
			if _, err := fmt.Fprintf(w, "Turn: %d  Tool: %s  Output: %s", evidence.TurnSequence, displayTool(evidence.ToolName), formatBytes(evidence.OutputBytes)); err != nil {
				return err
			}
			if evidence.EstimatedTokens.Available() {
				if _, err := fmt.Fprintf(w, "  Estimated tokens: %d", evidence.EstimatedTokens.ValueOrZero()); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "Recommendation: %s\n", finding.Recommendation); err != nil {
			return err
		}
	}

	return nil
}

func displayTool(name string) string {
	if name == "" {
		return "unknown"
	}
	return name
}

func formatBytes(value int64) string {
	if value < 1024 {
		return fmt.Sprintf("%d B", value)
	}
	if value < 1024*1024 {
		return fmt.Sprintf("%.1f KiB", float64(value)/1024)
	}
	return fmt.Sprintf("%.1f MiB", float64(value)/(1024*1024))
}
