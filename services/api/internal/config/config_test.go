package config

import (
	"strings"
	"testing"
)

func TestConfigurationSafety(t *testing.T) {
	base := map[string]string{"APP_ENV": "development", "AUTH_MODE": "dev", "DEV_AUTH_TOKEN": strings.Repeat("x", 32), "OWNER_ID": "11111111-1111-4111-8111-111111111111", "DATABASE_URL": "postgres://folio:local@localhost:5432/folio?sslmode=disable"}
	load := func(values map[string]string) (Config, error) {
		return FromEnv(func(k string) string { return values[k] })
	}
	config, err := load(base)
	if err != nil {
		t.Fatal(err)
	}
	if config.WebDistDir != "" {
		t.Fatalf("WEB_DIST_DIR should be unset by default, got %q", config.WebDistDir)
	}
	for _, change := range []map[string]string{{"APP_ENV": "production"}, {"DATABASE_URL": ""}, {"OWNER_ID": "arbitrary"}, {"AUTH_MODE": ""}, {"DEV_AUTH_TOKEN": "short"}, {"PORT": "70000"}, {"APP_ENV": "unexpected"}} {
		changed := make(map[string]string)
		for k, v := range base {
			changed[k] = v
		}
		for k, v := range change {
			changed[k] = v
		}
		if _, err := load(changed); err == nil {
			t.Errorf("unsafe configuration accepted: %v", change)
		}
	}
	configured := make(map[string]string)
	for k, v := range base {
		configured[k] = v
	}
	configured["WEB_DIST_DIR"] = "./apps/web/dist"
	config, err = load(configured)
	if err != nil {
		t.Fatal(err)
	}
	if config.WebDistDir != configured["WEB_DIST_DIR"] {
		t.Fatalf("WEB_DIST_DIR=%q want %q", config.WebDistDir, configured["WEB_DIST_DIR"])
	}
	configured["WEB_DIST_DIR"] = " \t"
	config, err = load(configured)
	if err != nil || config.WebDistDir != "" {
		t.Fatalf("blank WEB_DIST_DIR should preserve API-only behavior: config=%+v err=%v", config, err)
	}
	configured["WEB_DIST_DIR"] = "bad\x00path"
	if _, err := load(configured); err == nil {
		t.Fatal("WEB_DIST_DIR with NUL byte was accepted")
	}
	prod := map[string]string{"APP_ENV": "production", "AUTH_MODE": "jwt", "OWNER_ID": base["OWNER_ID"], "DATABASE_URL": "postgres://folio:local@db.example:5432/folio?sslmode=verify-full", "JWT_ISSUER": "https://project.example/auth/v1", "JWT_AUDIENCE": "authenticated"}
	if _, err := load(prod); err != nil {
		t.Fatal(err)
	}
	prod["DATABASE_URL"] = "postgres://folio:local@db.example:5432/folio?sslmode=require"
	if _, err := load(prod); err == nil {
		t.Fatal("production accepted unverifiable TLS")
	}
}
