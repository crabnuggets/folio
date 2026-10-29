package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crabnuggets/folio/services/api/internal/httpapi"
	"github.com/crabnuggets/folio/services/api/internal/store"
)

func staticHarness(t *testing.T) (http.Handler, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<!doctype html><div id=app>Folio</div>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "assets", "app-a1b2c3.js"), []byte("window.folio = true;"), 0o644); err != nil {
		t.Fatal(err)
	}
	handler, err := httpapi.NewWithWebDistDir(store.NewMemory(), ownerID, nil, root)
	if err != nil {
		t.Fatal(err)
	}
	return handler, root
}

func perform(handler http.Handler, method, target, accept string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, nil)
	if accept != "" {
		request.Header.Set("Accept", accept)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func assertJSONError(t *testing.T, response *httptest.ResponseRecorder, status int) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status=%d want %d body=%q", response.Code, status, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("Content-Type=%q want JSON", got)
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON error: %v: %s", err, response.Body.String())
	}
	if _, ok := body["error"]; !ok {
		t.Fatalf("missing error envelope: %s", response.Body.String())
	}
}

func TestWebDistServesRootRoutesAndAssetsWithCacheHeaders(t *testing.T) {
	handler, _ := staticHarness(t)

	root := perform(handler, http.MethodGet, "/", "text/html")
	if root.Code != http.StatusOK || !strings.Contains(root.Body.String(), "Folio") {
		t.Fatalf("root response status=%d body=%q", root.Code, root.Body.String())
	}
	if !strings.HasPrefix(root.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("root Content-Type=%q", root.Header().Get("Content-Type"))
	}
	if got := root.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("index Cache-Control=%q want no-cache", got)
	}
	for _, header := range []string{"X-Content-Type-Options", "X-Frame-Options", "Referrer-Policy", "Content-Security-Policy"} {
		if root.Header().Get(header) == "" {
			t.Errorf("missing static security header %s", header)
		}
	}

	route := perform(handler, http.MethodGet, "/monthly/overview", "text/html,application/xhtml+xml")
	if route.Code != http.StatusOK || !strings.Contains(route.Body.String(), "Folio") {
		t.Fatalf("SPA route status=%d body=%q", route.Code, route.Body.String())
	}
	if route.Header().Get("Vary") != "Accept" {
		t.Fatalf("SPA route Vary=%q want Accept", route.Header().Get("Vary"))
	}
	if got := route.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("SPA route Cache-Control=%q want no-cache", got)
	}

	asset := perform(handler, http.MethodGet, "/assets/app-a1b2c3.js", "*/*")
	if asset.Code != http.StatusOK || !strings.Contains(asset.Body.String(), "folio") {
		t.Fatalf("asset status=%d body=%q", asset.Code, asset.Body.String())
	}
	if !strings.Contains(asset.Header().Get("Content-Type"), "javascript") {
		t.Fatalf("asset Content-Type=%q", asset.Header().Get("Content-Type"))
	}
	if got := asset.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("asset Cache-Control=%q", got)
	}
}

func TestWebDistMissesAndReservedPathsStayStructuredJSON(t *testing.T) {
	handler, _ := staticHarness(t)

	for _, target := range []string{"/assets/missing.js", "/favicon.ico", "/api", "/api/v2/missing", "/healthz/missing", "/readyz/missing"} {
		response := perform(handler, http.MethodGet, target, "text/html")
		assertJSONError(t, response, http.StatusNotFound)
		if strings.Contains(response.Body.String(), "<html") || strings.Contains(response.Body.String(), "Folio") {
			t.Errorf("%s unexpectedly returned app HTML: %s", target, response.Body.String())
		}
	}

	unauthenticated := perform(handler, http.MethodGet, "/api/v1/settings", "text/html")
	assertJSONError(t, unauthenticated, http.StatusUnauthorized)

	for _, target := range []string{"/healthz", "/readyz"} {
		response := perform(handler, http.MethodGet, target, "text/html")
		if response.Code != http.StatusOK || !strings.HasPrefix(response.Header().Get("Content-Type"), "application/json") {
			t.Errorf("%s should remain a JSON health response: status=%d content-type=%q body=%q", target, response.Code, response.Header().Get("Content-Type"), response.Body.String())
		}
	}

	missingExtensionless := perform(handler, http.MethodGet, "/apiary", "application/json")
	assertJSONError(t, missingExtensionless, http.StatusNotFound)
	missingExtensionless = perform(handler, http.MethodGet, "/nonexistent", "*/*")
	assertJSONError(t, missingExtensionless, http.StatusNotFound)
}

func TestWebDistPathTraversalCannotReadOutsideBuildRoot(t *testing.T) {
	handler, root := staticHarness(t)
	secret := filepath.Join(filepath.Dir(root), "secret.txt")
	if err := os.WriteFile(secret, []byte("outside-secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"/%2e%2e/secret.txt", "/assets%2f..%2f..%2fsecret.txt", "/%5c..%5csecret.txt"} {
		response := perform(handler, http.MethodGet, target, "text/html")
		assertJSONError(t, response, http.StatusNotFound)
		if strings.Contains(response.Body.String(), "outside-secret") {
			t.Fatalf("path %q escaped WEB_DIST_DIR: %s", target, response.Body.String())
		}
		if strings.HasPrefix(response.Header().Get("Content-Type"), "text/html") && strings.Contains(response.Body.String(), "Folio") {
			t.Fatalf("path %q escaped to an SPA route: %s", target, response.Body.String())
		}
	}
}

func TestWebDistRequiresIndexAndEmptyPathKeepsAPIOnlyMode(t *testing.T) {
	empty := t.TempDir()
	if _, err := httpapi.NewWithWebDistDir(store.NewMemory(), ownerID, nil, empty); err == nil {
		t.Fatal("configured static directory without index.html was accepted")
	}

	handler, err := httpapi.NewWithWebDistDir(store.NewMemory(), ownerID, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	response := perform(handler, http.MethodGet, "/", "text/html")
	assertJSONError(t, response, http.StatusNotFound)
}
