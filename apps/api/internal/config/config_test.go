package config

import "testing"

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("WERK_DATABASE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected missing database URL to fail")
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("WERK_DATABASE_URL", "postgres://example")
	t.Setenv("WERK_ENV", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Environment != "development" || cfg.HTTPAddr != ":8080" {
		t.Fatalf("unexpected defaults: %#v", cfg)
	}
}
