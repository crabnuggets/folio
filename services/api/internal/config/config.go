package config

import (
	"errors"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/crabnuggets/folio/services/api/internal/auth"
	"github.com/crabnuggets/folio/services/api/internal/domain"
)

type Config struct {
	Address     string
	DatabaseURL string
	OwnerID     string
	Auth        auth.Config
}

// Load reads configuration from process environment variables, returning FromEnv errors.
func Load() (Config, error) { return FromEnv(os.Getenv) }
// FromEnv reads named environment variables through get, defaulting to development,
// port 8080, and JWT authentication. It validates owner, database URL, port, and
// authentication settings without connecting to dependencies. Invalid configuration
// returns an error, including auth.New errors; production requires verified DB TLS.
func FromEnv(get func(string) string) (Config, error) {
	env := get("APP_ENV")
	if env == "" {
		env = "development"
	}
	if env != "development" && env != "test" && env != "production" {
		return Config{}, errors.New("APP_ENV must be development, test or production")
	}
	owner := strings.ToLower(get("OWNER_ID"))
	if !domain.ValidID(owner) {
		return Config{}, errors.New("OWNER_ID must be a UUID")
	}
	databaseURL := get("DATABASE_URL")
	u, err := url.Parse(databaseURL)
	if err != nil || u.Host == "" || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		return Config{}, errors.New("DATABASE_URL must be an absolute PostgreSQL URL")
	}
	if env == "production" && u.Query().Get("sslmode") != "verify-full" {
		return Config{}, errors.New("production DATABASE_URL requires sslmode=verify-full")
	}
	port := get("PORT")
	if port == "" {
		port = "8080"
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return Config{}, errors.New("PORT must be between 1 and 65535")
	}
	mode := get("AUTH_MODE")
	if mode == "" {
		mode = "jwt"
	}
	a := auth.Config{Mode: mode, Environment: env, OwnerID: owner, DevToken: get("DEV_AUTH_TOKEN"), Issuer: get("JWT_ISSUER"), Audience: get("JWT_AUDIENCE"), JWKSURL: get("JWT_JWKS_URL")}
	if _, err := auth.New(a); err != nil {
		return Config{}, err
	}
	return Config{Address: ":" + port, DatabaseURL: databaseURL, OwnerID: owner, Auth: a}, nil
}
