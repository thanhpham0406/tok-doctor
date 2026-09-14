package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/thanhpham0406/tok-doctor/internal/gateway"
	"github.com/thanhpham0406/tok-doctor/internal/match"
	"github.com/thanhpham0406/tok-doctor/internal/model"
	"github.com/thanhpham0406/tok-doctor/internal/reconcile"
	"github.com/thanhpham0406/tok-doctor/internal/source/claude"
	"github.com/thanhpham0406/tok-doctor/internal/source/codex"
)

type InspectResult struct {
	Session             model.Session
	Reconciliation      *reconcile.Report
	UnavailableReason   string
	TurnReconciliations map[string]TurnReconciliation
}

type TurnReconciliation struct {
	Status match.Status
	Result *reconcile.Result
}

func (a *App) InspectReport(ctx context.Context, id string) (InspectResult, error) {
	session, err := a.Inspect(ctx, id)
	if err != nil {
		return InspectResult{}, err
	}
	report, reason, turns, err := a.reconcileSession(session)
	if err != nil {
		return InspectResult{}, err
	}
	result := InspectResult{
		Session:             session,
		UnavailableReason:   reason,
		TurnReconciliations: turns,
	}
	if reason == "" {
		reconciled := report
		result.Reconciliation = &reconciled
	}
	return result, nil
}

func (a *App) reconcileSession(session model.Session) (reconcile.Report, string, map[string]TurnReconciliation, error) {
	projector, ok := observationsForSession(session)
	if !ok {
		return reconcile.Report{}, fmt.Sprintf("reconciliation is not supported for source %q", sessionSourceName(session)), nil, nil
	}
	transcript, err := projector(session)
	if err != nil {
		return reconcile.Report{}, "", nil, fmt.Errorf("project session %s observations: %w", session.ID, err)
	}

	lower, upper, ok := sessionObservationWindow(session, match.DefaultTimeWindow)
	if !ok {
		return reconcile.Report{}, "session has no reliable timestamp for gateway reconciliation", nil, nil
	}

	profiles, err := a.reconciliationProfiles(sessionSourceName(session))
	if err != nil {
		return reconcile.Report{}, "", nil, err
	}
	if len(profiles) == 0 {
		return reconcile.Report{}, "gateway capture not available", nil, nil
	}

	gatewayObservations, found, readErr := a.gatewayObservations(profiles, lower, upper)
	if readErr != nil {
		return reconcile.Report{}, gatewayReadReason(readErr), nil, nil
	}
	if !found {
		return reconcile.Report{}, "gateway capture not available", nil, nil
	}

	observations := make([]model.Observation, 0, len(transcript)+len(gatewayObservations))
	observations = append(observations, transcript...)
	observations = append(observations, gatewayObservations...)

	report, err := reconcile.BuildReport(observations, match.Options{TimeWindow: match.DefaultTimeWindow})
	if err != nil {
		return reconcile.Report{}, "", nil, fmt.Errorf("build reconciliation report for session %s: %w", session.ID, err)
	}
	return report, "", turnReconciliations(session, transcript, report), nil
}

func observationsForSession(session model.Session) (func(model.Session) ([]model.Observation, error), bool) {
	switch sessionSourceName(session) {
	case string(model.AgentCodex):
		return codex.ObservationsFromSession, true
	case string(model.AgentClaude):
		return claude.ObservationsFromSession, true
	default:
		return nil, false
	}
}

func sessionSourceName(session model.Session) string {
	if session.Source != "" {
		return strings.ToLower(session.Source)
	}
	return strings.ToLower(string(session.Agent))
}

func (a *App) reconciliationProfiles(source string) ([]string, error) {
	if source == "" {
		return nil, nil
	}
	cfg, err := a.config.Load()
	if err != nil {
		return nil, fmt.Errorf("load config for reconciliation: %w", err)
	}
	selected := map[string]struct{}{source: {}}
	for name, profile := range cfg.Gateway.Profiles {
		if !profile.Enabled {
			continue
		}
		if strings.EqualFold(profile.Source, source) || strings.EqualFold(name, source) {
			selected[name] = struct{}{}
		}
	}
	profiles := make([]string, 0, len(selected))
	for name := range selected {
		profiles = append(profiles, name)
	}
	sort.Strings(profiles)
	return profiles, nil
}

func (a *App) gatewayObservations(profiles []string, lower, upper time.Time) ([]model.Observation, bool, error) {
	recorder, err := a.openRecorder()
	if err != nil {
		return nil, false, fmt.Errorf("open gateway capture: %w", err)
	}
	defer func() { _ = recorder.Close() }()

	var observations []model.Observation
	found := false
	var readErrs []error
	for _, profile := range profiles {
		exists, statErr := recorder.CaptureExists(profile)
		if statErr != nil {
			readErrs = append(readErrs, fmt.Errorf("profile %s: %w", profile, statErr))
			continue
		}
		if !exists {
			continue
		}
		found = true
		exchanges, listErr := recorder.ListExchanges(profile)
		if listErr != nil {
			readErrs = append(readErrs, fmt.Errorf("profile %s: %w", profile, listErr))
			continue
		}
		for _, exchange := range exchanges {
			if exchange.Kind != gateway.RequestKindModel {
				continue
			}
			observation, projectErr := gateway.ObservationFromExchange(exchange)
			if projectErr != nil {
				readErrs = append(readErrs, fmt.Errorf("profile %s: %w", profile, projectErr))
				continue
			}
			if !observationWithinWindow(observation, lower, upper) {
				continue
			}
			observations = append(observations, observation)
		}
	}
	if len(readErrs) > 0 {
		return nil, found, errors.Join(readErrs...)
	}
	return observations, found, nil
}

func observationWithinWindow(observation model.Observation, lower, upper time.Time) bool {
	if observation.StartedAt == nil {
		return false
	}
	return !observation.StartedAt.Before(lower) && !observation.StartedAt.After(upper)
}

func sessionObservationWindow(session model.Session, window time.Duration) (time.Time, time.Time, bool) {
	var times []time.Time
	if session.StartedAt != nil {
		times = append(times, *session.StartedAt)
	}
	if session.UpdatedAt != nil {
		times = append(times, *session.UpdatedAt)
	}
	for _, turn := range session.Turns {
		if turn.Timestamp != nil {
			times = append(times, *turn.Timestamp)
		}
	}
	if len(times) == 0 {
		return time.Time{}, time.Time{}, false
	}
	earliest, latest := times[0], times[0]
	for _, at := range times[1:] {
		if at.Before(earliest) {
			earliest = at
		}
		if at.After(latest) {
			latest = at
		}
	}
	return earliest.Add(-window), latest.Add(window), true
}

func turnReconciliations(session model.Session, transcript []model.Observation, report reconcile.Report) map[string]TurnReconciliation {
	turnObservations := make(map[string]string, len(session.Turns))
	for _, observation := range transcript {
		if observation.Scope != model.ObservationScopeTurn || observation.Identity.TurnID == "" {
			continue
		}
		turnObservations[observation.ID] = observation.Identity.TurnID
	}
	if len(turnObservations) == 0 {
		return nil
	}

	results := make(map[string]reconcile.Result, len(report.Reconciliations))
	for _, result := range report.Reconciliations {
		results[result.TranscriptObservationID] = result
	}

	turns := make(map[string]TurnReconciliation)
	for _, observation := range transcript {
		turnID := observation.Identity.TurnID
		if observation.Scope != model.ObservationScopeTurn || turnID == "" {
			continue
		}
		if result, ok := results[observation.ID]; ok {
			matched := result
			turns[turnID] = TurnReconciliation{Status: match.StatusMatched, Result: &matched}
		}
	}
	for _, observationMatch := range report.Matches {
		switch observationMatch.Status {
		case match.StatusUnmatched:
			if observationMatch.Side != match.SideTranscript {
				continue
			}
			turnID := turnObservations[observationMatch.TranscriptObservationID]
			if turnID == "" {
				continue
			}
			if _, exists := turns[turnID]; !exists {
				turns[turnID] = TurnReconciliation{Status: match.StatusUnmatched}
			}
		case match.StatusAmbiguous:
			for _, candidate := range observationMatch.CandidateTranscriptIDs {
				turnID := turnObservations[candidate]
				if turnID == "" {
					continue
				}
				if _, exists := turns[turnID]; !exists {
					turns[turnID] = TurnReconciliation{Status: match.StatusAmbiguous}
				}
			}
		}
	}
	if len(turns) == 0 {
		return nil
	}
	return turns
}

func gatewayReadReason(err error) string {
	return "gateway capture could not be read: " + safeErrorDetail(err)
}

func safeErrorDetail(err error) string {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return "file access error"
	}
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		return "file access error"
	}
	message := err.Error()
	if index := strings.IndexAny(message, "\r\n"); index >= 0 {
		message = message[:index]
	}
	return message
}
