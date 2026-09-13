package gateway

import "testing"

func TestAnthropic_MergeAcrossPartialDeltas_AllFieldsKept(t *testing.T) {
	state := newAnthropicStreamState()
	obs := AnthropicMessagesObserver{}

	parse := func(payload []byte) StreamFrameObservation {
		return obs.ParseStreamFrame(state, payload)
	}

	got := parse([]byte(`{"type":"message_start","message":{"id":"msg_x","usage":{"input_tokens":100,"output_tokens":0}}}`))
	if got.Terminal {
		t.Fatalf("message_start should not be terminal")
	}
	if got.ResponseObjectID != "msg_x" {
		t.Fatalf("message_start must capture message.id, got %q", got.ResponseObjectID)
	}
	got = parse([]byte(`{"type":"content_block_delta","delta":{"type":"text_delta","text":"x"}}`))
	if got.Terminal {
		t.Fatalf("content_block_delta should not be terminal")
	}
	got = parse([]byte(`{"type":"message_delta","usage":{"cache_creation_input_tokens":20}}`))
	if got.Usage == nil {
		t.Fatalf("delta cache_creation must keep prior fields")
	}
	got = parse([]byte(`{"type":"message_delta","usage":{"output_tokens":30}}`))
	if got.Usage == nil {
		t.Fatalf("delta output must keep prior fields")
	}
	got = parse([]byte(`{"type":"message_stop"}`))
	if !got.Terminal {
		t.Fatalf("message_stop should mark terminal")
	}
	got = parse([]byte(`{"type":"message_stop"}`))
	if !got.Terminal {
		t.Fatalf("second message_stop still terminal")
	}
	pu := got.Usage
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
	if got.ResponseObjectID != "msg_x" {
		t.Fatalf("message.id lost: %q", got.ResponseObjectID)
	}
}

func TestAnthropic_ExplicitZeroReplacesPriorSnapshot(t *testing.T) {
	state := newAnthropicStreamState()
	obs := AnthropicMessagesObserver{}

	obs.ParseStreamFrame(state, []byte(`{"type":"message_start","message":{"usage":{"input_tokens":100,"output_tokens":42}}}`))
	if state.snapshot.OutputTokens == nil {
		t.Fatalf("start snapshot missing output")
	}
	obs.ParseStreamFrame(state, []byte(`{"type":"message_delta","usage":{"output_tokens":0}}`))
	if state.snapshot.OutputTokens == nil || *state.snapshot.OutputTokens != 0 {
		t.Fatalf("zero must replace prior output, got %+v", state.snapshot.OutputTokens)
	}
	if state.snapshot.InputTokens == nil || *state.snapshot.InputTokens != 100 {
		t.Fatalf("input must survive zero replacement of output, got %+v", state.snapshot.InputTokens)
	}
}

func TestAnthropic_MalformedFrameDoesNotCorruptSnapshot(t *testing.T) {
	state := newAnthropicStreamState()
	obs := AnthropicMessagesObserver{}

	obs.ParseStreamFrame(state, []byte(`{"type":"message_start","message":{"id":"msg_x","usage":{"input_tokens":1,"output_tokens":2}}}`))
	obs.ParseStreamFrame(state, []byte(`{not valid json`))
	if state.snapshot.InputTokens == nil || *state.snapshot.InputTokens != 1 {
		t.Fatalf("malformed frame corrupted snapshot: %+v", state.snapshot)
	}
	if state.messageID != "msg_x" {
		t.Fatalf("malformed frame corrupted message id: %q", state.messageID)
	}
	got := obs.ParseStreamFrame(state, []byte(`{"type":"message_delta","usage":{"output_tokens":3}}`))
	pu := got.Usage
	if pu == nil || pu.OutputTokens == nil || *pu.OutputTokens != 3 {
		t.Fatalf("valid frame after malformed: %+v", pu)
	}
	if pu.InputTokens == nil || *pu.InputTokens != 1 {
		t.Fatalf("input must survive malformed frame: %+v", pu)
	}
	if got.ResponseObjectID != "msg_x" {
		t.Fatalf("message.id must survive malformed frame: %q", got.ResponseObjectID)
	}
}

func TestAnthropic_MultipleConcurrentStreamsCarrySeparateState(t *testing.T) {
	a := newAnthropicStreamState()
	b := newAnthropicStreamState()
	obs := AnthropicMessagesObserver{}

	obs.ParseStreamFrame(a, []byte(`{"type":"message_start","message":{"id":"msg_a","usage":{"input_tokens":100,"output_tokens":10}}}`))
	obs.ParseStreamFrame(b, []byte(`{"type":"message_start","message":{"id":"msg_b","usage":{"input_tokens":200,"output_tokens":20}}}`))

	ap := obs.ParseStreamFrame(a, []byte(`{"type":"message_delta","usage":{"output_tokens":11}}`)).Usage
	bp := obs.ParseStreamFrame(b, []byte(`{"type":"message_delta","usage":{"output_tokens":22}}`)).Usage
	if ap == nil || bp == nil {
		t.Fatalf("usage missing")
	}
	if *ap.InputTokens != 100 || *ap.OutputTokens != 11 {
		t.Fatalf("stream a usage wrong: %+v", ap)
	}
	if *bp.InputTokens != 200 || *bp.OutputTokens != 22 {
		t.Fatalf("stream b usage wrong: %+v", bp)
	}
	if a.messageID != "msg_a" || b.messageID != "msg_b" {
		t.Fatalf("message ids crossed between streams: a=%q b=%q", a.messageID, b.messageID)
	}
}

func TestAnthropic_NoMessageStartNeverReturnsUsage(t *testing.T) {
	state := newAnthropicStreamState()
	obs := AnthropicMessagesObserver{}
	got := obs.ParseStreamFrame(state, []byte(`{"type":"content_block_start","index":0}`))
	if got.Usage != nil {
		t.Fatalf("usage must be nil before message_start, got %+v", got.Usage)
	}
	if got.ResponseObjectID != "" {
		t.Fatalf("no message_start means no message.id: got %q", got.ResponseObjectID)
	}
}
