package pricing

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRemoteCatalogValidUpdate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"v1"`)
		_, _ = fmt.Fprint(w, testCatalogJSON("remote", "remote-model"))
	}))
	defer server.Close()

	store := testStore(t, server.URL, "")
	result, err := store.Update()
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !result.Updated || result.Version != "remote" || result.Profiles != 1 {
		t.Fatalf("result = %+v, want updated remote catalog", result)
	}
	active, err := store.Active()
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	if active.Source != SourceCache || active.Catalog.Version != "remote" {
		t.Fatalf("active = %+v, want cache remote", active)
	}
}

func TestDefaultRemoteCatalogPath(t *testing.T) {
	want := "https://raw.githubusercontent.com/thanhpham0406/tok-doctor/main/pricing/catalog.json"
	if got := DefaultRemoteURL(); got != want {
		t.Fatalf("DefaultRemoteURL = %q, want %q", got, want)
	}
}

func TestUpdateRequestsPublicPricingCatalogPath(t *testing.T) {
	seenPath := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		_, _ = fmt.Fprint(w, testCatalogJSON("remote", "remote-model"))
	}))
	defer server.Close()

	store := testStore(t, server.URL+"/pricing/catalog.json", "")
	if _, err := store.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if seenPath != "/pricing/catalog.json" {
		t.Fatalf("request path = %q, want /pricing/catalog.json", seenPath)
	}
}

func TestUpdateRejectsNonHTTPSRemoteCatalogURL(t *testing.T) {
	store := testStore(t, "http://example.test/pricing/catalog.json", "")
	if _, err := store.Update(); err == nil {
		t.Fatal("Update unexpectedly accepted non-HTTPS remote URL")
	}
}

func TestInvalidDownloadKeepsLastKnownGood(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			_, _ = fmt.Fprint(w, testCatalogJSON("good", "good-model"))
			return
		}
		_, _ = fmt.Fprint(w, `{"version":"","profiles":[{}]}`)
	}))
	defer server.Close()

	store := testStore(t, server.URL, "")
	if _, err := store.Update(); err != nil {
		t.Fatalf("first update: %v", err)
	}
	result, err := store.Update()
	if err == nil {
		t.Fatal("second update unexpectedly succeeded")
	}
	if !result.UsingCache || result.Version != "good" {
		t.Fatalf("result = %+v, want cached good catalog", result)
	}
	active, err := store.Active()
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	if active.Catalog.Version != "good" {
		t.Fatalf("version = %s, want last-known-good", active.Catalog.Version)
	}
}

func TestAtomicCacheBehaviorLeavesOldFileOnWriteFailure(t *testing.T) {
	store := testStore(t, "https://example.test/catalog.json", "")
	if err := os.MkdirAll(store.cacheDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Mkdir(store.catalogPath(), 0o755); err != nil {
		t.Fatalf("mkdir catalog path: %v", err)
	}
	err := store.writeCache([]byte(testCatalogJSON("new", "new-model")), CatalogMeta{Version: "new"})
	if err == nil {
		t.Fatal("writeCache unexpectedly replaced a directory")
	}
	if info, statErr := os.Stat(store.catalogPath()); statErr != nil || !info.IsDir() {
		t.Fatalf("catalog path was not preserved as directory after failed write")
	}
}

func TestEmbeddedFallback(t *testing.T) {
	store := testStore(t, "https://example.test/catalog.json", "")
	active, err := store.Active()
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	if active.Source != SourceEmbedded || active.Catalog.Version == "" {
		t.Fatalf("active = %+v, want embedded fallback", active)
	}
}

func TestCatalogPrecedenceOverrideDownloadedEmbedded(t *testing.T) {
	cacheServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, testCatalogJSON("cache", "cache-model"))
	}))
	defer cacheServer.Close()
	store := testStore(t, cacheServer.URL, "")
	if _, err := store.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	overridePath := filepath.Join(t.TempDir(), "override.json")
	if err := os.WriteFile(overridePath, []byte(testCatalogJSON("override", "override-model")), 0o600); err != nil {
		t.Fatalf("write override: %v", err)
	}
	store.overridePath = overridePath
	active, err := store.Active()
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	if active.Source != SourceOverride || active.Catalog.Version != "override" {
		t.Fatalf("active = %+v, want override before cache", active)
	}
}

func TestUserOverrideBeatsDownloadedAndEmbeddedProfile(t *testing.T) {
	cacheServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, testCatalogJSON("cache", "gpt-5.5"))
	}))
	defer cacheServer.Close()
	overridePath := filepath.Join(t.TempDir(), "override.json")
	if err := os.WriteFile(overridePath, []byte(`{
  "version": "override",
  "profiles": [{
    "provider": "openai",
    "sku": "gpt-5.5-custom",
    "model": "gpt-5.5",
    "currency": "USD",
    "rates": {"input_micros_per_million": 42, "output_micros_per_million": 99}
  }]
}`), 0o600); err != nil {
		t.Fatalf("write override: %v", err)
	}
	store := testStore(t, cacheServer.URL, overridePath)
	if _, err := store.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	active, profile, ok, err := store.Show("gpt-5.5")
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if !ok || active.Source != SourceOverride || profile.SKU != "gpt-5.5-custom" || profile.CatalogSource != SourceOverride {
		t.Fatalf("profile = %+v active=%+v, want user override", profile, active)
	}
}

func TestUpdateSendsETag(t *testing.T) {
	seen := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("If-None-Match")
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	now := time.Now().UTC()
	store := testStore(t, server.URL, "")
	if err := store.writeCache([]byte(testCatalogJSON("cache", "cache-model")), CatalogMeta{Version: "cache", FetchedAt: &now, ETag: `"abc"`}); err != nil {
		t.Fatalf("write cache: %v", err)
	}
	if _, err := store.Update(); err != nil {
		t.Fatalf("Update not modified: %v", err)
	}
	if seen != `"abc"` {
		t.Fatalf("If-None-Match = %q, want etag", seen)
	}
}

func testStore(t *testing.T, url, override string) Store {
	t.Helper()
	store := NewStore(filepath.Join(t.TempDir(), "pricing"), override)
	store.remoteURL = url
	store.client = &http.Client{Timeout: time.Second}
	return store
}

func testCatalogJSON(version, model string) string {
	out := strings.ReplaceAll(`{
  "version": "__version__",
  "generated_at": "2026-01-01T00:00:00Z",
  "profiles": [{
    "provider": "Test",
    "sku": "__model__",
    "model": "__model__",
    "currency": "USD",
    "rates": {"input_micros_per_million": 1000000}
  }]
}`, "__version__", version)
	return strings.ReplaceAll(out, "__model__", model)
}
