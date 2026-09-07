package router9

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/thanhpham0406/tok-doctor/internal/source"
)

func TestDetectConfiguredReadyEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Source", "9router")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	result := New().Detect(context.Background(), source.Override{
		Endpoint: server.URL,
		Origin:   source.OriginConfig,
	})

	if result.Status != source.StatusReady {
		t.Fatalf("Status = %q, want ready", result.Status)
	}
	if result.Origin != source.OriginConfig {
		t.Fatalf("Origin = %q, want config", result.Origin)
	}
}

func TestRejectsNonLoopbackEndpoint(t *testing.T) {
	result := New().Detect(context.Background(), source.Override{
		Endpoint: "https://example.com",
		Origin:   source.OriginConfig,
	})

	if result.Status != source.StatusBroken {
		t.Fatalf("Status = %q, want broken", result.Status)
	}
}
