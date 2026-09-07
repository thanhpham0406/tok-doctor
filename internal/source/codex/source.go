package codex

import (
	"context"

	"github.com/thanhpham0406/tok-doctor/internal/model"
	"github.com/thanhpham0406/tok-doctor/internal/source"
)

type Source struct{}

func New() *Source {
	return &Source{}
}

func (s *Source) Name() string {
	return string(model.AgentCodex)
}

func (s *Source) DisplayName() string {
	return "Codex"
}

func (s *Source) Kind() source.Kind {
	return source.KindPath
}

func (s *Source) Detect(ctx context.Context, override source.Override) source.Detection {
	return s.detect(ctx, override)
}

func (s *Source) Test(ctx context.Context, override source.Override) source.Detection {
	return s.detect(ctx, override)
}

func (s *Source) detect(ctx context.Context, override source.Override) source.Detection {
	_ = ctx

	if override.Path != "" {
		return source.DetectConfiguredPath(s.Name(), s.DisplayName(), override.Path, override.Origin, s.capabilities(), hasCodexSessionData)
	}

	return source.DetectAutoPathSource(s.Name(), s.DisplayName(), installPaths(), sessionPaths(), executableNames(), s.capabilities(), hasCodexSessionData)
}

func (s *Source) EmptySession(ctx context.Context) model.Session {
	_ = ctx

	return model.Session{
		ID:    "scaffold",
		Agent: model.AgentCodex,
	}
}

func (s *Source) capabilities() *source.Capabilities {
	return &source.Capabilities{
		SessionDiscovery: true,
		TokenUsage:       true,
		ToolCalls:        true,
		ToolOutput:       true,
	}
}

func hasCodexSessionData(path string) bool {
	return source.HasFileWithSuffix(path, ".jsonl", 4)
}
