package catalog

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/thanhpham0406/tok-doctor/internal/config"
	"github.com/thanhpham0406/tok-doctor/internal/source"
	"github.com/thanhpham0406/tok-doctor/internal/source/antigravity"
	"github.com/thanhpham0406/tok-doctor/internal/source/claude"
	"github.com/thanhpham0406/tok-doctor/internal/source/codex"
	"github.com/thanhpham0406/tok-doctor/internal/source/cursor"
	"github.com/thanhpham0406/tok-doctor/internal/source/kiro"
	"github.com/thanhpham0406/tok-doctor/internal/source/router9"
)

type Registry struct {
	detectors []source.Detector
}

func Builtins() Registry {
	return Registry{detectors: []source.Detector{
		codex.New(),
		router9.New(),
		cursor.New(),
		claude.New(),
		kiro.New(),
		antigravity.New(),
	}}
}

func (r Registry) All(ctx context.Context, cfg config.Config) source.ListResult {
	results := make([]source.Detection, 0, len(r.detectors))
	for _, detector := range r.detectors {
		results = append(results, detector.Detect(ctx, configOverride(cfg, detector.Name())))
	}
	return source.ListResult{Sources: results}
}

func (r Registry) Show(ctx context.Context, cfg config.Config, name string, cli source.Override) (source.Detection, error) {
	detector, err := r.find(name)
	if err != nil {
		return source.Detection{}, err
	}
	return detector.Detect(ctx, resolveOverride(configOverride(cfg, detector.Name()), cli)), nil
}

func (r Registry) Test(ctx context.Context, cfg config.Config, name string, cli source.Override) (source.Detection, error) {
	detector, err := r.find(name)
	if err != nil {
		return source.Detection{}, err
	}
	return detector.Test(ctx, resolveOverride(configOverride(cfg, detector.Name()), cli)), nil
}

func (r Registry) Kind(name string) (source.Kind, error) {
	detector, err := r.find(name)
	if err != nil {
		return "", err
	}
	return detector.Kind(), nil
}

func (r Registry) Detector(name string) (source.Detector, error) {
	return r.find(name)
}

func (r Registry) Detectors() []source.Detector {
	out := make([]source.Detector, len(r.detectors))
	copy(out, r.detectors)
	return out
}

func (r Registry) Names() []string {
	names := make([]string, 0, len(r.detectors))
	for _, detector := range r.detectors {
		names = append(names, detector.Name())
	}
	return names
}

func (r Registry) ValidateName(name string) bool {
	return slices.Contains(r.Names(), normalizeName(name))
}

func (r Registry) find(name string) (source.Detector, error) {
	normalized := normalizeName(name)
	for _, detector := range r.detectors {
		if detector.Name() == normalized {
			return detector, nil
		}
	}
	return nil, fmt.Errorf("unknown source %q", name)
}

func normalizeName(name string) string {
	name = strings.ToLower(name)
	if name == "router9" {
		return "9router"
	}
	return name
}

func configOverride(cfg config.Config, name string) source.Override {
	src := cfg.Sources[name]
	if name == "9router" && src.Endpoint == "" {
		src = cfg.Sources["router9"]
	}
	override := source.Override{Path: src.Path, Endpoint: src.Endpoint}
	if override.Path != "" || override.Endpoint != "" {
		override.Origin = source.OriginConfig
	}
	return override
}

func resolveOverride(cfg source.Override, cli source.Override) source.Override {
	if cli.Path != "" || cli.Endpoint != "" {
		cli.Origin = source.OriginCLI
		return cli
	}
	return cfg
}
