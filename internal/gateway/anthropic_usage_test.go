package gateway

import "testing"

func TestProviderUsage_Anthropic_NonStreamingJSON(t *testing.T) {
	body := []byte(`{
		"id":"msg_01",
		"content":[{"type":"text","text":"hello"}],
		"usage":{
			"input_tokens":120,
			"output_tokens":42,
			"cache_creation_input_tokens":10,
			"cache_read_input_tokens":50
		}
	}`)
	pu := (AnthropicMessagesObserver{}).ParseResponseUsage(body)
	if pu == nil {
		t.Fatalf("usage nil")
	}
	if pu.InputTokens == nil || *pu.InputTokens != 120 {
		t.Fatalf("input_tokens = %+v", pu.InputTokens)
	}
	if pu.OutputTokens == nil || *pu.OutputTokens != 42 {
		t.Fatalf("output_tokens = %+v", pu.OutputTokens)
	}
	if pu.CacheReadInputTokens == nil || *pu.CacheReadInputTokens != 50 {
		t.Fatalf("cache_read = %+v", pu.CacheReadInputTokens)
	}
	if pu.CacheCreationInputTokens == nil || *pu.CacheCreationInputTokens != 10 {
		t.Fatalf("cache_creation = %+v", pu.CacheCreationInputTokens)
	}
}

func TestProviderUsage_Anthropic_MissingCacheFieldsNil(t *testing.T) {
	body := []byte(`{"usage":{"input_tokens":5,"output_tokens":3}}`)
	pu := (AnthropicMessagesObserver{}).ParseResponseUsage(body)
	if pu == nil {
		t.Fatalf("usage nil")
	}
	if pu.CacheReadInputTokens != nil {
		t.Fatalf("cache_read should be nil: %+v", pu.CacheReadInputTokens)
	}
	if pu.CacheCreationInputTokens != nil {
		t.Fatalf("cache_creation should be nil: %+v", pu.CacheCreationInputTokens)
	}
}

func TestProviderUsage_Anthropic_ZeroUsagePreservedAsZero(t *testing.T) {
	body := []byte(`{"usage":{"input_tokens":0,"output_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}`)
	pu := (AnthropicMessagesObserver{}).ParseResponseUsage(body)
	if pu == nil {
		t.Fatalf("usage nil")
	}
	if pu.InputTokens == nil || *pu.InputTokens != 0 {
		t.Fatalf("expected explicit zero pointer, got %+v", pu.InputTokens)
	}
	if pu.CacheReadInputTokens == nil || *pu.CacheReadInputTokens != 0 {
		t.Fatalf("expected explicit zero cache_read: %+v", pu.CacheReadInputTokens)
	}
}

func TestProviderUsage_Anthropic_NormalizesTotalInput(t *testing.T) {
	body := []byte(`{"usage":{"input_tokens":100,"cache_creation_input_tokens":20,"cache_read_input_tokens":800,"output_tokens":30}}`)
	pu := (AnthropicMessagesObserver{}).ParseResponseUsage(body)
	if pu == nil {
		t.Fatalf("usage nil")
	}
	obs := ProviderUsageToObserved(pu)
	if obs.TotalInput != 920 {
		t.Fatalf("total input = %d, want 920", obs.TotalInput)
	}
	if obs.Output != 30 {
		t.Fatalf("output = %d, want 30", obs.Output)
	}
	if obs.Total != 950 {
		t.Fatalf("total tokens = %d, want 950", obs.Total)
	}
	if obs.RawInput != 100 {
		t.Fatalf("raw input = %d, want 100", obs.RawInput)
	}
	if obs.Cached != 800 {
		t.Fatalf("cached = %d, want 800", obs.Cached)
	}
	if obs.CacheCreation != 20 {
		t.Fatalf("cache creation = %d, want 20", obs.CacheCreation)
	}
}

func TestProviderUsage_OpenAI_NormalizesTotalInputAndReasoning(t *testing.T) {
	body := []byte(`{"usage":{"input_tokens":920,"output_tokens":30,"total_tokens":950,"input_tokens_details":{"cached_tokens":800},"output_tokens_details":{"reasoning_tokens":10}}}`)
	pu := OpenAIResponsesObserver{}.ParseResponseUsage(body)
	if pu == nil {
		t.Fatalf("usage nil")
	}
	obs := ProviderUsageToObserved(pu)
	if obs.TotalInput != 920 {
		t.Fatalf("total input = %d, want 920", obs.TotalInput)
	}
	if obs.Output != 30 {
		t.Fatalf("output = %d, want 30 (reasoning must not double-count)", obs.Output)
	}
	if obs.Reasoning != 10 {
		t.Fatalf("reasoning = %d, want 10", obs.Reasoning)
	}
	if obs.Total != 950 {
		t.Fatalf("total = %d, want 950", obs.Total)
	}
}

func TestProviderUsage_MissingUsageFieldReturnsNil(t *testing.T) {
	body := []byte(`{"id":"msg_x"}`)
	if got := (AnthropicMessagesObserver{}).ParseResponseUsage(body); got != nil {
		t.Fatalf("expected nil usage, got %+v", got)
	}
}

func TestProviderUsage_MalformedBodyReturnsNil(t *testing.T) {
	body := []byte(`{not json`)
	if got := (AnthropicMessagesObserver{}).ParseResponseUsage(body); got != nil {
		t.Fatalf("expected nil usage, got %+v", got)
	}
}

func TestProviderUsage_NoFieldsReportedNotCounted(t *testing.T) {
	body := []byte(`{"usage":{}}`)
	pu := (AnthropicMessagesObserver{}).ParseResponseUsage(body)
	if pu != nil {
		t.Fatalf("empty usage should yield nil, got %+v", pu)
	}
}

func TestProviderUsage_ProviderUsageAddPreservesMissingFields(t *testing.T) {
	in := int64(5)
	a := &ProviderUsage{InputTokens: &in}
	b := &ProviderUsage{OutputTokens: ptrInt64(7)}
	merged := ProviderUsageAdd(a, b)
	if merged.InputTokens == nil || *merged.InputTokens != 5 {
		t.Fatalf("merged.InputTokens = %+v", merged.InputTokens)
	}
	if merged.OutputTokens == nil || *merged.OutputTokens != 7 {
		t.Fatalf("merged.OutputTokens = %+v", merged.OutputTokens)
	}
	ProviderUsageAdd(merged, nil)
	if merged.InputTokens == nil || merged.OutputTokens == nil {
		t.Fatalf("ProviderUsageAdd(nil) must not destroy fields")
	}
}

func ptrInt64(v int64) *int64 { return &v }
