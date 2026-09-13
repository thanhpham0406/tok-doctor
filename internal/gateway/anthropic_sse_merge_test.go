package gateway

import "testing"

// TestAnthropic_MergeAcrossPartialDeltas_AllFieldsKept verifies that when
// different message_delta events carry disjoint fields, each field survives
// to the merged snapshot. Anthropic reports cumulative usage across events;
// summing them must not happen, and replacing a previously seen field with
// explicit zero must happen.
func TestAnthropic_MergeAcrossPartialDeltas_AllFieldsKept(t *testing.T) {
	state := newAnthropicStreamState()

	parse := func(payload []byte) (*ProviderUsage, bool) {
		return (AnthropicMessagesObserver{}).ParseStreamFrame(state, payload)
	}

	if usage, terminal := parse([]byte(`{"type":"message_start","message":{"usage":{"input_tokens":100,"output_tokens":0}}}`)); usage == nil || terminal {
		t.Fatalf("message_start unexpected: %+v %v", usage, terminal)
	}
	if _, terminal := parse([]byte(`{"type":"content_block_delta","delta":{"type":"text_delta","text":"x"}}`)); terminal {
		t.Fatalf("content_block_delta should not be terminal")
	}
	if usage, _ := parse([]byte(`{"type":"message_delta","usage":{"cache_creation_input_tokens":20}}`)); usage == nil {
		t.Fatalf("delta cache_creation must keep prior fields")
	}
	if usage, _ := parse([]byte(`{"type":"message_delta","usage":{"output_tokens":30}}`)); usage == nil {
		t.Fatalf("delta output must keep prior fields")
	}
	if _, terminal := parse([]byte(`{"type":"message_stop"}`)); !terminal {
		t.Fatalf("message_stop should mark terminal")
	}
	pu, terminal := parse([]byte(`{"type":"message_stop"}`))
	if !terminal {
		t.Fatalf("second message_stop still terminal")
	}
	if pu == nil {
		t.Fatalf("final snapshot must exist")
	}
	if pu.InputTokens == nil || *pu.InputTokens != 100 {
		t.Fatalf("input lost: %+v", pu.InputTokens)
	}
	if pu.CacheCreationInputTokens == nil || *pu.CacheCreationInputTokens != 20 {
		t.Fatalf("cache_creation lost: %+v", pu.CacheCreationInputTokens)
	}
	if pu.OutputTokens == nil || *pu.OutputTokens != 30 {
		t.Fatalf("output lost: %+v", pu.OutputTokens)
	}
}

// TestAnthropic_ExplicitZeroReplacesPriorSnapshot ensures a later event
// carrying an explicit zero replaces the prior value. Anthropic sends
// revised snapshots as the model runs; zero is the expected replacement,
// not a no-op.
func TestAnthropic_ExplicitZeroReplacesPriorSnapshot(t *testing.T) {
	state := newAnthropicStreamState()
	obs := AnthropicMessagesObserver{}

	if _, _ = obs.ParseStreamFrame(state, []byte(`{"type":"message_start","message":{"usage":{"input_tokens":100,"output_tokens":42}}}`)); state.snapshot.OutputTokens == nil {
		t.Fatalf("start snapshot missing output")
	}
	if _, _ = obs.ParseStreamFrame(state, []byte(`{"type":"message_delta","usage":{"output_tokens":0}}`)); state.snapshot.OutputTokens == nil || *state.snapshot.OutputTokens != 0 {
		t.Fatalf("zero must replace prior output, got %+v", state.snapshot.OutputTokens)
	}
	if state.snapshot.InputTokens == nil || *state.snapshot.InputTokens != 100 {
		t.Fatalf("input must survive zero replacement of output, got %+v", state.snapshot.InputTokens)
	}
}

// TestAnthropic_MalformedFrameDoesNotCorruptSnapshot covers the case where
// one `data:` payload carries invalid JSON between two valid snapshots.
// The valid snapshots must keep merging normally.
func TestAnthropic_MalformedFrameDoesNotCorruptSnapshot(t *testing.T) {
	state := newAnthropicStreamState()
	obs := AnthropicMessagesObserver{}

	_, _ = obs.ParseStreamFrame(state, []byte(`{"type":"message_start","message":{"usage":{"input_tokens":1,"output_tokens":2}}}`))
	_, _ = obs.ParseStreamFrame(state, []byte(`{not valid json`))
	if state.snapshot.InputTokens == nil || *state.snapshot.InputTokens != 1 {
		t.Fatalf("malformed frame corrupted snapshot: %+v", state.snapshot)
	}
	pu, _ := obs.ParseStreamFrame(state, []byte(`{"type":"message_delta","usage":{"output_tokens":3}}`))
	if pu == nil || pu.OutputTokens == nil || *pu.OutputTokens != 3 {
		t.Fatalf("valid frame after malformed: %+v", pu)
	}
	if pu.InputTokens == nil || *pu.InputTokens != 1 {
		t.Fatalf("input must survive malformed frame: %+v", pu)
	}
}

func TestAnthropic_MultipleConcurrentStreamsCarrySeparateState(t *testing.T) {
	a := newAnthropicStreamState()
	b := newAnthropicStreamState()
	obs := AnthropicMessagesObserver{}

	if _, _ = obs.ParseStreamFrame(a, []byte(`{"type":"message_start","message":{"usage":{"input_tokens":100,"output_tokens":10}}}`)); true {
	}
	if _, _ = obs.ParseStreamFrame(b, []byte(`{"type":"message_start","message":{"usage":{"input_tokens":200,"output_tokens":20}}}`)); true {
	}

	ap, _ := obs.ParseStreamFrame(a, []byte(`{"type":"message_delta","usage":{"output_tokens":11}}`))
	bp, _ := obs.ParseStreamFrame(b, []byte(`{"type":"message_delta","usage":{"output_tokens":22}}`))
	if ap == nil || bp == nil {
		t.Fatalf("usage missing")
	}
	if *ap.InputTokens != 100 || *ap.OutputTokens != 11 {
		t.Fatalf("stream a usage wrong: %+v", ap)
	}
	if *bp.InputTokens != 200 || *bp.OutputTokens != 22 {
		t.Fatalf("stream b usage wrong: %+v", bp)
	}
}

func TestAnthropic_NoMessageStartNeverReturnsUsage(t *testing.T) {
	state := newAnthropicStreamState()
	obs := AnthropicMessagesObserver{}
	if pu, _ := obs.ParseStreamFrame(state, []byte(`{"type":"content_block_start","index":0}`)); pu != nil {
		t.Fatalf("usage must be nil before message_start, got %+v", pu)
	}
}
