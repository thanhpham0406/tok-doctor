// Package coverage measures how much of the data a rule needs TokDoctor
// actually observed. It reads only the canonical model, so the numbers stay
// comparable across sources, and it never copies content, content hashes,
// paths, prompts, or evidence into its result.
package coverage

import (
	"math"
	"slices"
	"sort"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

// Scope names the selection a result describes.
type Scope string

const (
	ScopeSession Scope = "session"
	ScopeSource  Scope = "source"
	ScopeAll     Scope = "all"
)

// Ratio is a count over a denominator. Value is a fraction in [0,1] and stays
// nil when the denominator is zero, because coverage is then undefined rather
// than zero.
type Ratio struct {
	Count int      `json:"count"`
	Total int      `json:"total"`
	Value *float64 `json:"value"`
}

func newRatio(count, total int) Ratio {
	ratio := Ratio{Count: count, Total: total}
	if total == 0 {
		return ratio
	}
	value := float64(count) / float64(total)
	ratio.Value = &value
	return ratio
}

// Bytes describes the distribution of tool output sizes in bytes. The
// percentiles use nearest rank over the observed samples; every field stays nil
// when no sample carried a byte size.
type Bytes struct {
	Total   int64  `json:"total"`
	Samples int    `json:"samples"`
	Min     *int64 `json:"min"`
	P50     *int64 `json:"p50"`
	P90     *int64 `json:"p90"`
	P95     *int64 `json:"p95"`
	Max     *int64 `json:"max"`
}

// Completeness counts recognized tool outputs by how much of them the source
// let TokDoctor observe. An output whose source stated nothing is counted as
// unknown.
//
// Recognized means the source record was read well enough to know it holds a
// tool result: every member of this type is a model.ContextToolResult
// component. A record whose kind or content could not be read is not counted
// here, because there is no evidence it holds tool output at all. That other
// population is reported by Summary.UnreadableRecords and stays outside every
// denominator built from Completeness.
type Completeness struct {
	Complete    int `json:"complete"`
	Truncated   int `json:"truncated"`
	Unavailable int `json:"unavailable"`
	Unknown     int `json:"unknown"`
}

// Summary holds the coverage metrics of one selection of sessions.
type Summary struct {
	Sessions            int   `json:"sessions"`
	Turns               int   `json:"turns"`
	ToolOutputs         int   `json:"toolOutputs"`
	WithContentBytes    Ratio `json:"withContentBytes"`
	WithEstimatedTokens Ratio `json:"withEstimatedTokens"`
	WithToolCallID      Ratio `json:"withToolCallId"`
	WithToolName        Ratio `json:"withToolName"`
	// RecognizedToolOutputCompleteness counts how much of each recognized tool
	// output the source let TokDoctor observe. Its denominator is ToolOutputs,
	// so it answers "of the tool outputs that were recognized, how complete is
	// each one" and never "how much of the session was read". A record whose
	// content or kind could not be read is not part of it: it is not known to
	// hold tool output, so adding it would compare unlike records and overstate
	// both the count and the denominator. See UnreadableRecords.
	RecognizedToolOutputCompleteness Completeness      `json:"recognizedToolOutputCompleteness"`
	TrailingContext                  int               `json:"trailingContext"`
	WithFreshInput                   Ratio             `json:"withFreshInput"`
	ContentBytes                     Bytes             `json:"contentBytes"`
	EstimatedTokens                  model.Measurement `json:"estimatedTokens"`
	// UnreadableRecords counts source records TokDoctor read only partially or
	// not at all. These sit outside ToolOutputs and outside
	// RecognizedToolOutputCompleteness. A record that is already recognized as a
	// tool result is counted here as well when it was truncated or unavailable,
	// but a record that stayed unreadable is never added to the tool output
	// count, because nothing shows it holds tool output.
	UnreadableRecords  int `json:"unreadableRecords"`
	UnreadableSessions int `json:"unreadableSessions"`
}

// SourceSummary is the coverage of every session of one source.
type SourceSummary struct {
	Source  string  `json:"source"`
	Summary Summary `json:"summary"`
}

// Unsupported names a source left out of an aggregate and why.
type Unsupported struct {
	Source string `json:"source"`
	Reason string `json:"reason"`
}

// GatewayCoverage reports whether gateway capture contributes to this result.
type GatewayCoverage struct {
	Supported bool   `json:"supported"`
	Reason    string `json:"reason,omitempty"`
}

// UnsupportedGateway states why gateway capture is not part of the coverage of
// tool outputs. The recorder captures HTTP request and response bodies, not
// individual tool outputs, and no reader boundary maps its capture state onto
// canonical tool outputs, so reporting it here would compare unlike things.
func UnsupportedGateway() GatewayCoverage {
	return GatewayCoverage{
		Supported: false,
		Reason: "gateway capture records request and response bodies rather than tool outputs; " +
			"no reader exposes per-tool-output capture state",
	}
}

// Result is the coverage of the selection a user asked for. SessionID is set
// only when the user selected a single session.
type Result struct {
	Scope              Scope           `json:"scope"`
	Source             string          `json:"source,omitempty"`
	SessionID          string          `json:"sessionId,omitempty"`
	Overall            Summary         `json:"overall"`
	Sources            []SourceSummary `json:"sources,omitempty"`
	UnsupportedSources []Unsupported   `json:"unsupportedSources,omitempty"`
	Gateway            GatewayCoverage `json:"gateway"`
}

// Scan measures tool output coverage across sessions.
func Scan(sessions []model.Session) Summary {
	state := &scanState{}
	for _, session := range sessions {
		state.session(session)
	}
	return state.finish()
}

type scanState struct {
	summary            Summary
	sizes              []int64
	withContentBytes   int
	withEstimated      int
	withCallID         int
	withName           int
	withFreshInput     int
	complete           int
	truncated          int
	unavailable        int
	unknown            int
	estimatedTokens    int64
	estimatedTokensSet bool
}

func (s *scanState) session(session model.Session) {
	s.summary.Sessions++
	s.summary.Turns += len(session.Turns)
	unreadable := false
	for _, turn := range session.Turns {
		linked := model.TurnFreshInput(turn).HasAuthoritativeUsage()
		for _, component := range turn.ContextAttribution.Components {
			unreadable = s.component(component, linked) || unreadable
		}
	}
	for _, component := range session.TrailingContext {
		if component.Kind == model.ContextToolResult {
			s.summary.TrailingContext++
		}
		// Trailing context belongs to no turn, so it carries no fresh input to
		// link to.
		unreadable = s.component(component, false) || unreadable
	}
	if unreadable {
		s.summary.UnreadableSessions++
	}
}

// component folds one context component into the summary and reports whether
// the record it came from was only partially readable.
//
// Only a component the adapter recognized as a tool result enters the tool
// output count. A record TokDoctor could not read stays a component of unknown
// kind, so it raises the unreadable count without ever entering the tool output
// denominator.
func (s *scanState) component(component model.ContextComponent, linked bool) bool {
	if component.Kind == model.ContextToolResult {
		s.toolOutput(component, linked)
	}
	switch component.Completeness {
	case model.ContextCompletenessTruncated, model.ContextCompletenessUnavailable:
		s.summary.UnreadableRecords++
		return true
	default:
		return false
	}
}

func (s *scanState) toolOutput(component model.ContextComponent, linked bool) {
	s.summary.ToolOutputs++
	if component.ContentBytes != nil {
		s.withContentBytes++
		s.sizes = append(s.sizes, *component.ContentBytes)
	}
	if component.ToolCallID != "" {
		s.withCallID++
	}
	if component.ToolName != "" {
		s.withName++
	}
	if linked {
		s.withFreshInput++
	}
	// Only estimated measurements feed the token total, so a later adapter that
	// reports measured tool output tokens cannot be mixed into it.
	if component.Measurement.Kind == model.MeasurementEstimated && component.Measurement.Available() {
		s.withEstimated++
		s.estimatedTokens += component.Measurement.ValueOrZero()
		s.estimatedTokensSet = true
	}
	switch component.Completeness {
	case model.ContextCompletenessComplete:
		s.complete++
	case model.ContextCompletenessTruncated:
		s.truncated++
	case model.ContextCompletenessUnavailable:
		s.unavailable++
	default:
		s.unknown++
	}
}

func (s *scanState) finish() Summary {
	summary := s.summary
	total := summary.ToolOutputs
	summary.WithContentBytes = newRatio(s.withContentBytes, total)
	summary.WithEstimatedTokens = newRatio(s.withEstimated, total)
	summary.WithToolCallID = newRatio(s.withCallID, total)
	summary.WithToolName = newRatio(s.withName, total)
	summary.WithFreshInput = newRatio(s.withFreshInput, total)
	summary.RecognizedToolOutputCompleteness = Completeness{
		Complete:    s.complete,
		Truncated:   s.truncated,
		Unavailable: s.unavailable,
		Unknown:     s.unknown,
	}
	summary.ContentBytes = byteStats(s.sizes)
	if s.estimatedTokensSet {
		summary.EstimatedTokens = model.NewMeasurement(s.estimatedTokens, model.MeasurementEstimated)
	} else {
		summary.EstimatedTokens = model.Measurement{Kind: model.MeasurementUnknown}
	}
	return summary
}

func byteStats(sizes []int64) Bytes {
	if len(sizes) == 0 {
		return Bytes{}
	}
	sorted := slices.Clone(sizes)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	var total int64
	for _, size := range sorted {
		total += size
	}
	return Bytes{
		Total:   total,
		Samples: len(sorted),
		Min:     model.Int64(sorted[0]),
		P50:     model.Int64(percentile(sorted, 50)),
		P90:     model.Int64(percentile(sorted, 90)),
		P95:     model.Int64(percentile(sorted, 95)),
		Max:     model.Int64(sorted[len(sorted)-1]),
	}
}

// percentile returns the nearest-rank percentile of an ascending slice.
func percentile(sorted []int64, percent float64) int64 {
	rank := int(math.Ceil(percent / 100 * float64(len(sorted))))
	if rank < 1 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}
	return sorted[rank-1]
}
