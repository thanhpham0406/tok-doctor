package inspect

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/thanhpham0406/tok-doctor/internal/model"
	reportusage "github.com/thanhpham0406/tok-doctor/internal/report/usage"
)

const (
	defaultTopLimit    = 5
	defaultRecentLimit = 5
)

type Options struct {
	AllTurns bool
	Turn     int
}

func Render(w io.Writer, session model.Session, opts Options) error {
	renderSummary(w, session)
	renderReconciliationHint(w, session)

	if opts.Turn > 0 {
		renderSingleTurn(w, session, opts.Turn)
		return nil
	}
	if opts.AllTurns {
		renderAllTurns(w, session)
		return nil
	}
	renderTopAndRecent(w, session)
	return nil
}

func renderSummary(w io.Writer, session model.Session) {
	fmt.Fprintf(w, "Session %s\n", truncateSessionID(session.ID))
	fmt.Fprintln(w, strings.Repeat("─", 60))
	fmt.Fprintf(w, "Source          %s\n", orDash(session.Source))
	fmt.Fprintf(w, "Model           %s\n", orDash(session.Model))
	fmt.Fprintf(w, "Started         %s\n", orDash(formatTime(session.StartedAt)))
	fmt.Fprintf(w, "Updated         %s\n", orDash(formatTime(session.UpdatedAt)))
	fmt.Fprintf(w, "Model calls     %d\n\n", len(session.Turns))

	fmt.Fprintln(w, "Usage")
	fmt.Fprintln(w, strings.Repeat("─", 60))
	fmt.Fprintf(w, "Input           %s\n", formatUsageMetric(session.Usage.Input, session.Usage.Confidence))
	fmt.Fprintf(w, "Cached          %s\n", formatUsageMetric(session.Usage.Cached, session.Usage.Confidence))
	fmt.Fprintf(w, "Output          %s\n", formatUsageMetric(session.Usage.Output, session.Usage.Confidence))
	fmt.Fprintf(w, "Reasoning       %s\n", reportusage.FormatReasoning(session.Usage.Reasoning))
	fmt.Fprintf(w, "Total           %s\n", formatUsageMetric(session.Usage.Total, session.Usage.Confidence))
	if kind := dominantKind(session.Turns); kind != "" {
		fmt.Fprintf(w, "Turn measurement %s\n", kind)
	}
}

func renderReconciliationHint(w io.Writer, session model.Session) {
	if len(session.Turns) == 0 {
		return
	}
	comparison := model.ReconcileUsage(session)
	if comparison.Matches() {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Reconciliation  incomplete (turns do not fully account for authoritative session usage)")
}

func renderTopAndRecent(w io.Writer, session model.Session) {
	if len(session.Turns) == 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "No reliable Turn breakdown available for this session.")
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Top expensive turns")
	fmt.Fprintln(w, strings.Repeat("─", 96))
	renderTurnTable(w, topExpensive(session.Turns, defaultTopLimit), mixed(session.Turns))

	recent := recentTurns(session.Turns, defaultRecentLimit)
	if len(recent) > 0 && !sliceEqualsRecent(recent, topExpensive(session.Turns, defaultTopLimit)) {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Recent turns")
		fmt.Fprintln(w, strings.Repeat("─", 96))
		renderTurnTable(w, recent, mixed(session.Turns))
	}
}

func renderAllTurns(w io.Writer, session model.Session) {
	if len(session.Turns) == 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "No reliable Turn breakdown available for this session.")
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Turns")
	fmt.Fprintln(w, strings.Repeat("─", 96))
	renderTurnTable(w, session.Turns, mixed(session.Turns))
}

func renderSingleTurn(w io.Writer, session model.Session, sequence int) {
	if len(session.Turns) == 0 {
		fmt.Fprintf(w, "Turn %d not found in session %s (session has no reconstructable turns).\n", sequence, session.ID)
		return
	}
	var match *model.Turn
	for i := range session.Turns {
		if session.Turns[i].Sequence == sequence {
			match = &session.Turns[i]
			break
		}
	}
	if match == nil {
		fmt.Fprintf(w, "Turn %d not found in session %s\n", sequence, session.ID)
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Turn #%d\n", sequence)
	fmt.Fprintln(w, strings.Repeat("─", 60))
	fmt.Fprintf(w, "Session       %s\n", truncateSessionID(session.ID))
	fmt.Fprintf(w, "Source        %s\n", orDash(match.Model))
	fmt.Fprintf(w, "Model         %s\n", orDash(match.Model))
	fmt.Fprintf(w, "Time          %s\n", orDash(formatTurnTime(match.Timestamp)))
	fmt.Fprintf(w, "Measurement   %s\n", formatMeasurement(match.Measurement))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage")
	fmt.Fprintln(w, strings.Repeat("─", 60))
	fmt.Fprintf(w, "Input         %s\n", formatUsageMetric(match.Usage.Input, match.Confidence))
	fmt.Fprintf(w, "Cached        %s\n", formatUsageMetric(match.Usage.Cached, match.Confidence))
	fmt.Fprintf(w, "Output        %s\n", formatUsageMetric(match.Usage.Output, match.Confidence))
	fmt.Fprintf(w, "Reasoning     %s\n", reportusage.FormatReasoning(match.Usage.Reasoning))
	fmt.Fprintf(w, "Total         %s\n", formatUsageMetric(match.Usage.Total, match.Confidence))
}

func renderTurnTable(w io.Writer, turns []model.Turn, showKind bool) {
	if len(turns) == 0 {
		fmt.Fprintln(w, "(none)")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if showKind {
		fmt.Fprintln(tw, "#\tTime\tModel\tInput\tCached\tOutput\tReasoning\tTotal\tKind")
		for _, turn := range turns {
			fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				turn.Sequence,
				formatTurnTime(turn.Timestamp),
				orDash(turn.Model),
				formatUsageMetric(turn.Usage.Input, turn.Confidence),
				formatUsageMetric(turn.Usage.Cached, turn.Confidence),
				formatUsageMetric(turn.Usage.Output, turn.Confidence),
				reportusage.FormatReasoning(turn.Usage.Reasoning),
				formatUsageMetric(turn.Usage.Total, turn.Confidence),
				formatMeasurement(turn.Measurement),
			)
		}
	} else {
		fmt.Fprintln(tw, "#\tTime\tModel\tInput\tCached\tOutput\tReasoning\tTotal")
		for _, turn := range turns {
			fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				turn.Sequence,
				formatTurnTime(turn.Timestamp),
				orDash(turn.Model),
				formatUsageMetric(turn.Usage.Input, turn.Confidence),
				formatUsageMetric(turn.Usage.Cached, turn.Confidence),
				formatUsageMetric(turn.Usage.Output, turn.Confidence),
				reportusage.FormatReasoning(turn.Usage.Reasoning),
				formatUsageMetric(turn.Usage.Total, turn.Confidence),
			)
		}
	}
	tw.Flush()
}

func dominantKind(turns []model.Turn) string {
	if len(turns) == 0 {
		return ""
	}
	if mixed(turns) {
		return ""
	}
	return formatMeasurement(turns[0].Measurement)
}

func mixed(turns []model.Turn) bool {
	if len(turns) == 0 {
		return false
	}
	first := turns[0].Measurement
	for _, t := range turns[1:] {
		if t.Measurement != first {
			return true
		}
	}
	return false
}

func topExpensive(turns []model.Turn, limit int) []model.Turn {
	if limit <= 0 || len(turns) == 0 {
		return nil
	}
	sorted := make([]model.Turn, len(turns))
	copy(sorted, turns)
	slices.SortStableFunc(sorted, func(a, b model.Turn) int {
		if a.Usage.Total > b.Usage.Total {
			return -1
		}
		if a.Usage.Total < b.Usage.Total {
			return 1
		}
		return 0
	})
	if len(sorted) > limit {
		sorted = sorted[:limit]
	}
	slices.SortStableFunc(sorted, func(a, b model.Turn) int {
		return a.Sequence - b.Sequence
	})
	return sorted
}

func recentTurns(turns []model.Turn, limit int) []model.Turn {
	if limit <= 0 || len(turns) == 0 {
		return nil
	}
	ordered := make([]model.Turn, len(turns))
	copy(ordered, turns)
	slices.SortStableFunc(ordered, func(a, b model.Turn) int {
		return a.Sequence - b.Sequence
	})
	if len(ordered) > limit {
		ordered = ordered[len(ordered)-limit:]
	}
	return ordered
}

func sliceEqualsRecent(a, b []model.Turn) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Sequence != b[i].Sequence {
			return false
		}
	}
	return true
}

func truncateSessionID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}

func formatMeasurement(kind model.MeasurementKind) string {
	if kind == "" {
		return "-"
	}
	return string(kind)
}

func formatUsageMetric(value int64, confidence model.Confidence) string {
	if confidence == "" {
		return "-"
	}
	return reportusage.FormatTokenCount(value)
}

func formatTurnTime(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.Format("15:04:05")
}

func formatTime(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.Format("2006-01-02 15:04")
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

type InspectResult struct {
	Session model.Session `json:"session"`
}

func RenderJSON(w io.Writer, session model.Session) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(InspectResult{Session: session})
}
