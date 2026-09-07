package antigravity

import (
	"context"

	"github.com/thanhpham0406/tok-doctor/internal/source"
)

type Source struct{}

func New() *Source {
	return &Source{}
}

func (s *Source) Name() string {
	return "antigravity"
}

func (s *Source) DisplayName() string {
	return "Antigravity"
}

func (s *Source) Kind() source.Kind {
	return source.KindPath
}

func (s *Source) Detect(ctx context.Context, override source.Override) source.Detection {
	_ = ctx
	if override.Path != "" {
		return source.DetectConfiguredInstallPath(s.Name(), s.DisplayName(), override.Path, override.Origin, "source path exists but supported Antigravity data detection is not implemented yet")
	}
	return source.DetectAutoPathSource(s.Name(), s.DisplayName(), installPaths(), nil, executableNames(), nil, unsupportedReady)
}

func (s *Source) Test(ctx context.Context, override source.Override) source.Detection {
	return s.Detect(ctx, override)
}

func unsupportedReady(path string) bool {
	_ = path
	return false
}
