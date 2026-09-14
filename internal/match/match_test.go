package match

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

func testTime(offset time.Duration) *time.Time {
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	at := base.Add(offset)
	return &at
}

func gatewayObservation(id string, startedAt *time.Time, modelName string, usage model.ObservationUsage, identity model.ObservationIdentity) model.Observation {
	return model.Observation{
		SchemaVersion: model.ObservationSchemaVersion,
		ID:            id,
		Channel:       model.ObservationChannelGateway,
		Scope:         model.ObservationScopeRequest,
		Source:        "gateway",
		Identity:      identity,
		StartedAt:     startedAt,
		Model:         modelName,
		Usage:         usage,
		Outcome:       model.ObservationOutcomeSucceeded,
		Completeness:  model.ObservationCompletenessComplete,
	}
}

func transcriptObservation(id string, startedAt *time.Time, modelName string, usage model.ObservationUsage, identity model.ObservationIdentity) model.Observation {
	return model.Observation{
		SchemaVersion: model.ObservationSchemaVersion,
		ID:            id,
		Channel:       model.ObservationChannelSessionTranscript,
		Scope:         model.ObservationScopeTurn,
		Source:        "claude",
		Identity:      identity,
		StartedAt:     startedAt,
		Model:         modelName,
		Usage:         usage,
		Outcome:       model.ObservationOutcomeUnknown,
		Completeness:  model.ObservationCompletenessComplete,
	}
}

func measured(value int64) model.Measurement {
	return model.NewMeasurement(value, model.MeasurementMeasured)
}

func matchedUsage() model.ObservationUsage {
	return model.ObservationUsage{
		FreshInput:  measured(100),
		CachedInput: measured(40),
		TotalInput:  measured(140),
		Output:      measured(20),
		Total:       measured(160),
	}
}

func differentUsage() model.ObservationUsage {
	return model.ObservationUsage{
		FreshInput:  measured(999),
		CachedInput: measured(999),
		TotalInput:  measured(999),
		Output:      measured(999),
		Total:       measured(999),
	}
}

func options() Options {
	return Options{TimeWindow: 2 * time.Minute}
}

func matchResults(t *testing.T, observations []model.Observation) []ObservationMatch {
	t.Helper()
	matches, err := Match(observations, options())
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	return matches
}

func matchOne(t *testing.T, observations []model.Observation) ObservationMatch {
	t.Helper()
	matches := matchResults(t, observations)
	if len(matches) != 1 {
		t.Fatalf("matches = %d, want 1: %+v", len(matches), matches)
	}
	return matches[0]
}

func matchedResults(matches []ObservationMatch) []ObservationMatch {
	var matched []ObservationMatch
	for _, match := range matches {
		if match.Status == StatusMatched {
			matched = append(matched, match)
		}
	}
	return matched
}

func findGatewayResult(t *testing.T, matches []ObservationMatch, gatewayID string) ObservationMatch {
	t.Helper()
	for _, match := range matches {
		if match.GatewayObservationID == gatewayID {
			return match
		}
	}
	t.Fatalf("no result for gateway %q in %+v", gatewayID, matches)
	return ObservationMatch{}
}

func findTranscriptResult(t *testing.T, matches []ObservationMatch, transcriptID string) ObservationMatch {
	t.Helper()
	for _, match := range matches {
		if match.TranscriptObservationID == transcriptID {
			return match
		}
	}
	t.Fatalf("no result for transcript %q in %+v", transcriptID, matches)
	return ObservationMatch{}
}

func TestMatchSameNamespaceIdentityIsHigh(t *testing.T) {
	identity := model.ObservationIdentity{ProviderRequestID: "req-1"}
	gateway := gatewayObservation("gw-1", testTime(0), "gpt-5", matchedUsage(), identity)
	transcript := transcriptObservation("tr-1", testTime(6*time.Hour), "gpt-5", matchedUsage(), identity)

	match := matchOne(t, []model.Observation{gateway, transcript})
	if match.Status != StatusMatched || match.Confidence != ConfidenceHigh {
		t.Fatalf("status/confidence = %q/%q, want matched/high", match.Status, match.Confidence)
	}
	if !reflect.DeepEqual(match.Reasons, []Reason{ReasonSharedIdentity}) {
		t.Fatalf("reasons = %+v, want shared identity", match.Reasons)
	}
	if match.TranscriptObservationID != "tr-1" || match.GatewayObservationID != "gw-1" {
		t.Fatalf("ids = %q/%q, want gw-1/tr-1", match.GatewayObservationID, match.TranscriptObservationID)
	}
	if match.Side != "" {
		t.Fatalf("matched side = %q, want empty", match.Side)
	}
	if match.ID != "match:gw-1=tr-1" {
		t.Fatalf("id = %q, want match:gw-1=tr-1", match.ID)
	}
}

func TestMatchTimeModelUsageUniqueIsMedium(t *testing.T) {
	gateway := gatewayObservation("gw-1", testTime(0), "gpt-5", matchedUsage(), model.ObservationIdentity{})
	transcript := transcriptObservation("tr-1", testTime(30*time.Second), "gpt-5", matchedUsage(), model.ObservationIdentity{})

	match := matchOne(t, []model.Observation{gateway, transcript})
	if match.Status != StatusMatched || match.Confidence != ConfidenceMedium {
		t.Fatalf("status/confidence = %q/%q, want matched/medium", match.Status, match.Confidence)
	}
	if !reflect.DeepEqual(match.Reasons, []Reason{ReasonTime, ReasonModel, ReasonUsage}) {
		t.Fatalf("reasons = %+v, want time/model/usage", match.Reasons)
	}
}

func TestMatchTimeAndExactModelWithoutUsageIsMedium(t *testing.T) {
	gateway := gatewayObservation("gw-1", testTime(0), "gpt-5", matchedUsage(), model.ObservationIdentity{})
	transcript := transcriptObservation("tr-1", testTime(10*time.Second), "gpt-5", differentUsage(), model.ObservationIdentity{})

	match := matchOne(t, []model.Observation{gateway, transcript})
	if match.Status != StatusMatched || match.Confidence != ConfidenceMedium {
		t.Fatalf("status/confidence = %q/%q, want matched/medium", match.Status, match.Confidence)
	}
	if !reflect.DeepEqual(match.Reasons, []Reason{ReasonTime, ReasonModel}) {
		t.Fatalf("reasons = %+v, want time/model", match.Reasons)
	}
}

func TestMatchTimeAndUsageWithMissingModelIsMedium(t *testing.T) {
	gateway := gatewayObservation("gw-1", testTime(0), "", matchedUsage(), model.ObservationIdentity{})
	transcript := transcriptObservation("tr-1", testTime(10*time.Second), "gpt-5", matchedUsage(), model.ObservationIdentity{})

	match := matchOne(t, []model.Observation{gateway, transcript})
	if match.Status != StatusMatched || match.Confidence != ConfidenceMedium {
		t.Fatalf("status/confidence = %q/%q, want matched/medium", match.Status, match.Confidence)
	}
	if !reflect.DeepEqual(match.Reasons, []Reason{ReasonTime, ReasonUsage}) {
		t.Fatalf("reasons = %+v, want time/usage without model", match.Reasons)
	}
}

func TestMatchTimeOnlyWithBothModelsMissingIsUnmatched(t *testing.T) {
	gateway := gatewayObservation("gw-1", testTime(0), "", model.ObservationUsage{}, model.ObservationIdentity{})
	transcript := transcriptObservation("tr-1", testTime(10*time.Second), "", model.ObservationUsage{}, model.ObservationIdentity{})

	matches := matchResults(t, []model.Observation{gateway, transcript})
	if len(matches) != 2 {
		t.Fatalf("matches = %d, want gateway and transcript only", len(matches))
	}
	gatewayResult := findGatewayResult(t, matches, "gw-1")
	if gatewayResult.Status != StatusUnmatched || gatewayResult.Side != SideGateway {
		t.Fatalf("gateway result = %+v, want unmatched gateway", gatewayResult)
	}
	if gatewayResult.ID != "match:gateway:gw-1" {
		t.Fatalf("gateway result id = %q, want match:gateway:gw-1", gatewayResult.ID)
	}
	transcriptResult := findTranscriptResult(t, matches, "tr-1")
	if transcriptResult.Status != StatusUnmatched || transcriptResult.Side != SideTranscript {
		t.Fatalf("transcript result = %+v, want unmatched transcript", transcriptResult)
	}
	if transcriptResult.GatewayObservationID != "" {
		t.Fatalf("transcript-only gateway id = %q, want empty", transcriptResult.GatewayObservationID)
	}
	if transcriptResult.ID != "match:transcript:tr-1" {
		t.Fatalf("transcript result id = %q, want match:transcript:tr-1", transcriptResult.ID)
	}
}

func TestMatchTimeOnlyWithOneModelMissingAndDifferentUsageIsUnmatched(t *testing.T) {
	gateway := gatewayObservation("gw-1", testTime(0), "gpt-5", matchedUsage(), model.ObservationIdentity{})
	transcript := transcriptObservation("tr-1", testTime(10*time.Second), "", differentUsage(), model.ObservationIdentity{})

	matches := matchResults(t, []model.Observation{gateway, transcript})
	if matched := matchedResults(matches); len(matched) != 0 {
		t.Fatalf("matched results = %+v, want none", matched)
	}
}

func TestMatchExactTokensButDistantTimeDoesNotMatch(t *testing.T) {
	gateway := gatewayObservation("gw-1", testTime(0), "gpt-5", matchedUsage(), model.ObservationIdentity{})
	transcript := transcriptObservation("tr-1", testTime(10*time.Minute), "gpt-5", matchedUsage(), model.ObservationIdentity{})

	matches := matchResults(t, []model.Observation{gateway, transcript})
	if matched := matchedResults(matches); len(matched) != 0 {
		t.Fatalf("matched results = %+v, want none", matched)
	}
}

func TestMatchModelMismatchDoesNotMatch(t *testing.T) {
	gateway := gatewayObservation("gw-1", testTime(0), "gpt-5", matchedUsage(), model.ObservationIdentity{})
	transcript := transcriptObservation("tr-1", testTime(10*time.Second), "claude-opus-4", matchedUsage(), model.ObservationIdentity{})

	matches := matchResults(t, []model.Observation{gateway, transcript})
	if matched := matchedResults(matches); len(matched) != 0 {
		t.Fatalf("matched results = %+v, want none", matched)
	}
}

func TestMatchMissingTimestampWithoutIdentityIsUnmatched(t *testing.T) {
	gateway := gatewayObservation("gw-1", nil, "gpt-5", matchedUsage(), model.ObservationIdentity{})
	transcript := transcriptObservation("tr-1", testTime(0), "gpt-5", matchedUsage(), model.ObservationIdentity{})

	matches := matchResults(t, []model.Observation{gateway, transcript})
	if matched := matchedResults(matches); len(matched) != 0 {
		t.Fatalf("matched results = %+v, want none", matched)
	}
}

func TestMatchEquivalentCandidatesAreAmbiguous(t *testing.T) {
	gateway := gatewayObservation("gw-1", testTime(0), "gpt-5", matchedUsage(), model.ObservationIdentity{})
	first := transcriptObservation("tr-1", testTime(10*time.Second), "gpt-5", matchedUsage(), model.ObservationIdentity{})
	second := transcriptObservation("tr-2", testTime(10*time.Second), "gpt-5", matchedUsage(), model.ObservationIdentity{})

	matches := matchResults(t, []model.Observation{gateway, first, second})
	if len(matches) != 1 {
		t.Fatalf("matches = %d, want only the ambiguous gateway: %+v", len(matches), matches)
	}
	match := matches[0]
	if match.Status != StatusAmbiguous || match.Side != SideGateway {
		t.Fatalf("result = %+v, want ambiguous gateway", match)
	}
	if match.TranscriptObservationID != "" {
		t.Fatalf("ambiguous match selected transcript %q", match.TranscriptObservationID)
	}
	if match.ID != "match:gateway:gw-1" {
		t.Fatalf("ambiguous id = %q, want match:gateway:gw-1", match.ID)
	}
	if !reflect.DeepEqual(match.CandidateTranscriptIDs, []string{"tr-1", "tr-2"}) {
		t.Fatalf("candidates = %+v, want [tr-1 tr-2]", match.CandidateTranscriptIDs)
	}
}

func TestMatchIsOneToOne(t *testing.T) {
	gatewayOne := gatewayObservation("gw-1", testTime(0), "gpt-5", matchedUsage(), model.ObservationIdentity{})
	gatewayTwo := gatewayObservation("gw-2", testTime(10*time.Minute), "gpt-5", matchedUsage(), model.ObservationIdentity{})
	transcriptOne := transcriptObservation("tr-1", testTime(20*time.Second), "gpt-5", matchedUsage(), model.ObservationIdentity{})
	transcriptTwo := transcriptObservation("tr-2", testTime(10*time.Minute+20*time.Second), "gpt-5", matchedUsage(), model.ObservationIdentity{})

	matches := matchResults(t, []model.Observation{gatewayTwo, transcriptTwo, gatewayOne, transcriptOne})
	if len(matches) != 2 {
		t.Fatalf("matches = %d, want 2", len(matches))
	}
	pairs := map[string]string{}
	for _, match := range matches {
		pairs[match.GatewayObservationID] = match.TranscriptObservationID
	}
	if pairs["gw-1"] != "tr-1" || pairs["gw-2"] != "tr-2" {
		t.Fatalf("pairs = %+v, want gw-1->tr-1 and gw-2->tr-2", pairs)
	}
}

func TestMatchRequestDoesNotMatchSessionScope(t *testing.T) {
	gateway := gatewayObservation("gw-1", testTime(0), "gpt-5", matchedUsage(), model.ObservationIdentity{})
	session := transcriptObservation("tr-session", testTime(10*time.Second), "gpt-5", matchedUsage(), model.ObservationIdentity{})
	session.Scope = model.ObservationScopeSession

	match := matchOne(t, []model.Observation{gateway, session})
	if match.Status != StatusUnmatched {
		t.Fatalf("status = %q, want unmatched", match.Status)
	}
}

func TestMatchDoesNotUseObservationID(t *testing.T) {
	gateway := gatewayObservation("gw-marker", testTime(0), "gpt-5", matchedUsage(), model.ObservationIdentity{})
	transcript := transcriptObservation("tr-marker", testTime(6*time.Hour), "gpt-5", matchedUsage(), model.ObservationIdentity{})

	matches := matchResults(t, []model.Observation{gateway, transcript})
	if matched := matchedResults(matches); len(matched) != 0 {
		t.Fatalf("matched results = %+v, want none when ids are unrelated", matched)
	}
	gatewayResult := findGatewayResult(t, matches, "gw-marker")
	if gatewayResult.Status != StatusUnmatched {
		t.Fatalf("gateway status = %q, want unmatched", gatewayResult.Status)
	}
}

func TestMatchCrossNamespaceIdentityIsNotIdentity(t *testing.T) {
	gateway := gatewayObservation("gw-1", testTime(0), "gpt-5", matchedUsage(), model.ObservationIdentity{ExchangeID: "shared"})
	transcript := transcriptObservation("tr-1", testTime(6*time.Hour), "gpt-5", matchedUsage(), model.ObservationIdentity{SessionID: "shared"})

	matches := matchResults(t, []model.Observation{gateway, transcript})
	if matched := matchedResults(matches); len(matched) != 0 {
		t.Fatalf("matched results = %+v, want none for cross-namespace identity", matched)
	}
}

func TestMatchIsOrderIndependent(t *testing.T) {
	gatewayOne := gatewayObservation("gw-1", testTime(0), "gpt-5", matchedUsage(), model.ObservationIdentity{})
	gatewayTwo := gatewayObservation("gw-2", testTime(10*time.Minute), "gpt-5", matchedUsage(), model.ObservationIdentity{})
	unmatchedGateway := gatewayObservation("gw-3", testTime(3*time.Hour), "", matchedUsage(), model.ObservationIdentity{})
	transcriptOne := transcriptObservation("tr-1", testTime(20*time.Second), "gpt-5", matchedUsage(), model.ObservationIdentity{})
	transcriptTwo := transcriptObservation("tr-2", testTime(10*time.Minute+20*time.Second), "gpt-5", matchedUsage(), model.ObservationIdentity{})

	ordered := []model.Observation{gatewayOne, gatewayTwo, unmatchedGateway, transcriptOne, transcriptTwo}
	reversed := make([]model.Observation, 0, len(ordered))
	for i := len(ordered) - 1; i >= 0; i-- {
		reversed = append(reversed, ordered[i])
	}
	shuffled := []model.Observation{transcriptTwo, unmatchedGateway, gatewayOne, transcriptOne, gatewayTwo}

	first := matchResults(t, ordered)
	if second := matchResults(t, reversed); !reflect.DeepEqual(first, second) {
		t.Fatalf("reversed input differs:\nfirst  = %+v\nsecond = %+v", first, second)
	}
	if third := matchResults(t, shuffled); !reflect.DeepEqual(first, third) {
		t.Fatalf("shuffled input differs:\nfirst  = %+v\nsecond = %+v", first, third)
	}
}

func TestMatchExplicitZeroParticipatesInSimilarity(t *testing.T) {
	usage := model.ObservationUsage{FreshInput: measured(0)}
	gateway := gatewayObservation("gw-1", testTime(0), "", usage, model.ObservationIdentity{})
	transcript := transcriptObservation("tr-1", testTime(10*time.Second), "", usage, model.ObservationIdentity{})

	match := matchOne(t, []model.Observation{gateway, transcript})
	if match.Status != StatusMatched {
		t.Fatalf("status = %q, want matched", match.Status)
	}
	if !reflect.DeepEqual(match.Reasons, []Reason{ReasonTime, ReasonUsage}) {
		t.Fatalf("reasons = %+v, want explicit zero to add usage", match.Reasons)
	}
}

func TestMatchMissingIsNotEmptyZero(t *testing.T) {
	gatewayUsage := model.ObservationUsage{FreshInput: measured(0)}
	transcriptUsage := model.ObservationUsage{}
	gateway := gatewayObservation("gw-1", testTime(0), "", gatewayUsage, model.ObservationIdentity{})
	transcript := transcriptObservation("tr-1", testTime(10*time.Second), "", transcriptUsage, model.ObservationIdentity{})

	matches := matchResults(t, []model.Observation{gateway, transcript})
	if matched := matchedResults(matches); len(matched) != 0 {
		t.Fatalf("matched results = %+v, missing must not count as zero", matched)
	}
}

func TestMatchTranscriptOnlyIsUnmatched(t *testing.T) {
	transcript := transcriptObservation("tr-1", testTime(0), "gpt-5", matchedUsage(), model.ObservationIdentity{})

	matches := matchResults(t, []model.Observation{transcript})
	if len(matches) != 1 {
		t.Fatalf("matches = %d, want 1: %+v", len(matches), matches)
	}
	result := matches[0]
	if result.Status != StatusUnmatched || result.Side != SideTranscript {
		t.Fatalf("result = %+v, want unmatched transcript", result)
	}
	if result.GatewayObservationID != "" || result.TranscriptObservationID != "tr-1" {
		t.Fatalf("ids = %q/%q, want empty/tr-1", result.GatewayObservationID, result.TranscriptObservationID)
	}
	if result.ID != "match:transcript:tr-1" {
		t.Fatalf("id = %q, want match:transcript:tr-1", result.ID)
	}
	if len(result.CandidateTranscriptIDs) != 0 {
		t.Fatalf("transcript-only candidates = %+v, want empty", result.CandidateTranscriptIDs)
	}
}

func TestMatchOneGatewayTwoTranscripts(t *testing.T) {
	gateway := gatewayObservation("gw-1", testTime(0), "gpt-5", matchedUsage(), model.ObservationIdentity{})
	chosen := transcriptObservation("tr-1", testTime(10*time.Second), "gpt-5", matchedUsage(), model.ObservationIdentity{})
	leftover := transcriptObservation("tr-2", testTime(10*time.Second), "gpt-5", differentUsage(), model.ObservationIdentity{})

	matches := matchResults(t, []model.Observation{gateway, chosen, leftover})
	if len(matches) != 2 {
		t.Fatalf("matches = %d, want matched plus leftover: %+v", len(matches), matches)
	}
	matched := matchedResults(matches)
	if len(matched) != 1 || matched[0].GatewayObservationID != "gw-1" || matched[0].TranscriptObservationID != "tr-1" {
		t.Fatalf("matched = %+v, want gw-1=tr-1", matched)
	}
	leftoverResult := findTranscriptResult(t, matches, "tr-2")
	if leftoverResult.Status != StatusUnmatched || leftoverResult.Side != SideTranscript {
		t.Fatalf("leftover = %+v, want unmatched transcript", leftoverResult)
	}
}

func TestMatchMatchedTranscriptIsNotAlsoUnmatched(t *testing.T) {
	gateway := gatewayObservation("gw-1", testTime(0), "gpt-5", matchedUsage(), model.ObservationIdentity{})
	transcript := transcriptObservation("tr-1", testTime(10*time.Second), "gpt-5", matchedUsage(), model.ObservationIdentity{})

	matches := matchResults(t, []model.Observation{gateway, transcript})
	if len(matches) != 1 {
		t.Fatalf("matches = %d, want only the matched pair: %+v", len(matches), matches)
	}
	if matches[0].Status != StatusMatched {
		t.Fatalf("status = %q, want matched", matches[0].Status)
	}
	if len(matches[0].CandidateTranscriptIDs) != 0 {
		t.Fatalf("matched result candidates = %+v, want empty", matches[0].CandidateTranscriptIDs)
	}
}

func TestMatchEveryObservationAccountedOnce(t *testing.T) {
	gatewayMatched := gatewayObservation("gw-match", testTime(0), "gpt-5", matchedUsage(), model.ObservationIdentity{})
	transcriptMatched := transcriptObservation("tr-match", testTime(5*time.Second), "gpt-5", matchedUsage(), model.ObservationIdentity{})
	gatewayLonely := gatewayObservation("gw-lonely", testTime(3*time.Hour), "gpt-5", matchedUsage(), model.ObservationIdentity{})
	transcriptLonely := transcriptObservation("tr-lonely", testTime(5*time.Hour), "gpt-5", matchedUsage(), model.ObservationIdentity{})

	matches := matchResults(t, []model.Observation{gatewayMatched, transcriptMatched, gatewayLonely, transcriptLonely})
	if len(matches) != 3 {
		t.Fatalf("matches = %d, want 3: %+v", len(matches), matches)
	}
	counts := map[string]int{}
	for _, match := range matches {
		if match.GatewayObservationID != "" {
			counts[match.GatewayObservationID]++
		}
		if match.TranscriptObservationID != "" {
			counts[match.TranscriptObservationID]++
		}
	}
	for _, id := range []string{"gw-match", "tr-match", "gw-lonely", "tr-lonely"} {
		if counts[id] != 1 {
			t.Fatalf("observation %q accounted %d times, want exactly once", id, counts[id])
		}
	}
	gatewayResult := findGatewayResult(t, matches, "gw-lonely")
	if gatewayResult.Status != StatusUnmatched || gatewayResult.Side != SideGateway {
		t.Fatalf("lonely gateway = %+v, want unmatched gateway", gatewayResult)
	}
	if len(gatewayResult.CandidateTranscriptIDs) != 0 {
		t.Fatalf("unmatched gateway candidates = %+v, want empty", gatewayResult.CandidateTranscriptIDs)
	}
	transcriptResult := findTranscriptResult(t, matches, "tr-lonely")
	if transcriptResult.Status != StatusUnmatched || transcriptResult.Side != SideTranscript {
		t.Fatalf("lonely transcript = %+v, want unmatched transcript", transcriptResult)
	}
	if len(transcriptResult.CandidateTranscriptIDs) != 0 {
		t.Fatalf("unmatched transcript candidates = %+v, want empty", transcriptResult.CandidateTranscriptIDs)
	}
}

func TestMatchAmbiguousGatewayTranscriptAccounting(t *testing.T) {
	gateway := gatewayObservation("gw-1", testTime(0), "gpt-5", matchedUsage(), model.ObservationIdentity{})
	first := transcriptObservation("tr-1", testTime(10*time.Second), "gpt-5", matchedUsage(), model.ObservationIdentity{})
	second := transcriptObservation("tr-2", testTime(10*time.Second), "gpt-5", matchedUsage(), model.ObservationIdentity{})

	matches := matchResults(t, []model.Observation{gateway, first, second})
	if len(matches) != 1 {
		t.Fatalf("matches = %d, want only the ambiguous gateway: %+v", len(matches), matches)
	}
	result := matches[0]
	if result.Status != StatusAmbiguous || result.Side != SideGateway {
		t.Fatalf("result = %+v, want ambiguous gateway", result)
	}
	if result.GatewayObservationID != "gw-1" || result.TranscriptObservationID != "" {
		t.Fatalf("ids = %q/%q, want gw-1/empty", result.GatewayObservationID, result.TranscriptObservationID)
	}
	if result.ID != "match:gateway:gw-1" {
		t.Fatalf("id = %q, want match:gateway:gw-1", result.ID)
	}
	if !reflect.DeepEqual(result.CandidateTranscriptIDs, []string{"tr-1", "tr-2"}) {
		t.Fatalf("candidates = %+v, want sorted [tr-1 tr-2]", result.CandidateTranscriptIDs)
	}
	for _, match := range matches {
		if match.TranscriptObservationID != "" {
			t.Fatalf("transcript %q was emitted as a result despite ambiguity", match.TranscriptObservationID)
		}
	}
}

func TestMatchThreeEquivalentCandidates(t *testing.T) {
	gateway := gatewayObservation("gw-1", testTime(0), "gpt-5", matchedUsage(), model.ObservationIdentity{})
	first := transcriptObservation("tr-1", testTime(10*time.Second), "gpt-5", matchedUsage(), model.ObservationIdentity{})
	second := transcriptObservation("tr-2", testTime(10*time.Second), "gpt-5", matchedUsage(), model.ObservationIdentity{})
	third := transcriptObservation("tr-3", testTime(10*time.Second), "gpt-5", matchedUsage(), model.ObservationIdentity{})

	matches := matchResults(t, []model.Observation{gateway, first, second, third})
	if len(matches) != 1 {
		t.Fatalf("matches = %d, want only the ambiguous gateway: %+v", len(matches), matches)
	}
	if !reflect.DeepEqual(matches[0].CandidateTranscriptIDs, []string{"tr-1", "tr-2", "tr-3"}) {
		t.Fatalf("candidates = %+v, want three sorted ids", matches[0].CandidateTranscriptIDs)
	}
}

func TestMatchAmbiguousCandidatesInputOrderIndependent(t *testing.T) {
	gateway := gatewayObservation("gw-1", testTime(0), "gpt-5", matchedUsage(), model.ObservationIdentity{})
	first := transcriptObservation("tr-1", testTime(10*time.Second), "gpt-5", matchedUsage(), model.ObservationIdentity{})
	second := transcriptObservation("tr-2", testTime(10*time.Second), "gpt-5", matchedUsage(), model.ObservationIdentity{})
	third := transcriptObservation("tr-3", testTime(10*time.Second), "gpt-5", matchedUsage(), model.ObservationIdentity{})

	ordered := []model.Observation{gateway, first, second, third}
	reversed := []model.Observation{third, second, first, gateway}
	shuffled := []model.Observation{second, gateway, third, first}

	want := []string{"tr-1", "tr-2", "tr-3"}
	baseline := matchResults(t, ordered)
	if !reflect.DeepEqual(baseline[0].CandidateTranscriptIDs, want) {
		t.Fatalf("ordered candidates = %+v, want %+v", baseline[0].CandidateTranscriptIDs, want)
	}
	if reversedRun := matchResults(t, reversed); !reflect.DeepEqual(baseline, reversedRun) {
		t.Fatalf("reversed input differs:\nfirst  = %+v\nsecond = %+v", baseline, reversedRun)
	}
	if shuffledRun := matchResults(t, shuffled); !reflect.DeepEqual(baseline, shuffledRun) {
		t.Fatalf("shuffled input differs:\nfirst  = %+v\nsecond = %+v", baseline, shuffledRun)
	}
}

func TestMatchTranscriptSideAmbiguityKeepsCandidateTranscript(t *testing.T) {
	gatewayOne := gatewayObservation("gw-1", testTime(0), "gpt-5", matchedUsage(), model.ObservationIdentity{})
	gatewayTwo := gatewayObservation("gw-2", testTime(0), "gpt-5", matchedUsage(), model.ObservationIdentity{})
	transcript := transcriptObservation("tr-1", testTime(10*time.Second), "gpt-5", matchedUsage(), model.ObservationIdentity{})

	matches := matchResults(t, []model.Observation{gatewayOne, gatewayTwo, transcript})
	if len(matches) != 2 {
		t.Fatalf("matches = %d, want both ambiguous gateways: %+v", len(matches), matches)
	}
	for _, match := range matches {
		if match.Status != StatusAmbiguous || match.Side != SideGateway {
			t.Fatalf("result = %+v, want ambiguous gateway", match)
		}
		if match.TranscriptObservationID != "" {
			t.Fatalf("gateway %q selected transcript %q", match.GatewayObservationID, match.TranscriptObservationID)
		}
		if !reflect.DeepEqual(match.CandidateTranscriptIDs, []string{"tr-1"}) {
			t.Fatalf("candidates = %+v, want [tr-1]", match.CandidateTranscriptIDs)
		}
	}
}

func TestMatchCandidateTranscriptIDsAreIsolated(t *testing.T) {
	gateway := gatewayObservation("gw-1", testTime(0), "gpt-5", matchedUsage(), model.ObservationIdentity{})
	first := transcriptObservation("tr-1", testTime(10*time.Second), "gpt-5", matchedUsage(), model.ObservationIdentity{})
	second := transcriptObservation("tr-2", testTime(10*time.Second), "gpt-5", matchedUsage(), model.ObservationIdentity{})
	observations := []model.Observation{gateway, first, second}

	matches := matchResults(t, observations)
	if len(matches[0].CandidateTranscriptIDs) == 0 {
		t.Fatal("expected candidate transcript ids")
	}
	matches[0].CandidateTranscriptIDs[0] = "mutated"

	again := matchResults(t, observations)
	if !reflect.DeepEqual(again[0].CandidateTranscriptIDs, []string{"tr-1", "tr-2"}) {
		t.Fatalf("candidates = %+v, want unaffected [tr-1 tr-2]", again[0].CandidateTranscriptIDs)
	}
}

func TestMatchRejectsInvalidTimeWindow(t *testing.T) {
	_, err := Match(nil, Options{})
	if err == nil || !strings.Contains(err.Error(), "invalid time window") {
		t.Fatalf("error = %v, want invalid time window", err)
	}
}

func TestMatchRejectsDuplicateObservationID(t *testing.T) {
	gateway := gatewayObservation("dup", testTime(0), "gpt-5", matchedUsage(), model.ObservationIdentity{})
	transcript := transcriptObservation("dup", testTime(10*time.Second), "gpt-5", matchedUsage(), model.ObservationIdentity{})

	_, err := Match([]model.Observation{gateway, transcript}, options())
	if err == nil || !strings.Contains(err.Error(), "duplicate observation id \"dup\"") {
		t.Fatalf("error = %v, want duplicate observation id", err)
	}
}

func TestMatchRejectsInvalidObservation(t *testing.T) {
	invalid := gatewayObservation("gw-1", testTime(0), "gpt-5", matchedUsage(), model.ObservationIdentity{})
	invalid.Source = ""

	_, err := Match([]model.Observation{invalid}, options())
	if err == nil || !strings.Contains(err.Error(), "gw-1") {
		t.Fatalf("error = %v, want observation id in error", err)
	}
}
