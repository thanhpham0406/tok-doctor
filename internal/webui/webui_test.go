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
