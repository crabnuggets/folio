package chatgpt

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/crabnuggets/folio/services/api/internal/domain"
	"github.com/crabnuggets/folio/services/api/internal/store"
	"github.com/golang-jwt/jwt/v5"
)

const testOwner = "11111111-1111-4111-8111-111111111111"

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func responseJSON(value any) *http.Response {
	b, _ := json.Marshal(value)
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(b))), Header: http.Header{}}
}
func newTestClient(t *testing.T) *Client {
	t.Helper()
	c, err := New(store.NewMemory(), testOwner, Config{RedirectURI: "http://127.0.0.1:8080/auth/chatgpt/callback", EncryptionKey: base64.StdEncoding.EncodeToString(make([]byte, 32))})
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func seed(t *testing.T, c *Client, expires time.Time) {
	t.Helper()
	err := c.save(context.Background(), &domain.ChatGPTConnection{HostID: "urn:uuid:" + domain.NewID()}, &credentials{ClientID: "oaiapp_test", Subject: "subject", Email: "owner@example.test", AccessToken: "synthetic-access-token", RefreshToken: "synthetic-refresh-token", Scope: planScope, ExpiresAt: expires})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAuthorizationValidatesStateIdentityAndEncryptsCredentials(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	address, binding, err := c.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	authorization, _ := url.Parse(address)
	params := authorization.Query()
	if params.Get("client_id") != "dynamic_agent_client" || params.Get("code_challenge_method") != "S256" || !hasScope(params.Get("scope"), planScope) {
		t.Fatal("incomplete authorization", params)
	}
	if params.Get("redirect_uri") != c.config.RedirectURI || params.Get("ext_agent_host_id") == "" {
		t.Fatal("missing host or callback")
	}
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	claims := identityClaims{RegisteredClaims: jwt.RegisteredClaims{Issuer: issuer, Subject: "subject", Audience: jwt.ClaimStrings{"oaiapp_test"}, IssuedAt: jwt.NewNumericDate(time.Now()), ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}, Nonce: c.pending.nonce, Email: "owner@example.test"}
	idToken := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	idToken.Header["kid"] = "test-key"
	signed, err := idToken.SignedString(private)
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	c.http.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		requests++
		switch r.URL.Path {
		case "/api/accounts/oauth/token":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.Form.Get("client_id") != "oaiapp_test" || r.Form.Get("code_verifier") == "" || r.Form.Get("redirect_uri") != c.config.RedirectURI {
				t.Fatal("wrong token exchange")
			}
			return responseJSON(tokenResponse{AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh", IDToken: signed, Scope: planScope, TokenType: "Bearer", ExpiresIn: 3600}), nil
		case "/.well-known/jwks.json":
			return responseJSON(map[string]any{"keys": []map[string]string{{"kty": "EC", "crv": "P-256", "kid": "test-key", "alg": "ES256", "x": base64.RawURLEncoding.EncodeToString(private.X.FillBytes(make([]byte, 32))), "y": base64.RawURLEncoding.EncodeToString(private.Y.FillBytes(make([]byte, 32)))}}}), nil
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
			return nil, nil
		}
	})
	callback := url.Values{"state": {params.Get("state")}, "code": {"synthetic-code"}, "client_id": {"oaiapp_test"}}
	if c.Callback(ctx, callback, "wrong-browser") == nil {
		t.Fatal("accepted wrong browser binding")
	}
	if requests != 0 {
		t.Fatal("redeemed code before binding verification")
	}
	if err = c.Callback(ctx, callback, binding); err != nil {
		t.Fatal(err)
	}
	if c.Callback(ctx, callback, binding) == nil {
		t.Fatal("accepted reused callback")
	}
	state, err := c.db.Read(ctx, testOwner)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(state.ChatGPTConnection)
	if strings.Contains(string(data), "synthetic-access") || strings.Contains(string(data), "synthetic-refresh") || strings.Contains(string(data), "owner@example.test") {
		t.Fatal("credentials stored in plaintext")
	}
	_, stored, err := c.saved(ctx)
	if err != nil || stored.Subject != "subject" {
		t.Fatal("identity not saved", err)
	}
	second, _, err := c.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	returning, _ := url.Parse(second)
	if returning.Query().Get("client_id") != "oaiapp_test" || returning.Query().Get("ext_agent_host_id") != params.Get("ext_agent_host_id") || returning.Query().Get("agent_name_hint") != "" {
		t.Fatal("registration or host not retained")
	}
}

func TestIdentityRejectsInvalidClaims(t *testing.T) {
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"nonce", "issuer", "audience", "expiry", "signature"} {
		t.Run(field, func(t *testing.T) {
			c := newTestClient(t)
			claims := identityClaims{RegisteredClaims: jwt.RegisteredClaims{Issuer: issuer, Subject: "subject", Audience: jwt.ClaimStrings{"client"}, IssuedAt: jwt.NewNumericDate(time.Now()), ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}, Nonce: "nonce"}
			switch field {
			case "nonce":
				claims.Nonce = "wrong"
			case "issuer":
				claims.Issuer = "https://wrong.test"
			case "audience":
				claims.Audience = jwt.ClaimStrings{"wrong"}
			case "expiry":
				claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Hour))
			}
			signer := private
			if field == "signature" {
				signer, _ = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			}
			token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
			token.Header["kid"] = "key"
			raw, _ := token.SignedString(signer)
			c.http.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
				return responseJSON(map[string]any{"keys": []map[string]string{{"kty": "EC", "crv": "P-256", "kid": "key", "x": base64.RawURLEncoding.EncodeToString(private.X.FillBytes(make([]byte, 32))), "y": base64.RawURLEncoding.EncodeToString(private.Y.FillBytes(make([]byte, 32)))}}}), nil
			})
			if _, err = c.identity(context.Background(), raw, "client", "nonce"); err == nil {
				t.Fatal("accepted invalid", field)
			}
		})
	}
}

func TestRespondRefreshesTokensAndRequiresCompletedStream(t *testing.T) {
	for _, complete := range []bool{false, true} {
		t.Run(map[bool]string{false: "truncated", true: "completed"}[complete], func(t *testing.T) {
			c := newTestClient(t)
			seed(t, c, time.Now().Add(-time.Minute))
			c.http.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
				switch r.URL.Path {
				case "/api/accounts/oauth/token":
					r.ParseForm()
					if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "synthetic-refresh-token" {
						t.Fatal("wrong refresh")
					}
					return responseJSON(tokenResponse{AccessToken: "rotated-access", RefreshToken: "rotated-refresh", TokenType: "Bearer", ExpiresIn: 3600}), nil
				case "/v1/models":
					if r.Header.Get("Authorization") != "Bearer rotated-access" {
						t.Fatal("old token used")
					}
					return responseJSON(map[string]any{"models": []map[string]string{{"slug": "test-model", "display_name": "Test model", "visibility": "list"}}}), nil
				case "/v1/responses":
					var body map[string]any
					if json.NewDecoder(r.Body).Decode(&body) != nil || body["store"] != false || body["stream"] != true || body["model"] != "test-model" {
						t.Fatal("incorrect response request")
					}
					if _, unsupported := body["max_output_tokens"]; unsupported {
						t.Fatal("ChatGPT plan requests do not support max_output_tokens")
					}
					data := `data: {"type":"response.output_text.delta","delta":"partial"}` + "\n\n"
					if complete {
						data += `data: {"type":"response.completed","response":{"status":"completed","output":[{"content":[{"type":"output_text","text":"complete"}]}]}}` + "\n\n"
					}
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(data))}, nil
				default:
					t.Fatalf("unexpected path %s", r.URL.Path)
					return nil, nil
				}
			})
			result, err := c.Respond(context.Background(), "", "instruction", []map[string]any{{"type": "input_text", "text": "test"}})
			if complete && (err != nil || result != "complete") {
				t.Fatal(result, err)
			}
			if !complete && err == nil {
				t.Fatal("accepted truncated response")
			}
			_, saved, err := c.saved(context.Background())
			if err != nil || saved.RefreshToken != "rotated-refresh" || saved.AccessToken != "rotated-access" {
				t.Fatal("rotating credentials not persisted", err)
			}
		})
	}
}

func TestUnavailableAndInvalidConfiguration(t *testing.T) {
	var client *Client
	status, err := client.Connection(context.Background())
	if err != nil || status.Available || status.Connected || status.Models == nil || status.Reason == "" {
		t.Fatal(status, err)
	}
	if _, err = client.Respond(context.Background(), "", "", nil); err == nil {
		t.Fatal("nil client accepted inference")
	}
	for _, redirect := range []string{"https://folio.example/auth/chatgpt/callback", "http://localhost:8080/auth/chatgpt/callback", "http://127.0.0.1:8080/wrong"} {
		if Validate(Config{RedirectURI: redirect, EncryptionKey: base64.StdEncoding.EncodeToString(make([]byte, 32))}) == nil {
			t.Fatal("accepted unsupported callback", redirect)
		}
	}
}

func TestUnreadableCredentialsRemainDisconnectable(t *testing.T) {
	c := newTestClient(t)
	seed(t, c, time.Now().Add(time.Hour))
	key := make([]byte, 32)
	key[0] = 1
	replacement, err := New(c.db, c.owner, Config{RedirectURI: c.config.RedirectURI, EncryptionKey: base64.StdEncoding.EncodeToString(key)})
	if err != nil {
		t.Fatal(err)
	}
	status, err := replacement.Connection(context.Background())
	if err != nil || !status.Connected || status.Sharing || status.Reason == "" {
		t.Fatal("unreadable credentials must offer disconnect", status, err)
	}
	if err = replacement.Disconnect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, _, err = replacement.Begin(context.Background()); err != nil {
		t.Fatal("could not reconnect after clearing unreadable credentials", err)
	}
}
