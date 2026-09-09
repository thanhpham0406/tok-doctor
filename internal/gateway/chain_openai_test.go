package gateway

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/thanhpham0406/tok-doctor/internal/model"
	"github.com/thanhpham0406/tok-doctor/internal/source"
)

func TestOpenAICorrelatorRegisteredForProtocol(t *testing.T) {
	builder := NewChainBuilder()
	if _, ok := builder.correlators[ProtocolOpenAIResponses]; !ok {
		t.Fatalf("OpenAI correlator not registered")
	}
	if _, ok := builder.correlators[ProtocolAnthropicMessages]; !ok {
		t.Fatalf("Anthropic correlator not registered")
	}
}

func TestSourceDoesNotDetermineCorrelator(t *testing.T) {
	ex := anthropicExchange("gw-1", "codex", "m", 0, anthropicBody("m", nil, nil, true))
	got := NewChainBuilder().Build([]Exchange{ex})
	if len(got.Chains) != 1 {
		t.Fatalf("expected anthropic chain from codex source, got %+v", got)
	}
	if got.Chains[0].Protocol != ProtocolAnthropicMessages {
		t.Fatalf("protocol = %q, want anthropic_messages", got.Chains[0].Protocol)
	}

	ex.Protocol = ProtocolOpenAIResponses
	ex.Request.Endpoint = "/responses"
	ex.ID = "gw-2"
	ex.StartedAt = time.Unix(1, 0)
	ex.Model = "m"
	ex.Request.Metadata = ExchangeRequestMetadata{}
	ex.Request.Components = nil
	ex.Response = ExchangeResponse{}
	ex.Request.BodyHash = ""
	got = NewChainBuilder().Build([]Exchange{ex})
	if len(got.Uncorrelated) != 1 {
		t.Fatalf("expected uncorrelated, got %+v", got)
	}
}

func TestIsChainModelCall_ProtocolAware(t *testing.T) {
	cases := []struct {
		name     string
		protocol Protocol
		endpoint string
		want     bool
	}{
		{"anthropic /v1/messages", ProtocolAnthropicMessages, "/v1/messages", true},
		{"anthropic count_tokens", ProtocolAnthropicMessages, "/v1/messages/count_tokens", false},
		{"openai /responses", ProtocolOpenAIResponses, "/responses", true},
		{"openai /v1/responses", ProtocolOpenAIResponses, "/v1/responses", true},
		{"openai /models", ProtocolOpenAIResponses, "/models", false},
		{"openai /v1/models", ProtocolOpenAIResponses, "/v1/models", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ex := Exchange{Protocol: tc.protocol, Request: ExchangeRequest{Method: "POST", Endpoint: tc.endpoint}}
			if got := isChainModelCall(ex); got != tc.want {
				t.Fatalf("isChainModelCall(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

func TestModelsEndpointDoesNotCreateChain(t *testing.T) {
	modelsEx := openAIExchange("gw-models", "codex", "m", 0, openAIModelsBody())
	responsesEx := openAIExchange("gw-resp", "codex", "m", 1, openAIResponsesBody("m", nil, true))
	got := NewChainBuilder().Build([]Exchange{modelsEx, responsesEx})
	if len(got.Chains) != 1 {
		t.Fatalf("chains = %d, want 1", len(got.Chains))
	}
	if got.Chains[0].Exchanges[0].ID != "gw-resp" {
		t.Fatalf("chain[0] = %s, want gw-resp", got.Chains[0].Exchanges[0].ID)
	}
}

func TestModelsBetweenResponsesDoesNotBreakLineage(t *testing.T) {
	ex1 := openAIExchangeWithMeta("gw-a", "codex", "m", 0,
		openAIResponsesBody("m", nil, true),
		nil,
		&OpenAIResponsesResponseMeta{ResponseID: "resp_A", OutputItemCallIDs: []string{"call_1"}},
	)
	exMid := openAIExchange("gw-mid", "codex", "m", 1, openAIModelsBody())
	ex2 := openAIExchangeWithMeta("gw-b", "codex", "m", 2,
		openAIResponsesBodyWithOutputCall("m", []string{"call_1"}),
		&OpenAIResponsesMetadata{FunctionCallOutputIDs: []string{"call_1"}},
		nil,
	)
	got := NewChainBuilder().Build([]Exchange{ex1, exMid, ex2})
	if len(got.Chains) != 1 {
		t.Fatalf("chains = %d, want 1", len(got.Chains))
	}
	if len(got.Chains[0].Exchanges) != 2 {
		t.Fatalf("chain exchanges = %d, want 2 (ex1, ex2). /models must not break or join.", len(got.Chains[0].Exchanges))
	}
	if got.Chains[0].Exchanges[0].ID != "gw-a" || got.Chains[0].Exchanges[1].ID != "gw-b" {
		t.Fatalf("chain order = %v, want gw-a,gw-b", chainIDs(got.Chains[0]))
	}
}

func TestOpenAIExplicitIDLineageContinues(t *testing.T) {
	prev := openAIExchangeWithMeta("gw-a", "codex", "m", 0,
		openAIResponsesBody("m", nil, true),
		nil,
		&OpenAIResponsesResponseMeta{ResponseID: "resp_A"},
	)
	cur := openAIExchangeWithMeta("gw-b", "codex", "m", 1,
		openAIResponsesBody("m", nil, true),
		&OpenAIResponsesMetadata{PreviousResponseID: "resp_A"},
		nil,
	)
	got := OpenAIResponsesCorrelator{}.Correlate(prev, cur)
	if got.Relation != ChainRelationContinue || got.Method != CorrelationExplicitID {
		t.Fatalf("correlation = %+v, want explicit_id continue", got)
	}
	if len(got.Evidence) != 1 || got.Evidence[0].Value != "resp_A" {
		t.Fatalf("evidence = %+v", got.Evidence)
	}
}

func TestOpenAIExplicitIDNonMatchingDoesNotContinue(t *testing.T) {
	prev := openAIExchangeWithMeta("gw-a", "codex", "m", 0,
		openAIResponsesBody("m", nil, true),
		nil,
		&OpenAIResponsesResponseMeta{ResponseID: "resp_A"},
	)
	cur := openAIExchangeWithMeta("gw-b", "codex", "m", 1,
		openAIResponsesBody("m", nil, true),
		&OpenAIResponsesMetadata{PreviousResponseID: "resp_Z"},
		nil,
	)
	got := OpenAIResponsesCorrelator{}.Correlate(prev, cur)
	if got.Relation == ChainRelationContinue && got.Method == CorrelationExplicitID {
		t.Fatalf("correlation = %+v, want non-continuation", got)
	}
}

func TestOpenAIExplicitIDWithUnknownParentReturnsUnknown(t *testing.T) {
	prev := openAIExchangeWithMeta("gw-a", "codex", "m", 0,
		openAIResponsesBody("m", nil, true),
		nil,
		nil,
	)
	cur := openAIExchangeWithMeta("gw-b", "codex", "m", 1,
		openAIResponsesBody("m", nil, true),
		&OpenAIResponsesMetadata{PreviousResponseID: "resp_X"},
		nil,
	)
	got := OpenAIResponsesCorrelator{}.Correlate(prev, cur)
	if got.Relation == ChainRelationContinue {
		t.Fatalf("correlation = %+v, want unknown when parent response_id not observed", got)
	}
}

func TestOpenAIToolLineageContinues(t *testing.T) {
	prev := openAIExchangeWithMeta("gw-a", "codex", "m", 0,
		openAIResponsesBody("m", nil, true),
		nil,
		&OpenAIResponsesResponseMeta{ResponseID: "resp_A", OutputItemCallIDs: []string{"call_1", "call_2"}},
	)
	cur := openAIExchangeWithMeta("gw-b", "codex", "m", 1,
		openAIResponsesBodyWithOutputCall("m", []string{"call_1"}),
		&OpenAIResponsesMetadata{FunctionCallOutputIDs: []string{"call_1"}},
		nil,
	)
	got := OpenAIResponsesCorrelator{}.Correlate(prev, cur)
	if got.Relation != ChainRelationContinue || got.Method != CorrelationToolLineage {
		t.Fatalf("correlation = %+v, want tool_lineage", got)
	}
	if len(got.Evidence) != 1 || got.Evidence[0].Value != "call_1" {
		t.Fatalf("evidence = %+v", got.Evidence)
	}
}

func TestOpenAIToolLineageNonMatching(t *testing.T) {
	prev := openAIExchangeWithMeta("gw-a", "codex", "m", 0,
		openAIResponsesBody("m", nil, true),
		nil,
		&OpenAIResponsesResponseMeta{OutputItemCallIDs: []string{"call_1"}},
	)
	cur := openAIExchangeWithMeta("gw-b", "codex", "m", 1,
		openAIResponsesBodyWithOutputCall("m", []string{"call_X"}),
		&OpenAIResponsesMetadata{FunctionCallOutputIDs: []string{"call_X"}},
		nil,
	)
	got := OpenAIResponsesCorrelator{}.Correlate(prev, cur)
	if got.Relation == ChainRelationContinue && got.Method == CorrelationToolLineage {
		t.Fatalf("correlation = %+v, want non-continuation", got)
	}
}

func TestOpenAIToolLineageMultipleCallIDs(t *testing.T) {
	prev := openAIExchangeWithMeta("gw-a", "codex", "m", 0,
		openAIResponsesBody("m", nil, true),
		nil,
		&OpenAIResponsesResponseMeta{OutputItemCallIDs: []string{"call_a", "call_b"}},
	)
	cur := openAIExchangeWithMeta("gw-b", "codex", "m", 1,
		openAIResponsesBodyWithOutputCall("m", []string{"call_a", "call_b"}),
		&OpenAIResponsesMetadata{FunctionCallOutputIDs: []string{"call_b", "call_a"}},
		nil,
	)
	got := OpenAIResponsesCorrelator{}.Correlate(prev, cur)
	if got.Relation != ChainRelationContinue {
		t.Fatalf("correlation = %+v, want continue", got)
	}
	if len(got.Evidence) != 2 {
		t.Fatalf("evidence len = %d, want 2", len(got.Evidence))
	}
}

func TestOpenAIExplicitIDBeatsContext(t *testing.T) {
	prev := openAIExchangeWithMeta("gw-a", "codex", "m", 0,
		openAIResponsesBody("m", nil, true),
		nil,
		&OpenAIResponsesResponseMeta{ResponseID: "resp_A"},
	)
	cur := openAIExchangeWithMeta("gw-b", "codex", "m", 1,
		openAIResponsesBody("m", nil, true),
		&OpenAIResponsesMetadata{PreviousResponseID: "resp_A"},
		nil,
	)
	prev.Request.Components = openAIHashedHistory("hash1")
	cur.Request.Components = openAIHashedHistory("hash1", "hash2")
	got := OpenAIResponsesCorrelator{}.Correlate(prev, cur)
	if got.Method != CorrelationExplicitID {
		t.Fatalf("method = %q, want explicit_id", got.Method)
	}
}

func TestOpenAIToolLineageBeatsWeakContext(t *testing.T) {
	prev := openAIExchangeWithMeta("gw-a", "codex", "m", 0,
		openAIResponsesBody("m", nil, true),
		nil,
		&OpenAIResponsesResponseMeta{OutputItemCallIDs: []string{"call_1"}},
	)
	cur := openAIExchangeWithMeta("gw-b", "codex", "m", 1,
		openAIResponsesBodyWithOutputCall("m", []string{"call_1"}),
		&OpenAIResponsesMetadata{FunctionCallOutputIDs: []string{"call_1"}},
		nil,
	)
	prev.Request.Components = openAIHashedHistory("same")
	cur.Request.Components = openAIHashedHistory("same", "extra")
	got := OpenAIResponsesCorrelator{}.Correlate(prev, cur)
	if got.Method != CorrelationToolLineage {
		t.Fatalf("method = %q, want tool_lineage", got.Method)
	}
}

func TestOpenAISameProfileModelDoesNotContinue(t *testing.T) {
	prev := openAIExchange("gw-a", "codex", "m", 0, openAIStaticBody("m", "inst", nil))
	cur := openAIExchange("gw-b", "codex", "m", 1, openAIStaticBody("m", "inst", nil))
	got := OpenAIResponsesCorrelator{}.Correlate(prev, cur)
	if got.Relation != ChainRelationUnknown {
		t.Fatalf("correlation = %+v, want unknown", got)
	}
}

func TestOpenAISameInstructionsDoesNotContinue(t *testing.T) {
	prev := openAIExchange("gw-a", "codex", "m", 0, openAIStaticBody("m", "same", nil))
	cur := openAIExchange("gw-b", "codex", "m", 1, openAIStaticBody("m", "same", nil))
	got := OpenAIResponsesCorrelator{}.Correlate(prev, cur)
	if got.Relation != ChainRelationUnknown {
		t.Fatalf("correlation = %+v, want unknown", got)
	}
}

func TestOpenAISameToolsDoesNotContinue(t *testing.T) {
	prev := openAIExchange("gw-a", "codex", "m", 0, openAIToolsBody("m", "same"))
	cur := openAIExchange("gw-b", "codex", "m", 1, openAIToolsBody("m", "same"))
	got := OpenAIResponsesCorrelator{}.Correlate(prev, cur)
	if got.Relation != ChainRelationUnknown {
		t.Fatalf("correlation = %+v, want unknown", got)
	}
}

func TestOpenAISameInstructionsAndToolsDoesNotContinue(t *testing.T) {
	prev := openAIExchange("gw-a", "codex", "m", 0, openAIStaticBody("m", "same", []map[string]any{openAITool("same")}))
	cur := openAIExchange("gw-b", "codex", "m", 1, openAIStaticBody("m", "same", []map[string]any{openAITool("same")}))
	got := OpenAIResponsesCorrelator{}.Correlate(prev, cur)
	if got.Relation != ChainRelationUnknown {
		t.Fatalf("correlation = %+v, want unknown", got)
	}
}

func TestOpenAICloseTimestampsDoNotContinue(t *testing.T) {
	prev := openAIExchange("gw-a", "codex", "m", 0, openAIResponsesBody("m", nil, true))
	cur := openAIExchange("gw-b", "codex", "m", 1, openAIResponsesBody("m", nil, true))
	got := OpenAIResponsesCorrelator{}.Correlate(prev, cur)
	if got.Relation != ChainRelationUnknown {
		t.Fatalf("correlation = %+v, want unknown", got)
	}
}

func TestOpenAIFarTimestampsWithExplicitIDContinue(t *testing.T) {
	prev := openAIExchangeWithMeta("gw-a", "codex", "m", 0,
		openAIResponsesBody("m", nil, true),
		nil,
		&OpenAIResponsesResponseMeta{ResponseID: "resp_A"},
	)
	cur := openAIExchangeWithMeta("gw-b", "codex", "m", 86400,
		openAIResponsesBody("m", nil, true),
		&OpenAIResponsesMetadata{PreviousResponseID: "resp_A"},
		nil,
	)
	got := OpenAIResponsesCorrelator{}.Correlate(prev, cur)
	if got.Relation != ChainRelationContinue {
		t.Fatalf("correlation = %+v, want continue", got)
	}
}

func TestOpenAIContextLineageMayContinue(t *testing.T) {
	prev := openAIExchange("gw-a", "codex", "m", 0, openAIResponsesBody("m", nil, true))
	cur := openAIExchange("gw-b", "codex", "m", 1, openAIResponsesBody("m", nil, true))
	prev.Request.Components = openAIHashedHistory("hash1")
	cur.Request.Components = openAIHashedHistory("hash1", "hash2")
	got := OpenAIResponsesCorrelator{}.Correlate(prev, cur)
	if got.Relation != ChainRelationContinue || got.Method != CorrelationContextLineage {
		t.Fatalf("correlation = %+v, want context lineage", got)
	}
}

func TestOpenAIAmbiguousContextLineageUnknown(t *testing.T) {
	prev := openAIExchange("gw-a", "codex", "m", 0, openAIResponsesBody("m", nil, true))
	cur := openAIExchange("gw-b", "codex", "m", 1, openAIResponsesBody("m", nil, true))
	prev.Request.Components = openAIHashedHistory("hash1")
	cur.Request.Components = openAIHashedHistory("hash_other")
	got := OpenAIResponsesCorrelator{}.Correlate(prev, cur)
	if got.Relation == ChainRelationContinue {
		t.Fatalf("correlation = %+v, want not continue", got)
	}
}

func TestOpenAITwoResponsesExplicitLineageOneChain(t *testing.T) {
	prev := openAIExchangeWithMeta("gw-a", "codex", "m", 0,
		openAIResponsesBody("m", nil, true),
		nil,
		&OpenAIResponsesResponseMeta{ResponseID: "resp_A"},
	)
	cur := openAIExchangeWithMeta("gw-b", "codex", "m", 1,
		openAIResponsesBody("m", nil, true),
		&OpenAIResponsesMetadata{PreviousResponseID: "resp_A"},
		nil,
	)
	mid1 := openAIExchange("gw-m1", "codex", "m", 2, openAIModelsBody())
	mid2 := openAIExchange("gw-m2", "codex", "m", 3, openAIModelsBody())
	got := NewChainBuilder().Build([]Exchange{prev, mid1, mid2, cur})
	if len(got.Chains) != 1 {
		t.Fatalf("chains = %d, want 1", len(got.Chains))
	}
	if len(got.Chains[0].Exchanges) != 2 {
		t.Fatalf("model exchanges = %d, want 2", len(got.Chains[0].Exchanges))
	}
}

func TestOpenAITwoIndependentResponsesRemainSeparate(t *testing.T) {
	prev := openAIExchange("gw-a", "codex", "m", 0, openAIResponsesBody("m", nil, true))
	cur := openAIExchange("gw-b", "codex", "m", 1, openAIResponsesBody("m", nil, true))
	got := NewChainBuilder().Build([]Exchange{prev, cur})
	if len(got.Chains) != 2 {
		t.Fatalf("chains = %d, want 2", len(got.Chains))
	}
}

func TestOpenAIMetadataNoRawContent(t *testing.T) {
	body := []byte(`{"previous_response_id":"resp_A","input":[{"type":"function_call_output","call_id":"call_1","output":"SECRET_OUTPUT"}]}`)
	meta := OpenAIResponsesObserver{}.ParseMetadata(body)
	raw, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "SECRET_OUTPUT") {
		t.Fatalf("metadata leaked raw output: %s", raw)
	}
	if strings.Contains(string(raw), "call_1") && !strings.Contains(string(raw), "functionCallOutputIds") {
		t.Fatalf("metadata missing structured call id: %s", raw)
	}
	if !strings.Contains(string(raw), "resp_A") {
		t.Fatalf("metadata missing previous_response_id: %s", raw)
	}
}

func TestOpenAIResponseMetaNoRawContent(t *testing.T) {
	body := []byte(`{"id":"resp_A","output":[{"type":"function_call","call_id":"call_1","arguments":"SECRET_ARGS"}]}`)
	meta := OpenAIResponsesObserver{}.ParseResponse(body)
	raw, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := string(raw)
	if strings.Contains(out, "SECRET_ARGS") {
		t.Fatalf("response meta leaked raw arguments: %s", out)
	}
	if !strings.Contains(out, "resp_A") {
		t.Fatalf("response meta missing id: %s", out)
	}
	if !strings.Contains(out, "call_1") {
		t.Fatalf("response meta missing call id: %s", out)
	}
}

func TestOpenAIChainResultNoRawBodiesOrSecrets(t *testing.T) {
	prev := openAIExchange("gw-a", "codex", "m", 0, []byte(`{"model":"m","input":[{"type":"message","role":"user","content":"SECRET_PROMPT"}]}`))
	cur := openAIExchange("gw-b", "codex", "m", 1, []byte(`{"model":"m","input":[{"type":"message","role":"user","content":"SECRET_PROMPT_TWO"}]}`))
	prev.Response.OpenAIResponses = &OpenAIResponsesResponseMeta{ResponseID: "resp_A"}
	cur.Request.Metadata.OpenAIResponses = &OpenAIResponsesMetadata{PreviousResponseID: "resp_A"}
	got := NewChainBuilder().Build([]Exchange{prev, cur})
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, secret := range []string{"SECRET_PROMPT", "SECRET_PROMPT_TWO", "Authorization", "Bearer"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("chain result leaked %q", secret)
		}
	}
}

func openAIExchange(id, profile, modelName string, seconds int64, body []byte) Exchange {
	return openAIExchangeWithMeta(id, profile, modelName, seconds, body, nil, nil)
}

func openAIExchangeWithMeta(id, profile, modelName string, seconds int64, body []byte, reqMeta *OpenAIResponsesMetadata, respMeta *OpenAIResponsesResponseMeta) Exchange {
	components := OpenAIResponsesObserver{}.Parse(body)
	meta := OpenAIResponsesObserver{}.ParseMetadata(body)
	if reqMeta != nil {
		meta.OpenAIResponses = reqMeta
	}
	return Exchange{
		ID:        id,
		Profile:   profile,
		Protocol:  ProtocolOpenAIResponses,
		StartedAt: time.Unix(seconds, 0),
		Model:     modelName,
		Request: ExchangeRequest{
			Method:     "POST",
			Endpoint:   "/responses",
			BodyHash:   contentHash(body),
			Components: components,
			Metadata:   meta,
		},
		Response: ExchangeResponse{
			OpenAIResponses: respMeta,
		},
	}
}

func openAIResponsesBody(modelName string, functionCallOutputs []string, fresh bool) []byte {
	return openAIResponsesBodyWithOutputCall(modelName, functionCallOutputs)
}

func openAIResponsesBodyWithOutputCall(modelName string, functionCallOutputs []string) []byte {
	var input []map[string]any
	for _, callID := range functionCallOutputs {
		input = append(input, map[string]any{
			"type":    "function_call_output",
			"call_id": callID,
			"output":  "ok",
		})
	}
	if len(input) == 0 {
		input = append(input, map[string]any{
			"type":    "message",
			"role":    "user",
			"content": []map[string]any{{"type": "text", "text": "hi"}},
		})
	}
	raw, _ := json.Marshal(map[string]any{
		"model": modelName,
		"input": input,
	})
	return raw
}

func openAIStaticBody(modelName, instructions string, tools []map[string]any) []byte {
	body := map[string]any{
		"model":        modelName,
		"instructions": instructions,
		"input": []map[string]any{
			{"type": "message", "role": "user", "content": []map[string]any{{"type": "text", "text": "hi"}}},
		},
	}
	if len(tools) > 0 {
		body["tools"] = tools
	}
	raw, _ := json.Marshal(body)
	return raw
}

func openAITool(name string) map[string]any {
	return map[string]any{"name": name}
}

func openAIToolsBody(modelName, toolName string) []byte {
	return openAIStaticBody(modelName, "", []map[string]any{openAITool(toolName)})
}

func openAIModelsBody() []byte {
	return []byte(`{"model":"any"}`)
}

func openAIHashedHistory(hashes ...string) []model.ContextComponent {
	out := make([]model.ContextComponent, 0, len(hashes))
	for _, h := range hashes {
		out = append(out, model.ContextComponent{
			Kind:        model.ContextHistory,
			ContentHash: source.ContentHash(h),
		})
	}
	return out
}
