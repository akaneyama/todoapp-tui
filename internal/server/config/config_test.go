package config

import (
	"testing"
	"time"
)

func TestLoadDefaultsAndOverrides(t *testing.T) {
	t.Setenv("DB_DSN", "u:p@tcp(localhost:3306)/todoapp")
	t.Setenv("SESSION_TTL", "48h")
	t.Setenv("TRUST_PROXY", "true")

	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":8080" || !cfg.AutoMigrate || cfg.SessionTouchInterval != time.Hour {
		t.Errorf("defaults not applied: %+v", cfg)
	}
	if cfg.SessionTTL != 48*time.Hour || !cfg.TrustProxy {
		t.Errorf("overrides not applied: %+v", cfg)
	}
}

func TestLoadErrors(t *testing.T) {
	t.Setenv("DB_DSN", "")
	t.Setenv("SESSION_TTL", "banana")
	if _, err := Load(""); err == nil {
		t.Fatal("expected error for missing DB_DSN and bad SESSION_TTL")
	}
}
