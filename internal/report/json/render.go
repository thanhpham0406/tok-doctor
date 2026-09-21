package json

import (
	"encoding/json"
	"io"

	"github.com/thanhpham0406/tok-doctor/internal/analyze"
	"github.com/thanhpham0406/tok-doctor/internal/model"
)

type Result struct {
	Source   string          `json:"source"`
	Session  model.Session   `json:"session"`
	Summary  analyze.Summary `json:"summary"`
	Findings []model.Finding `json:"findings"`
}

func Render(w io.Writer, result analyze.Result) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(redactedResult(result))
}

// redactedResult copies an analysis result and clears the internal digests that
// TOOL002 needs in memory but that must not leave the process: the tool call
// argument fingerprint and the context content hash of every context component.
// Redaction is scoped to this doctor JSON output. The canonical model keeps the
// digests, this renderer never mutates the session it is given, and the other
// report packages keep serializing model.Session unchanged.
func redactedResult(result analyze.Result) Result {
	return Result{
		Source:   result.Source,
		Session:  redactedSession(result.Session),
		Summary:  result.Summary,
		Findings: result.Findings,
	}
}

func redactedSession(session model.Session) model.Session {
	redacted := session
	redacted.Turns = redactedTurns(session.Turns)
	redacted.TrailingContext = redactedComponents(session.TrailingContext)
	return redacted
}

func redactedTurns(turns []model.Turn) []model.Turn {
	if turns == nil {
		return nil
	}
	redacted := make([]model.Turn, len(turns))
	for i, turn := range turns {
		turn.ContextAttribution.Components = redactedComponents(turn.ContextAttribution.Components)
		redacted[i] = turn
	}
	return redacted
}

func redactedComponents(components []model.ContextComponent) []model.ContextComponent {
	if components == nil {
		return nil
	}
	redacted := make([]model.ContextComponent, len(components))
	for i, component := range components {
		component.ContentHash = ""
		component.ToolCallFingerprint = ""
		redacted[i] = component
	}
	return redacted
}
