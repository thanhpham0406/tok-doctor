package gateway

import (
	"sort"
	"strings"

	"github.com/thanhpham0406/tok-doctor/internal/model"
	"github.com/thanhpham0406/tok-doctor/internal/source"
)

type Observer interface {
	Protocol() Protocol
	Parse(body []byte) []model.ContextComponent
}

type MetadataObserver interface {
	ParseMetadata(body []byte) ExchangeRequestMetadata
}

func ObserverFor(name string) Observer {
	switch Protocol(name) {
	case ProtocolAnthropicMessages:
		return AnthropicMessagesObserver{}
	case ProtocolOpenAIResponses:
		return OpenAIResponsesObserver{}
	}
	return nil
}

const ObservationScope = "client_outbound"

func ComponentFor(kind model.ContextComponentKind, position int, text string, path string) model.ContextComponent {
	return model.ContextComponent{
		Kind:        kind,
		Position:    position,
		Path:        path,
		ContentHash: source.ContentHash(text),
		Observation: ObservationScope,
		Measurement: source.EstimatedTextMeasurement(text),
	}
}

func ComponentTools(position int, body []byte) model.ContextComponent {
	text := string(body)
	return model.ContextComponent{
		Kind:        model.ContextToolDefinition,
		Position:    position,
		ContentHash: source.ContentHash(text),
		Observation: ObservationScope,
		Measurement: source.EstimatedTextMeasurement(text),
	}
}

func dedupeComponents(in []model.ContextComponent) []model.ContextComponent {
	seen := map[string]struct{}{}
	out := make([]model.ContextComponent, 0, len(in))
	for _, c := range in {
		key := strings.Join([]string{
			string(c.Kind),
			c.Path,
			c.ContentHash,
		}, "|")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Position < out[j].Position
	})
	return out
}
