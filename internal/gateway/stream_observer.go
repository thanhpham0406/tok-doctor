package gateway

import (
	"bytes"
	"io"
	"sync"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

type streamObserver struct {
	observer StreamUsageObserver
	upstream io.ReadCloser
	exchange *Exchange
	recorder Recorder

	once  sync.Once
	mu    sync.Mutex
	state any

	usage        *ProviderUsage
	terminal     bool
	overran      bool
	recordCalled bool
	maxEvent     int

	pending []byte
}

func newStreamObserver(observer StreamUsageObserver, upstream io.ReadCloser, recorder Recorder, exchange *Exchange) *streamObserver {
	max := observer.MaxStreamEventBytes()
	if max <= 0 {
		max = maxSSEResponseEventBytes
	}
	return &streamObserver{
		observer: observer,
		upstream: upstream,
		exchange: exchange,
		recorder: recorder,
		state:    observer.NewStreamState(),
		maxEvent: max,
	}
}

func (s *streamObserver) Read(p []byte) (int, error) {
	n, err := s.upstream.Read(p)
	if n > 0 {
		s.observe(p[:n])
	}
	if err == io.EOF {
		s.finalize()
	}
	return n, err
}

func (s *streamObserver) observe(chunk []byte) {
	if s.overran || len(chunk) == 0 {
		return
	}
	s.pending = append(s.pending, chunk...)
	s.drainFrames()
}

func (s *streamObserver) drainFrames() {
	for {
		_, end, hasFrame := nextSSEFrame(s.pending)
		if !hasFrame {
			if len(s.pending) > s.maxEvent {
				s.overran = true
				s.pending = nil
			}
			return
		}
		frame := s.pending[:end]
		s.pending = s.pending[end:]
		s.feed(frame)
	}
}

func (s *streamObserver) feed(frame []byte) {
	if s.overran {
		return
	}
	if len(frame) > s.maxEvent {
		s.overran = true
		return
	}
	for _, payload := range extractDataPayloads(frame) {
		s.mu.Lock()
		usage, terminal := s.observer.ParseStreamFrame(s.state, payload)
		if terminal {
			s.terminal = true
		}
		if usage != nil {
			s.usage = usage
		}
		s.mu.Unlock()
	}
}

func extractDataPayloads(frame []byte) [][]byte {
	var out [][]byte
	for _, line := range splitSSELines(frame) {
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(line[len("data:"):])
		if len(payload) == 0 {
			continue
		}
		out = append(out, payload)
	}
	return out
}

func nextSSEFrame(buf []byte) (_, end int, hasFrame bool) {
	if i := bytes.Index(buf, []byte("\n\n")); i >= 0 {
		return 0, i + 2, true
	}
	if i := bytes.Index(buf, []byte("\r\n\r\n")); i >= 0 {
		return 0, i + 4, true
	}
	return 0, 0, false
}

func (s *streamObserver) finalize() {
	s.once.Do(func() {
		s.drainFrames()
		s.mu.Lock()
		usage := s.usage
		terminal := s.terminal
		s.pending = nil
		s.mu.Unlock()
		if s.exchange == nil {
			return
		}
		if usage != nil {
			s.exchange.Response.ProviderUsage = usage
		}
		if !terminal && streamRequiresTerminal(s.observer) {
			s.exchange.Outcome = OutcomeStreamTruncated
			if s.exchange.Response.Usage == nil {
				s.exchange.Response.Usage = &ObservedUsage{Source: providerUsageKind(usage)}
			}
			s.exchange.Response.Usage.Truncated = true
		} else {
			s.exchange.Outcome = OutcomeUpstreamOK
		}
		ProviderUsageToObservedApply(s.exchange)
		s.record()
	})
}

func (s *streamObserver) record() {
	if s.recorder == nil || s.recordCalled {
		return
	}
	s.recordCalled = true
	_ = s.recorder.Record(*s.exchange)
}

func (s *streamObserver) Close() error {
	s.finalize()
	if s.upstream == nil {
		return nil
	}
	err := s.upstream.Close()
	s.upstream = nil
	return err
}

func (s *streamObserver) Usage() *ProviderUsage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.usage
}

func streamRequiresTerminal(observer StreamUsageObserver) bool {
	switch observer.(type) {
	case AnthropicMessagesObserver:
		return true
	default:
		return false
	}
}

// ProviderUsageToObservedApply writes a derived ObservedUsage into the
// exchange without overwriting a field the caller has already populated.
func ProviderUsageToObservedApply(exchange *Exchange) {
	if exchange == nil || exchange.Response.ProviderUsage == nil {
		return
	}
	observed := ProviderUsageToObserved(exchange.Response.ProviderUsage)
	if exchange.Response.Usage == nil {
		exchange.Response.Usage = &observed
		return
	}
	if exchange.Response.Usage.RawInput == 0 && observed.RawInput != 0 {
		exchange.Response.Usage.RawInput = observed.RawInput
	}
	if exchange.Response.Usage.Cached == 0 && observed.Cached != 0 {
		exchange.Response.Usage.Cached = observed.Cached
	}
	if exchange.Response.Usage.CacheCreation == 0 && observed.CacheCreation != 0 {
		exchange.Response.Usage.CacheCreation = observed.CacheCreation
	}
	if exchange.Response.Usage.Output == 0 && observed.Output != 0 {
		exchange.Response.Usage.Output = observed.Output
	}
	if exchange.Response.Usage.Reasoning == 0 && observed.Reasoning != 0 {
		exchange.Response.Usage.Reasoning = observed.Reasoning
	}
}

func providerUsageKind(p *ProviderUsage) model.MeasurementKind {
	if p == nil {
		return model.MeasurementDerived
	}
	return model.MeasurementMeasured
}
