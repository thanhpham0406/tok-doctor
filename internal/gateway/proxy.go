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
	"hash"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
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
		capture, err := p.captureRequest(r)
		if err != nil {
			http.Error(w, "read request body", http.StatusBadRequest)
			return
		}

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
				Method:   r.Method,
				Endpoint: r.URL.Path,
				Bytes:    capture.observedBytes(),
				Capture:  capture.status,
			},
		}
		// A truncated body is a JSON prefix, so parsing it would invent
		// components and usage the client never sent.
		if !capture.status.Truncated() {
			exchange.Request.BodyHash = contentHash(capture.prefix)
			exchange.Request.Components = p.parseComponents(capture.prefix)
			exchange.Request.Metadata = p.parseMetadata(capture.prefix)
			if p.Observer != nil {
				exchange.Model = extractModel(capture.prefix, p.Protocol)
			}
		}

		ctx := contextWithForwardedBody(r.Context(), capture.forwarded)
		ctx = contextWithExchange(ctx, &exchange)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type requestCapture struct {
	prefix    []byte
	forwarded *forwardedBody
	status    *CaptureStatus
}

// observedBytes reports the request size known before forwarding. A truncated
// body has no measured size yet, so only the declared length is usable and the
// streamed count replaces it once forwarding completes.
func (c requestCapture) observedBytes() int64 {
	if c.status.Truncated() {
		return c.status.TotalBytes
	}
	return int64(len(c.prefix))
}

// captureRequest buffers at most MaxCaptureBytes of the request body for
// observation and keeps the forwarded body byte for byte identical. A body
// above the limit is streamed behind the captured prefix, so a large request
// is neither rejected nor held in memory in full.
func (p *Proxy) captureRequest(r *http.Request) (requestCapture, error) {
	if r.Body == nil || r.Body == http.NoBody {
		return requestCapture{status: &CaptureStatus{State: model.ContextCompletenessComplete}}, nil
	}

	buf := bytes.NewBuffer(make([]byte, 0, 64*1024))
	if _, err := io.Copy(buf, io.LimitReader(r.Body, MaxCaptureBytes+1)); err != nil {
		_ = r.Body.Close()
		return requestCapture{}, fmt.Errorf("read request body: %w", err)
	}
	captured := buf.Bytes()
	prefix := captured
	state := model.ContextCompletenessComplete
	if len(captured) > MaxCaptureBytes {
		prefix = captured[:MaxCaptureBytes]
		state = model.ContextCompletenessTruncated
	}
	if p.Observer == nil {
		state = model.ContextCompletenessUnavailable
		prefix = nil
	}

	status := &CaptureStatus{
		State:         state,
		CapturedBytes: int64(len(prefix)),
		LimitBytes:    MaxCaptureBytes,
	}
	if r.ContentLength >= 0 {
		status.TotalBytes = r.ContentLength
	}

	forwarded := newForwardedBody(io.MultiReader(bytes.NewReader(captured), r.Body), r.Body)
	r.Body = forwarded
	r.GetBody = nil
	if !status.Truncated() {
		r.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(prefix)), nil
		}
	}
	return requestCapture{prefix: prefix, forwarded: forwarded, status: status}, nil
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
		prefix, capture, err := p.captureResponseBody(resp)
		if err != nil {
			exchange.Outcome = OutcomeTransportFailure
			p.recordExchange(resp.Request.Context(), exchange)
			return err
		}
		exchange.Response.Capture = capture
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
		p.recordExchange(resp.Request.Context(), exchange)
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
	p.recordExchange(resp.Request.Context(), exchange)
	return nil
}

// captureResponseBody buffers at most MaxCaptureBytes of the upstream
// response for observation and re-wraps the body so the reverse proxy still
// forwards every byte. A response above the limit is streamed rather than
// held in memory, and its capture is reported as truncated so no consumer
// treats a partial read as the whole answer.
func (p *Proxy) captureResponseBody(resp *http.Response) ([]byte, *CaptureStatus, error) {
	if resp == nil || resp.Body == nil {
		return nil, &CaptureStatus{State: model.ContextCompletenessUnavailable}, nil
	}
	buf := bytes.NewBuffer(make([]byte, 0, 64*1024))
	if _, err := io.Copy(buf, io.LimitReader(resp.Body, MaxCaptureBytes+1)); err != nil {
		_ = resp.Body.Close()
		return nil, nil, fmt.Errorf("read upstream response body: %w", err)
	}
	captured := buf.Bytes()
	prefix := captured
	state := model.ContextCompletenessComplete
	if len(captured) > MaxCaptureBytes {
		prefix = captured[:MaxCaptureBytes]
		state = model.ContextCompletenessTruncated
	}
	status := &CaptureStatus{
		State:         state,
		CapturedBytes: int64(len(prefix)),
		LimitBytes:    MaxCaptureBytes,
	}
	if resp.ContentLength >= 0 {
		status.TotalBytes = resp.ContentLength
	}
	resp.Body = readCloser{Reader: io.MultiReader(bytes.NewReader(captured), resp.Body), Closer: resp.Body}
	// The forwarded length is unchanged, so upstream Content-Length stays
	// valid. Only a body we fully buffered can be re-declared.
	if !status.Truncated() && resp.Header != nil {
		resp.ContentLength = int64(len(captured))
		resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(captured)))
	}
	return prefix, status, nil
}

// readCloser pairs a reassembled body with the upstream body it came from so
// closing the forwarded stream still closes the upstream connection.
type readCloser struct {
	io.Reader
	io.Closer
}

// forwardedBody counts and hashes the bytes actually written upstream, so the
// exchange reports the true request size and hash without ever buffering a
// large body. The hash is only usable once the body reached EOF.
type forwardedBody struct {
	reader   io.Reader
	closer   io.Closer
	hash     hash.Hash
	bytes    int64
	complete bool
}

func newForwardedBody(reader io.Reader, closer io.Closer) *forwardedBody {
	return &forwardedBody{reader: reader, closer: closer, hash: sha256.New()}
}

func (b *forwardedBody) Read(p []byte) (int, error) {
	n, err := b.reader.Read(p)
	if n > 0 {
		b.bytes += int64(n)
		_, _ = b.hash.Write(p[:n])
	}
	if err == io.EOF {
		b.complete = true
	}
	return n, err
}

func (b *forwardedBody) Close() error { return b.closer.Close() }

// forwarded reports the byte count and body hash of a body that was read to
// the end. A body that was not fully forwarded reports nothing, because a
// partial count and a partial hash would both be indistinguishable from the
// real request.
func (b *forwardedBody) forwarded() (int64, string, bool) {
	if !b.complete {
		return 0, "", false
	}
	return b.bytes, contentHashDigest(b.hash), true
}

func contentHashDigest(digest hash.Hash) string {
	sum := digest.Sum(nil)
	return "sha256:" + hex.EncodeToString(sum)
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

func (p *Proxy) recordExchange(ctx context.Context, exchange *Exchange) {
	if exchange == nil || p.Recorder == nil {
		return
	}
	applyForwardedRequest(ctx, exchange)
	if err := p.Recorder.Record(*exchange); err != nil {
		p.Sink.RecordRecorderFailure(exchange.Profile, asRecorderError(err))
	}
}

// applyForwardedRequest replaces the request size and hash estimated from the
// captured prefix with the values measured while streaming to upstream.
func applyForwardedRequest(ctx context.Context, exchange *Exchange) {
	body, ok := forwardedBodyFromContext(ctx)
	if !ok || body == nil {
		return
	}
	size, bodyHash, complete := body.forwarded()
	if !complete {
		return
	}
	exchange.Request.Bytes = size
	exchange.Request.BodyHash = bodyHash
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
	p.recordExchange(r.Context(), exchange)
	http.Error(w, "upstream gateway unreachable", http.StatusBadGateway)
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
