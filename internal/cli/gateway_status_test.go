package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/thanhpham0406/tok-doctor/internal/gateway"
)

func runGatewayStatus(t *testing.T, args ...string) string {
	t.Helper()
	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs(append([]string{"gateway", "status"}, args...))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute gateway status: %v", err)
	}
	return stdout.String()
}

func writeGatewayConfig(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func freeLoopbackAddr(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve loopback port: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release loopback port: %v", err)
	}
	return addr
}

func statusRows(out string) (string, []string) {
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 2 {
		return "", nil
	}
	return lines[0], lines[2:]
}

func statusCell(t *testing.T, header, row, column string, width int) string {
	t.Helper()
	offset := strings.Index(header, column)
	if offset < 0 {
		t.Fatalf("header = %q, want column %q", header, column)
	}
	cells := []rune(row)
	at := utf8.RuneCountInString(header[:offset])
	if len(cells) < at+width {
		return strings.TrimSpace(string(cells[at:]))
	}
	return strings.TrimSpace(string(cells[at : at+width]))
}

func TestGatewayStatusShowsUpstreamColumn(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	t.Setenv("TOKDOCTOR_CONFIG", configPath)
	t.Setenv("TOKDOCTOR_GATEWAY_DIR", filepath.Join(dir, "gateway"))
	writeGatewayConfig(t, configPath, `
[gateway.profiles.codex]
enabled = "true"
listen = "127.0.0.1:8787"
protocol = "openai_responses"
source = "codex"
upstream = "https://api.openai.com/v1"

[gateway.profiles.claude]
enabled = "true"
listen = "127.0.0.1:8788"
protocol = "anthropic_messages"
source = "claude"
upstream = "https://api.anthropic.com"
`)

	out := runGatewayStatus(t)
	header, rows := statusRows(out)
	for _, want := range []string{"Profile", "State", "Proxy", "Upstream", "Requests", "Last request"} {
		if !strings.Contains(header, want) {
			t.Fatalf("header = %q, want %q", header, want)
		}
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %v, want 2 profiles", rows)
	}
	wantUpstreams := map[string]string{
		"codex":  "https://api.openai.com/v1",
		"claude": "https://api.anthropic.com",
	}
	for _, row := range rows {
		profile := statusCell(t, header, row, "Profile", 15)
		if got := statusCell(t, header, row, "Upstream", 26); got != wantUpstreams[profile] {
			t.Fatalf("profile %s upstream = %q, want %q\n%s", profile, got, wantUpstreams[profile], out)
		}
		if got := statusCell(t, header, row, "State", 7); got != "stopped" {
			t.Fatalf("profile %s state = %q, want stopped", profile, got)
		}
		if got := statusCell(t, header, row, "Requests", 9); got != "0" {
			t.Fatalf("profile %s requests = %q, want 0", profile, got)
		}
	}
	if !strings.Contains(out, "http://127.0.0.1:8787") || !strings.Contains(out, "http://127.0.0.1:8788") {
		t.Fatalf("output = %q, want proxy URLs", out)
	}
}

func TestGatewayStatusShowsUpstreamForRunningProfile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	captureDir := filepath.Join(dir, "gateway")
	t.Setenv("TOKDOCTOR_CONFIG", configPath)
	t.Setenv("TOKDOCTOR_GATEWAY_DIR", captureDir)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	listen := freeLoopbackAddr(t)
	writeGatewayConfig(t, configPath, fmt.Sprintf(`
[gateway.profiles.live]
enabled = "true"
listen = %q
protocol = "openai_responses"
source = "codex"
upstream = %q
`, listen, upstream.URL))

	recorder, err := gateway.NewFileRecorder(captureDir)
	if err != nil {
		t.Fatalf("recorder: %v", err)
	}
	t.Cleanup(func() { _ = recorder.Close() })
	runtime := gateway.NewRuntime(recorder)
	if err := runtime.Start(context.Background(), []gateway.Profile{{
		Name:     "live",
		Enabled:  true,
		Listen:   listen,
		Protocol: string(gateway.ProtocolOpenAIResponses),
		Source:   "codex",
		Upstream: upstream.URL,
	}}); err != nil {
		t.Fatalf("start runtime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Shutdown(context.Background()) })

	out := runGatewayStatus(t)
	header, rows := statusRows(out)
	if len(rows) != 1 {
		t.Fatalf("rows = %v, want 1 profile\n%s", rows, out)
	}
	if got := statusCell(t, header, rows[0], "State", 7); got != "running" {
		t.Fatalf("state = %q, want running\n%s", got, out)
	}
	if got := statusCell(t, header, rows[0], "Upstream", 26); got != upstream.URL {
		t.Fatalf("upstream = %q, want %q\n%s", got, upstream.URL, out)
	}
}

func TestGatewayStatusRedactsUpstreamCredentials(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	t.Setenv("TOKDOCTOR_CONFIG", configPath)
	t.Setenv("TOKDOCTOR_GATEWAY_DIR", filepath.Join(dir, "gateway"))
	writeGatewayConfig(t, configPath, `
[gateway.profiles.codex]
enabled = "true"
listen = "127.0.0.1:8787"
protocol = "openai_responses"
source = "codex"
upstream = "https://router-user:router-secret@router.internal/v1?api_key=sk-live-abc#frag"
`)

	out := runGatewayStatus(t)
	header, rows := statusRows(out)
	if len(rows) != 1 {
		t.Fatalf("rows = %v, want 1 profile", rows)
	}
	if got := statusCell(t, header, rows[0], "Upstream", 26); got != "https://router.internal/v1" {
		t.Fatalf("upstream = %q, want scheme://host/path\n%s", got, out)
	}
	for _, secret := range []string{"router-user", "router-secret", "api_key", "sk-live-abc", "frag", "?"} {
		if strings.Contains(out, secret) {
			t.Fatalf("status output leaks %q: %s", secret, out)
		}
	}
}

func TestGatewayStatusJSONKeepsUpstreamFieldRedacted(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	t.Setenv("TOKDOCTOR_CONFIG", configPath)
	t.Setenv("TOKDOCTOR_GATEWAY_DIR", filepath.Join(dir, "gateway"))
	writeGatewayConfig(t, configPath, `
[gateway.profiles.codex]
enabled = "true"
listen = "127.0.0.1:8787"
protocol = "openai_responses"
source = "codex"
upstream = "https://router-user:router-secret@router.internal/v1?api_key=sk-live-abc#frag"
`)

	out := runGatewayStatus(t, "--format", "json")
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("decode json: %v\n%s", err, out)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	for _, key := range []string{"Profile", "State", "proxy", "Upstream", "Requests", "LastSeen"} {
		if _, ok := rows[0][key]; !ok {
			t.Fatalf("json row = %v, want key %q", rows[0], key)
		}
	}
	if got := rows[0]["Upstream"]; got != "https://router.internal/v1" {
		t.Fatalf("json upstream = %v, want redacted scheme://host/path", got)
	}
	for _, secret := range []string{"router-user", "router-secret", "api_key", "sk-live-abc", "frag"} {
		if strings.Contains(out, secret) {
			t.Fatalf("json output leaks %q: %s", secret, out)
		}
	}
}

func TestGatewayStatusTruncatesLongUpstream(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	t.Setenv("TOKDOCTOR_CONFIG", configPath)
	t.Setenv("TOKDOCTOR_GATEWAY_DIR", filepath.Join(dir, "gateway"))
	long := "https://gateway.internal.example.com/openai/deployments/very-long-path-name"
	writeGatewayConfig(t, configPath, fmt.Sprintf(`
[gateway.profiles.codex]
enabled = "true"
listen = "127.0.0.1:8787"
protocol = "openai_responses"
source = "codex"
upstream = %q

[gateway.profiles.claude]
enabled = "true"
listen = "127.0.0.1:8788"
protocol = "anthropic_messages"
source = "claude"
upstream = "https://api.anthropic.com"
`, long))

	out := runGatewayStatus(t)
	header, rows := statusRows(out)
	if len(rows) != 2 {
		t.Fatalf("rows = %v, want 2 profiles", rows)
	}
	for _, row := range rows {
		if utf8.RuneCountInString(row) != utf8.RuneCountInString(rows[0]) {
			t.Fatalf("row = %q is not aligned with %q, layout broken\n%s", row, rows[0], out)
		}
		if got := statusCell(t, header, row, "Requests", 9); got != "0" {
			t.Fatalf("requests column shifted: %q in %q", got, row)
		}
	}
	for _, row := range rows {
		cell := statusCell(t, header, row, "Upstream", 26)
		switch statusCell(t, header, row, "Profile", 15) {
		case "codex":
			if !strings.HasPrefix(cell, "https://gateway.internal") || !strings.HasSuffix(cell, "…") {
				t.Fatalf("upstream cell = %q, want truncated gateway host", cell)
			}
		case "claude":
			if cell != "https://api.anthropic.com" {
				t.Fatalf("upstream cell = %q, want full short URL", cell)
			}
		}
	}
}

func TestGatewayStatusKeepsRequestsAndLastRequest(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	captureDir := filepath.Join(dir, "gateway")
	t.Setenv("TOKDOCTOR_CONFIG", configPath)
	t.Setenv("TOKDOCTOR_GATEWAY_DIR", captureDir)
	writeGatewayConfig(t, configPath, `
[gateway.profiles.codex]
enabled = "true"
listen = "127.0.0.1:8787"
protocol = "openai_responses"
source = "codex"
upstream = "https://api.openai.com"
`)

	seedGatewayCapture(t, captureDir, []gateway.Exchange{
		{
			ID:        "gw-status-1",
			Profile:   "codex",
			Model:     "gpt-5",
			StartedAt: time.Now().Add(-2 * time.Minute),
			Protocol:  gateway.ProtocolOpenAIResponses,
		},
		{
			ID:        "gw-status-2",
			Profile:   "codex",
			Model:     "gpt-5",
			StartedAt: time.Now().Add(-time.Minute),
			Protocol:  gateway.ProtocolOpenAIResponses,
		},
	})

	out := runGatewayStatus(t)
	header, rows := statusRows(out)
	if len(rows) != 1 {
		t.Fatalf("rows = %v, want 1 profile", rows)
	}
	if got := statusCell(t, header, rows[0], "Requests", 9); got != "2" {
		t.Fatalf("requests = %q, want 2\n%s", got, out)
	}
	last := statusCell(t, header, rows[0], "Last request", 13)
	if last == "" || last == "—" {
		t.Fatalf("last request = %q, want rendered timestamp for captured requests\n%s", last, out)
	}
	if got := statusCell(t, header, rows[0], "Proxy", 23); got != "http://127.0.0.1:8787" {
		t.Fatalf("proxy = %q, want configured proxy URL", got)
	}
}

func TestSafeUpstreamURL(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty", raw: "", want: ""},
		{name: "blank", raw: "   ", want: ""},
		{name: "relative", raw: "api.openai.com", want: ""},
		{name: "plain", raw: "https://api.openai.com", want: "https://api.openai.com"},
		{name: "path", raw: "https://api.openai.com/v1/", want: "https://api.openai.com/v1/"},
		{name: "userinfo", raw: "https://user:secret@api.openai.com", want: "https://api.openai.com"},
		{name: "query", raw: "https://api.openai.com/v1?key=secret", want: "https://api.openai.com/v1"},
		{name: "fragment", raw: "https://api.openai.com/v1#token", want: "https://api.openai.com/v1"},
		{name: "port", raw: "http://127.0.0.1:20128/v1", want: "http://127.0.0.1:20128/v1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := safeUpstreamURL(test.raw); got != test.want {
				t.Fatalf("safeUpstreamURL(%q) = %q, want %q", test.raw, got, test.want)
			}
		})
	}
}

func TestUpstreamColumnValueFallsBackToDash(t *testing.T) {
	for _, raw := range []string{"", "   ", "api.openai.com", "not a url"} {
		if got := upstreamColumnValue(raw); got != "-" {
			t.Fatalf("upstreamColumnValue(%q) = %q, want -", raw, got)
		}
	}
	if got := upstreamColumnValue("https://api.openai.com"); got != "https://api.openai.com" {
		t.Fatalf("upstreamColumnValue = %q, want safe URL", got)
	}
}
