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

	RequestBodyHook func(profile string, body []byte) error
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
			ID:         newExchangeID(),
			Profile:    p.Profile,
			SourceHint: p.Source,
			Protocol:   p.Protocol,
			StartedAt:  start,
			Upstream:   sanitisedUpstream(p.Upstream),
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
	exchange.Response.Stream = strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream")
	exchange.Response.ResponseID = firstNonEmpty(
		resp.Header.Get("x-request-id"),
		resp.Header.Get("request-id"),
		resp.Header.Get("anthropic-request-id"),
	)
	exchange.Response.Model = firstNonEmpty(
		resp.Header.Get("x-model"),
		resp.Header.Get("anthropic-model"),
	)
	exchange.Response.LatencyMs = time.Since(exchange.StartedAt).Milliseconds()

	if usage := parseUsageFromHeaders(resp.Header); usage != nil {
		usage.Source = model.MeasurementDerived
		exchange.Response.Usage = usage
	}

	p.Recorder.Record(*exchange)
	return nil
}

func (p *Proxy) errorHandler(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, context.Canceled) {
		return
	}
	if exchange, ok := exchangeFromContext(r.Context()); ok {
		exchange.Response.Status = http.StatusBadGateway
		exchange.Response.Finish = err.Error()
		exchange.Response.LatencyMs = time.Since(exchange.StartedAt).Milliseconds()
		p.Recorder.Record(*exchange)
	}
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

func parseUsageFromHeaders(h http.Header) *ObservedUsage {
	if h.Get("x-usage-input-tokens") == "" && h.Get("anthropic-input-tokens") == "" {
		return nil
	}
	parse := func(a, b string) int64 {
		v := firstNonEmpty(h.Get(a), h.Get(b))
		if v == "" {
			return 0
		}
		var n int64
		_, _ = fmt.Sscanf(v, "%d", &n)
		return n
	}
	return &ObservedUsage{
		Input:     parse("x-usage-input-tokens", "anthropic-input-tokens"),
		Output:    parse("x-usage-output-tokens", "anthropic-output-tokens"),
		Cached:    parse("x-usage-cached-input-tokens", "anthropic-cached-input-tokens"),
		Reasoning: parse("x-usage-reasoning-tokens", "anthropic-reasoning-tokens"),
		Total:     parse("x-usage-total-tokens", "anthropic-total-tokens"),
	}
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
