package gateway

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/thanhpham0406/tok-doctor/internal/model"
	"github.com/thanhpham0406/tok-doctor/internal/source"
)

func TestAnthropicCorrelator_ToolUseResultContinues(t *testing.T) {
	prev := anthropicExchange("gw-a", "p", "m", 0, anthropicBody("m", []string{"toolu_a"}, nil, true))
	cur := anthropicExchange("gw-b", "p", "m", 1, anthropicBody("m", nil, []string{"toolu_a"}, false))
	got := AnthropicMessagesCorrelator{}.Correlate(prev, cur)
	if got.Relation != ChainRelationContinue || got.Method != CorrelationToolLineage {
		t.Fatalf("correlation = %+v, want tool continuation", got)
	}
	if len(got.Evidence) != 1 || got.Evidence[0].Value != "toolu_a" {
		t.Fatalf("evidence = %+v", got.Evidence)
	}
}

func TestAnthropicCorrelator_NonMatchingToolIDsUnknown(t *testing.T) {
	prev := anthropicExchange("gw-a", "p", "m", 0, anthropicBody("m", []string{"toolu_a"}, nil, true))
	cur := anthropicExchange("gw-b", "p", "m", 1, anthropicBody("m", nil, []string{"toolu_b"}, false))
	got := AnthropicMessagesCorrelator{}.Correlate(prev, cur)
	if got.Relation != ChainRelationUnknown {
		t.Fatalf("correlation = %+v, want unknown", got)
	}
}

func TestAnthropicCorrelator_MultipleMatchingToolIDs(t *testing.T) {
	prev := anthropicExchange("gw-a", "p", "m", 0, anthropicBody("m", []string{"toolu_b", "toolu_a"}, nil, true))
	cur := anthropicExchange("gw-b", "p", "m", 1, anthropicBody("m", nil, []string{"toolu_a", "toolu_b"}, false))
	got := AnthropicMessagesCorrelator{}.Correlate(prev, cur)
	if got.Relation != ChainRelationContinue || len(got.Evidence) != 2 {
		t.Fatalf("correlation = %+v, want two matches", got)
	}
	if got.Evidence[0].Value != "toolu_a" || got.Evidence[1].Value != "toolu_b" {
		t.Fatalf("evidence order = %+v", got.Evidence)
	}
}

func TestChainBuilder_ToolResultContinuesSameChain(t *testing.T) {
	exchanges := []Exchange{
		anthropicExchange("gw-a", "p", "m", 0, anthropicBody("m", []string{"toolu_a"}, nil, true)),
		anthropicExchange("gw-b", "p", "m", 1, anthropicBody("m", []string{"toolu_b"}, []string{"toolu_a"}, false)),
		anthropicExchange("gw-c", "p", "m", 2, anthropicBody("m", nil, []string{"toolu_b"}, false)),
	}
	got := NewChainBuilder().Build(exchanges)
	if len(got.Chains) != 1 {
		t.Fatalf("chains = %d, want 1", len(got.Chains))
	}
	if ids := chainIDs(got.Chains[0]); strings.Join(ids, ",") != "gw-a,gw-b,gw-c" {
		t.Fatalf("chain ids = %v", ids)
	}
}

func TestChainBuilder_UnrelatedStartCreatesNewChain(t *testing.T) {
	exchanges := []Exchange{
		anthropicExchange("gw-a", "p", "m", 0, anthropicBody("m", nil, nil, true)),
		anthropicExchange("gw-b", "p", "m", 1, anthropicBody("m", nil, nil, true)),
	}
	got := NewChainBuilder().Build(exchanges)
	if len(got.Chains) != 2 {
		t.Fatalf("chains = %d, want 2", len(got.Chains))
	}
}

func TestAnthropicCorrelator_StaticModelProfileToolsDoNotContinue(t *testing.T) {
	prev := anthropicExchange("gw-a", "p", "m", 0, anthropicStaticBody("m", "same", "tool"))
	cur := anthropicExchange("gw-b", "p", "m", 1, anthropicStaticBody("m", "same", "tool"))
	got := AnthropicMessagesCorrelator{}.Correlate(prev, cur)
	if got.Relation != ChainRelationUnknown {
		t.Fatalf("correlation = %+v, want unknown", got)
	}
}

func TestAnthropicCorrelator_StaticInstructionsToolsDoNotContinue(t *testing.T) {
	prev := anthropicExchange("gw-a", "p", "m1", 0, anthropicStaticBody("m1", "same", "tool"))
	cur := anthropicExchange("gw-b", "p", "m2", 1, anthropicStaticBody("m2", "same", "tool"))
	got := AnthropicMessagesCorrelator{}.Correlate(prev, cur)
	if got.Relation != ChainRelationUnknown {
		t.Fatalf("correlation = %+v, want unknown", got)
	}
}

func TestAnthropicCorrelator_ContextLineageMayContinue(t *testing.T) {
	prev := bareAnthropicExchange("gw-a", "p", "m", 0, []model.ContextComponent{
		hashComponent(model.ContextHistory, "assistant previous"),
	})
	cur := bareAnthropicExchange("gw-b", "p", "m", 1, []model.ContextComponent{
		hashComponent(model.ContextHistory, "assistant previous"),
		hashComponent(model.ContextHistory, "assistant next"),
	})
	got := AnthropicMessagesCorrelator{}.Correlate(prev, cur)
	if got.Relation != ChainRelationContinue || got.Method != CorrelationContextLineage {
		t.Fatalf("correlation = %+v, want context lineage", got)
	}
}

func TestChainBuilder_AmbiguousContextLineageUncorrelated(t *testing.T) {
	prevA := bareAnthropicExchange("gw-a", "p", "m", 0, []model.ContextComponent{hashComponent(model.ContextHistory, "same")})
	prevB := bareAnthropicExchange("gw-b", "p", "m", 1, []model.ContextComponent{hashComponent(model.ContextHistory, "same")})
	cur := bareAnthropicExchange("gw-c", "p", "m", 2, []model.ContextComponent{
		hashComponent(model.ContextHistory, "same"),
		hashComponent(model.ContextHistory, "new"),
	})
	prevA.Request.Metadata = AnthropicMessagesObserver{}.ParseMetadata(anthropicBody("m", nil, nil, true))
	prevB.Request.Metadata = AnthropicMessagesObserver{}.ParseMetadata(anthropicBody("m", nil, nil, true))
	cur.Request.Metadata = AnthropicMessagesObserver{}.ParseMetadata(anthropicBody("m", nil, nil, false))
	got := NewChainBuilder().Build([]Exchange{prevA, prevB, cur})
	if len(got.Uncorrelated) != 1 || got.Uncorrelated[0].ID != "gw-c" {
		t.Fatalf("uncorrelated = %+v, want gw-c", got.Uncorrelated)
	}
}

func TestAnthropicCorrelator_CloseTimestampsDoNotCorrelate(t *testing.T) {
	prev := anthropicExchange("gw-a", "p", "m", 0, anthropicBody("m", nil, nil, true))
	cur := anthropicExchange("gw-b", "p", "m", 1, anthropicBody("m", nil, nil, true))
	got := AnthropicMessagesCorrelator{}.Correlate(prev, cur)
	if got.Relation != ChainRelationUnknown {
		t.Fatalf("correlation = %+v, want unknown", got)
	}
}

func TestAnthropicCorrelator_FarTimestampsWithToolLineageContinue(t *testing.T) {
	prev := anthropicExchange("gw-a", "p", "m", 0, anthropicBody("m", []string{"toolu_a"}, nil, true))
	cur := anthropicExchange("gw-b", "p", "m", 86400, anthropicBody("m", nil, []string{"toolu_a"}, false))
	got := AnthropicMessagesCorrelator{}.Correlate(prev, cur)
	if got.Relation != ChainRelationContinue {
		t.Fatalf("correlation = %+v, want continue", got)
	}
}

func TestChainBuilder_CountTokensIgnored(t *testing.T) {
	ex := anthropicExchange("gw-a", "p", "m", 0, anthropicBody("m", nil, nil, true))
	ex.Request.Endpoint = "/v1/messages/count_tokens"
	got := NewChainBuilder().Build([]Exchange{ex})
	if len(got.Chains) != 0 || len(got.Uncorrelated) != 0 {
		t.Fatalf("result = %+v, want ignored", got)
	}
}

func TestChainBuilder_SixAnthropicRequestsOneChain(t *testing.T) {
	var exchanges []Exchange
	for i := 0; i < 6; i++ {
		var uses []string
		var results []string
		if i < 5 {
			uses = []string{"toolu_" + strconv.Itoa(i)}
		}
		if i > 0 {
			results = []string{"toolu_" + strconv.Itoa(i-1)}
		}
		exchanges = append(exchanges, anthropicExchange("gw-"+strconv.Itoa(i), "p", "m", int64(i), anthropicBody("m", uses, results, i == 0)))
	}
	got := NewChainBuilder().Build(exchanges)
	if len(got.Chains) != 1 || len(got.Chains[0].Exchanges) != 6 {
		t.Fatalf("result = %+v, want one six-call chain", got)
	}
}

func TestChainBuilder_TwoIndependentActionsTwoChains(t *testing.T) {
	exchanges := []Exchange{
		anthropicExchange("gw-a1", "p", "m", 0, anthropicBody("m", []string{"toolu_a"}, nil, true)),
		anthropicExchange("gw-a2", "p", "m", 1, anthropicBody("m", nil, []string{"toolu_a"}, false)),
		anthropicExchange("gw-b1", "p", "m", 2, anthropicBody("m", []string{"toolu_b"}, nil, true)),
		anthropicExchange("gw-b2", "p", "m", 3, anthropicBody("m", nil, []string{"toolu_b"}, false)),
	}
	got := NewChainBuilder().Build(exchanges)
	if len(got.Chains) != 2 {
		t.Fatalf("chains = %d, want 2", len(got.Chains))
	}
}

func TestChainBuilder_UncorrelatedNotForced(t *testing.T) {
	exchanges := []Exchange{
		anthropicExchange("gw-a", "p", "m", 0, anthropicBody("m", []string{"toolu_a"}, nil, true)),
		anthropicExchange("gw-b", "p", "m", 1, anthropicBody("m", nil, []string{"toolu_missing"}, false)),
		anthropicExchange("gw-c", "p", "m", 2, anthropicBody("m", nil, []string{"toolu_a"}, false)),
	}
	got := NewChainBuilder().Build(exchanges)
	if len(got.Uncorrelated) != 1 || got.Uncorrelated[0].ID != "gw-b" {
		t.Fatalf("uncorrelated = %+v, want gw-b", got.Uncorrelated)
	}
	if len(got.Chains) != 1 || len(got.Chains[0].Exchanges) != 2 {
		t.Fatalf("chains = %+v, want gw-a and gw-c", got.Chains)
	}
}

func TestChainBuilder_OrderDeterministic(t *testing.T) {
	exchanges := []Exchange{
		anthropicExchange("gw-c", "p", "m", 2, anthropicBody("m", nil, []string{"toolu_b"}, false)),
		anthropicExchange("gw-a", "p", "m", 0, anthropicBody("m", []string{"toolu_a"}, nil, true)),
		anthropicExchange("gw-b", "p", "m", 1, anthropicBody("m", []string{"toolu_b"}, []string{"toolu_a"}, false)),
	}
	got := NewChainBuilder().Build(exchanges)
	if ids := chainIDs(got.Chains[0]); strings.Join(ids, ",") != "gw-a,gw-b,gw-c" {
		t.Fatalf("order = %v", ids)
	}
}

func TestAnthropicMetadataStoresToolIDsWithoutRawContent(t *testing.T) {
	secret := "SECRET_TOOL_ARGUMENT"
	body := []byte(`{"model":"m","messages":[{"role":"assistant","content":[{"type":"tool_use","id":"toolu_a","name":"x","input":{"q":"` + secret + `"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_a","content":"SECRET_RESULT"}]}]}`)
	meta := AnthropicMessagesObserver{}.ParseMetadata(body)
	raw, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), "toolu_a") {
		t.Fatalf("metadata missing tool id: %s", raw)
	}
	if strings.Contains(string(raw), secret) || strings.Contains(string(raw), "SECRET_RESULT") {
		t.Fatalf("metadata leaked raw content: %s", raw)
	}
}

func TestChainCorrelationDoesNotRequireRawPromptText(t *testing.T) {
	prev := anthropicExchange("gw-a", "p", "m", 0, anthropicBody("m", []string{"toolu_a"}, nil, true))
	cur := anthropicExchange("gw-b", "p", "m", 1, anthropicBody("m", nil, []string{"toolu_a"}, false))
	prev.Request.Components = nil
	cur.Request.Components = nil
	got := AnthropicMessagesCorrelator{}.Correlate(prev, cur)
	if got.Relation != ChainRelationContinue {
		t.Fatalf("correlation = %+v, want continue", got)
	}
}

func TestChainResultDoesNotExposeRawBodiesOrSecrets(t *testing.T) {
	exchanges := []Exchange{
		anthropicExchange("gw-a", "p", "m", 0, []byte(`{"model":"m","messages":[{"role":"user","content":"SECRET_PROMPT"},{"role":"assistant","content":[{"type":"tool_use","id":"toolu_a","input":{"q":"SECRET_ARG"}}]}]}`)),
		anthropicExchange("gw-b", "p", "m", 1, []byte(`{"model":"m","messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_a","content":"SECRET_RESULT"}]}]}`)),
	}
	got := NewChainBuilder().Build(exchanges)
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, secret := range []string{"SECRET_PROMPT", "SECRET_ARG", "SECRET_RESULT"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("chain result leaked %q: %s", secret, raw)
		}
	}
}

func TestChainBuilder_OpenAIResponsesUnchanged(t *testing.T) {
	ex := Exchange{
		ID:        "gw-openai",
		Profile:   "p",
		Protocol:  ProtocolOpenAIResponses,
		StartedAt: time.Unix(0, 0),
		Request: ExchangeRequest{
			Method:   "POST",
			Endpoint: "/v1/responses",
		},
	}
	got := NewChainBuilder().Build([]Exchange{ex})
	if len(got.Uncorrelated) != 1 {
		t.Fatalf("result = %+v, want uncorrelated", got)
	}
}

func anthropicExchange(id, profile, modelName string, seconds int64, body []byte) Exchange {
	components := AnthropicMessagesObserver{}.Parse(body)
	return Exchange{
		ID:        id,
		Profile:   profile,
		Protocol:  ProtocolAnthropicMessages,
		StartedAt: time.Unix(seconds, 0),
		Model:     modelName,
		Request: ExchangeRequest{
			Method:     "POST",
			Endpoint:   "/v1/messages",
			BodyHash:   contentHash(body),
			Components: components,
			Metadata:   AnthropicMessagesObserver{}.ParseMetadata(body),
		},
	}
}

func bareAnthropicExchange(id, profile, modelName string, seconds int64, components []model.ContextComponent) Exchange {
	return Exchange{
		ID:        id,
		Profile:   profile,
		Protocol:  ProtocolAnthropicMessages,
		StartedAt: time.Unix(seconds, 0),
		Model:     modelName,
		Request: ExchangeRequest{
			Method:     "POST",
			Endpoint:   "/v1/messages",
			Components: components,
		},
	}
}

func anthropicBody(modelName string, toolUses, toolResults []string, fresh bool) []byte {
	var messages []map[string]any
	if fresh {
		messages = append(messages, map[string]any{"role": "user", "content": []map[string]any{{"type": "text", "text": "fresh"}}})
	}
	if len(toolUses) > 0 {
		var blocks []map[string]any
		for _, id := range toolUses {
			blocks = append(blocks, map[string]any{"type": "tool_use", "id": id, "name": "read", "input": map[string]any{"path": "x"}})
		}
		messages = append(messages, map[string]any{"role": "assistant", "content": blocks})
	}
	if len(toolResults) > 0 {
		var blocks []map[string]any
		for _, id := range toolResults {
			blocks = append(blocks, map[string]any{"type": "tool_result", "tool_use_id": id, "content": "ok"})
		}
		messages = append(messages, map[string]any{"role": "user", "content": blocks})
	}
	raw, _ := json.Marshal(map[string]any{
		"model":    modelName,
		"messages": messages,
	})
	return raw
}

func anthropicStaticBody(modelName, system, toolName string) []byte {
	raw, _ := json.Marshal(map[string]any{
		"model":  modelName,
		"system": system,
		"tools":  []map[string]any{{"name": toolName}},
		"messages": []map[string]any{
			{"role": "user", "content": []map[string]any{{"type": "text", "text": "fresh"}}},
		},
	})
	return raw
}

func hashComponent(kind model.ContextComponentKind, text string) model.ContextComponent {
	return model.ContextComponent{
		Kind:        kind,
		ContentHash: source.ContentHash(text),
		Observation: ObservationScope,
		Measurement: model.NewMeasurement(int64(len(text)), model.MeasurementEstimated),
	}
}

func chainIDs(chain Chain) []string {
	ids := make([]string, 0, len(chain.Exchanges))
	for _, exchange := range chain.Exchanges {
		ids = append(ids, exchange.ID)
	}
	return ids
}
