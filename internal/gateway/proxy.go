package gateway

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

const MaxCaptureBytes = 4 * 1024 * 1024

type Proxy struct {
	Profile    string
	Source     string
	Protocol   Protocol
	Upstream   *url.URL
	Observer   Observer
	Recorder   Recorder
	HTTPClient *http.Client
	Sink       RecorderSink
}

func NewProxy(profile Profile, observer Observer, recorder Recorder) (*Proxy, error) {
	upstream, err := url.Parse(profile.Upstream)
	if err != nil {
		return nil, fmt.Errorf("parse upstream for %s: %w", profile.Name, err)
	}
	if upstream.Scheme != "http" && upstream.Scheme != "https" {
		return nil, fmt.Errorf("upstream %s must use http or https", profile.Name)
	}
	return &Proxy{
		Profile:    profile.Name,
		Source:     profile.Source,
		Protocol:   Protocol(profile.Protocol),
		Upstream:   upstream,
		Observer:   observer,
		Recorder:   recorder,
		HTTPClient: &http.Client{Timeout: 0},
		Sink:       noopSink{},
	}, nil
}

func (p *Proxy) Handler() http.Handler {
	rp := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.Out.URL = p.upstreamURL(r.In)
			r.Out.Host = ""
			r.Out.Header = r.In.Header.Clone()
		},
		Transport: &http.Transport{
			Proxy:                 nil,
			ResponseHeaderTimeout: 0,
			IdleConnTimeout:       90 * time.Second,
			DisableCompression:    true,
		},
		FlushInterval: -1,
		ErrorHandler:  p.errorHandler,
	}
	rp.ModifyResponse = p.observeResponse
	return p.captureMiddleware(rp)
}

func (p *Proxy) upstreamURL(in *http.Request) *url.URL {
	target := *p.Upstream
	target.Path = singleSlashJoin(p.Upstream.Path, in.URL.Path)
	target.RawQuery = in.URL.RawQuery
	return &target
}

func singleSlashJoin(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}
	return a + b
}

func (p *Proxy) captureMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		body, err := readCappedBody(r.Body)
		if err != nil {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(body)), nil
		}
		r.ContentLength = int64(len(body))

		components := p.parseComponents(body)
		metadata := p.parseMetadata(body)
		exchange := Exchange{
			SchemaVersion: ExchangeSchemaVersion,
			ID:            newExchangeID(),
			Profile:       p.Profile,
			SourceHint:    p.Source,
			Protocol:      p.Protocol,
			StartedAt:     start,
			Upstream:      sanitisedUpstream(p.Upstream),
			Kind:          classifyRequestKind(Protocol(p.Protocol), r.URL.Path, r.Method),
			Outcome:       OutcomeUnknown,
			Request: ExchangeRequest{
				Method:     r.Method,
				Endpoint:   r.URL.Path,
				Bytes:      int64(len(body)),
				BodyHash:   contentHash(body),
				Components: components,
				Metadata:   metadata,
			},
		}
		if p.Observer != nil {
			exchange.Model = extractModel(body, p.Protocol)
		}

		ctx := contextWithExchange(r.Context(), &exchange)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func classifyRequestKind(protocol Protocol, path, method string) RequestKind {
	if method != http.MethodPost {
		return classifyNonModelRequest(protocol, path)
	}
	endpoints, ok := chainEligibleEndpoints[protocol]
	if !ok {
		return RequestKindUnclassified
	}
	if v, has := endpoints[path]; has && v {
		return RequestKindModel
	}
	return classifyNonModelRequest(protocol, path)
}

func classifyNonModelRequest(protocol Protocol, path string) RequestKind {
	if path == "" {
		return RequestKindUnclassified
	}
	endpoints, ok := chainEligibleEndpoints[protocol]
	if !ok {
		return RequestKindUnclassified
	}
	if v, has := endpoints[path]; has && !v {
		return RequestKindNonModel
	}
	return RequestKindUnknown
}

func (p *Proxy) parseComponents(body []byte) []model.ContextComponent {
	if p.Observer == nil || len(body) == 0 {
		return nil
	}
	defer func() {
		_ = recover()
	}()
	components := p.Observer.Parse(body)
	if len(components) == 0 {
		return nil
	}
	return components
}

func (p *Proxy) parseMetadata(body []byte) ExchangeRequestMetadata {
	observer, ok := p.Observer.(MetadataObserver)
	if !ok || len(body) == 0 {
		return ExchangeRequestMetadata{}
	}
	defer func() {
		_ = recover()
	}()
	return observer.ParseMetadata(body)
}

func (p *Proxy) observeResponse(resp *http.Response) error {
	exchange, ok := exchangeFromContext(resp.Request.Context())
	if !ok {
		return nil
	}
	exchange.Response.Status = resp.StatusCode
	stream := strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream")
	exchange.Response.Stream = stream
	exchange.Response.ProviderRequestID = firstNonEmpty(
		resp.Header.Get("x-request-id"),
		resp.Header.Get("request-id"),
		resp.Header.Get("anthropic-request-id"),
	)
	exchange.Response.Model = firstNonEmpty(
		resp.Header.Get("x-model"),
		resp.Header.Get("anthropic-model"),
	)
	exchange.Response.LatencyMs = time.Since(exchange.StartedAt).Milliseconds()

	if !stream {
		prefix, err := p.pipeNonStreamBody(resp)
		if err != nil {
			exchange.Outcome = OutcomeTransportFailure
			p.recordExchange(exchange)
			return err
		}
		if respObserver, hasResp := p.Observer.(ResponseObserver); hasResp && len(prefix) > 0 {
			if meta := respObserver.ParseResponse(prefix); meta != nil {
				if meta.ResponseObjectID != "" && exchange.Response.ResponseObjectID == "" {
					exchange.Response.ResponseObjectID = meta.ResponseObjectID
				}
				if meta.OpenAIResponses != nil {
					exchange.Response.OpenAIResponses = meta.OpenAIResponses
				}
			}
			if usage := respObserver.ParseResponseUsage(prefix); usage != nil {
				exchange.Response.ProviderUsage = usage
			}
		}
		exchange.Outcome = outcomeFromStatus(resp.StatusCode)
		ProviderUsageToObservedApply(exchange)
		p.recordExchange(exchange)
		return nil
	}

	if streamObserver, hasStream := p.Observer.(StreamUsageObserver); hasStream {
		wrapper := newStreamObserver(streamObserver, resp.Body, p.Recorder, p.Sink, exchange)
		resp.Body = wrapper
		exchange.Outcome = OutcomeUpstreamOK
		ProviderUsageToObservedApply(exchange)
		return nil
	}

	exchange.Outcome = outcomeFromStatus(resp.StatusCode)
	ProviderUsageToObservedApply(exchange)
	p.recordExchange(exchange)
	return nil
}

// pipeNonStreamBody reads the upstream response body once, captures a
// bounded observation prefix, and re-wraps the rest so the reverse proxy
// forwards the full body unchanged. The captured prefix is stashed on the
// request context for usage parsing.
func (p *Proxy) pipeNonStreamBody(resp *http.Response) ([]byte, error) {
	if resp == nil || resp.Body == nil {
		return nil, nil
	}
	full, err := io.ReadAll(resp.Body)
	if err != nil {
		_ = resp.Body.Close()
		return nil, err
	}
	_ = resp.Body.Close()
	prefix := full
	if len(prefix) > MaxCaptureBytes {
		prefix = prefix[:MaxCaptureBytes]
	}
	resp.Body = io.NopCloser(bytes.NewReader(full))
	resp.ContentLength = int64(len(full))
	if resp.Header != nil {
		resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(full)))
	}
	if resp.Request != nil {
		ctx := resp.Request.Context()
		if existing, ok := ctx.Value(prefixAttachmentKey{}).(*prefixAttachment); ok && existing != nil {
			existing.prefix = prefix
			existing.full = full
		} else {
			ctx = context.WithValue(ctx, prefixAttachmentKey{}, &prefixAttachment{prefix: prefix, full: full})
			resp.Request = resp.Request.WithContext(ctx)
		}
	}
	return prefix, nil
}

type prefixAttachmentKey struct{}

type prefixAttachment struct {
	prefix []byte
	full   []byte
}

func peekCapturedBody(resp *http.Response) ([]byte, bool) {
	if resp == nil || resp.Request == nil {
		return nil, false
	}
	if att := resp.Request.Context().Value(prefixAttachmentKey{}); att != nil {
		if a, ok := att.(*prefixAttachment); ok {
			return a.prefix, true
		}
	}
	return nil, false
}

func outcomeFromStatus(status int) RequestOutcome {
	if status >= 200 && status < 400 {
		return OutcomeUpstreamOK
	}
	if status == http.StatusBadGateway || status >= 500 {
		return OutcomeUpstreamHTTPError
	}
	return OutcomeUpstreamHTTPError
}

func (p *Proxy) recordExchange(exchange *Exchange) {
	if exchange == nil || p.Recorder == nil {
		return
	}
	if err := p.Recorder.Record(*exchange); err != nil {
		p.Sink.RecordRecorderFailure(exchange.Profile, asRecorderError(err))
	}
}

func asRecorderError(err error) *RecorderError {
	if err == nil {
		return nil
	}
	var re *RecorderError
	if errors.As(err, &re) {
		return re
	}
	return &RecorderError{Profile: "", Op: "record", Err: err}
}

func (p *Proxy) errorHandler(w http.ResponseWriter, r *http.Request, err error) {
	exchange, ok := exchangeFromContext(r.Context())
	if !ok {
		http.Error(w, "upstream gateway unreachable", http.StatusBadGateway)
		return
	}
	exchange.Response.Status = http.StatusBadGateway
	exchange.Response.LatencyMs = time.Since(exchange.StartedAt).Milliseconds()
	if errors.Is(err, context.Canceled) {
		exchange.Outcome = OutcomeClientCanceled
	} else {
		exchange.Outcome = OutcomeTransportFailure
	}
	p.recordExchange(exchange)
	http.Error(w, "upstream gateway unreachable", http.StatusBadGateway)
}

func readCappedBody(r io.ReadCloser) ([]byte, error) {
	if r == nil {
		return nil, nil
	}
	defer func() { _ = r.Close() }()
	buf := bytes.NewBuffer(nil)
	limited := io.LimitReader(r, MaxCaptureBytes+1)
	if _, err := io.Copy(buf, limited); err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
	}
	if buf.Len() > MaxCaptureBytes {
		return nil, fmt.Errorf("request body exceeds %d bytes", MaxCaptureBytes)
	}
	return buf.Bytes(), nil
}

func contentHash(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func newExchangeID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("gw-%d", time.Now().UnixNano())
	}
	return "gw-" + hex.EncodeToString(b[:])
}

func sanitisedUpstream(u *url.URL) string {
	if u.User != nil {
		stripped := *u
		stripped.User = nil
		return stripped.String()
	}
	return u.String()
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func extractModel(body []byte, protocol Protocol) string {
	if len(body) == 0 {
		return ""
	}
	switch protocol {
	case ProtocolAnthropicMessages:
		var s struct {
			Model string `json:"model"`
		}
		_ = jsonUnmarshal(body, &s)
		return s.Model
	case ProtocolOpenAIResponses:
		var s struct {
			Model string `json:"model"`
		}
		_ = jsonUnmarshal(body, &s)
		return s.Model
	}
	return ""
}

func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func IsLoopbackHost(host string) bool {
	if host == "" {
		return false
	}
	if strings.HasPrefix(host, "127.") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

func IsLoopbackAddress(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if host == "" {
		return true
	}
	return IsLoopbackHost(host)
}

type prefixBuffer struct {
	mu     []byte
	limit  int
	trunc  bool
	closed atomic.Bool
}

func newPrefixBuffer(limit int) *prefixBuffer {
	return &prefixBuffer{limit: limit}
}

func (p *prefixBuffer) Write(b []byte) (int, error) {
	if p.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	remaining := p.limit - len(p.mu)
	if remaining <= 0 {
		p.trunc = true
		return len(b), nil
	}
	if len(b) > remaining {
		p.mu = append(p.mu, b[:remaining]...)
		p.trunc = true
		return len(b), nil
	}
	p.mu = append(p.mu, b...)
	return len(b), nil
}

func (p *prefixBuffer) Bytes() []byte {
	if p == nil {
		return nil
	}
	return append([]byte(nil), p.mu...)
}

func (p *prefixBuffer) Truncated() bool { return p.trunc }

func (p *prefixBuffer) Close() error {
	p.closed.Store(true)
	return nil
}
