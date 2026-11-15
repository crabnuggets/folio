package httpapi_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/crabnuggets/folio/services/api/internal/auth"
	"github.com/crabnuggets/folio/services/api/internal/domain"
	"github.com/crabnuggets/folio/services/api/internal/httpapi"
	"github.com/crabnuggets/folio/services/api/internal/store"
	"github.com/golang-jwt/jwt/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type bearerTransport struct{ token string }

func (t bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	copy := r.Clone(r.Context())
	copy.Header.Set("Authorization", "Bearer "+t.token)
	return http.DefaultTransport.RoundTrip(copy)
}

func TestMCPAuthenticatedStreamableToolsAndUIAcceptance(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	encode := base64.RawURLEncoding.EncodeToString
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/jwks.json" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{"kid": "one", "kty": "EC", "alg": "ES256", "crv": "P-256", "x": encode(key.X.FillBytes(make([]byte, 32))), "y": encode(key.Y.FillBytes(make([]byte, 32)))}}})
	}))
	defer issuer.Close()
	const origin = "https://folio.example"
	clientID := domain.NewID()
	sign := func(audience, subject, client, scope string, expiry time.Time) string {
		token := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{"iss": issuer.URL, "aud": audience, "sub": subject, "exp": expiry.Unix(), "iat": time.Now().Add(-time.Minute).Unix(), "client_id": client, "scope": scope, "role": "authenticated"})
		token.Header["kid"] = "one"
		value, err := token.SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	cfg := auth.Config{Mode: "jwt", Environment: "test", OwnerID: ownerID, Issuer: issuer.URL, Audience: "authenticated"}
	browserAuth, err := auth.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	db := store.NewMemory()
	handler, err := httpapi.WithMCP(httpapi.New(db, ownerID, browserAuth), db, ownerID, origin, clientID, cfg)
	if err != nil {
		t.Fatal(err)
	}
	host := httptest.NewServer(handler)
	defer host.Close()
	request := func(method, path, token, body, etag string) *http.Response {
		t.Helper()
		r, _ := http.NewRequest(method, host.URL+path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Idempotency-Key", domain.NewID())
		if etag != "" {
			r.Header.Set("If-Match", etag)
		}
		resp, err := host.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { resp.Body.Close() })
		return resp
	}
	metadata := request("GET", "/.well-known/oauth-protected-resource/mcp", "", "", "")
	var discovery map[string]any
	_ = json.NewDecoder(metadata.Body).Decode(&discovery)
	if metadata.StatusCode != 200 || discovery["resource"] != origin+"/mcp" || discovery["authorization_servers"].([]any)[0] != issuer.URL {
		t.Fatal("invalid discovery", discovery)
	}
	valid := sign(origin+"/mcp", ownerID, clientID, "openid", time.Now().Add(time.Hour))
	for _, tc := range []struct {
		name, token string
		status      int
	}{
		{"anonymous", "", 401}, {"browser token", sign("authenticated", ownerID, "", "", time.Now().Add(time.Hour)), 401},
		{"wrong audience", sign("https://elsewhere.example/mcp", ownerID, clientID, "openid", time.Now().Add(time.Hour)), 401},
		{"wrong client", sign(origin+"/mcp", ownerID, domain.NewID(), "openid", time.Now().Add(time.Hour)), 401},
		{"wrong owner", sign(origin+"/mcp", domain.NewID(), clientID, "openid", time.Now().Add(time.Hour)), 403},
		{"missing scope", sign(origin+"/mcp", ownerID, clientID, "", time.Now().Add(time.Hour)), 401},
		{"expired", sign(origin+"/mcp", ownerID, clientID, "openid", time.Now().Add(-time.Minute)), 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := request("POST", "/mcp", tc.token, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, "")
			if resp.StatusCode != tc.status || !strings.Contains(resp.Header.Get("WWW-Authenticate"), "resource_metadata") {
				t.Fatalf("status/challenge %d %s", resp.StatusCode, resp.Header.Get("WWW-Authenticate"))
			}
		})
	}
	for _, tc := range []struct {
		origin  string
		allowed bool
	}{{"https://claude.ai", true}, {"https://claude.com", true}, {origin, true}, {"https://evil.example", false}} {
		r, _ := http.NewRequest("POST", host.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "application/json, text/event-stream")
		r.Header.Set("Authorization", "Bearer "+valid)
		r.Header.Set("Origin", tc.origin)
		// The test server listens on loopback, like the app behind Render's proxy.
		r.Host = "folio.example"
		resp, err := host.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if (resp.StatusCode == 403) == tc.allowed {
			t.Fatalf("origin %s: status %d", tc.origin, resp.StatusCode)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "synthetic-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: host.URL + "/mcp", HTTPClient: &http.Client{Transport: bearerTransport{valid}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	tools, err := session.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 4 {
		t.Fatalf("tools: %+v %v", tools, err)
	}
	for _, tool := range tools.Tools {
		if strings.Contains(tool.Name, "accept") || strings.Contains(tool.Name, "reject") {
			t.Fatal("MCP exposes review commands")
		}
	}
	call := func(name string, args map[string]any) map[string]any {
		t.Helper()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil || result.IsError {
			t.Fatalf("tool %s: %+v %v", name, result, err)
		}
		data, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		var value map[string]any
		if err = json.Unmarshal(data, &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	sheet := call("read_monthly_sheet", map[string]any{"month": "2026-11"})
	if sheet["money_out_minor"] != "0" {
		t.Fatal("empty totals")
	}
	input := map[string]any{"month": "2026-11", "section_id": cardSection, "name": "MCP card", "amount_minor": "1250", "note": "Test", "evidence": "Synthetic source", "source_id": "test:mcp", "external_id": "one", "sheet_version": sheet["version"], "explanations": []map[string]any{{"name": "Grocer", "amount_minor": "1000"}}}
	created := call("create_monthly_item_draft", input)
	draft := created["draft"].(map[string]any)
	if parts := draft["explanations"].([]any); created["outcome"] != "created" || len(parts) != 1 || parts[0].(map[string]any)["amount_minor"] != "1000" {
		t.Fatalf("breakdown not stored: %v", created)
	}
	retry := call("create_monthly_item_draft", input)
	if draft["id"] != retry["draft"].(map[string]any)["id"] || draft["state"] != "pending" || retry["outcome"] != "already_proposed" {
		t.Fatal("proposal not deduplicated")
	}
	reread := map[string]any{}
	for k, v := range input {
		reread[k] = v
	}
	reread["note"], reread["evidence"] = "Same statement uploaded again", "Synthetic source, page 1"
	if again := call("create_monthly_item_draft", reread); again["outcome"] != "already_proposed" || again["draft"].(map[string]any)["id"] != draft["id"] {
		t.Fatalf("re-read of the same document not reported: %v", again)
	}
	reread["amount_minor"] = "1300"
	if conflict, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "create_monthly_item_draft", Arguments: reread}); err != nil || !conflict.IsError || !strings.Contains(conflict.Content[0].(*mcp.TextContent).Text, draft["id"].(string)) {
		t.Fatalf("conflict does not name the existing proposal: %+v %v", conflict, err)
	}
	pending := call("list_pending_monthly_drafts", map[string]any{"month": "2026-11"})
	if len(pending["items"].([]any)) != 1 {
		t.Fatal("wrong pending list")
	}
	state, err := db.Read(ctx, ownerID)
	if err != nil {
		t.Fatal(err)
	}
	entered, _ := state.GetMonthlySheet("2026-11")
	if entered.MoneyOutMinor != 0 {
		t.Fatal("MCP changed totals")
	}
	for _, aud := range []string{origin + "/mcp", "authenticated"} {
		denied := request("POST", "/api/v1/monthly-drafts/"+draft["id"].(string)+"/accept", sign(aud, ownerID, clientID, "openid", time.Now().Add(time.Hour)), `{"sheet_version":1}`, `"1"`)
		if denied.StatusCode != 401 && denied.StatusCode != 403 {
			t.Fatal("OAuth token reached owner acceptance", denied.StatusCode)
		}
	}
	uiToken := sign("authenticated", ownerID, "", "", time.Now().Add(time.Hour))
	accepted := request("POST", "/api/v1/monthly-drafts/"+draft["id"].(string)+"/accept", uiToken, `{"sheet_version":1}`, `"1"`)
	body, _ := io.ReadAll(accepted.Body)
	if accepted.StatusCode != 200 {
		t.Fatalf("UI accept %d: %s", accepted.StatusCode, body)
	}
	assertSchema(t, "MonthlySheet", body)
	state, _ = db.Read(ctx, ownerID)
	entered, _ = state.GetMonthlySheet("2026-11")
	if entered.MoneyOutMinor != 1250 {
		t.Fatal("acceptance failed")
	}
	if count := call("list_pending_monthly_drafts", map[string]any{"month": "2026-11"}); len(count["items"].([]any)) != 0 {
		t.Fatal("accepted proposal still pending")
	}
	if replayed := call("create_monthly_item_draft", input); replayed["outcome"] != "already_proposed" || replayed["draft"].(map[string]any)["state"] != "accepted" {
		t.Fatalf("exact retry reported a stale proposal: %v", replayed)
	}
	sheet = call("read_monthly_sheet", map[string]any{"month": "2026-11"})
	var itemID string
	for _, section := range sheet["sections"].([]any) {
		for _, item := range section.(map[string]any)["items"].([]any) {
			if item.(map[string]any)["name"] == "MCP card" {
				itemID = item.(map[string]any)["id"].(string)
			}
		}
	}
	amended := call("propose_monthly_item_amendment", map[string]any{"month": "2026-11", "item_id": itemID, "amount_minor": "1250", "note": "Adds the cafe charge", "evidence": "Synthetic source", "source_id": "test:mcp", "external_id": "one-detail", "sheet_version": sheet["version"], "explanations": []map[string]any{{"name": "Grocer", "amount_minor": "1000"}, {"name": "Cafe", "amount_minor": "250"}}})
	amendment := amended["draft"].(map[string]any)
	if base := amendment["amendment"].(map[string]any); amended["outcome"] != "created" || base["item_id"] != itemID || base["base_amount_minor"] != "1250" || amendment["name"] != "MCP card" {
		t.Fatalf("amendment not staged: %v", amended)
	}
	listed := request("GET", "/api/v1/monthly-drafts?month=2026-11", uiToken, "", "")
	body, _ = io.ReadAll(listed.Body)
	assertSchema(t, "MonthlyDraftPage", body)
	accepted = request("POST", "/api/v1/monthly-drafts/"+amendment["id"].(string)+"/accept", uiToken, fmt.Sprintf(`{"sheet_version":%v}`, sheet["version"]), `"1"`)
	body, _ = io.ReadAll(accepted.Body)
	if accepted.StatusCode != 200 {
		t.Fatalf("amendment accept %d: %s", accepted.StatusCode, body)
	}
	state, _ = db.Read(ctx, ownerID)
	entered, _ = state.GetMonthlySheet("2026-11")
	if entered.MoneyOutMinor != 1250 || state.MonthlySheet.Rows["2026-11"][itemID].Explanations[1].Name != "Cafe" {
		t.Fatal("amendment did not add the explanation")
	}
	for _, path := range []string{"/accounts", "/entries", "/statements", "/drafts", "/months/2026-11/summary", "/legacy-month-summaries", "/imports"} {
		if resp := request("GET", "/api/v1"+path, uiToken, "", ""); resp.StatusCode != 404 {
			t.Fatalf("retired route %s still active", path)
		}
	}
}
