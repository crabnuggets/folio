package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/crabnuggets/folio/services/api/internal/domain"
	"github.com/golang-jwt/jwt/v5"
)

func tokenRequest(middleware func(http.Handler) http.Handler, token string) int {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/settings", nil)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })).ServeHTTP(w, r)
	return w.Code
}
func TestDevelopmentAuthFailsClosed(t *testing.T) {
	owner := domain.NewID()
	token := strings.Repeat("x", 32)
	if _, err := New(Config{Mode: "dev", Environment: "production", OwnerID: owner, DevToken: token}); err == nil {
		t.Fatal("production development auth accepted")
	}
	if _, err := New(Config{Mode: "dev", Environment: "development", OwnerID: owner, DevToken: "short"}); err == nil {
		t.Fatal("short token accepted")
	}
	middleware, err := New(Config{Mode: "dev", Environment: "development", OwnerID: owner, DevToken: token})
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "wrong", token + " ", token + "\t"} {
		if got := tokenRequest(middleware, bad); got != 401 {
			t.Fatalf("bad token status %d", got)
		}
	}
	if got := tokenRequest(middleware, token); got != 204 {
		t.Fatal(got)
	}
}
func TestJWTClaimsSignaturesAndOwnerIsolation(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	encode := base64.RawURLEncoding.EncodeToString
	var fetches atomic.Int32
	keys := map[string]any{"keys": []any{map[string]string{"kty": "RSA", "kid": "rsa", "alg": "RS256", "use": "sig", "n": encode(rsaKey.N.Bytes()), "e": encode(big.NewInt(int64(rsaKey.E)).Bytes())}, map[string]string{"kty": "EC", "kid": "ec", "alg": "ES256", "use": "sig", "crv": "P-256", "x": encode(ecKey.X.FillBytes(make([]byte, 32))), "y": encode(ecKey.Y.FillBytes(make([]byte, 32)))}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/jwks.json" {
			t.Errorf("unexpected JWKS path %s", r.URL.Path)
		}
		fetches.Add(1)
		_ = json.NewEncoder(w).Encode(keys)
	}))
	defer server.Close()
	owner := domain.NewID()
	middleware, err := New(Config{Mode: "jwt", Environment: "test", OwnerID: owner, Issuer: server.URL, Audience: "authenticated"})
	if err != nil {
		t.Fatal(err)
	}
	claims := func() jwt.MapClaims {
		return jwt.MapClaims{"sub": owner, "iss": server.URL, "aud": "authenticated", "exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Add(-time.Minute).Unix()}
	}
	sign := func(method jwt.SigningMethod, key any, kid string, c jwt.MapClaims) string {
		token := jwt.NewWithClaims(method, c)
		token.Header["kid"] = kid
		encoded, e := token.SignedString(key)
		if e != nil {
			t.Fatal(e)
		}
		return encoded
	}
	tests := []struct {
		name   string
		mutate func(jwt.MapClaims)
		method jwt.SigningMethod
		key    any
		kid    string
		status int
	}{
		{"RSA", func(jwt.MapClaims) {}, jwt.SigningMethodRS256, rsaKey, "rsa", 204},
		{"ECDSA", func(jwt.MapClaims) {}, jwt.SigningMethodES256, ecKey, "ec", 204},
		{"expired", func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-time.Minute).Unix() }, jwt.SigningMethodRS256, rsaKey, "rsa", 401},
		{"no-expiry", func(c jwt.MapClaims) { delete(c, "exp") }, jwt.SigningMethodRS256, rsaKey, "rsa", 401},
		{"wrong-issuer", func(c jwt.MapClaims) { c["iss"] = "https://attacker.invalid" }, jwt.SigningMethodRS256, rsaKey, "rsa", 401},
		{"wrong-audience", func(c jwt.MapClaims) { c["aud"] = "other" }, jwt.SigningMethodRS256, rsaKey, "rsa", 401},
		{"different-owner", func(c jwt.MapClaims) { c["sub"] = domain.NewID() }, jwt.SigningMethodRS256, rsaKey, "rsa", 403},
		{"not-yet-valid", func(c jwt.MapClaims) { c["nbf"] = time.Now().Add(time.Hour).Unix() }, jwt.SigningMethodRS256, rsaKey, "rsa", 401},
		{"future-issued", func(c jwt.MapClaims) { c["iat"] = time.Now().Add(time.Hour).Unix() }, jwt.SigningMethodRS256, rsaKey, "rsa", 401},
		{"unknown-key", func(jwt.MapClaims) {}, jwt.SigningMethodRS256, rsaKey, "missing", 401},
		{"algorithm-confusion", func(jwt.MapClaims) {}, jwt.SigningMethodHS256, []byte("public-key-is-not-a-secret"), "rsa", 401},
		{"kid-type-mismatch", func(jwt.MapClaims) {}, jwt.SigningMethodES256, ecKey, "rsa", 401},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := claims()
			tt.mutate(c)
			got := tokenRequest(middleware, sign(tt.method, tt.key, tt.kid, c))
			if got != tt.status {
				t.Fatalf("got %d want %d", got, tt.status)
			}
		})
	}
	if got := fetches.Load(); got != 1 {
		t.Fatalf("JWKS fetched %d times; cache/unknown-kid backoff broken", got)
	}
}
func TestJWTUntrustedEndpointRejected(t *testing.T) {
	base := Config{Mode: "jwt", Environment: "production", OwnerID: domain.NewID(), Issuer: "https://project.example/auth/v1", Audience: "authenticated"}
	bad := base
	bad.JWKSURL = "https://attacker.example/keys"
	if _, err := New(bad); err == nil {
		t.Fatal("unrelated JWKS accepted")
	}
	bad = base
	bad.Issuer = "http://project.example/auth/v1"
	if _, err := New(bad); err == nil {
		t.Fatal("insecure issuer accepted")
	}
}
func TestJWKSFailureDoesNotExposeProviderResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "private-provider-diagnostic", 503) }))
	defer server.Close()
	owner := domain.NewID()
	middleware, _ := New(Config{Mode: "jwt", Environment: "test", OwnerID: owner, Issuer: server.URL, Audience: "authenticated"})
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{"sub": owner, "iss": server.URL, "aud": "authenticated", "exp": time.Now().Add(time.Hour).Unix()})
	token.Header["kid"] = "rsa"
	encoded, _ := token.SignedString(key)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer "+encoded)
	middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("auth failed open") })).ServeHTTP(w, r)
	if w.Code != 401 || strings.Contains(w.Body.String(), "private-provider") {
		t.Fatal(w.Code, w.Body.String())
	}
	if w.Header().Get("X-Request-ID") == "" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("missing request/security headers")
	}
}
