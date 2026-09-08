package inspect

import (
	"bytes"
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
	AllTurns     bool
	Turn         int
	ShowEvidence bool
}

func Render(w io.Writer, session model.Session, opts Options) error {
	renderSummary(w, session, opts.ShowEvidence)
	renderReconciliationHint(w, session)

	if opts.Turn > 0 {
		renderSingleTurn(w, session, opts.Turn, opts.ShowEvidence)
		return nil
	}
	if opts.AllTurns {
		renderAllTurns(w, session)
		return nil
	}
	renderTopAndRecent(w, session)
	return nil
}

func renderSummary(w io.Writer, session model.Session, showEvidence bool) {
	fmt.Fprintf(w, "Session %s\n", truncateSessionID(session.ID))
	fmt.Fprintln(w, strings.Repeat("─", 60))
	fmt.Fprintf(w, "Source          %s\n", orDash(session.Source))
	fmt.Fprintf(w, "Model           %s\n", orDash(session.Model))
	fmt.Fprintf(w, "Started         %s\n", orDash(formatTime(session.StartedAt)))
	fmt.Fprintf(w, "Updated         %s\n", orDash(formatTime(session.UpdatedAt)))
	fmt.Fprintf(w, "Model calls     %d\n\n", len(session.Turns))

	fmt.Fprintln(w, "Usage")
	fmt.Fprintln(w, strings.Repeat("─", 60))
	renderUsageMetric(w, "Input", session.Usage.Input)
	renderUsageMetric(w, "Cached", session.Usage.Cached)
	renderUsageMetric(w, "Output", session.Usage.Output)
	renderUsageMetric(w, "Reasoning", session.Usage.Reasoning)
	renderUsageMetric(w, "Total", session.Usage.Total)

	if showEvidence {
		if ev := renderSessionEvidence(session); ev != "" {
			fmt.Fprintln(w)
			fmt.Fprintln(w, "Evidence")
			fmt.Fprintln(w, strings.Repeat("─", 60))
			fmt.Fprint(w, ev)
		}
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

func renderSingleTurn(w io.Writer, session model.Session, sequence int, showEvidence bool) {
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
	fmt.Fprintf(w, "Source        %s\n", orDash(session.Source))
	fmt.Fprintf(w, "Model         %s\n", orDash(match.Model))
	fmt.Fprintf(w, "Time          %s\n", orDash(formatTurnTime(match.Timestamp)))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage")
	fmt.Fprintln(w, strings.Repeat("─", 60))
	renderUsageMetric(w, "Input", match.Usage.Input)
	renderUsageMetric(w, "Cached", match.Usage.Cached)
	renderUsageMetric(w, "Output", match.Usage.Output)
	renderUsageMetric(w, "Reasoning", match.Usage.Reasoning)
	renderUsageMetric(w, "Total", match.Usage.Total)

	if showEvidence {
		if ev := renderTurnEvidence(*match); ev != "" {
			fmt.Fprintln(w)
			fmt.Fprintln(w, "Evidence")
			fmt.Fprintln(w, strings.Repeat("─", 60))
			fmt.Fprint(w, ev)
		}
	}
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
				formatUsageMetric(turn.Usage.Input),
				formatUsageMetric(turn.Usage.Cached),
				formatUsageMetric(turn.Usage.Output),
				reportusage.FormatReasoning(turn.Usage.Reasoning),
				formatUsageMetric(turn.Usage.Total),
				formatMeasurement(turnKind(turn)),
			)
		}
	} else {
		fmt.Fprintln(tw, "#\tTime\tModel\tInput\tCached\tOutput\tReasoning\tTotal")
		for _, turn := range turns {
			fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				turn.Sequence,
				formatTurnTime(turn.Timestamp),
				orDash(turn.Model),
				formatUsageMetric(turn.Usage.Input),
				formatUsageMetric(turn.Usage.Cached),
				formatUsageMetric(turn.Usage.Output),
				reportusage.FormatReasoning(turn.Usage.Reasoning),
				formatUsageMetric(turn.Usage.Total),
			)
		}
	}
	tw.Flush()
}

func mixed(turns []model.Turn) bool {
	if len(turns) == 0 {
		return false
	}
	first := turnKind(turns[0])
	for _, t := range turns[1:] {
		if turnKind(t) != first {
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
		if a.Usage.Total.ValueOrZero() > b.Usage.Total.ValueOrZero() {
			return -1
		}
		if a.Usage.Total.ValueOrZero() < b.Usage.Total.ValueOrZero() {
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

func formatUsageMetric(metric model.Measurement) string {
	return reportusage.FormatTokenCount(metric)
}

func renderUsageMetric(w io.Writer, name string, metric model.Measurement) {
	fmt.Fprintf(w, "%-15s %-12s %s\n", name, formatUsageMetric(metric), formatMeasurement(metric.DisplayKind()))
}

func turnKind(turn model.Turn) model.MeasurementKind {
	kinds := map[model.MeasurementKind]struct{}{}
	for _, metric := range []model.Measurement{
		turn.Usage.Input,
		turn.Usage.Cached,
		turn.Usage.Output,
		turn.Usage.Reasoning,
		turn.Usage.Total,
	} {
		if metric.Available() {
			kinds[metric.Kind] = struct{}{}
		}
	}
	if len(kinds) != 1 {
		return ""
	}
	for kind := range kinds {
		return kind
	}
	return ""
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

func renderSessionEvidence(session model.Session) string {
	usage := session.Usage
	if !usage.HasUsage() {
		return ""
	}
	var out bytes.Buffer
	seen := map[string]struct{}{}
	rendered := false
	for _, metric := range []struct {
		name  string
		value model.Measurement
	}{
		{"Input", usage.Input},
		{"Cached", usage.Cached},
		{"Output", usage.Output},
		{"Reasoning", usage.Reasoning},
		{"Total", usage.Total},
	} {
		for _, ev := range metric.value.Evidence {
			key := evidenceKey(ev)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			if rendered {
				fmt.Fprintln(&out)
			}
			fmt.Fprintf(&out, "%s\n", metric.name)
			writeEvidenceBlock(&out, ev)
			rendered = true
		}
	}
	if !rendered {
		return ""
	}
	return out.String()
}

func renderTurnEvidence(turn model.Turn) string {
	usage := turn.Usage
	if !usage.HasUsage() {
		return ""
	}
	perMetric := []struct {
		name  string
		value model.Measurement
	}{
		{"Input", usage.Input},
		{"Cached", usage.Cached},
		{"Output", usage.Output},
		{"Reasoning", usage.Reasoning},
		{"Total", usage.Total},
	}
	var out bytes.Buffer
	rendered := false
	for _, metric := range perMetric {
		if len(metric.value.Evidence) == 0 {
			continue
		}
		if rendered {
			fmt.Fprintln(&out)
		}
		fmt.Fprintf(&out, "%s\n", metric.name)
		writeEvidenceBlock(&out, metric.value.Evidence[0])
		rendered = true
	}
	if !rendered {
		return ""
	}
	return out.String()
}

func writeEvidenceBlock(out io.Writer, ev model.Evidence) {
	fmt.Fprintf(out, "  Method      %s\n", formatEvidenceMethod(ev))
	fmt.Fprintf(out, "  Source      %s\n", orDash(ev.Source))
	if ev.Field != "" {
		fmt.Fprintf(out, "  Field       %s\n", ev.Field)
	}
	if ev.Record != "" {
		fmt.Fprintf(out, "  Record      %s\n", ev.Record)
	}
	if ev.Previous != "" || ev.Current != "" {
		fmt.Fprintf(out, "  Previous    %s\n", orDash(ev.Previous))
		fmt.Fprintf(out, "  Current     %s\n", orDash(ev.Current))
	}
	if ev.Operation != "" {
		fmt.Fprintf(out, "  Operation   %s\n", ev.Operation)
	}
	if ev.Count > 0 {
		fmt.Fprintf(out, "  Count       %d\n", ev.Count)
	}
}

func formatEvidenceMethod(ev model.Evidence) string {
	switch ev.Kind {
	case model.EvidenceSourceValue:
		return "source value"
	case model.EvidenceCumulativeDelta:
		return "cumulative delta"
	case model.EvidenceAggregate:
		return "aggregate"
	default:
		return string(ev.Kind)
	}
}

func evidenceKey(ev model.Evidence) string {
	return string(ev.Kind) + "|" + ev.Source + "|" + ev.Record + "|" + ev.Field + "|" + ev.Previous + "|" + ev.Current + "|" + string(ev.Operation)
}
