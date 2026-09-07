package analyze

import (
	"context"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

type Analyzer struct{}

type Result struct {
	Source   string          `json:"source"`
	Session  model.Session   `json:"session"`
	Summary  Summary         `json:"summary"`
	Findings []model.Finding `json:"findings"`
}

type Summary struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func New() *Analyzer {
	return &Analyzer{}
}

func (a *Analyzer) Analyze(ctx context.Context, session model.Session) Result {
	_ = ctx

	return Result{
		Source:  string(session.Agent),
		Session: session,
		Summary: Summary{
			Status:  "ready",
			Message: "TokDoctor is initialized. No diagnostic rules are enabled in this scaffold.",
		},
		Findings: []model.Finding{},
	}
}
