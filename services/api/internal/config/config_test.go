package config

import (
	"strings"
	"testing"
)

func TestConfigurationSafety(t *testing.T) {
	base := map[string]string{"APP_ENV": "development", "AUTH_MODE": "dev", "DEV_AUTH_TOKEN": strings.Repeat("x", 32), "OWNER_ID": "11111111-1111-4111-8111-111111111111", "DATABASE_URL": "postgres://folio:local@localhost:5432/folio?sslmode=disable"}
	load := func(values map[string]string) error {
		_, err := FromEnv(func(k string) string { return values[k] })
		return err
	}
	if err := load(base); err != nil {
		t.Fatal(err)
	}
	for _, change := range []map[string]string{{"APP_ENV": "production"}, {"DATABASE_URL": ""}, {"OWNER_ID": "arbitrary"}, {"AUTH_MODE": ""}, {"DEV_AUTH_TOKEN": "short"}, {"PORT": "70000"}, {"APP_ENV": "unexpected"}} {
		changed := make(map[string]string)
		for k, v := range base {
			changed[k] = v
		}
		for k, v := range change {
			changed[k] = v
		}
		if err := load(changed); err == nil {
			t.Errorf("unsafe configuration accepted: %v", change)
		}
	}
	prod := map[string]string{"APP_ENV": "production", "AUTH_MODE": "jwt", "OWNER_ID": base["OWNER_ID"], "DATABASE_URL": "postgres://folio:local@db.example:5432/folio?sslmode=verify-full", "JWT_ISSUER": "https://project.example/auth/v1", "JWT_AUDIENCE": "authenticated"}
	if err := load(prod); err != nil {
		t.Fatal(err)
	}
	prod["DATABASE_URL"] = "postgres://folio:local@db.example:5432/folio?sslmode=require"
	if err := load(prod); err == nil {
		t.Fatal("production accepted unverifiable TLS")
	}
}
