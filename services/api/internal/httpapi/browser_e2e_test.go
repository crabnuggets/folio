package httpapi_test

import (
	"bytes"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/crabnuggets/folio/services/api/internal/auth"
	"github.com/crabnuggets/folio/services/api/internal/httpapi"
	"github.com/crabnuggets/folio/services/api/internal/store"
)

// This synthetic credential exists only in a _test.go binary. Never configure a
// deployed service with it. Browser tests deliberately exercise real auth.New.
const browserBearer = "folio-browser-e2e-synthetic-bearer-only"

// TestBrowserServer is an opt-in, loopback-only acceptance-test server, not a
// production memory-store fallback. Playwright owns its process lifetime.
// Run from services/api with FOLIO_BROWSER_E2E=1:
// go test ./internal/httpapi -run ^TestBrowserServer$ -count=1 -timeout=30m
func TestBrowserServer(t *testing.T) {
	if os.Getenv("FOLIO_BROWSER_E2E") != "1" {
		t.Skip("set FOLIO_BROWSER_E2E=1 explicitly to run the browser test server")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:18080")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{
		Handler: browserHandler(t, os.Getenv("FOLIO_BROWSER_WEB_DIST_DIR")), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second,
		IdleTimeout: 30 * time.Second,
	}
	t.Cleanup(func() { _ = server.Close() })
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		t.Fatal(err)
	}
}

func browserHandler(t *testing.T, webDistDir string) http.Handler {
	t.Helper()
	middleware, err := auth.New(auth.Config{
		Mode: "dev", Environment: "test", OwnerID: ownerID, DevToken: browserBearer,
	})
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.RWMutex
	newAPI := func() (http.Handler, error) {
		if webDistDir == "" {
			return httpapi.New(store.NewMemory(), ownerID, middleware), nil
		}
		return httpapi.NewWithWebDistDir(store.NewMemory(), ownerID, middleware, webDistDir, auth.BrowserConfig{DevelopmentKey: true})
	}
	api, err := newAPI()
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	// Reset lives outside the production API, requires the test bearer, and is
	// serialized with requests so each Playwright test starts with isolated data.
	mux.Handle("POST /__e2e/reset", middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		next, err := newAPI()
		if err != nil {
			http.Error(w, "Test application reset failed", http.StatusInternalServerError)
			return
		}
		api = next
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusNoContent)
	})))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		defer mu.RUnlock()
		api.ServeHTTP(w, r)
	})
	return mux
}

func TestBrowserHarnessAuthenticationAndReset(t *testing.T) {
	handler := browserHandler(t, "")
	request := func(method, path, token string, body []byte) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", "22222222-2222-4222-8222-222222222222")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	for _, path := range []string{"/api/v1/accounts", "/__e2e/reset"} {
		if got := request("POST", path, "wrong-synthetic-token", nil).Code; got != 401 {
			t.Fatalf("unauthenticated %s status=%d", path, got)
		}
	}
	created := request("POST", "/api/v1/accounts", browserBearer, mustJSON(accountInput("Synthetic browser account")))
	if created.Code != 201 {
		t.Fatalf("create status=%d: %s", created.Code, created.Body)
	}
	if got := request("POST", "/__e2e/reset", browserBearer, nil).Code; got != 204 {
		t.Fatalf("reset status=%d", got)
	}
	page := request("GET", "/api/v1/accounts", browserBearer, nil)
	if page.Code != 200 || !bytes.Contains(page.Body.Bytes(), []byte(`"items":[]`)) {
		t.Fatalf("reset did not isolate state: %d %s", page.Code, page.Body)
	}
}
