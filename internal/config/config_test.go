package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetLoadResetSource(t *testing.T) {
	store := NewStoreAt(filepath.Join(t.TempDir(), "config.toml"))

	if err := store.SetSource("codex", Source{Path: "/tmp/codex"}); err != nil {
		t.Fatalf("SetSource codex: %v", err)
	}
	if err := store.SetSource("claude", Source{Path: "/tmp/claude"}); err != nil {
		t.Fatalf("SetSource claude: %v", err)
	}

	cfg, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Sources["codex"].Path != "/tmp/codex" {
		t.Fatalf("codex path = %q", cfg.Sources["codex"].Path)
	}

	if err := store.ResetSource("codex"); err != nil {
		t.Fatalf("ResetSource: %v", err)
	}
	cfg, err = store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := cfg.Sources["codex"]; ok {
		t.Fatalf("expected codex to be removed")
	}
}

func TestLoadGatewayProfiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := `
[pricing]
override_path = "/tmp/overrides.toml"

[sources.codex]
path = "/tmp/codex"

[gateway.profiles.claude-minimax]
enabled = "true"
listen = "127.0.0.1:8788"
protocol = "anthropic_messages"
source = "claude"
upstream = "https://router.example.com"
provider = "anthropic"

[gateway.profiles.codex-personal]
enabled = "false"
listen = "127.0.0.1:8787"
protocol = "openai_responses"
source = "codex"
upstream = "http://127.0.0.1:20128"
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	store := NewStoreAt(path)
	cfg, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Pricing.OverridePath != "/tmp/overrides.toml" {
		t.Fatalf("pricing override lost")
	}
	if cfg.Sources["codex"].Path != "/tmp/codex" {
		t.Fatalf("source lost")
	}
	if got := len(cfg.Gateway.Profiles); got != 2 {
		t.Fatalf("expected 2 profiles, got %d", got)
	}
	if p := cfg.Gateway.Profiles["claude-minimax"]; !p.Enabled || p.Listen != "127.0.0.1:8788" || p.Protocol != "anthropic_messages" || p.Source != "claude" || p.Upstream != "https://router.example.com" || p.ProviderTag != "anthropic" {
		t.Fatalf("claude-minimax profile = %+v", p)
	}
	if p := cfg.Gateway.Profiles["codex-personal"]; p.Enabled || p.Listen != "127.0.0.1:8787" || p.Protocol != "openai_responses" {
		t.Fatalf("codex-personal profile = %+v", p)
	}
}

func TestLoadInvalidSectionIgnored(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := `
[unknown.section]
key = "value"

[gateway.profiles.alpha]
listen = "127.0.0.1:9001"
protocol = "anthropic_messages"
source = "claude"
upstream = "http://127.0.0.1:20128"
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	store := NewStoreAt(path)
	cfg, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, ok := cfg.Gateway.Profiles["alpha"]; !ok {
		t.Fatalf("gateway profile missing")
	}
}

func TestParseString(t *testing.T) {
	got, err := parseString(`"hello"`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got != "hello" {
		t.Fatalf("got %q", got)
	}
	if _, err := parseString("notquoted"); err == nil {
		t.Fatalf("expected error for unquoted string")
	}
}

func TestQuoteRoundTrip(t *testing.T) {
	value := "https://router.example.com/path"
	quoted := quoteString(value)
	parsed, err := parseString(quoted)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed != value {
		t.Fatalf("round trip = %q", parsed)
	}
}

func TestQuoteEscapes(t *testing.T) {
	if !strings.HasPrefix(quoteString(`"`), `"\"`) {
		t.Fatalf("quote should escape double quote")
	}
}
