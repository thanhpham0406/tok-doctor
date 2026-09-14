package match

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

type Status string

const (
	StatusMatched   Status = "matched"
	StatusUnmatched Status = "unmatched"
	StatusAmbiguous Status = "ambiguous"
)

type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
)

type Reason string

const (
	ReasonSharedIdentity Reason = "shared_identity"
	ReasonTime           Reason = "time"
	ReasonModel          Reason = "model"
	ReasonUsage          Reason = "usage"
)

type Side string

const (
	SideGateway    Side = "gateway"
	SideTranscript Side = "transcript"
)

const DefaultTimeWindow = 2 * time.Minute

type Options struct {
	TimeWindow time.Duration
}

type ObservationMatch struct {
	ID                      string
	GatewayObservationID    string
	TranscriptObservationID string
	Status                  Status
	Confidence              Confidence
	Reasons                 []Reason
	Side                    Side
	CandidateTranscriptIDs  []string
}

func Match(observations []model.Observation, options Options) ([]ObservationMatch, error) {
	if options.TimeWindow <= 0 {
		return nil, fmt.Errorf("match: invalid time window %s", options.TimeWindow)
	}
	if err := validateObservations(observations); err != nil {
		return nil, err
	}

	var gateways, transcripts []model.Observation
	for _, observation := range observations {
		switch {
		case observation.Channel == model.ObservationChannelGateway && observation.Scope == model.ObservationScopeRequest:
			gateways = append(gateways, observation)
		case observation.Channel == model.ObservationChannelSessionTranscript && observation.Scope == model.ObservationScopeTurn:
			transcripts = append(transcripts, observation)
		}
	}
	sort.Slice(gateways, func(i, j int) bool { return gateways[i].ID < gateways[j].ID })
	sort.Slice(transcripts, func(i, j int) bool { return transcripts[i].ID < transcripts[j].ID })

	gatewayRelations := make(map[string]map[string]relation, len(gateways))
	transcriptRelations := make(map[string]map[string]relation, len(transcripts))
	for _, gateway := range gateways {
		for _, transcript := range transcripts {
			if sharesIdentity(gateway.Identity, transcript.Identity) {
				addRelation(gatewayRelations, gateway.ID, transcript.ID, relation{identity: true})
				addRelation(transcriptRelations, transcript.ID, gateway.ID, relation{identity: true})
				continue
			}
			score, ok := heuristicScore(gateway, transcript, options.TimeWindow)
			if !ok {
				continue
			}
			addRelation(gatewayRelations, gateway.ID, transcript.ID, relation{score: score})
			addRelation(transcriptRelations, transcript.ID, gateway.ID, relation{score: score})
		}
	}

	gatewayPreferences := make(map[string]preference, len(gateways))
	for _, gateway := range gateways {
		gatewayPreferences[gateway.ID] = choosePreference(gatewayRelations[gateway.ID])
	}
	transcriptPreferences := make(map[string]preference, len(transcripts))
	for _, transcript := range transcripts {
		transcriptPreferences[transcript.ID] = choosePreference(transcriptRelations[transcript.ID])
	}

	matches := make([]ObservationMatch, 0, len(gateways)+len(transcripts))
	consumedTranscripts := make(map[string]struct{}, len(transcripts))
	for _, gateway := range gateways {
		gatewayPreference := gatewayPreferences[gateway.ID]
		result := resolve(gateway.ID, gatewayPreference, transcriptPreferences)
		matches = append(matches, result)
		switch result.Status {
		case StatusMatched:
			consumedTranscripts[result.TranscriptObservationID] = struct{}{}
		case StatusAmbiguous:
			for _, transcriptID := range result.CandidateTranscriptIDs {
				consumedTranscripts[transcriptID] = struct{}{}
			}
		}
	}
	for _, transcript := range transcripts {
		if _, consumed := consumedTranscripts[transcript.ID]; consumed {
			continue
		}
		matches = append(matches, ObservationMatch{
			ID:                      transcriptOnlyID(transcript.ID),
			TranscriptObservationID: transcript.ID,
			Status:                  StatusUnmatched,
			Side:                    SideTranscript,
		})
	}
	return matches, nil
}

func validateObservations(observations []model.Observation) error {
	seen := make(map[string]struct{}, len(observations))
	for _, observation := range observations {
		if err := observation.Validate(); err != nil {
			return err
		}
		if _, ok := seen[observation.ID]; ok {
			return fmt.Errorf("match: duplicate observation id %q", observation.ID)
		}
		seen[observation.ID] = struct{}{}
	}
	return nil
}

type relation struct {
	identity bool
	score    pairScore
}

type prefKind int

const (
	prefIdentity prefKind = iota + 1
	prefHeuristic
)

type preference struct {
	kind  prefKind
	picks []string
	score pairScore
}

type pairScore struct {
	delta        time.Duration
	modelMatch   bool
	usageMatches int
}

func addRelation(relations map[string]map[string]relation, from, to string, value relation) {
	byTarget := relations[from]
	if byTarget == nil {
		byTarget = make(map[string]relation)
		relations[from] = byTarget
	}
	byTarget[to] = value
}

func choosePreference(relations map[string]relation) preference {
	var identityPicks []string
	for id, rel := range relations {
		if rel.identity {
			identityPicks = append(identityPicks, id)
		}
	}
	if len(identityPicks) > 0 {
		sort.Strings(identityPicks)
		return preference{kind: prefIdentity, picks: identityPicks}
	}
	if len(relations) == 0 {
		return preference{}
	}

	var top []string
	var topScore pairScore
	first := true
	for id, rel := range relations {
		switch {
		case first:
			top = []string{id}
			topScore = rel.score
			first = false
		case better(rel.score, topScore):
			top = []string{id}
			topScore = rel.score
		case !better(topScore, rel.score):
			top = append(top, id)
		}
	}
	sort.Strings(top)
	return preference{kind: prefHeuristic, picks: top, score: topScore}
}

func resolve(gatewayID string, gatewayPreference preference, transcriptPreferences map[string]preference) ObservationMatch {
	result := ObservationMatch{
		ID:                   gatewayOnlyID(gatewayID),
		GatewayObservationID: gatewayID,
		Status:               StatusUnmatched,
		Side:                 SideGateway,
	}
	switch gatewayPreference.kind {
	case prefIdentity:
		if len(gatewayPreference.picks) != 1 {
			result.Status = StatusAmbiguous
			result.CandidateTranscriptIDs = candidateIDs(gatewayPreference.picks)
			return result
		}
		transcriptID := gatewayPreference.picks[0]
		switch resolveTranscript(transcriptID, transcriptPreferences, gatewayID, prefIdentity) {
		case resolutionMatch:
			result = matchedResult(gatewayID, transcriptID, ConfidenceHigh, []Reason{ReasonSharedIdentity})
		case resolutionAmbiguous:
			result.Status = StatusAmbiguous
			result.CandidateTranscriptIDs = []string{transcriptID}
		}
		return result
	case prefHeuristic:
		if len(gatewayPreference.picks) != 1 {
			result.Status = StatusAmbiguous
			result.CandidateTranscriptIDs = candidateIDs(gatewayPreference.picks)
			return result
		}
		transcriptID := gatewayPreference.picks[0]
		switch resolveTranscript(transcriptID, transcriptPreferences, gatewayID, prefHeuristic) {
		case resolutionMatch:
			result = matchedResult(gatewayID, transcriptID, ConfidenceMedium, mediumReasons(gatewayPreference.score))
		case resolutionAmbiguous:
			result.Status = StatusAmbiguous
			result.CandidateTranscriptIDs = []string{transcriptID}
		}
		return result
	default:
		return result
	}
}

func matchedResult(gatewayID, transcriptID string, confidence Confidence, reasons []Reason) ObservationMatch {
	return ObservationMatch{
		ID:                      matchID(gatewayID, transcriptID),
		GatewayObservationID:    gatewayID,
		TranscriptObservationID: transcriptID,
		Status:                  StatusMatched,
		Confidence:              confidence,
		Reasons:                 reasons,
	}
}

func candidateIDs(picks []string) []string {
	candidates := append([]string(nil), picks...)
	sort.Strings(candidates)
	return candidates
}

func gatewayOnlyID(gatewayID string) string {
	return "match:gateway:" + gatewayID
}

func transcriptOnlyID(transcriptID string) string {
	return "match:transcript:" + transcriptID
}

type resolution int

const (
	resolutionUnmatched resolution = iota
	resolutionMatch
	resolutionAmbiguous
)

func resolveTranscript(transcriptID string, preferences map[string]preference, gatewayID string, want prefKind) resolution {
	transcriptPreference, ok := preferences[transcriptID]
	if !ok || transcriptPreference.kind != want {
		return resolutionUnmatched
	}
	if len(transcriptPreference.picks) != 1 {
		return resolutionAmbiguous
	}
	if transcriptPreference.picks[0] == gatewayID {
		return resolutionMatch
	}
	return resolutionUnmatched
}

func mediumReasons(score pairScore) []Reason {
	reasons := []Reason{ReasonTime}
	if score.modelMatch {
		reasons = append(reasons, ReasonModel)
	}
	if score.usageMatches > 0 {
		reasons = append(reasons, ReasonUsage)
	}
	return reasons
}

func matchID(gatewayID, transcriptID string) string {
	return "match:" + gatewayID + "=" + transcriptID
}

func heuristicScore(gateway, transcript model.Observation, window time.Duration) (pairScore, bool) {
	if gateway.StartedAt == nil || transcript.StartedAt == nil {
		return pairScore{}, false
	}
	delta := gateway.StartedAt.Sub(*transcript.StartedAt)
	if delta < 0 {
		delta = -delta
	}
	if delta > window {
		return pairScore{}, false
	}
	sameModel, compatible := compatibleModel(gateway.Model, transcript.Model)
	if !compatible {
		return pairScore{}, false
	}
	matches := usageSimilarity(gateway.Usage, transcript.Usage)
	if !sameModel && matches == 0 {
		return pairScore{}, false
	}
	return pairScore{
		delta:        delta,
		modelMatch:   sameModel,
		usageMatches: matches,
	}, true
}

func better(a, b pairScore) bool {
	if a.delta != b.delta {
		return a.delta < b.delta
	}
	if a.usageMatches != b.usageMatches {
		return a.usageMatches > b.usageMatches
	}
	if a.modelMatch != b.modelMatch {
		return a.modelMatch
	}
	return false
}

func compatibleModel(a, b string) (match, compatible bool) {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false, true
	}
	if strings.EqualFold(a, b) {
		return true, true
	}
	return false, false
}

func usageSimilarity(a, b model.ObservationUsage) (matches int) {
	fields := []struct {
		left  model.Measurement
		right model.Measurement
	}{
		{a.FreshInput, b.FreshInput},
		{a.CachedInput, b.CachedInput},
		{a.CacheCreationInput, b.CacheCreationInput},
		{a.TotalInput, b.TotalInput},
		{a.Output, b.Output},
		{a.ReasoningOutput, b.ReasoningOutput},
		{a.Total, b.Total},
	}
	for _, field := range fields {
		if !field.left.Available() || !field.right.Available() {
			continue
		}
		if field.left.ValueOrZero() == field.right.ValueOrZero() {
			matches++
		}
	}
	return matches
}

var identityFields = []func(model.ObservationIdentity) string{
	func(i model.ObservationIdentity) string { return i.ExchangeID },
	func(i model.ObservationIdentity) string { return i.AgentRequestID },
	func(i model.ObservationIdentity) string { return i.ProviderRequestID },
	func(i model.ObservationIdentity) string { return i.ResponseObjectID },
	func(i model.ObservationIdentity) string { return i.ParentResponseObjectID },
	func(i model.ObservationIdentity) string { return i.SessionID },
	func(i model.ObservationIdentity) string { return i.TurnID },
	func(i model.ObservationIdentity) string { return i.InvocationID },
	func(i model.ObservationIdentity) string { return i.ChainID },
}

func sharesIdentity(a, b model.ObservationIdentity) bool {
	for _, value := range identityFields {
		left := value(a)
		if left == "" {
			continue
		}
		if left == value(b) {
			return true
		}
	}
	return false
}
