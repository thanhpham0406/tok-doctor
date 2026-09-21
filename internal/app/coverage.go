package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/thanhpham0406/tok-doctor/internal/coverage"
	"github.com/thanhpham0406/tok-doctor/internal/model"
	"github.com/thanhpham0406/tok-doctor/internal/source"
)

// CoverageSession reports tool output coverage for one session, addressed by
// the same id resolution tok inspect uses.
func (a *App) CoverageSession(ctx context.Context, id string) (coverage.Result, error) {
	session, err := a.Inspect(ctx, id)
	if err != nil {
		return coverage.Result{}, err
	}
	name := sessionSourceName(session)
	if _, err := a.toolOutputReader(ctx, name); err != nil {
		return coverage.Result{}, err
	}
	return coverage.Result{
		Scope:     coverage.ScopeSession,
		Source:    name,
		SessionID: session.ID,
		Overall:   coverage.Scan([]model.Session{session}),
		Gateway:   coverage.UnsupportedGateway(),
	}, nil
}

// CoverageSource reports tool output coverage across every session of a source.
func (a *App) CoverageSource(ctx context.Context, name string) (coverage.Result, error) {
	reader, err := a.toolOutputReader(ctx, name)
	if err != nil {
		return coverage.Result{}, err
	}
	sessions, err := reader.ReadSessions(ctx)
	if err != nil {
		return coverage.Result{}, fmt.Errorf("tool output coverage for %s: %w", normalizeName(name), err)
	}
	// Coverage reports how much of what a source holds was read, so it scans
	// every session the adapter returns: filtering by authoritative provider
	// usage would hide a session that has tool output but no usage record.
	summary := coverage.Scan(sessions)
	return coverage.Result{
		Scope:   coverage.ScopeSource,
		Source:  normalizeName(name),
		Overall: summary,
		Sources: []coverage.SourceSummary{{Source: normalizeName(name), Summary: summary}},
		Gateway: coverage.UnsupportedGateway(),
	}, nil
}

// CoverageAll reports tool output coverage across every source that captures
// tool output. Sources that do not are listed with a reason instead of being
// counted as zero coverage.
func (a *App) CoverageAll(ctx context.Context) (coverage.Result, error) {
	result := coverage.Result{Scope: coverage.ScopeAll, Gateway: coverage.UnsupportedGateway()}
	var pooled []model.Session
	for _, detector := range a.sources.Detectors() {
		reader, reason := toolOutputReaderFor(ctx, detector)
		if reader == nil {
			result.UnsupportedSources = append(result.UnsupportedSources, coverage.Unsupported{
				Source: detector.Name(),
				Reason: reason,
			})
			continue
		}
		sessions, err := reader.ReadSessions(ctx)
		if err != nil {
			return coverage.Result{}, fmt.Errorf("tool output coverage for %s: %w", detector.Name(), err)
		}
		// No authoritative usage filter here either: see CoverageSource.
		result.Sources = append(result.Sources, coverage.SourceSummary{
			Source:  detector.Name(),
			Summary: coverage.Scan(sessions),
		})
		pooled = append(pooled, sessions...)
	}
	result.Overall = coverage.Scan(pooled)
	return result, nil
}

func (a *App) toolOutputReader(ctx context.Context, name string) (sessionSource, error) {
	detector, err := a.sources.Detector(normalizeName(name))
	if err != nil {
		return nil, err
	}
	reader, reason := toolOutputReaderFor(ctx, detector)
	if reader == nil {
		return nil, errors.New(reason)
	}
	return reader, nil
}

// toolOutputReaderFor returns the session reader of a detector that reports tool
// output, or the reason the source cannot be covered. Detection only supplies
// the adapter capabilities here, so the path and endpoint overrides of the
// source configuration are not needed.
func toolOutputReaderFor(ctx context.Context, detector source.Detector) (sessionSource, string) {
	reader, ok := detector.(sessionSource)
	if !ok {
		return nil, fmt.Sprintf("tool output coverage not supported for %s: source exposes no sessions", detector.Name())
	}
	detection := detector.Detect(ctx, source.Override{})
	if detection.Capabilities == nil || !detection.Capabilities.ToolOutput {
		return nil, fmt.Sprintf("tool output coverage not supported for %s: source does not report tool output", detector.Name())
	}
	return reader, ""
}
