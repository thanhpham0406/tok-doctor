package app

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/thanhpham0406/tok-doctor/internal/analyze"
	"github.com/thanhpham0406/tok-doctor/internal/config"
	"github.com/thanhpham0406/tok-doctor/internal/model"
	"github.com/thanhpham0406/tok-doctor/internal/source"
	"github.com/thanhpham0406/tok-doctor/internal/source/catalog"
	"github.com/thanhpham0406/tok-doctor/internal/source/codex"
)

type App struct {
	codex    *codex.Source
	analyzer *analyze.Analyzer
	sources  catalog.Registry
	config   config.Store
}

func New() *App {
	store, err := config.NewStore()
	if err != nil {
		store = config.NewStoreAt("")
	}
	return NewWithStore(store)
}

func NewWithStore(store config.Store) *App {
	return &App{
		codex:    codex.New(),
		analyzer: analyze.New(),
		sources:  catalog.Builtins(),
		config:   store,
	}
}

func (a *App) Doctor(ctx context.Context) (analyze.Result, error) {
	session := a.codex.EmptySession(ctx)
	return a.analyzer.Analyze(ctx, session), nil
}

type usageSource interface {
	Sessions(ctx context.Context) ([]source.SessionRef, error)
	Usage(ctx context.Context, refs []source.SessionRef) (model.Usage, error)
}

func (a *App) Usage(ctx context.Context, name string) (model.UsageEntry, error) {
	name = normalizeName(name)
	detector, err := a.sources.Detector(name)
	if err != nil {
		return model.UsageEntry{}, err
	}
	src, ok := detector.(usageSource)
	if !ok {
		return model.UsageEntry{}, fmt.Errorf("usage not supported for %s", name)
	}
	refs, err := src.Sessions(ctx)
	if err != nil {
		return model.UsageEntry{}, fmt.Errorf("usage for %s: %w", name, err)
	}
	usage, err := src.Usage(ctx, refs)
	if err != nil {
		return model.UsageEntry{}, fmt.Errorf("usage for %s: %w", name, err)
	}
	return model.UsageEntry{Source: name, Usage: usage}, nil
}

func (a *App) UsageAll(ctx context.Context) (model.UsageResult, error) {
	var result model.UsageResult
	for _, detector := range a.sources.Detectors() {
		src, ok := detector.(usageSource)
		if !ok {
			continue
		}
		refs, err := src.Sessions(ctx)
		if err != nil {
			return model.UsageResult{}, fmt.Errorf("usage for %s: %w", detector.Name(), err)
		}
		usage, err := src.Usage(ctx, refs)
		if err != nil {
			return model.UsageResult{}, fmt.Errorf("usage for %s: %w", detector.Name(), err)
		}
		result.Sources = append(result.Sources, model.UsageEntry{Source: detector.Name(), Usage: usage})
	}
	return result, nil
}

func (a *App) Sources(ctx context.Context) (source.ListResult, error) {
	cfg, err := a.config.Load()
	if err != nil {
		return source.ListResult{}, err
	}
	return a.sources.All(ctx, cfg), nil
}

func (a *App) ShowSource(ctx context.Context, name string, override source.Override) (source.Detection, error) {
	cfg, err := a.config.Load()
	if err != nil {
		return source.Detection{}, err
	}
	name = normalizeName(name)
	if err := a.validateCLIOverride(name, override); err != nil {
		return source.Detection{}, err
	}
	return a.sources.Show(ctx, cfg, name, override)
}

func (a *App) TestSource(ctx context.Context, name string, override source.Override) (source.Detection, error) {
	cfg, err := a.config.Load()
	if err != nil {
		return source.Detection{}, err
	}
	name = normalizeName(name)
	if err := a.validateCLIOverride(name, override); err != nil {
		return source.Detection{}, err
	}
	return a.sources.Test(ctx, cfg, name, override)
}

func (a *App) SetSource(name string, override source.Override) error {
	name = normalizeName(name)
	kind, err := a.sources.Kind(name)
	if err != nil {
		return err
	}
	if err := validateOverride(kind, override); err != nil {
		return err
	}

	switch kind {
	case source.KindPath:
		return a.config.SetSource(name, config.Source{Path: override.Path})
	case source.KindEndpoint:
		return a.config.SetSource(name, config.Source{Endpoint: override.Endpoint})
	default:
		return fmt.Errorf("unsupported source kind %q", kind)
	}
}

func (a *App) ResetSource(name string) error {
	name = normalizeName(name)
	if !a.sources.ValidateName(name) {
		return fmt.Errorf("unknown source %q", name)
	}
	return a.config.ResetSource(name)
}

func validateOverride(kind source.Kind, override source.Override) error {
	switch kind {
	case source.KindPath:
		if override.Path == "" {
			return fmt.Errorf("path is required for this source")
		}
		if !filepath.IsAbs(override.Path) {
			return fmt.Errorf("path must be absolute")
		}
		if override.Endpoint != "" {
			return fmt.Errorf("endpoint is not supported for this source")
		}
	case source.KindEndpoint:
		if override.Endpoint == "" {
			return fmt.Errorf("endpoint is required for this source")
		}
		if _, err := url.ParseRequestURI(override.Endpoint); err != nil {
			return fmt.Errorf("invalid endpoint: %w", err)
		}
		if err := source.ValidateLocalEndpoint(override.Endpoint); err != nil {
			return err
		}
		if override.Path != "" {
			return fmt.Errorf("path is not supported for this source")
		}
	default:
		return fmt.Errorf("unsupported source kind %q", kind)
	}
	return nil
}

func (a *App) validateCLIOverride(name string, override source.Override) error {
	if override.Path == "" && override.Endpoint == "" {
		return nil
	}
	kind, err := a.sources.Kind(name)
	if err != nil {
		return err
	}
	return validateOverride(kind, override)
}

func normalizeName(name string) string {
	if strings.EqualFold(name, "router9") {
		return "9router"
	}
	return strings.ToLower(name)
}
