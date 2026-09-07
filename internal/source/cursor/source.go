package cursor

import (
	"context"

	"github.com/thanhpham0406/tok-doctor/internal/source"
)

type Source struct{}

func New() *Source {
	return &Source{}
}

func (s *Source) Name() string {
	return "cursor"
}

func (s *Source) DisplayName() string {
	return "Cursor"
}

func (s *Source) Kind() source.Kind {
	return source.KindPath
}

func (s *Source) Detect(ctx context.Context, override source.Override) source.Detection {
	_ = ctx
	if override.Path != "" {
		return source.DetectConfiguredPath(s.Name(), s.DisplayName(), override.Path, override.Origin, nil, hasCursorSupportedData)
	}
	return source.DetectAutoPathSource(s.Name(), s.DisplayName(), installPaths(), readyPaths(), executableNames(), nil, hasCursorSupportedData)
}

func (s *Source) Test(ctx context.Context, override source.Override) source.Detection {
	return s.Detect(ctx, override)
}

func hasCursorSupportedData(path string) bool {
	return source.HasFileWithSuffix(path, ".vscdb", 1)
}
