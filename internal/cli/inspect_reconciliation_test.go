package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thanhpham0406/tok-doctor/internal/gateway"
)

func fixtureModTime(t *testing.T, relative string) time.Time {
	t.Helper()
	info, err := os.Stat(filepath.Join(os.Getenv("HOME"), relative))
	if err != nil {
		t.Fatalf("stat fixture: %v", err)
	}
	return info.ModTime()
}

func TestInspectCommandShowsReconciliationUnavailable(t *testing.T) {
	withCLIFixtureHome(t)
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	t.Setenv("TOKDOCTOR_GATEWAY_DIR", filepath.Join(t.TempDir(), "gateway"))

	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"inspect", "sess-1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute inspect: %v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "Reconciliation") || !strings.Contains(got, "Unavailable: gateway capture not available") {
		t.Fatalf("output = %q, want unavailable reconciliation section", got)
	}
}

func TestInspectCommandJSONReconciliationUnavailableSchema(t *testing.T) {
	withCLIFixtureHome(t)
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	t.Setenv("TOKDOCTOR_GATEWAY_DIR", filepath.Join(t.TempDir(), "gateway"))

	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"inspect", "sess-1", "--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute inspect json: %v", err)
	}
	var decoded struct {
		Session        map[string]any `json:"session"`
		Reconciliation map[string]any `json:"reconciliation"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("decode json: %v\n%s", err, stdout.String())
	}
	if decoded.Reconciliation["available"] != false {
		t.Fatalf("available = %v, want false", decoded.Reconciliation["available"])
	}
	if decoded.Reconciliation["reason"] != "gateway capture not available" {
		t.Fatalf("reason = %v", decoded.Reconciliation["reason"])
	}
	if _, ok := decoded.Reconciliation["summary"]; ok {
		t.Fatalf("unavailable reconciliation must not include summary: %s", stdout.String())
	}
	if decoded.Session["id"] != "sess-1" {
		t.Fatalf("session id = %v, want sess-1", decoded.Session["id"])
	}
}

func TestInspectCommandJSONReconciliationAvailableSchema(t *testing.T) {
	withCLIFixtureHome(t)
	dir := t.TempDir()
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(dir, "config.toml"))
	gatewayDir := filepath.Join(dir, "gateway")
	t.Setenv("TOKDOCTOR_GATEWAY_DIR", gatewayDir)

	at := fixtureModTime(t, filepath.Join(".codex", "sessions", "basic-session.jsonl")).Add(time.Second)
	seedGatewayCapture(t, gatewayDir, []gateway.Exchange{{
		ID:         "gw-req-1",
		Profile:    "codex",
		SourceHint: "codex",
		Protocol:   gateway.ProtocolOpenAIResponses,
		StartedAt:  at,
		Kind:       gateway.RequestKindModel,
		Outcome:    gateway.OutcomeUpstreamOK,
		Model:      "gpt-5",
		Response:   gateway.ExchangeResponse{Status: 200, Model: "gpt-5"},
	}})

	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
	cmd.SetArgs([]string{"inspect", "sess-1", "--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute inspect json: %v", err)
	}
	var decoded struct {
		Reconciliation struct {
			Available bool `json:"available"`
			Summary   struct {
				GatewayRequests int `json:"gatewayRequests"`
			} `json:"summary"`
			Matches         []json.RawMessage `json:"matches"`
			Reconciliations []json.RawMessage `json:"reconciliations"`
		} `json:"reconciliation"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("decode json: %v\n%s", err, stdout.String())
	}
	if !decoded.Reconciliation.Available {
		t.Fatalf("available = false, want true\n%s", stdout.String())
	}
	if decoded.Reconciliation.Summary.GatewayRequests != 1 {
		t.Fatalf("gatewayRequests = %d, want 1", decoded.Reconciliation.Summary.GatewayRequests)
	}
	if decoded.Reconciliation.Matches == nil || decoded.Reconciliation.Reconciliations == nil {
		t.Fatalf("matches/reconciliations must be present arrays: %s", stdout.String())
	}
}

func TestInspectCommandReconciliationDoesNotLeakSecrets(t *testing.T) {
	withCLIFixtureHome(t)
	t.Setenv("TOKDOCTOR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	t.Setenv("TOKDOCTOR_GATEWAY_DIR", filepath.Join(t.TempDir(), "gateway"))

	for _, args := range [][]string{
		{"inspect", "sess-1"},
		{"inspect", "sess-1", "--format", "json"},
		{"inspect", "sess-2", "--turn", "1", "--evidence"},
	} {
		var stdout bytes.Buffer
		cmd := newRootCommand(context.Background(), &stdout, &bytes.Buffer{}, slog.Default())
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute %v: %v", args, err)
		}
		got := stdout.String()
		for _, marker := range []string{"Bearer ", "sk-", "api_key", "Authorization", "SECRET_"} {
			if strings.Contains(got, marker) {
				t.Fatalf("output for %v leaks %q: %s", args, marker, got)
			}
		}
	}
}
