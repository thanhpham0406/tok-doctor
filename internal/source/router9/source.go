package router9

import (
	"context"

	"github.com/thanhpham0406/tok-doctor/internal/source"
)

const defaultEndpoint = "http://127.0.0.1:20128"

type Source struct{}

func New() *Source {
	return &Source{}
}

func (s *Source) Name() string {
	return "router9"
}

func (s *Source) DisplayName() string {
	return "9Router"
}

func (s *Source) Kind() source.Kind {
	return source.KindEndpoint
}

func (s *Source) Detect(ctx context.Context, override source.Override) source.Detection {
	endpoint := defaultEndpoint
	origin := source.OriginAuto
	if override.Endpoint != "" {
		endpoint = override.Endpoint
		origin = override.Origin
	}
	result := source.ProbeEndpoint(ctx, s.Name(), s.DisplayName(), endpoint, origin)
	if override.Endpoint != "" || result.Status == source.StatusReady || result.Status == source.StatusBroken {
		return result
	}

	installed := source.DetectAutoPathSource(s.Name(), s.DisplayName(), installPaths(), nil, executableNames(), nil, unsupportedReady)
	if installed.Status == source.StatusInstalled {
		installed.Kind = source.KindEndpoint
		installed.Endpoint = endpoint
		installed.Evidence = append(result.Evidence, installed.Evidence...)
		return installed
	}
	return result
}

func (s *Source) Test(ctx context.Context, override source.Override) source.Detection {
	return s.Detect(ctx, override)
}

func unsupportedReady(path string) bool {
	_ = path
	return false
}
