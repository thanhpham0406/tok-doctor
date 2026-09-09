package gateway

import (
	"bytes"
	"io"
	"sync"
)

type streamObserver struct {
	observer StreamUsageObserver
	upstream io.ReadCloser
	exchange *Exchange
	recorder Recorder

	once        sync.Once
	mu          sync.Mutex
	usage       *ProviderUsage
	sawTerminal bool
	overran     bool

	maxEvent int

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
	if len(frame) == 0 {
		return
	}
	usage := s.observer.ParseStreamEvent(frame)
	if usage == nil {
		return
	}
	s.mu.Lock()
	if !s.sawTerminal {
		s.usage = usage
		s.sawTerminal = true
	}
	s.mu.Unlock()
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
		s.pending = nil
		s.mu.Unlock()
		if s.exchange == nil || s.recorder == nil {
			return
		}
		if usage != nil {
			s.exchange.Response.ProviderUsage = usage
			ApplyProviderUsageToExchange(s.exchange)
		}
		s.recorder.Record(*s.exchange)
	})
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
