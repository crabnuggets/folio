// Package auth verifies single-owner bearer tokens; it never authenticates cookies.
package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/crabnuggets/folio/services/api/internal/domain"
	"github.com/golang-jwt/jwt/v5"
)

type Config struct{ Mode, Environment, OwnerID, DevToken, Issuer, Audience, JWKSURL string }
type verifier struct {
	config               Config
	client               *http.Client
	mu                   sync.Mutex
	keys                 map[string]any
	expiresAt, lastFetch time.Time
}

// New validates configuration and creates single-owner bearer authentication
// middleware without fetching keys. Development tokens require development or test
// mode and at least 32 bytes. JWT mode checks RS256/ES256 signatures, issuer, audience,
// expiration, and time claims with ten seconds of leeway using issuer-pinned JWKS.
// Invalid configuration returns an error. The middleware answers invalid tokens
// and key-fetch failures with 401, and valid tokens for another subject with 403.
func New(config Config) (func(http.Handler) http.Handler, error) {
	if !domain.ValidID(config.OwnerID) {
		return nil, errors.New("OWNER_ID must be a UUID")
	}
	config.OwnerID = strings.ToLower(config.OwnerID)
	if config.Mode == "dev" {
		if config.Environment != "development" && config.Environment != "test" {
			return nil, errors.New("development authentication is disabled outside development and test")
		}
		if len(config.DevToken) < 32 {
			return nil, errors.New("DEV_AUTH_TOKEN must contain at least 32 characters")
		}
	} else if config.Mode == "jwt" {
		issuer, err := url.Parse(config.Issuer)
		if err != nil || issuer.Host == "" || issuer.RawQuery != "" || issuer.Fragment != "" || issuer.User != nil {
			return nil, errors.New("JWT_ISSUER must be an absolute issuer URL")
		}
		if issuer.Scheme != "https" && !(config.Environment == "test" && issuer.Scheme == "http") {
			return nil, errors.New("JWT_ISSUER must use HTTPS")
		}
		if config.Audience == "" {
			return nil, errors.New("JWT_AUDIENCE is required")
		}
		config.Issuer = strings.TrimRight(config.Issuer, "/")
		canonical := config.Issuer + "/.well-known/jwks.json"
		if config.JWKSURL == "" {
			config.JWKSURL = canonical
		}
		if config.JWKSURL != canonical {
			return nil, errors.New("JWT_JWKS_URL must be the issuer's well-known JWKS endpoint")
		}
	} else {
		return nil, errors.New("AUTH_MODE must be dev or jwt")
	}
	v := &verifier{config: config, client: &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("JWKS redirects are disabled") }}, keys: make(map[string]any)}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") || len(header) > 16384 {
				writeError(w, 401, "UNAUTHENTICATED", "A valid bearer access token is required")
				return
			}
			token := strings.TrimPrefix(header, "Bearer ")
			if token == "" || strings.ContainsAny(token, " \t\r\n") {
				writeError(w, 401, "UNAUTHENTICATED", "A valid bearer access token is required")
				return
			}
			if config.Mode == "dev" {
				if subtle.ConstantTimeCompare([]byte(token), []byte(config.DevToken)) != 1 {
					writeError(w, 401, "UNAUTHENTICATED", "A valid bearer access token is required")
					return
				}
			} else {
				claims := &jwt.RegisteredClaims{}
				parsed, err := jwt.ParseWithClaims(token, claims, v.key, jwt.WithValidMethods([]string{"RS256", "ES256"}), jwt.WithIssuer(config.Issuer), jwt.WithAudience(config.Audience), jwt.WithExpirationRequired(), jwt.WithIssuedAt(), jwt.WithLeeway(10*time.Second))
				if err != nil || !parsed.Valid {
					writeError(w, 401, "UNAUTHENTICATED", "A valid bearer access token is required")
					return
				}
				if strings.ToLower(claims.Subject) != config.OwnerID {
					writeError(w, 403, "FORBIDDEN", "This application is restricted to its owner")
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}, nil
}
// writeError sends an uncached authentication error with a bearer challenge and
// request ID, generating an ID when absent. Response encoding errors are ignored.
func writeError(w http.ResponseWriter, status int, code, message string) {
	requestID := w.Header().Get("X-Request-ID")
	if requestID == "" {
		requestID = domain.NewID()
		w.Header().Set("X-Request-ID", requestID)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("WWW-Authenticate", "Bearer")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": code, "message": message, "fields": map[string]string{}, "request_id": requestID}})
}
// key resolves a token's kid to a compatible cached RSA or EC public key, refreshing
// expired or unknown keys at most once per minute. Missing, unavailable, unknown,
// or incompatible keys and unsupported methods return errors; refresh errors propagate.
func (v *verifier) key(token *jwt.Token) (any, error) {
	kid, ok := token.Header["kid"].(string)
	if !ok || kid == "" || len(kid) > 200 {
		return nil, errors.New("missing signing key")
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	now := time.Now()
	value, known := v.keys[kid]
	if now.After(v.expiresAt) || !known {
		if !v.lastFetch.IsZero() && now.Sub(v.lastFetch) < time.Minute {
			return nil, errors.New("signing key unavailable")
		}
		v.lastFetch = now
		if err := v.refresh(); err != nil {
			return nil, err
		}
		value, known = v.keys[kid]
	}
	if !known {
		return nil, errors.New("unknown signing key")
	}
	switch token.Method.Alg() {
	case "RS256":
		if _, ok := value.(*rsa.PublicKey); !ok {
			return nil, errors.New("incompatible signing key")
		}
	case "ES256":
		if _, ok := value.(*ecdsa.PublicKey); !ok {
			return nil, errors.New("incompatible signing key")
		}
	default:
		return nil, errors.New("unsupported signing method")
	}
	return value, nil
}
// refresh replaces signing keys from the configured JWKS endpoint for one hour.
// The caller must hold v.mu. Unsupported or malformed individual keys are skipped;
// fetch failures, invalid documents, duplicate accepted key IDs, or no usable keys
// return redacted errors and leave the existing cache unchanged.
func (v *verifier) refresh() error {
	response, err := v.client.Get(v.config.JWKSURL)
	if err != nil {
		return errors.New("signing keys unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return errors.New("signing keys unavailable")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return errors.New("invalid signing keys")
	}
	var document struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			Alg string `json:"alg"`
			Use string `json:"use"`
			N   string `json:"n"`
			E   string `json:"e"`
			Crv string `json:"crv"`
			X   string `json:"x"`
			Y   string `json:"y"`
		} `json:"keys"`
	}
	if json.Unmarshal(data, &document) != nil || len(document.Keys) == 0 || len(document.Keys) > 32 {
		return errors.New("invalid signing keys")
	}
	keys := make(map[string]any)
	for _, jwk := range document.Keys {
		if jwk.Kid == "" || len(jwk.Kid) > 200 || (jwk.Use != "" && jwk.Use != "sig") {
			continue
		}
		if _, exists := keys[jwk.Kid]; exists {
			return errors.New("duplicate signing key")
		}
		decode := base64.RawURLEncoding.DecodeString
		switch jwk.Kty {
		case "RSA":
			if jwk.Alg != "" && jwk.Alg != "RS256" {
				continue
			}
			n, e := decode(jwk.N)
			if e != nil {
				continue
			}
			exp, e := decode(jwk.E)
			if e != nil || len(exp) == 0 || len(exp) > 4 {
				continue
			}
			exponent := new(big.Int).SetBytes(exp).Int64()
			modulus := new(big.Int).SetBytes(n)
			if modulus.BitLen() < 2048 || exponent < 3 || exponent > 2147483647 || exponent%2 == 0 {
				continue
			}
			keys[jwk.Kid] = &rsa.PublicKey{N: modulus, E: int(exponent)}
		case "EC":
			if jwk.Crv != "P-256" || (jwk.Alg != "" && jwk.Alg != "ES256") {
				continue
			}
			x, e := decode(jwk.X)
			if e != nil || len(x) != 32 {
				continue
			}
			y, e := decode(jwk.Y)
			if e != nil || len(y) != 32 {
				continue
			}
			key := &ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(x), Y: new(big.Int).SetBytes(y)}
			if !key.Curve.IsOnCurve(key.X, key.Y) {
				continue
			}
			keys[jwk.Kid] = key
		}
	}
	if len(keys) == 0 {
		return errors.New("no supported signing keys")
	}
	v.keys = keys
	v.expiresAt = time.Now().Add(time.Hour)
	return nil
}
