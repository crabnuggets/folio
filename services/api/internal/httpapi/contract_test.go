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
	"strings"
	"sync"
	"testing"

	"github.com/crabnuggets/folio/services/api/internal/domain"
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
func accountInput(alias string) map[string]any {
	return map[string]any{"alias": alias, "kind": "bank", "currency": "SGD", "active": true, "include_in_liquid_cash": true, "funding_boundary": []any{}}
}
func createAccount(t *testing.T, h *harness, alias string) (string, string) {
	t.Helper()
	response := h.request(t, http.MethodPost, "/api/v1/accounts", accountInput(alias), domain.NewID(), "")
	payload := expect(t, response, 201, "Account")
	return payload["id"].(string), response.headers.Get("ETag")
}

func TestHTTPIdempotencyConcurrencyAndPreconditions(t *testing.T) {
	h := newHarness(t)
	key := domain.NewID()
	body := accountInput("Main")
	first := h.request(t, http.MethodPost, "/api/v1/accounts", body, key, "")
	payload := expect(t, first, 201, "Account")
	replay := h.request(t, http.MethodPost, "/api/v1/accounts", body, key, "")
	expect(t, replay, 201, "Account")
	if !bytes.Equal(first.body, replay.body) || first.headers.Get("ETag") != replay.headers.Get("ETag") {
		t.Fatal("retry did not reproduce committed response")
	}
	if replay.headers.Get("Idempotency-Replayed") != "true" {
		t.Fatal("replay was not marked")
	}
	changed := accountInput("Other")
	conflict := h.request(t, http.MethodPost, "/api/v1/accounts", changed, key, "")
	assertError(t, conflict, 409, "IDEMPOTENCY_CONFLICT")
	path := "/api/v1/accounts/" + payload["id"].(string)
	missing := h.request(t, http.MethodPatch, path, changed, domain.NewID(), "")
	assertError(t, missing, 428, "PRECONDITION_REQUIRED")
	updated := h.request(t, http.MethodPatch, path, changed, domain.NewID(), first.headers.Get("ETag"))
	expect(t, updated, 200, "Account")
	stale := h.request(t, http.MethodPatch, path, changed, domain.NewID(), first.headers.Get("ETag"))
	assertError(t, stale, 412, "STALE_VERSION")

	// Concurrent identical retries must create one additional account, with exactly
	// the same ID and body returned to every caller.
	concurrentKey := domain.NewID()
	results := make(chan wireResponse, 8)
	var group sync.WaitGroup
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			results <- h.request(t, http.MethodPost, "/api/v1/accounts", accountInput("Concurrent"), concurrentKey, "")
		}()
	}
	group.Wait()
	close(results)
	var original []byte
	for response := range results {
		expect(t, response, 201, "Account")
		if original == nil {
			original = response.body
		} else if !bytes.Equal(original, response.body) {
			t.Fatal("concurrent retry returned different posting")
		}
	}
	page := expect(t, h.request(t, http.MethodGet, "/api/v1/accounts", nil, "", ""), 200, "AccountPage")
	if len(page["items"].([]any)) != 2 {
		t.Fatalf("duplicate accounts: %s", mustJSON(page))
	}
}

func TestHTTPMalformedAndBoundedRequests(t *testing.T) {
	h := newHarness(t)
	cases := []struct {
		name, path string
		body       []byte
		want       int
	}{
		{"unknown field", "/api/v1/accounts", []byte(`{"alias":"Main","kind":"bank","currency":"SGD","active":true,"owner_id":"spoofed"}`), 400},
		{"trailing object", "/api/v1/accounts", []byte(`{} {}`), 400},
		{"null object", "/api/v1/accounts", []byte(`null`), 400},
		{"array object", "/api/v1/accounts", []byte(`[]`), 400},
		{"missing explicit active", "/api/v1/accounts", []byte(`{"alias":"Main","kind":"bank","currency":"SGD"}`), 400},
		{"numeric money", "/api/v1/entries", []byte(`{"kind":"income","amount_minor":500000,"currency":"SGD","effective_date":"2026-10-25"}`), 400},
		{"overflow money", "/api/v1/entries", []byte(`{"kind":"income","amount_minor":"9223372036854775808","currency":"SGD","effective_date":"2026-10-25"}`), 400},
		{"body limit", "/api/v1/accounts", []byte(`{"alias":"` + strings.Repeat("x", int(httpapi.MaxBodyBytes)) + `","kind":"bank","currency":"SGD","active":true}`), 413},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			expect(t, h.request(t, http.MethodPost, test.path, test.body, domain.NewID(), ""), test.want, "ErrorEnvelope")
		})
	}
	expect(t, h.request(t, http.MethodPost, "/api/v1/accounts", accountInput("No key"), "", ""), 400, "ErrorEnvelope")
	for _, path := range []string{"/api/v1/accounts?limit=0", "/api/v1/accounts?limit=101", "/api/v1/accounts?cursor=bad", "/api/v1/entries?month=2026-13"} {
		expect(t, h.request(t, http.MethodGet, path, nil, "", ""), 400, "ErrorEnvelope")
	}
}

func TestHTTPHealthAuthAndDeferredRoutes(t *testing.T) {
	h := newHarness(t)
	for _, path := range []string{"/healthz", "/readyz"} {
		req, _ := http.NewRequest(http.MethodGet, h.server.URL+path, nil)
		response, err := h.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		content, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatalf("health status=%d: %s", response.StatusCode, content)
		}
		assertSchema(t, "Health", content)
	}
	request, _ := http.NewRequest(http.MethodGet, h.server.URL+"/api/v1/accounts", nil)
	response, err := h.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	content, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatalf("unauthenticated status=%d", response.StatusCode)
	}
	assertSchema(t, "ErrorEnvelope", content)
	for _, test := range []struct{ method, path string }{{"POST", "/api/v1/imports"}, {"GET", "/api/v1/imports/11111111-1111-4111-8111-111111111111"}, {"POST", "/api/v1/gmail/connect"}, {"GET", "/api/v1/gmail/callback?code=synthetic&state=synthetic"}, {"POST", "/api/v1/gmail/sync"}, {"DELETE", "/api/v1/gmail/connection"}, {"POST", "/api/v1/capture/text"}} {
		assertError(t, h.request(t, test.method, test.path, map[string]any{}, domain.NewID(), ""), 501, "FEATURE_NOT_AVAILABLE")
	}
}

func TestHTTPSharedFinancialFixtures(t *testing.T) {
	for _, name := range []string{"recognized-spending.json", "funding-boundary.json"} {
		t.Run(name, func(t *testing.T) { runFixture(t, name) })
	}
}
func runFixture(t *testing.T, name string) {
	t.Helper()
	h := newHarness(t)
	raw, err := os.ReadFile(filepath.Join(repositoryRoot(t), "fixtures", name))
	if err != nil {
		t.Fatal(err)
	}
	var fixture map[string]any
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{}
	versions := map[string]string{}
	for _, group := range []struct{ key, path, schema string }{{"accounts", "accounts", "Account"}, {"entries", "entries", "Entry"}, {"statements", "statements", "Statement"}, {"claims", "claims", "Claim"}} {
		for _, rawItem := range fixture[group.key].([]any) {
			item := rawItem.(map[string]any)
			input := resolveRefs(t, item["input"], ids)
			assertSchema(t, group.schema+"Input", mustJSON(input))
			response := h.request(t, http.MethodPost, "/api/v1/"+group.path, input, domain.NewID(), "")
			payload := expect(t, response, 201, group.schema)
			refName := item["ref"].(string)
			ids[refName] = payload["id"].(string)
			versions[refName] = response.headers.Get("ETag")
		}
	}
	month := fixture["month"].(string)
	before := expect(t, h.request(t, http.MethodGet, "/api/v1/months/"+month+"/summary", nil, "", ""), 200, "Summary")
	for _, rawItem := range fixture["settlements"].([]any) {
		item := rawItem.(map[string]any)
		statementRef := item["statement_ref"].(string)
		input := map[string]any{"entry_id": ids[item["entry_ref"].(string)], "amount_minor": item["amount_minor"]}
		expect(t, h.request(t, http.MethodPost, "/api/v1/statements/"+ids[statementRef]+"/settlements", input, domain.NewID(), versions[statementRef]), 201, "Settlement")
	}
	stateResponse := h.request(t, http.MethodGet, "/api/v1/months/"+month, nil, "", "")
	expect(t, stateResponse, 200, "Month")
	expect(t, h.request(t, http.MethodPost, "/api/v1/months/"+month+"/close", map[string]any{"checklist": fixture["checklist"]}, domain.NewID(), stateResponse.headers.Get("ETag")), 200, "Month")
	after := expect(t, h.request(t, http.MethodGet, "/api/v1/months/"+month+"/summary", nil, "", ""), 200, "Summary")
	for key, want := range fixture["expected_summary"].(map[string]any) {
		if got := after[key]; got != want {
			t.Errorf("%s=%v want %v", key, got, want)
		}
	}
	if before["known_spending_minor"] != after["known_spending_minor"] {
		t.Error("settlement/close changed recognized spending")
	}
	balances := map[string]map[string]any{}
	for _, value := range after["statements"].([]any) {
		row := value.(map[string]any)
		balances[row["statement_id"].(string)] = row
	}
	for statementRef, want := range fixture["expected_remaining_payable"].(map[string]any) {
		if row := balances[ids[statementRef]]; row == nil || row["remaining_minor"] != want {
			t.Errorf("remaining %s=%v want %v", statementRef, row, want)
		}
	}
	expect(t, h.request(t, http.MethodGet, "/api/v1/exports", nil, "", ""), 200, "Export")
	expect(t, h.request(t, http.MethodGet, "/api/v1/months/"+month+"/insights", nil, "", ""), 200, "Insights")
}

func TestHTTPClosedMonthDraftBoundary(t *testing.T) {
	h := newHarness(t)
	id, _ := createAccount(t, h, "Main")
	checklist := map[string]any{"income_reviewed": true, "cards_reviewed": true, "expenses_reviewed": true, "allocations_reviewed": true, "no_statement_reasons": map[string]string{}}
	monthResponse := h.request(t, http.MethodGet, "/api/v1/months/2026-10", nil, "", "")
	expect(t, monthResponse, 200, "Month")
	expect(t, h.request(t, http.MethodPost, "/api/v1/months/2026-10/close", map[string]any{"checklist": checklist}, domain.NewID(), monthResponse.headers.Get("ETag")), 200, "Month")
	before := h.request(t, http.MethodGet, "/api/v1/months/2026-10/summary", nil, "", "")
	expect(t, before, 200, "Summary")
	entry := map[string]any{"kind": "income", "amount_minor": "500000", "currency": "SGD", "effective_date": "2026-10-25", "destination_account_id": id}
	assertError(t, h.request(t, http.MethodPost, "/api/v1/entries", entry, domain.NewID(), ""), 409, "MONTH_CLOSED")
	draftInput := map[string]any{"source_id": "synthetic-import", "external_id": "synthetic-observation-1", "entry": entry, "evidence": "Synthetic salary observation"}
	created := h.request(t, http.MethodPost, "/api/v1/drafts", draftInput, domain.NewID(), "")
	draft := expect(t, created, 201, "Draft")
	draftPath := "/api/v1/drafts/" + draft["id"].(string)
	assertError(t, h.request(t, http.MethodPost, draftPath+"/accept", map[string]any{}, domain.NewID(), created.headers.Get("ETag")), 409, "MONTH_CLOSED")
	after := h.request(t, http.MethodGet, "/api/v1/months/2026-10/summary", nil, "", "")
	if !bytes.Equal(before.body, after.body) {
		t.Fatalf("closed summary changed when staging/rejecting posting:\nbefore=%s\nafter=%s", before.body, after.body)
	}
	currentMonth := h.request(t, http.MethodGet, "/api/v1/months/2026-10", nil, "", "")
	expect(t, h.request(t, http.MethodPost, "/api/v1/months/2026-10/reopen", map[string]any{"reason": "Accept synthetic salary"}, domain.NewID(), currentMonth.headers.Get("ETag")), 200, "Month")
	acceptKey := domain.NewID()
	acceptedResponse := h.request(t, http.MethodPost, draftPath+"/accept", map[string]any{}, acceptKey, created.headers.Get("ETag"))
	accepted := expect(t, acceptedResponse, 200, "Draft")
	if accepted["state"] != "accepted" || accepted["accepted_record_id"] == nil {
		t.Fatalf("draft was not accepted: %s", acceptedResponse.body)
	}
	replay := h.request(t, http.MethodPost, draftPath+"/accept", map[string]any{}, acceptKey, created.headers.Get("ETag"))
	expect(t, replay, 200, "Draft")
	if !bytes.Equal(acceptedResponse.body, replay.body) {
		t.Fatal("draft acceptance retry changed response")
	}
	page := expect(t, h.request(t, http.MethodGet, "/api/v1/entries", nil, "", ""), 200, "EntryPage")
	if len(page["items"].([]any)) != 1 {
		t.Fatal("draft acceptance duplicated posting")
	}
}

func resolveRefs(t *testing.T, value any, ids map[string]string) any {
	t.Helper()
	switch v := value.(type) {
	case string:
		if strings.HasPrefix(v, "$") {
			id, ok := ids[strings.TrimPrefix(v, "$")]
			if !ok {
				t.Fatalf("unknown fixture reference %s", v)
			}
			return id
		}
		return v
	case map[string]any:
		result := map[string]any{}
		for key, item := range v {
			result[key] = resolveRefs(t, item, ids)
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = resolveRefs(t, item, ids)
		}
		return result
	default:
		return v
	}
}
func mustJSON(value any) []byte {
	content, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("marshal synthetic fixture: %v", err))
	}
	return content
}
