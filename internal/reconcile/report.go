package reconcile

import (
	"fmt"

	"github.com/thanhpham0406/tok-doctor/internal/match"
	"github.com/thanhpham0406/tok-doctor/internal/model"
)

type Report struct {
	Summary         Summary
	Matches         []match.ObservationMatch
	Reconciliations []Result
}

type Summary struct {
	GatewayRequests      int
	TranscriptTurns      int
	Matched              int
	Equal                int
	Different            int
	Unavailable          int
	AmbiguousGateways    int
	UnmatchedGateways    int
	UnmatchedTranscripts int
}

func BuildReport(observations []model.Observation, options match.Options) (Report, error) {
	matches, err := match.Match(observations, options)
	if err != nil {
		return Report{}, fmt.Errorf("build reconciliation report: match observations: %w", err)
	}
	return reportFromMatches(observations, matches)
}

func reportFromMatches(observations []model.Observation, matches []match.ObservationMatch) (Report, error) {
	byID := make(map[string]model.Observation, len(observations))
	for _, observation := range observations {
		byID[observation.ID] = observation
	}

	report := Report{
		Matches:         cloneMatches(matches),
		Reconciliations: make([]Result, 0, len(matches)),
	}
	for _, observation := range observations {
		switch {
		case isGatewayRequest(observation):
			report.Summary.GatewayRequests++
		case isTranscriptTurn(observation):
			report.Summary.TranscriptTurns++
		}
	}

	for _, observationMatch := range matches {
		if err := report.accumulate(observationMatch, byID); err != nil {
			return Report{}, err
		}
	}
	return report, nil
}

func (r *Report) accumulate(observationMatch match.ObservationMatch, byID map[string]model.Observation) error {
	switch observationMatch.Status {
	case match.StatusMatched:
		if observationMatch.GatewayObservationID == "" || observationMatch.TranscriptObservationID == "" {
			return fmt.Errorf("build reconciliation report: match %s: missing observation id", matchResultLabel(observationMatch))
		}
		gateway, ok := byID[observationMatch.GatewayObservationID]
		if !ok {
			return fmt.Errorf("build reconciliation report: gateway observation %q not found", observationMatch.GatewayObservationID)
		}
		if !isGatewayRequest(gateway) {
			return fmt.Errorf("build reconciliation report: observation %q is not a gateway request", gateway.ID)
		}
		transcript, ok := byID[observationMatch.TranscriptObservationID]
		if !ok {
			return fmt.Errorf("build reconciliation report: transcript observation %q not found", observationMatch.TranscriptObservationID)
		}
		if !isTranscriptTurn(transcript) {
			return fmt.Errorf("build reconciliation report: observation %q is not a transcript turn", transcript.ID)
		}
		result, err := Reconcile(Pair{Gateway: gateway, Transcript: transcript, Match: observationMatch})
		if err != nil {
			return fmt.Errorf("build reconciliation report: reconcile %s: %w", matchResultLabel(observationMatch), err)
		}
		r.Reconciliations = append(r.Reconciliations, result)
		r.Summary.Matched++
		switch result.Status {
		case StatusEqual:
			r.Summary.Equal++
		case StatusDifferent:
			r.Summary.Different++
		case StatusUnavailable:
			r.Summary.Unavailable++
		}
	case match.StatusAmbiguous:
		if observationMatch.Side == match.SideGateway {
			r.Summary.AmbiguousGateways++
		}
	case match.StatusUnmatched:
		switch observationMatch.Side {
		case match.SideGateway:
			r.Summary.UnmatchedGateways++
		case match.SideTranscript:
			r.Summary.UnmatchedTranscripts++
		}
	}
	return nil
}

func cloneMatches(matches []match.ObservationMatch) []match.ObservationMatch {
	if matches == nil {
		return nil
	}
	cloned := make([]match.ObservationMatch, len(matches))
	for i, observationMatch := range matches {
		cloned[i] = observationMatch
		if observationMatch.Reasons != nil {
			cloned[i].Reasons = append([]match.Reason(nil), observationMatch.Reasons...)
		}
		if observationMatch.CandidateTranscriptIDs != nil {
			cloned[i].CandidateTranscriptIDs = append([]string(nil), observationMatch.CandidateTranscriptIDs...)
		}
	}
	return cloned
}

func isGatewayRequest(observation model.Observation) bool {
	return observation.Channel == model.ObservationChannelGateway && observation.Scope == model.ObservationScopeRequest
}

func isTranscriptTurn(observation model.Observation) bool {
	return observation.Channel == model.ObservationChannelSessionTranscript && observation.Scope == model.ObservationScopeTurn
}
