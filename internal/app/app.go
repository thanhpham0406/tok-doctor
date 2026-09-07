package app

import (
	"context"

	"github.com/thanhpham0406/tok-doctor/internal/analyze"
	"github.com/thanhpham0406/tok-doctor/internal/source/codex"
)

type App struct {
	codex    *codex.Source
	analyzer *analyze.Analyzer
}

func New() *App {
	return &App{
		codex:    codex.New(),
		analyzer: analyze.New(),
	}
}

func (a *App) Doctor(ctx context.Context) (analyze.Result, error) {
	session := a.codex.EmptySession(ctx)
	return a.analyzer.Analyze(ctx, session), nil
}
