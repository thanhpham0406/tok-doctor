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
	ShowContext  bool
	ContextAll   bool
}

func Render(w io.Writer, session model.Session, opts Options) error {
	renderSummary(w, session, opts.ShowEvidence)
	renderReconciliationHint(w, session)

	if opts.Turn > 0 {
		renderSingleTurn(w, session, opts.Turn, opts)
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

func renderSingleTurn(w io.Writer, session model.Session, sequence int, opts Options) {
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

	if opts.ShowContext {
		renderContext(w, *match, opts.ContextAll)
	}

	if opts.ShowEvidence {
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

func renderContext(w io.Writer, turn model.Turn, showAll bool) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Context")
	fmt.Fprintln(w, strings.Repeat("─", 60))
	summary := summarizeContext(turn.ContextAttribution.Components)
	if len(summary) == 0 {
		fmt.Fprintln(w, "No attributable context provenance available for this turn.")
	} else {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "Type\tTokens\tKind\tPayload")
		for _, row := range summary {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", contextKindLabel(row.Kind), formatUsageMetric(row.Measurement), formatMeasurement(row.Measurement.DisplayKind()), "unknown")
		}
		tw.Flush()
	}

	renderContextReconciliation(w, model.ReconcileContext(turn))
	renderTopFiles(w, turn.ContextAttribution.Components)
	if showAll {
		renderAllContextComponents(w, turn.ContextAttribution.Components)
	}
}

type contextSummaryRow struct {
	Kind        model.ContextComponentKind
	Measurement model.Measurement
}

func summarizeContext(components []model.ContextComponent) []contextSummaryRow {
	order := []model.ContextComponentKind{
		model.ContextInstructions,
		model.ContextUserPrompt,
		model.ContextHistory,
		model.ContextFile,
		model.ContextToolResult,
		model.ContextOther,
		model.ContextUnknown,
	}
	byKind := map[model.ContextComponentKind][]model.ContextComponent{}
	for _, component := range components {
		byKind[component.Kind] = append(byKind[component.Kind], component)
	}
	var rows []contextSummaryRow
	for _, kind := range order {
		group := byKind[kind]
		if len(group) == 0 {
			continue
		}
		measurement, ok := model.SumAttributionMeasurements(group)
		if !ok {
			measurement = model.Measurement{Kind: model.MeasurementUnknown}
		}
		rows = append(rows, contextSummaryRow{Kind: kind, Measurement: measurement})
	}
	return rows
}

func renderContextReconciliation(w io.Writer, rec model.ContextReconciliation) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Reconciliation")
	fmt.Fprintln(w, strings.Repeat("─", 60))
	fmt.Fprintf(w, "%-22s %-12s %s\n", "Fresh input", formatUsageMetric(rec.FreshInput), formatMeasurement(rec.FreshInput.DisplayKind()))
	fmt.Fprintf(w, "%-22s %-12s %s\n", "Fresh attributed", formatUsageMetric(rec.FreshAttributed), formatMeasurement(rec.FreshAttributed.DisplayKind()))
	fmt.Fprintf(w, "%-22s %-12s %s\n", "Fresh unknown", formatUsageMetric(rec.FreshUnknown), formatMeasurement(rec.FreshUnknown.DisplayKind()))
	fmt.Fprintf(w, "%-22s %s\n", "Fresh coverage", formatCoverage(rec.FreshCoverage))
	fmt.Fprintln(w)
	fmt.Fprintf(w, "%-22s %-12s %s\n", "Cached context", formatUsageMetric(rec.CachedContext), formatMeasurement(rec.CachedContext.DisplayKind()))
	fmt.Fprintf(w, "%-22s %-12s %s\n", "Cached attributed", formatUsageMetric(rec.CachedAttributed), formatMeasurement(rec.CachedAttributed.DisplayKind()))
	fmt.Fprintf(w, "%-22s %s\n", "Cached coverage", formatCoverage(rec.CachedCoverage))
	fmt.Fprintln(w)
	fmt.Fprintf(w, "%-22s %s\n", "Full payload coverage", formatCoverage(rec.FullPayloadCoverage))
	if rec.Conflict {
		fmt.Fprintln(w, "Conflict              fresh attributed context exceeds fresh input")
	}
}

func formatCoverage(value *float64) string {
	if value == nil {
		return "unavailable"
	}
	return fmt.Sprintf("%.1f%%", *value*100)
}

func renderTopFiles(w io.Writer, components []model.ContextComponent) {
	files := topFileComponents(components, defaultTopLimit)
	if len(files) == 0 {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Top files")
	fmt.Fprintln(w, strings.Repeat("─", 60))
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Path\tTokens\tKind\tPayload")
	for _, component := range files {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", component.Path, formatUsageMetric(component.Measurement), formatMeasurement(component.Measurement.DisplayKind()), "unknown")
	}
	tw.Flush()
}

func topFileComponents(components []model.ContextComponent, limit int) []model.ContextComponent {
	var files []model.ContextComponent
	for _, component := range components {
		if component.Kind == model.ContextFile && component.Path != "" {
			files = append(files, component)
		}
	}
	slices.SortStableFunc(files, func(a, b model.ContextComponent) int {
		if a.Measurement.ValueOrZero() > b.Measurement.ValueOrZero() {
			return -1
		}
		if a.Measurement.ValueOrZero() < b.Measurement.ValueOrZero() {
			return 1
		}
		return strings.Compare(a.Path, b.Path)
	})
	if len(files) > limit {
		files = files[:limit]
	}
	return files
}

func renderAllContextComponents(w io.Writer, components []model.ContextComponent) {
	if len(components) == 0 {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Context components")
	fmt.Fprintln(w, strings.Repeat("─", 96))
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Type\tSource\tRecord\tPath\tTokens\tKind\tObservation\tPayload")
	for _, component := range components {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			contextKindLabel(component.Kind),
			orDash(component.Source),
			orDash(component.Record),
			orDash(component.Path),
			formatUsageMetric(component.Measurement),
			formatMeasurement(component.Measurement.DisplayKind()),
			orDash(string(component.Observation)),
			"unknown",
		)
	}
	tw.Flush()
}

func contextKindLabel(kind model.ContextComponentKind) string {
	switch kind {
	case model.ContextInstructions:
		return "Instructions"
	case model.ContextUserPrompt:
		return "User prompt"
	case model.ContextHistory:
		return "History"
	case model.ContextFile:
		return "Files"
	case model.ContextToolResult:
		return "Tool results"
	case model.ContextOther:
		return "Other"
	case model.ContextUnknown:
		return "Unknown"
	default:
		return string(kind)
	}
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
	Turn    *model.Turn   `json:"turn,omitempty"`
}

func RenderJSON(w io.Writer, session model.Session) error {
	return RenderJSONWithOptions(w, session, Options{})
}

func RenderJSONWithOptions(w io.Writer, session model.Session, opts Options) error {
	result := InspectResult{Session: session}
	if opts.Turn > 0 && opts.ShowContext {
		result.Session.Turns = nil
		for i := range session.Turns {
			if session.Turns[i].Sequence == opts.Turn {
				turn := session.Turns[i]
				turn.ContextAttribution.Reconciliation = model.ReconcileContext(turn)
				result.Turn = &turn
				break
			}
		}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
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
