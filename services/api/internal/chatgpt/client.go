// Package chatgpt connects an owner's ChatGPT plan using OpenAI's loopback OAuth flow.
package chatgpt

import (
	"bufio"
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
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
	"github.com/crabnuggets/folio/services/api/internal/store"
	"github.com/golang-jwt/jwt/v5"
)

const issuer = "https://auth.openai.com"
const resource = "https://api.openai.com/v1"
const planScope = "chatgpt.tokens.use.direct"

type Config struct{ RedirectURI, EncryptionKey string }
type Model struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type Status struct {
	Available bool    `json:"available"`
	Connected bool    `json:"connected"`
	Sharing   bool    `json:"sharing"`
	Email     string  `json:"email"`
	Reason    string  `json:"reason"`
	Models    []Model `json:"models"`
}
type credentials struct {
	ClientID     string    `json:"client_id"`
	Subject      string    `json:"subject"`
	Email        string    `json:"email"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	IDToken      string    `json:"id_token"`
	Scope        string    `json:"scope"`
	ExpiresAt    time.Time `json:"expires_at"`
}
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresIn    int64  `json:"expires_in"`
}
type attempt struct {
	state, nonce, verifier, binding, clientID string
	expires                                   time.Time
}
type Client struct {
	mu      sync.Mutex
	db      store.Store
	owner   string
	config  Config
	aead    cipher.AEAD
	http    *http.Client
	pending *attempt
}

// Validate accepts only OpenAI's documented loopback callback and an AES-256 key.
func Validate(config Config) error {
	if config.RedirectURI == "" && config.EncryptionKey == "" {
		return nil
	}
	u, err := url.Parse(config.RedirectURI)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.Path != "/auth/chatgpt/callback" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return errors.New("CHATGPT_REDIRECT_URI must be http://127.0.0.1:PORT/auth/chatgpt/callback; ChatGPT plan authorization requires a local callback")
	}
	key, err := base64.StdEncoding.DecodeString(config.EncryptionKey)
	if err != nil || len(key) != 32 {
		return errors.New("CHATGPT_ENCRYPTION_KEY must be a base64-encoded 32-byte key")
	}
	return nil
}

// New leaves the integration unavailable when configuration is absent.
func New(db store.Store, owner string, config Config) (*Client, error) {
	if err := Validate(config); err != nil {
		return nil, err
	}
	if config.RedirectURI == "" {
		return nil, nil
	}
	key, _ := base64.StdEncoding.DecodeString(config.EncryptionKey)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Client{db: db, owner: owner, config: config, aead: aead, http: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("provider redirects are disabled") }}}, nil
}

func unavailable() error {
	return domain.Fail(503, "CHATGPT_UNAVAILABLE", "ChatGPT is not configured. Configure a local callback and encryption key on the server. Hosted use needs a supported OpenAI authorization setup.")
}
func reconnect() error {
	return domain.Fail(409, "CHATGPT_RECONNECT_REQUIRED", "Reconnect ChatGPT and allow ChatGPT plan usage to continue.")
}
func providerError() error {
	return domain.Fail(502, "CHATGPT_REQUEST_FAILED", "ChatGPT could not complete this request. Check your connection and plan usage, then try again.")
}
func randomValue() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func hasScope(scopes, scope string) bool {
	for _, v := range strings.Fields(scopes) {
		if v == scope {
			return true
		}
	}
	return false
}

func (c *Client) saved(ctx context.Context) (*domain.ChatGPTConnection, *credentials, error) {
	st, err := c.db.Read(ctx, c.owner)
	if err != nil {
		return nil, nil, err
	}
	connection := st.ChatGPTConnection
	if connection == nil || connection.Credentials == "" {
		return connection, nil, nil
	}
	data, err := base64.RawStdEncoding.DecodeString(connection.Credentials)
	if err != nil || len(data) < c.aead.NonceSize() {
		return connection, nil, reconnect()
	}
	plain, err := c.aead.Open(nil, data[:c.aead.NonceSize()], data[c.aead.NonceSize():], []byte(c.owner))
	if err != nil {
		return connection, nil, reconnect()
	}
	var creds credentials
	if json.Unmarshal(plain, &creds) != nil {
		return connection, nil, reconnect()
	}
	return connection, &creds, nil
}
func (c *Client) save(ctx context.Context, connection *domain.ChatGPTConnection, creds *credentials) error {
	value := *connection
	if creds != nil {
		plain, err := json.Marshal(creds)
		if err != nil {
			return err
		}
		nonce := make([]byte, c.aead.NonceSize())
		if _, err = rand.Read(nonce); err != nil {
			return err
		}
		value.Credentials = base64.RawStdEncoding.EncodeToString(c.aead.Seal(nonce, nonce, plain, []byte(c.owner)))
	} else {
		value.Credentials = ""
	}
	_, _, err := c.db.Execute(ctx, c.owner, domain.NewID(), randomValue(), func(st *domain.State) (store.Result, error) {
		st.ChatGPTConnection = &value
		return store.JSON(200, map[string]bool{"saved": true}, 0)
	})
	return err
}

// Begin creates one short-lived authorization attempt without replacing an existing account.
func (c *Client) Begin(ctx context.Context) (string, string, error) {
	if c == nil {
		return "", "", unavailable()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	connection, creds, err := c.saved(ctx)
	if err != nil {
		return "", "", err
	}
	if connection == nil {
		connection = &domain.ChatGPTConnection{HostID: "urn:uuid:" + domain.NewID()}
		if err = c.save(ctx, connection, nil); err != nil {
			return "", "", err
		}
	}
	a := &attempt{state: randomValue(), nonce: randomValue(), verifier: randomValue(), binding: randomValue(), expires: time.Now().Add(10 * time.Minute)}
	challenge := sha256.Sum256([]byte(a.verifier))
	params := url.Values{"client_id": {"dynamic_agent_client"}, "agent_name_hint": {"Folio"}, "ext_agent_host_id": {connection.HostID}, "response_type": {"code"}, "redirect_uri": {c.config.RedirectURI}, "scope": {"openid profile email offline_access resource.invoke " + planScope}, "resource": {resource}, "state": {a.state}, "nonce": {a.nonce}, "code_challenge_method": {"S256"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])}}
	if creds != nil {
		params.Set("client_id", creds.ClientID)
		params.Del("agent_name_hint")
		params.Set("id_token_hint", creds.IDToken)
	} else if connection.ClientID != "" {
		params.Set("client_id", connection.ClientID)
		params.Del("agent_name_hint")
	}
	a.clientID = params.Get("client_id")
	c.pending = a
	return issuer + "/api/accounts/authorize?" + params.Encode(), a.binding, nil
}

// Callback consumes the attempt before exchanging a code and verifies the OIDC identity.
func (c *Client) Callback(ctx context.Context, values url.Values, binding string) error {
	if c == nil {
		return unavailable()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	a := c.pending
	if a == nil || time.Now().After(a.expires) || subtle.ConstantTimeCompare([]byte(values.Get("state")), []byte(a.state)) != 1 || subtle.ConstantTimeCompare([]byte(binding), []byte(a.binding)) != 1 {
		return domain.Fail(400, "CHATGPT_SIGN_IN_FAILED", "ChatGPT sign-in expired or could not be verified. Start again in Folio.")
	}
	c.pending = nil
	if values.Get("error") != "" || values.Get("code") == "" {
		return domain.Fail(400, "CHATGPT_SIGN_IN_FAILED", "ChatGPT sign-in was cancelled or incomplete. Start again in Folio.")
	}
	connection, existing, err := c.saved(ctx)
	if err != nil {
		return err
	}
	if connection == nil {
		return reconnect()
	}
	clientID := values.Get("client_id")
	if a.clientID != "dynamic_agent_client" {
		if clientID != "" && clientID != a.clientID {
			return reconnect()
		}
		clientID = a.clientID
	}
	if clientID == "" || clientID == "dynamic_agent_client" || len(clientID) > 512 {
		return reconnect()
	}
	if connection.ClientID != "" && connection.ClientID != clientID {
		return reconnect()
	}
	// Registration survives an expired code so the next attempt reuses its issued ID.
	if connection.ClientID == "" {
		connection.ClientID = clientID
		if err = c.save(ctx, connection, existing); err != nil {
			return err
		}
	}
	tokens, err := c.token(ctx, url.Values{"grant_type": {"authorization_code"}, "client_id": {clientID}, "code": {values.Get("code")}, "code_verifier": {a.verifier}, "redirect_uri": {c.config.RedirectURI}, "resource": {resource}})
	if err != nil {
		return err
	}
	identity, err := c.identity(ctx, tokens.IDToken, clientID, a.nonce)
	if err != nil {
		return err
	}
	if existing != nil && existing.Subject != identity.Subject {
		return reconnect()
	}
	creds := &credentials{ClientID: clientID, Subject: identity.Subject, Email: identity.Email, AccessToken: tokens.AccessToken, RefreshToken: tokens.RefreshToken, IDToken: tokens.IDToken, Scope: tokens.Scope, ExpiresAt: time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second)}
	return c.save(ctx, connection, creds)
}

func (c *Client) token(ctx context.Context, values url.Values) (*tokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", issuer+"/api/accounts/oauth/token", strings.NewReader(values.Encode()))
	if err != nil {
		return nil, providerError()
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := c.http.Do(req)
	if err != nil {
		return nil, providerError()
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, reconnect()
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return nil, providerError()
	}
	var tokens tokenResponse
	if json.Unmarshal(data, &tokens) != nil || tokens.AccessToken == "" || !strings.EqualFold(tokens.TokenType, "Bearer") || tokens.ExpiresIn <= 0 || tokens.ExpiresIn > 86400*30 {
		return nil, reconnect()
	}
	return &tokens, nil
}

func (c *Client) active(ctx context.Context) (*credentials, error) {
	connection, creds, err := c.saved(ctx)
	if err != nil {
		return nil, err
	}
	if creds == nil {
		return nil, reconnect()
	}
	if time.Now().Add(time.Minute).Before(creds.ExpiresAt) {
		return creds, nil
	}
	if creds.RefreshToken == "" {
		return nil, reconnect()
	}
	tokens, err := c.token(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {creds.RefreshToken}, "client_id": {creds.ClientID}, "resource": {resource}})
	if err != nil {
		return nil, err
	}
	creds.AccessToken = tokens.AccessToken
	creds.ExpiresAt = time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second)
	if tokens.RefreshToken != "" {
		creds.RefreshToken = tokens.RefreshToken
	}
	if tokens.Scope != "" {
		creds.Scope = tokens.Scope
	}
	if err = c.save(ctx, connection, creds); err != nil {
		return nil, err
	}
	return creds, nil
}

// Connection returns display information only. It never returns OAuth credentials.
func (c *Client) Connection(ctx context.Context) (Status, error) {
	status := Status{Models: []Model{}}
	if c == nil {
		status.Reason = unavailable().Error()
		return status, nil
	}
	status.Available = true
	c.mu.Lock()
	defer c.mu.Unlock()
	connection, saved, err := c.saved(ctx)
	status.Connected = connection != nil && connection.Credentials != ""
	if err != nil {
		status.Reason = err.Error()
		return status, nil
	}
	if saved == nil {
		return status, nil
	}
	status.Connected = true
	status.Email = saved.Email
	creds, err := c.active(ctx)
	if err != nil {
		status.Reason = err.Error()
		return status, nil
	}
	status.Sharing = hasScope(creds.Scope, planScope)
	if !status.Sharing {
		status.Reason = "Allow ChatGPT plan usage when reconnecting to use chat and uploads."
		return status, nil
	}
	status.Models, err = c.models(ctx, creds.AccessToken)
	if err != nil {
		status.Reason = err.Error()
		status.Models = []Model{}
	}
	return status, nil
}

// Disconnect clears credentials while retaining this installation's host ID.
func (c *Client) Disconnect(ctx context.Context) error {
	if c == nil {
		return unavailable()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pending = nil
	connection, _, err := c.saved(ctx)
	if err != nil {
		st, readErr := c.db.Read(ctx, c.owner)
		if readErr != nil {
			return readErr
		}
		connection = st.ChatGPTConnection
	}
	if connection == nil {
		return nil
	}
	return c.save(ctx, connection, nil)
}

func (c *Client) models(ctx context.Context, token string) ([]Model, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", resource+"/models", nil)
	if err != nil {
		return nil, providerError()
	}
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := c.http.Do(req)
	if err != nil {
		return nil, providerError()
	}
	defer response.Body.Close()
	if response.StatusCode == 401 || response.StatusCode == 403 {
		return nil, reconnect()
	}
	if response.StatusCode != 200 {
		return nil, providerError()
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
	if err != nil || len(data) > 2<<20 {
		return nil, providerError()
	}
	var list struct {
		Models []struct {
			Slug        string `json:"slug"`
			DisplayName string `json:"display_name"`
			Visibility  string `json:"visibility"`
		} `json:"models"`
	}
	if json.Unmarshal(data, &list) != nil {
		return nil, providerError()
	}
	result := []Model{}
	for _, m := range list.Models {
		if m.Visibility == "list" && m.Slug != "" {
			result = append(result, Model{m.Slug, m.DisplayName})
		}
	}
	return result, nil
}

// Respond uses a currently available model and accepts only completed streamed responses.
func (c *Client) Respond(ctx context.Context, model, instructions string, content []map[string]any) (string, error) {
	if c == nil {
		return "", unavailable()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	creds, err := c.active(ctx)
	if err != nil {
		return "", err
	}
	if !hasScope(creds.Scope, planScope) {
		return "", reconnect()
	}
	models, err := c.models(ctx, creds.AccessToken)
	if err != nil {
		return "", err
	}
	if len(models) == 0 {
		return "", domain.Fail(409, "CHATGPT_NO_MODELS", "No models are available for this ChatGPT connection.")
	}
	if model == "" {
		model = models[0].ID
	}
	found := false
	for _, m := range models {
		if m.ID == model {
			found = true
		}
	}
	if !found {
		return "", domain.Invalid("Choose a model available to your ChatGPT account.")
	}
	body, err := json.Marshal(map[string]any{"model": model, "instructions": instructions, "input": []map[string]any{{"role": "user", "content": content}}, "store": false, "stream": true})
	if err != nil {
		return "", providerError()
	}
	req, err := http.NewRequestWithContext(ctx, "POST", resource+"/responses", bytes.NewReader(body))
	if err != nil {
		return "", providerError()
	}
	req.Header.Set("Authorization", "Bearer "+creds.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	response, err := c.http.Do(req)
	if err != nil {
		return "", providerError()
	}
	defer response.Body.Close()
	if response.StatusCode == 401 || response.StatusCode == 403 {
		return "", reconnect()
	}
	if response.StatusCode != 200 {
		return "", providerError()
	}
	scanner := bufio.NewScanner(io.LimitReader(response.Body, 8<<20))
	scanner.Buffer(make([]byte, 4096), 2<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var event struct {
			Type     string `json:"type"`
			Response struct {
				Status string `json:"status"`
				Output []struct {
					Content []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"content"`
				} `json:"output"`
			} `json:"response"`
		}
		if json.Unmarshal([]byte(data), &event) != nil {
			return "", providerError()
		}
		if event.Type == "error" || event.Type == "response.failed" || event.Type == "response.incomplete" {
			return "", providerError()
		}
		if event.Type == "response.completed" && event.Response.Status == "completed" {
			var output strings.Builder
			for _, item := range event.Response.Output {
				for _, part := range item.Content {
					if part.Type == "output_text" {
						output.WriteString(part.Text)
					}
				}
			}
			if output.Len() == 0 {
				return "", providerError()
			}
			return output.String(), nil
		}
	}
	return "", providerError()
}

type identityClaims struct {
	jwt.RegisteredClaims
	Nonce string `json:"nonce"`
	Email string `json:"email"`
}

func (c *Client) identity(ctx context.Context, raw, clientID, nonce string) (*identityClaims, error) {
	if raw == "" {
		return nil, reconnect()
	}
	claims := &identityClaims{}
	parsed, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) { return c.signingKey(ctx, token) }, jwt.WithValidMethods([]string{"RS256", "ES256"}), jwt.WithIssuer(issuer), jwt.WithAudience(clientID), jwt.WithExpirationRequired(), jwt.WithIssuedAt(), jwt.WithLeeway(5*time.Second))
	if err != nil || !parsed.Valid || claims.Subject == "" || claims.IssuedAt == nil || subtle.ConstantTimeCompare([]byte(claims.Nonce), []byte(nonce)) != 1 {
		return nil, reconnect()
	}
	return claims, nil
}
func (c *Client) signingKey(ctx context.Context, token *jwt.Token) (any, error) {
	kid, ok := token.Header["kid"].(string)
	if !ok || kid == "" {
		return nil, errors.New("missing signing key")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", issuer+"/.well-known/jwks.json", nil)
	if err != nil {
		return nil, err
	}
	response, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, errors.New("signing keys unavailable")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return nil, errors.New("invalid signing keys")
	}
	var document struct {
		Keys []struct{ Kty, Kid, Use, Alg, N, E, Crv, X, Y string } `json:"keys"`
	}
	if json.Unmarshal(data, &document) != nil {
		return nil, errors.New("invalid signing keys")
	}
	for _, key := range document.Keys {
		if key.Kid != kid || (key.Use != "" && key.Use != "sig") || (key.Alg != "" && key.Alg != token.Method.Alg()) {
			continue
		}
		if token.Method.Alg() == "RS256" && key.Kty == "RSA" {
			n, e1 := base64.RawURLEncoding.DecodeString(key.N)
			e, e2 := base64.RawURLEncoding.DecodeString(key.E)
			if e1 != nil || e2 != nil || len(n) < 256 || len(n) > 1024 || len(e) == 0 || len(e) > 4 {
				continue
			}
			exponent := new(big.Int).SetBytes(e).Int64()
			if exponent < 3 || exponent > 1<<31-1 {
				continue
			}
			return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(exponent)}, nil
		}
		if token.Method.Alg() == "ES256" && key.Kty == "EC" && key.Crv == "P-256" {
			x, e1 := base64.RawURLEncoding.DecodeString(key.X)
			y, e2 := base64.RawURLEncoding.DecodeString(key.Y)
			if e1 != nil || e2 != nil || len(x) != 32 || len(y) != 32 {
				continue
			}
			px, py := new(big.Int).SetBytes(x), new(big.Int).SetBytes(y)
			if !elliptic.P256().IsOnCurve(px, py) {
				continue
			}
			return &ecdsa.PublicKey{Curve: elliptic.P256(), X: px, Y: py}, nil
		}
	}
	return nil, errors.New("unknown signing key")
}
