package analyze

import (
	"context"
	"fmt"

	"github.com/thanhpham0406/tok-doctor/internal/model"
	ruletool "github.com/thanhpham0406/tok-doctor/internal/rule/tool"
)

type rule interface {
	Analyze(context.Context, model.Session) []model.Finding
}

type Analyzer struct {
	rules []rule
}

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
	return &Analyzer{rules: []rule{ruletool.NewOversizedOutput(0)}}
}

func (a *Analyzer) Analyze(ctx context.Context, session model.Session) Result {
	var findings []model.Finding
	for _, diagnostic := range a.rules {
		findings = append(findings, diagnostic.Analyze(ctx, session)...)
	}
	status := "healthy"
	message := "No token-efficiency findings detected."
	if len(findings) > 0 {
		status = "findings"
		message = fmt.Sprintf("TokDoctor found %d token-efficiency issue(s).", len(findings))
	}

	return Result{
		Source:  string(session.Agent),
		Session: session,
		Summary: Summary{
			Status:  status,
			Message: message,
		},
		Findings: findings,
	}
}
