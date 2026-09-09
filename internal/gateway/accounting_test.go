package gateway

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

func TestSummarizeRequest_AggregatesByKind(t *testing.T) {
	exchange := Exchange{
		ID:        "gw-1",
		Profile:   "codex",
		Model:     "gpt-x",
		StartedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Protocol:  ProtocolOpenAIResponses,
		Request: ExchangeRequest{
			Method:   "POST",
			Endpoint: "/responses",
			Bytes:    1234,
			Components: []model.ContextComponent{
				{Kind: model.ContextInstructions, Measurement: model.NewMeasurement(10, model.MeasurementEstimated)},
				{Kind: model.ContextHistory, Measurement: model.NewMeasurement(50, model.MeasurementEstimated)},
				{Kind: model.ContextHistory, Measurement: model.NewMeasurement(7, model.MeasurementEstimated)},
				{Kind: model.ContextUserPrompt, Measurement: model.NewMeasurement(5, model.MeasurementEstimated)},
				{Kind: model.ContextToolResult, Measurement: model.NewMeasurement(8, model.MeasurementEstimated)},
				{Kind: model.ContextToolDefinition, Measurement: model.NewMeasurement(11, model.MeasurementEstimated)},
				{Kind: model.ContextFile, Path: "AGENTS.md", Measurement: model.NewMeasurement(20, model.MeasurementEstimated)},
				{Kind: model.ContextOther, Measurement: model.NewMeasurement(3, model.MeasurementEstimated)},
			},
		},
	}
	s := SummarizeRequest(exchange)

	if s.Categories.Instructions.ValueOrZero() != 10 {
		t.Fatalf("instructions = %d, want 10", s.Categories.Instructions.ValueOrZero())
	}
	if s.Categories.History.ValueOrZero() != 57 {
		t.Fatalf("history = %d, want 57", s.Categories.History.ValueOrZero())
	}
	if s.Categories.UserPrompt.ValueOrZero() != 5 {
		t.Fatalf("user prompt = %d", s.Categories.UserPrompt.ValueOrZero())
	}
	if s.Categories.ToolResult.ValueOrZero() != 8 {
		t.Fatalf("tool result = %d", s.Categories.ToolResult.ValueOrZero())
	}
	if s.Categories.ToolDefinition.ValueOrZero() != 11 {
		t.Fatalf("tool def = %d", s.Categories.ToolDefinition.ValueOrZero())
	}
	if s.Categories.File.ValueOrZero() != 20 {
		t.Fatalf("file = %d", s.Categories.File.ValueOrZero())
	}
	if s.Categories.Other.ValueOrZero() != 3 {
		t.Fatalf("other = %d", s.Categories.Other.ValueOrZero())
	}
	if got := s.Attributed.Value.ValueOrZero(); got != 114 {
		t.Fatalf("attributed total = %d, want 114", got)
	}
	if s.Attributed.Value.Kind != model.MeasurementEstimated {
		t.Fatalf("attributed kind = %q, want estimated", s.Attributed.Value.Kind)
	}
	if s.PayloadBytes != 1234 {
		t.Fatalf("payload bytes = %d", s.PayloadBytes)
	}
}

func TestSummarizeRequest_EstimatedStaysEstimated(t *testing.T) {
	exchange := Exchange{
		Request: ExchangeRequest{
			Components: []model.ContextComponent{
				{Kind: model.ContextHistory, Measurement: model.NewMeasurement(10, model.MeasurementEstimated)},
				{Kind: model.ContextHistory, Measurement: model.NewMeasurement(5, model.MeasurementEstimated)},
			},
		},
	}
	s := SummarizeRequest(exchange)
	if s.Categories.History.Kind != model.MeasurementEstimated {
		t.Fatalf("history kind = %q, want estimated", s.Categories.History.Kind)
	}
}

func TestSummarizeRequest_CountedMixedWithEstimatedDowngradesToEstimated(t *testing.T) {
	exchange := Exchange{
		Request: ExchangeRequest{
			Components: []model.ContextComponent{
				{Kind: model.ContextHistory, Measurement: model.NewMeasurement(10, model.MeasurementCounted)},
				{Kind: model.ContextHistory, Measurement: model.NewMeasurement(5, model.MeasurementEstimated)},
			},
		},
	}
	s := SummarizeRequest(exchange)
	if s.Categories.History.Kind != model.MeasurementEstimated {
		t.Fatalf("history kind = %q, want estimated", s.Categories.History.Kind)
	}
	if s.Categories.History.ValueOrZero() != 15 {
		t.Fatalf("history = %d", s.Categories.History.ValueOrZero())
	}
}

func TestSummarizeRequest_CountedOnlyStaysCounted(t *testing.T) {
	exchange := Exchange{
		Request: ExchangeRequest{
			Components: []model.ContextComponent{
				{Kind: model.ContextHistory, Measurement: model.NewMeasurement(10, model.MeasurementCounted)},
				{Kind: model.ContextHistory, Measurement: model.NewMeasurement(5, model.MeasurementCounted)},
			},
		},
	}
	s := SummarizeRequest(exchange)
	if s.Categories.History.Kind != model.MeasurementCounted {
		t.Fatalf("history kind = %q, want counted", s.Categories.History.Kind)
	}
}

func TestSummarizeRequest_ByteCountedToolsDoNotEnterTokenTotals(t *testing.T) {
	exchange := Exchange{
		Request: ExchangeRequest{
			Components: []model.ContextComponent{
				{Kind: model.ContextToolDefinition, Measurement: model.NewMeasurement(115119, model.MeasurementCounted)},
				{Kind: model.ContextHistory, Measurement: model.NewMeasurement(10, model.MeasurementEstimated)},
			},
		},
	}
	s := SummarizeRequest(exchange)
	if s.Categories.ToolDefinition.Available() {
		t.Fatalf("tool definition = %+v, want unavailable", s.Categories.ToolDefinition)
	}
	if s.Attributed.Value.ValueOrZero() != 10 {
		t.Fatalf("attributed total = %d, want 10", s.Attributed.Value.ValueOrZero())
	}
	if s.Attributed.Value.Kind != model.MeasurementEstimated {
		t.Fatalf("attributed kind = %q, want estimated", s.Attributed.Value.Kind)
	}
}

func TestSummarizeRequest_UnknownDoesNotBecomeZero(t *testing.T) {
	exchange := Exchange{
		Request: ExchangeRequest{
			Components: []model.ContextComponent{
				{Kind: model.ContextHistory, Measurement: model.NewMeasurement(10, model.MeasurementCounted)},
				{Kind: model.ContextOther, Measurement: model.NewMeasurement(0, model.MeasurementUnknown)},
			},
		},
	}
	s := SummarizeRequest(exchange)
	if s.Categories.Other.ValueOrZero() != 0 {
		t.Fatalf("other = %d, want 0 for unknown", s.Categories.Other.ValueOrZero())
	}
	if got := s.Attributed.Value.ValueOrZero(); got != 10 {
		t.Fatalf("attributed total = %d, want 10", got)
	}
}

func TestSummarizeRequest_FileComponentsAggregateByPath(t *testing.T) {
	exchange := Exchange{
		Request: ExchangeRequest{
			Components: []model.ContextComponent{
				{Kind: model.ContextFile, Path: "a/AGENTS.md", Measurement: model.NewMeasurement(5, model.MeasurementEstimated)},
				{Kind: model.ContextFile, Path: "a/AGENTS.md", Measurement: model.NewMeasurement(3, model.MeasurementEstimated)},
				{Kind: model.ContextFile, Path: "internal/gateway/proxy.go", Measurement: model.NewMeasurement(9, model.MeasurementEstimated)},
			},
		},
	}
	s := SummarizeRequest(exchange)
	if len(s.Files) != 2 {
		t.Fatalf("file entries = %d, want 2", len(s.Files))
	}
	if s.Files[0].Path != "internal/gateway/proxy.go" || s.Files[0].Measurement.ValueOrZero() != 9 {
		t.Fatalf("first entry = %+v, want internal/gateway/proxy.go 9", s.Files[0])
	}
	if s.Files[1].Path != "a/AGENTS.md" || s.Files[1].Measurement.ValueOrZero() != 8 {
		t.Fatalf("second entry = %+v, want a/AGENTS.md 8", s.Files[1])
	}
}

func TestSummarizeRequest_FileMissingPathIgnored(t *testing.T) {
	exchange := Exchange{
		Request: ExchangeRequest{
			Components: []model.ContextComponent{
				{Kind: model.ContextFile, Path: "", Measurement: model.NewMeasurement(7, model.MeasurementEstimated)},
				{Kind: model.ContextFile, Path: "/abs/path", Measurement: model.NewMeasurement(4, model.MeasurementEstimated)},
			},
		},
	}
	s := SummarizeRequest(exchange)
	if len(s.Files) != 1 {
		t.Fatalf("file entries = %d, want 1", len(s.Files))
	}
	if s.Files[0].Path != "/abs/path" {
		t.Fatalf("file path = %q, want /abs/path", s.Files[0].Path)
	}
}

func TestSummarizeRequest_FilePathsStableForEqualEstimates(t *testing.T) {
	exchange := Exchange{
		Request: ExchangeRequest{
			Components: []model.ContextComponent{
				{Kind: model.ContextFile, Path: "b.txt", Measurement: model.NewMeasurement(4, model.MeasurementEstimated)},
				{Kind: model.ContextFile, Path: "a.txt", Measurement: model.NewMeasurement(4, model.MeasurementEstimated)},
			},
		},
	}
	s := SummarizeRequest(exchange)
	if len(s.Files) != 2 {
		t.Fatalf("file entries = %d, want 2", len(s.Files))
	}
	if s.Files[0].Path != "a.txt" || s.Files[1].Path != "b.txt" {
		t.Fatalf("file ordering = %+v, want a then b", s.Files)
	}
}

func TestRecorder_ListExchangesOrdersNewestFirst(t *testing.T) {
	dir := t.TempDir()
	rec, err := NewFileRecorder(dir)
	if err != nil {
		t.Fatalf("recorder: %v", err)
	}
	t.Cleanup(func() { _ = rec.Close() })

	rec.Record(Exchange{ID: "gw-1", Profile: "alpha", StartedAt: time.Unix(100, 0)})
	rec.Record(Exchange{ID: "gw-2", Profile: "alpha", StartedAt: time.Unix(200, 0)})
	rec.Record(Exchange{ID: "gw-3", Profile: "beta", StartedAt: time.Unix(150, 0)})

	exchanges, err := rec.ListExchanges("alpha")
	if err != nil {
		t.Fatalf("ListExchanges: %v", err)
	}
	if len(exchanges) != 2 {
		t.Fatalf("alpha exchanges = %d, want 2", len(exchanges))
	}
	if exchanges[0].ID != "gw-2" || exchanges[1].ID != "gw-1" {
		t.Fatalf("order = %s,%s, want gw-2,gw-1", exchanges[0].ID, exchanges[1].ID)
	}
}

func TestRecorder_FindExchange_ExactID(t *testing.T) {
	dir := t.TempDir()
	rec, err := NewFileRecorder(dir)
	if err != nil {
		t.Fatalf("recorder: %v", err)
	}
	t.Cleanup(func() { _ = rec.Close() })

	rec.Record(Exchange{ID: "gw-aaaa", Profile: "alpha", StartedAt: time.Unix(100, 0)})
	rec.Record(Exchange{ID: "gw-bbbb", Profile: "alpha", StartedAt: time.Unix(200, 0)})

	got, err := rec.FindExchange("alpha", "gw-bbbb")
	if err != nil {
		t.Fatalf("FindExchange: %v", err)
	}
	if got.ID != "gw-bbbb" {
		t.Fatalf("found id = %q, want gw-bbbb", got.ID)
	}
}

func TestRecorder_FindExchange_UniquePrefix(t *testing.T) {
	dir := t.TempDir()
	rec, err := NewFileRecorder(dir)
	if err != nil {
		t.Fatalf("recorder: %v", err)
	}
	t.Cleanup(func() { _ = rec.Close() })

	rec.Record(Exchange{ID: "gw-aaaa1111", Profile: "alpha", StartedAt: time.Unix(100, 0)})
	rec.Record(Exchange{ID: "gw-bbbb2222", Profile: "alpha", StartedAt: time.Unix(200, 0)})

	got, err := rec.FindExchange("alpha", "gw-aaaa")
	if err != nil {
		t.Fatalf("FindExchange unique prefix: %v", err)
	}
	if got.ID != "gw-aaaa1111" {
		t.Fatalf("found id = %q, want gw-aaaa1111", got.ID)
	}
}

func TestRecorder_FindExchange_AmbiguousPrefix(t *testing.T) {
	dir := t.TempDir()
	rec, err := NewFileRecorder(dir)
	if err != nil {
		t.Fatalf("recorder: %v", err)
	}
	t.Cleanup(func() { _ = rec.Close() })

	rec.Record(Exchange{ID: "gw-aaaa1111", Profile: "alpha", StartedAt: time.Unix(100, 0)})
	rec.Record(Exchange{ID: "gw-aaaa2222", Profile: "alpha", StartedAt: time.Unix(200, 0)})

	_, err = rec.FindExchange("alpha", "gw-aaaa")
	if !errors.Is(err, ErrExchangeAmbiguous) {
		t.Fatalf("expected ambiguous, got %v", err)
	}
}

func TestRecorder_FindExchange_NotFound(t *testing.T) {
	dir := t.TempDir()
	rec, err := NewFileRecorder(dir)
	if err != nil {
		t.Fatalf("recorder: %v", err)
	}
	t.Cleanup(func() { _ = rec.Close() })

	rec.Record(Exchange{ID: "gw-1", Profile: "alpha", StartedAt: time.Unix(1, 0)})
	_, err = rec.FindExchange("alpha", "gw-missing")
	if !errors.Is(err, ErrExchangeNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestRecorder_FindExchange_MissingFileClean(t *testing.T) {
	dir := t.TempDir()
	rec, err := NewFileRecorder(dir)
	if err != nil {
		t.Fatalf("recorder: %v", err)
	}
	t.Cleanup(func() { _ = rec.Close() })

	_, err = rec.FindExchange("alpha", "gw-anything")
	if !errors.Is(err, ErrExchangeNotFound) {
		t.Fatalf("expected not found on missing capture, got %v", err)
	}
}

func TestComponentTools_UsesEstimatedNotCounted(t *testing.T) {
	body := []byte(`[{"name":"a","description":"hello there"}]`)
	c := ComponentTools(0, body)

	if c.Kind != model.ContextToolDefinition {
		t.Fatalf("kind = %q", c.Kind)
	}
	if c.Measurement.Kind != model.MeasurementEstimated {
		t.Fatalf("tools kind = %q, want estimated (byte counts must not become counted tokens)", c.Measurement.Kind)
	}
	if c.Measurement.ValueOrZero() == int64(len(body)) {
		t.Fatalf("tools value = byte length %d; expected an estimated token count", c.Measurement.ValueOrZero())
	}
}

func TestRenderRequests_ShowsContextAndPayloadSeparately(t *testing.T) {
	summaries := []RequestSummary{
		{
			ExchangeID:   "gw-aaaa11112222",
			Model:        "gpt-x",
			StartedAt:    "2026-09-09T12:17:39Z",
			PayloadBytes: 312 * 1024,
			Attributed: AttributedTotal{
				Value: model.NewMeasurement(118305, model.MeasurementEstimated),
			},
		},
	}
	var buf strings.Builder
	if err := RenderRequests(&buf, summaries); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "118,305*") {
		t.Fatalf("output missing attributed context: %q", out)
	}
	if !strings.Contains(out, "312 KB") {
		t.Fatalf("output missing payload humanised: %q", out)
	}
	if !strings.Contains(out, "gw-aaaa1111") {
		t.Fatalf("output missing short exchange id: %q", out)
	}
}

func TestRenderRequests_EmptyShowsHelpfulMessage(t *testing.T) {
	var buf strings.Builder
	if err := RenderRequests(&buf, nil); err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(buf.String(), "No captured gateway requests") {
		t.Fatalf("empty output = %q", buf.String())
	}
}

func TestRenderInspect_HidesRawBodies(t *testing.T) {
	summary := RequestSummary{
		ExchangeID:   "gw-aaaa11112222",
		Profile:      "codex",
		Model:        "gpt-x",
		Endpoint:     "/responses",
		PayloadBytes: 500,
		Categories: CategoryBreakdown{
			Instructions:   model.NewMeasurement(10, model.MeasurementEstimated),
			History:        model.NewMeasurement(50, model.MeasurementEstimated),
			UserPrompt:     model.NewMeasurement(5, model.MeasurementEstimated),
			ToolResult:     model.NewMeasurement(8, model.MeasurementEstimated),
			ToolDefinition: model.NewMeasurement(11, model.MeasurementEstimated),
			File:           model.NewMeasurement(20, model.MeasurementEstimated),
			Other:          model.NewMeasurement(3, model.MeasurementEstimated),
		},
		Attributed: AttributedTotal{
			Value: model.NewMeasurement(107, model.MeasurementEstimated),
		},
		Files: []FileAttribution{
			{Path: "AGENTS.md", Measurement: model.NewMeasurement(20, model.MeasurementEstimated)},
		},
	}
	var buf strings.Builder
	if err := RenderInspect(&buf, summary); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "Authorization") || strings.Contains(out, "Bearer") {
		t.Fatalf("output leaks auth header: %q", out)
	}
	if strings.Contains(out, "sk-") || strings.Contains(out, "secret") {
		t.Fatalf("output leaks secret marker: %q", out)
	}
	for _, banned := range []string{"hello", "secret prompt", "goodbye"} {
		if strings.Contains(strings.ToLower(out), banned) {
			t.Fatalf("output unexpectedly contains %q", banned)
		}
	}
	if !strings.Contains(out, "estimated") {
		t.Fatalf("output missing estimated marker: %q", out)
	}
	if !strings.Contains(out, "AGENTS.md") {
		t.Fatalf("output missing file path: %q", out)
	}
}

func TestRenderRequests_NumericJSONKeepsValuesNumeric(t *testing.T) {
	summaries := []RequestSummary{
		{
			ExchangeID:   "gw-aaaa11112222",
			Model:        "gpt-x",
			StartedAt:    "2026-09-09T12:17:39Z",
			PayloadBytes: 319488,
			Attributed:   AttributedTotal{Value: model.NewMeasurement(118305, model.MeasurementEstimated)},
		},
	}
	summaries[0].Categories.History = model.NewMeasurement(73225, model.MeasurementEstimated)
	raw, err := json.Marshal(summaries)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	compact := strings.Join(strings.Fields(string(raw)), "")
	if !strings.Contains(compact, `"payloadBytes":319488`) {
		t.Fatalf("payloadBytes not numeric in JSON: %s", compact)
	}
	if !strings.Contains(compact, `"value":118305`) {
		t.Fatalf("attributed value not numeric in JSON: %s", compact)
	}
	if strings.Contains(compact, `"118,305"`) || strings.Contains(compact, `"73,225"`) {
		t.Fatalf("numeric value was stringified: %s", compact)
	}
}

func TestObservers_ClassifyComponentsStillWork(t *testing.T) {
	body := []byte(`{
		"model":"gpt-x",
		"instructions":"stay calm",
		"input":[
			{"type":"message","role":"user","content":[{"text":"hi"}]},
			{"type":"message","role":"assistant","content":[{"text":"hello"}]},
			{"type":"function_call_output","output":"ok"}
		],
		"tools":[{"name":"a"}]
	}`)
	components := OpenAIResponsesObserver{}.Parse(body)
	if len(components) == 0 {
		t.Fatalf("OpenAI parser returned no components")
	}
	for _, c := range components {
		if c.Observation != ObservationScope {
			t.Fatalf("observation scope = %q", c.Observation)
		}
	}
	tools := AnthropicMessagesObserver{}.Parse([]byte(`{"model":"c","system":"s","messages":[{"role":"user","content":"hi"}],"tools":[{"name":"a"}]}`))
	if len(tools) == 0 {
		t.Fatalf("Anthropic parser returned no components")
	}
}

func TestAnthropicMessagesObserver_ClassifiesClaudeCodeBlocks(t *testing.T) {
	body := []byte(`{
		"model":"mnm/MiniMax-M3",
		"system":[{"type":"text","text":"system guidance"}],
		"messages":[
			{"role":"user","content":[{"type":"text","text":"old question"}]},
			{"role":"assistant","content":[
				{"type":"text","text":"old answer"},
				{"type":"tool_use","id":"toolu_1","name":"Read","input":{"file_path":"AGENTS.md"}}
			]},
			{"role":"user","content":[
				{"type":"tool_result","tool_use_id":"toolu_1","content":[{"type":"text","text":"tool output"}]},
				{"type":"text","text":"current question"}
			]}
		],
		"tools":[{"name":"Read","description":"read a file"}]
	}`)

	components := AnthropicMessagesObserver{}.Parse(body)
	counts := countKinds(components)

	if counts[model.ContextInstructions] != 1 {
		t.Fatalf("instructions components = %d, want 1", counts[model.ContextInstructions])
	}
	if counts[model.ContextHistory] != 3 {
		t.Fatalf("history components = %d, want earlier user, assistant text, assistant tool_use", counts[model.ContextHistory])
	}
	if counts[model.ContextUserPrompt] != 0 {
		t.Fatalf("user prompt components = %d, want 0", counts[model.ContextUserPrompt])
	}
	if counts[model.ContextOther] != 1 {
		t.Fatalf("other components = %d, want latest text only", counts[model.ContextOther])
	}
	if counts[model.ContextToolResult] != 1 {
		t.Fatalf("tool result components = %d, want 1", counts[model.ContextToolResult])
	}
	if counts[model.ContextToolDefinition] != 1 {
		t.Fatalf("tool definition components = %d, want 1", counts[model.ContextToolDefinition])
	}
	for _, c := range components {
		if c.Measurement.Available() && c.Measurement.Kind != model.MeasurementEstimated {
			t.Fatalf("%s kind = %q, want estimated", c.Kind, c.Measurement.Kind)
		}
		if c.Source != string(ProtocolAnthropicMessages) {
			t.Fatalf("%s source = %q, want %q", c.Kind, c.Source, ProtocolAnthropicMessages)
		}
	}
}

func TestAnthropicMessagesObserver_ToolResultIsNotUserPrompt(t *testing.T) {
	body := []byte(`{
		"messages":[
			{"role":"user","content":[
				{"type":"tool_result","content":"result text"},
				{"type":"text","text":"next instruction"}
			]}
		]
	}`)

	summary := SummarizeRequest(Exchange{Request: ExchangeRequest{Components: AnthropicMessagesObserver{}.Parse(body)}})
	if summary.Categories.ToolResult.ValueOrZero() == 0 {
		t.Fatalf("tool result was not attributed")
	}
	if summary.Categories.UserPrompt.Available() {
		t.Fatalf("user prompt = %+v, want unavailable", summary.Categories.UserPrompt)
	}
	if got, want := summary.Categories.Other.ValueOrZero(), estimateTokens("next instruction"); got != want {
		t.Fatalf("other = %d, want %d", got, want)
	}
}

func TestAnthropicMessagesObserver_LatestUserTextIsOtherWithBlockMetadata(t *testing.T) {
	body := []byte(`{
		"system":"system guidance",
		"messages":[
			{"role":"user","content":[{"type":"text","text":"old question"}]},
			{"role":"assistant","content":[{"type":"text","text":"old answer"}]},
			{"role":"user","content":[{"type":"text","text":"Hi thang ngu"}]}
		],
		"tools":[{"name":"Read"}]
	}`)

	components := AnthropicMessagesObserver{}.Parse(body)
	counts := countKinds(components)

	if counts[model.ContextUserPrompt] != 0 {
		t.Fatalf("user prompt components = %d, want 0", counts[model.ContextUserPrompt])
	}
	if counts[model.ContextOther] != 1 {
		t.Fatalf("other components = %d, want latest user text", counts[model.ContextOther])
	}
	if counts[model.ContextHistory] != 2 {
		t.Fatalf("history components = %d, want previous user and assistant", counts[model.ContextHistory])
	}
	if counts[model.ContextInstructions] != 1 {
		t.Fatalf("instructions components = %d, want system", counts[model.ContextInstructions])
	}
	if counts[model.ContextToolDefinition] != 1 {
		t.Fatalf("tool definitions = %d, want 1", counts[model.ContextToolDefinition])
	}

	latest := firstKind(components, model.ContextOther)
	if latest.Record != "messages[2].content[0]" {
		t.Fatalf("latest record = %q, want messages[2].content[0]", latest.Record)
	}
	if latest.Source != string(ProtocolAnthropicMessages) {
		t.Fatalf("latest source = %q, want %q", latest.Source, ProtocolAnthropicMessages)
	}
	if len(latest.Evidence) != 1 {
		t.Fatalf("latest evidence = %d, want 1", len(latest.Evidence))
	}
	ev := latest.Evidence[0]
	if ev.Kind != model.EvidenceProvenance || ev.Source != string(ProtocolAnthropicMessages) || ev.Record != "messages[2].content[0]" || ev.Field != "role=user,type=text" {
		t.Fatalf("latest evidence = %+v", ev)
	}
}

func TestAnthropicMessagesObserver_MixedUnknownAndFileBlocks(t *testing.T) {
	body := []byte(`{
		"messages":[
			{"role":"user","content":[
				{"type":"document","name":"notes.md","content":[{"type":"text","text":"file text"}]},
				{"type":"image","source":{"type":"base64","media_type":"image/png","data":"abc"}},
				{"type":"text","text":"latest ask"}
			]}
		]
	}`)

	components := AnthropicMessagesObserver{}.Parse(body)
	counts := countKinds(components)
	if counts[model.ContextFile] != 1 {
		t.Fatalf("file components = %d, want 1", counts[model.ContextFile])
	}
	if counts[model.ContextOther] != 2 {
		t.Fatalf("other components = %d, want unknown block and latest user text", counts[model.ContextOther])
	}
	other := firstKind(components, model.ContextOther)
	if other.Measurement.Available() {
		t.Fatalf("unknown block measurement = %+v, want unavailable", other.Measurement)
	}
	file := firstKind(components, model.ContextFile)
	if file.Path != "notes.md" {
		t.Fatalf("file path = %q, want notes.md", file.Path)
	}
}

func TestAnthropicMessagesObserver_ContextChangesWithStableTools(t *testing.T) {
	first := SummarizeRequest(Exchange{
		Request: ExchangeRequest{
			Bytes: 200,
			Components: AnthropicMessagesObserver{}.Parse([]byte(`{
				"messages":[{"role":"user","content":"short"}],
				"tools":[{"name":"Read","description":"read a file"}]
			}`)),
		},
	})
	second := SummarizeRequest(Exchange{
		Request: ExchangeRequest{
			Bytes: 500,
			Components: AnthropicMessagesObserver{}.Parse([]byte(`{
				"messages":[{"role":"user","content":"short plus more context in the latest request"}],
				"tools":[{"name":"Read","description":"read a file"}]
			}`)),
		},
	})

	if first.Categories.ToolDefinition.ValueOrZero() != second.Categories.ToolDefinition.ValueOrZero() {
		t.Fatalf("stable tools changed: %d vs %d", first.Categories.ToolDefinition.ValueOrZero(), second.Categories.ToolDefinition.ValueOrZero())
	}
	if first.Attributed.Value.ValueOrZero() == second.Attributed.Value.ValueOrZero() {
		t.Fatalf("attributed total did not change: %d", first.Attributed.Value.ValueOrZero())
	}
	if first.PayloadBytes == first.Attributed.Value.ValueOrZero() || second.PayloadBytes == second.Attributed.Value.ValueOrZero() {
		t.Fatalf("payload bytes leaked into token total")
	}
}

func countKinds(components []model.ContextComponent) map[model.ContextComponentKind]int {
	out := map[model.ContextComponentKind]int{}
	for _, c := range components {
		out[c.Kind]++
	}
	return out
}

func firstKind(components []model.ContextComponent, kind model.ContextComponentKind) model.ContextComponent {
	for _, c := range components {
		if c.Kind == kind {
			return c
		}
	}
	return model.ContextComponent{}
}

func estimateTokens(text string) int64 {
	return ComponentFor(model.ContextOther, 0, text, "").Measurement.ValueOrZero()
}

func TestRecorder_FindProfileIsolation(t *testing.T) {
	dir := t.TempDir()
	rec, err := NewFileRecorder(dir)
	if err != nil {
		t.Fatalf("recorder: %v", err)
	}
	t.Cleanup(func() { _ = rec.Close() })

	rec.Record(Exchange{ID: "gw-1", Profile: "alpha", StartedAt: time.Unix(1, 0)})
	if _, err := rec.FindExchange("beta", "gw-1"); !errors.Is(err, ErrExchangeNotFound) {
		t.Fatalf("expected cross-profile not found, got %v", err)
	}
}
