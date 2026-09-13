package gateway

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAccountProfile_BucketInvariantAcrossKinds(t *testing.T) {
	in := int64(10)
	out := int64(2)
	cases := []struct {
		name             string
		exchanges        []Exchange
		wantModel        int
		wantNonModel     int
		wantUnknown      int
		wantUnclassified int
	}{
		{
			name: "known_model_endpoint",
			exchanges: []Exchange{mkBucketEx("gw-1", ProtocolAnthropicMessages,
				http.MethodPost, "/v1/messages", RequestKindModel,
				&ProviderUsage{Source: "anthropic_messages", InputTokens: &in, OutputTokens: &out})},
			wantModel: 1,
		},
		{
			name: "known_non_model_endpoint",
			exchanges: []Exchange{mkBucketEx("gw-2", ProtocolAnthropicMessages,
				http.MethodGet, "/v1/messages/count_tokens", RequestKindNonModel, nil)},
			wantNonModel: 1,
		},
		{
			name: "unknown_endpoint_post",
			exchanges: []Exchange{mkBucketEx("gw-3", ProtocolAnthropicMessages,
				http.MethodPost, "/v1/unknown", RequestKindUnknown, nil)},
			wantUnknown: 1,
		},
		{
			name: "unknown_endpoint_get",
			exchanges: []Exchange{mkBucketEx("gw-4", ProtocolAnthropicMessages,
				http.MethodGet, "/v1/unknown", RequestKindUnknown, nil)},
			wantUnknown: 1,
		},
		{
			name: "unsupported_protocol",
			exchanges: []Exchange{mkBucketEx("gw-5", Protocol("bogus"),
				http.MethodPost, "/anything", RequestKindUnclassified, nil)},
			wantUnclassified: 1,
		},
		{
			name: "mixed",
			exchanges: []Exchange{
				mkBucketEx("gw-m1", ProtocolAnthropicMessages, http.MethodPost, "/v1/messages", RequestKindModel,
					&ProviderUsage{Source: "anthropic_messages", InputTokens: &in, OutputTokens: &out}),
				mkBucketEx("gw-m2", ProtocolAnthropicMessages, http.MethodPost, "/v1/messages", RequestKindModel, nil),
				mkBucketEx("gw-n1", ProtocolAnthropicMessages, http.MethodGet, "/v1/messages/count_tokens", RequestKindNonModel, nil),
				mkBucketEx("gw-u1", ProtocolAnthropicMessages, http.MethodPost, "/v1/unknown", RequestKindUnknown, nil),
				mkBucketEx("gw-c1", ProtocolOpenAIResponses, http.MethodPost, "/anything", RequestKindUnclassified, nil),
			},
			wantModel: 2, wantNonModel: 1, wantUnknown: 1, wantUnclassified: 1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			acc := AccountProfile("p", tc.exchanges, ChainBuildResult{}, RecorderFailureRead{})
			c := acc.Counts
			sum := c.ModelRequests + c.NonModelRequests + c.UnknownRequests + c.UnclassifiedRequests
			if c.HTTPRequests != sum {
				t.Fatalf("invariant broken: HTTP=%d but sum=%d (M=%d NM=%d U=%d UC=%d)",
					c.HTTPRequests, sum,
					c.ModelRequests, c.NonModelRequests, c.UnknownRequests, c.UnclassifiedRequests)
			}
			if c.ModelRequests != tc.wantModel {
				t.Fatalf("model=%d want %d", c.ModelRequests, tc.wantModel)
			}
			if c.NonModelRequests != tc.wantNonModel {
				t.Fatalf("nonmodel=%d want %d", c.NonModelRequests, tc.wantNonModel)
			}
			if c.UnknownRequests != tc.wantUnknown {
				t.Fatalf("unknown=%d want %d", c.UnknownRequests, tc.wantUnknown)
			}
			if c.UnclassifiedRequests != tc.wantUnclassified {
				t.Fatalf("unclassified=%d want %d", c.UnclassifiedRequests, tc.wantUnclassified)
			}
		})
	}
}

func TestAccountProfile_SchemaVersionIsOne(t *testing.T) {
	acc := AccountProfile("p", nil, ChainBuildResult{}, RecorderFailureRead{})
	raw, err := json.Marshal(acc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"schemaVersion":1`) {
		t.Fatalf("missing schemaVersion:1 in %s", raw)
	}
}

func TestAccountProfile_UnknownTrafficDoesNotAffectCompleteness(t *testing.T) {
	in, out := int64(10), int64(2)
	exs := []Exchange{
		mkBucketEx("gw-m", ProtocolAnthropicMessages, http.MethodPost, "/v1/messages",
			RequestKindModel, &ProviderUsage{Source: "anthropic_messages", InputTokens: &in, OutputTokens: &out}),
		mkBucketEx("gw-u", ProtocolAnthropicMessages, http.MethodPost, "/v1/unknown",
			RequestKindUnknown, nil),
	}
	acc := AccountProfile("p", exs, ChainBuildResult{}, RecorderFailureRead{})
	if !acc.Observed.Complete {
		t.Fatalf("unknown traffic leaked into completeness: %+v", acc.Observed)
	}
	if acc.Counts.UnknownRequests != 1 {
		t.Fatalf("UnknownRequests = %d, want 1", acc.Counts.UnknownRequests)
	}
}

func mkBucketEx(id string, p Protocol, method, endpoint string, kind RequestKind, pu *ProviderUsage) Exchange {
	return Exchange{
		ID:        id,
		Profile:   "p",
		Protocol:  p,
		Kind:      kind,
		Outcome:   OutcomeUpstreamOK,
		StartedAt: time.Unix(0, 0),
		Request: ExchangeRequest{
			Method:   method,
			Endpoint: endpoint,
		},
		Response: ExchangeResponse{ProviderUsage: pu},
	}
}
