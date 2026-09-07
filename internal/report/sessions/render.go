package sessions

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/thanhpham0406/tok-doctor/internal/model"
	reportusage "github.com/thanhpham0406/tok-doctor/internal/report/usage"
)

func Render(w io.Writer, result model.SessionsResult) error {
	if len(result.Sessions) == 0 {
		_, err := fmt.Fprintln(w, "No sessions available")
		return err
	}
	if _, err := fmt.Fprintln(w, "Sessions"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "Source\tSession\tUpdated\tModel\tInput\tCached\tOutput\tReasoning\tTotal"); err != nil {
		return err
	}
	for _, session := range result.Sessions {
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			session.Source,
			shortID(session.ID),
			formatUpdated(session.UpdatedAt),
			formatModel(session.Model),
			formatUsageMetric(session.Usage.Input, session.Usage.Confidence),
			formatUsageMetric(session.Usage.Cached, session.Usage.Confidence),
			formatUsageMetric(session.Usage.Output, session.Usage.Confidence),
			reportusage.FormatReasoning(session.Usage.Reasoning),
			formatUsageMetric(session.Usage.Total, session.Usage.Confidence),
		); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func shortID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}

func formatUpdated(t *time.Time) string {
	if t == nil {
		return "-"
	}
	now := time.Now()
	if t.After(now) {
		return t.Format("2006-01-02")
	}
	d := now.Sub(*t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return t.Format("2006-01-02")
	}
}

func formatModel(modelName string) string {
	if strings.TrimSpace(modelName) == "" {
		return "-"
	}
	return modelName
}

func formatUsageMetric(value int64, confidence model.Confidence) string {
	if confidence == "" {
		return "-"
	}
	return reportusage.FormatTokenCount(value)
}
