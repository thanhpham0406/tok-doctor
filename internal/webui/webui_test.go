package webui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/thanhpham0406/tok-doctor/internal/analyze"
)

func TestAPIResult(t *testing.T) {
	handler, err := NewHandler(analyze.Result{Source: "codex"})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/result", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var result analyze.Result
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if result.Source != "codex" {
		t.Fatalf("Source = %q, want codex", result.Source)
	}
}

// The UI reads source, summary.status, summary.message, and findings, and shows
// its unavailable state for any response it cannot parse. A result with no
// session still has to encode every one of those fields.
func TestAPIResultEncodesFieldsReadByWebUI(t *testing.T) {
	handler, err := NewHandler(analyze.Result{})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/result", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	var body map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	for _, field := range []string{"source", "summary", "findings"} {
		if _, ok := body[field]; !ok {
			t.Errorf("response is missing %q", field)
		}
	}

	var summary map[string]json.RawMessage
	if err := json.Unmarshal(body["summary"], &summary); err != nil {
		t.Fatalf("unmarshal summary: %v", err)
	}
	for _, field := range []string{"status", "message"} {
		if _, ok := summary[field]; !ok {
			t.Errorf("summary is missing %q", field)
		}
	}

	var findings []json.RawMessage
	if err := json.Unmarshal(body["findings"], &findings); err != nil {
		t.Fatalf("unmarshal findings: %v", err)
	}
}

func TestEmbeddedIndex(t *testing.T) {
	handler, err := NewHandler(analyze.Result{Source: "codex"})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
