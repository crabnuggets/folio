package httpapi_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/crabnuggets/folio/services/api/internal/httpapi"
	"github.com/crabnuggets/folio/services/api/internal/store"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

const ownerID = "11111111-1111-4111-8111-111111111111"
const bearer = "contract-test-local-owner-token-only"

var schemaCache sync.Map

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate shared contract")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(source), "../../../.."))
}

// Validate the actual wire payload, including required fields, nullable amounts,
// UUIDs and forbidden extra properties, against the shared client contract.
func assertSchema(t *testing.T, name string, body []byte) {
	t.Helper()
	var compiled *jsonschema.Schema
	if cached, ok := schemaCache.Load(name); ok {
		compiled = cached.(*jsonschema.Schema)
	} else {
		spec, err := os.ReadFile(filepath.Join(repositoryRoot(t), "api/openapi.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		compiler := jsonschema.NewCompiler()
		compiler.Draft = jsonschema.Draft2020
		if err := compiler.AddResource("https://folio.test/openapi.json", bytes.NewReader(spec)); err != nil {
			t.Fatal(err)
		}
		compiled, err = compiler.Compile("https://folio.test/openapi.json#/components/schemas/" + name)
		if err != nil {
			t.Fatalf("compile %s: %v", name, err)
		}
		schemaCache.Store(name, compiled)
	}
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("invalid JSON: %v: %s", err, body)
	}
	if err := compiled.Validate(payload); err != nil {
		t.Fatalf("%s contract: %v\n%s", name, err, body)
	}
}

type harness struct {
	server *httptest.Server
	client *http.Client
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	middleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer "+bearer {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "UNAUTHENTICATED", "message": "Authentication is required", "fields": map[string]string{}, "request_id": w.Header().Get("X-Request-ID")}})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
	server := httptest.NewServer(httpapi.New(store.NewMemory(), ownerID, middleware))
	t.Cleanup(server.Close)
	return &harness{server: server, client: server.Client()}
}

type wireResponse struct {
	status  int
	body    []byte
	headers http.Header
}

func (h *harness) request(t *testing.T, method, path string, body any, key, etag string) wireResponse {
	t.Helper()
	var content []byte
	var err error
	if raw, ok := body.([]byte); ok {
		content = raw
	} else if body != nil {
		content, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	request, err := http.NewRequest(method, h.server.URL+path, bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+bearer)
	request.Header.Set("Content-Type", "application/json")
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	if etag != "" {
		request.Header.Set("If-Match", etag)
	}
	response, err := h.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	result, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.Header.Get("X-Request-ID") == "" {
		t.Fatal("missing request ID")
	}
	if response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("financial response may be cached")
	}
	return wireResponse{response.StatusCode, result, response.Header}
}
func expect(t *testing.T, response wireResponse, status int, schema string) map[string]any {
	t.Helper()
	if response.status != status {
		t.Fatalf("status=%d want %d: %s", response.status, status, response.body)
	}
	assertSchema(t, schema, response.body)
	var payload map[string]any
	if err := json.Unmarshal(response.body, &payload); err != nil {
		t.Fatal(err)
	}
	if status >= 400 {
		detail := payload["error"].(map[string]any)
		if detail["request_id"] != response.headers.Get("X-Request-ID") {
			t.Fatal("error correlation differs from header")
		}
	}
	return payload
}
func assertError(t *testing.T, response wireResponse, status int, code string) {
	t.Helper()
	payload := expect(t, response, status, "ErrorEnvelope")
	if got := payload["error"].(map[string]any)["code"]; got != code {
		t.Fatalf("error code=%v want %s", got, code)
	}
}
func mustJSON(value any) []byte {
	content, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("marshal synthetic fixture: %v", err))
	}
	return content
}
