package config

import "testing"

func TestLoadUsesDevelopmentDefaults(t *testing.T) {
	cfg, err := load(environment(map[string]string{}))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Environment != "development" {
		t.Fatalf("environment = %q, want development", cfg.Environment)
	}
	if cfg.HTTPAddress != ":8080" {
		t.Fatalf("http address = %q, want :8080", cfg.HTTPAddress)
	}
	if cfg.BootstrapAdminPassword != "werk-development" {
		t.Fatalf("bootstrap password = %q, want development default", cfg.BootstrapAdminPassword)
	}
	if cfg.DevelopmentWorkPassword != "werk-worker-development" {
		t.Fatalf("development work password = %q, want development default", cfg.DevelopmentWorkPassword)
	}
	if cfg.WorkerConcurrency != 4 {
		t.Fatalf("worker concurrency = %d, want 4", cfg.WorkerConcurrency)
	}
	if len(cfg.AllowedOrigins) != 2 {
		t.Fatalf("allowed origins = %v", cfg.AllowedOrigins)
	}
}

func TestLoadRejectsInvalidWorkerConcurrency(t *testing.T) {
	if _, err := load(environment(map[string]string{"WERK_WORKER_CONCURRENCY": "0"})); err == nil {
		t.Fatal("invalid worker concurrency was accepted")
	}
}

func TestLoadRejectsWeakBootstrapPassword(t *testing.T) {
	_, err := load(environment(map[string]string{"WERK_BOOTSTRAP_ADMIN_PASSWORD": "short"}))
	if err == nil {
		t.Fatal("expected weak bootstrap password to fail")
	}
}

func TestLoadRejectsDevelopmentWorkAccountOutsideDevelopment(t *testing.T) {
	_, err := load(environment(map[string]string{
		"WERK_ENV":                 "test",
		"WERK_DEV_WORKER_PASSWORD": "werk-worker-development",
	}))
	if err == nil {
		t.Fatal("development work account was accepted outside development")
	}
}

func TestLoadEnablesDevelopmentMFAAndRequiresEncryptionKey(t *testing.T) {
	cfg, err := load(environment(map[string]string{}))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.IdentityMFAEnabled || len(cfg.IdentityMFAKey) != 32 || cfg.IdentityMFACurrentKeyID != "default" || len(cfg.IdentityMFAKeys) != 1 {
		t.Fatal("development MFA is not active with a valid default key")
	}
	if _, err := load(environment(map[string]string{"WERK_ENV": "test", "WERK_IDENTITY_MFA_ENABLED": "true"})); err == nil {
		t.Fatal("MFA without encryption key was accepted")
	}
	cfg, err = load(environment(map[string]string{
		"WERK_IDENTITY_MFA_ENABLED": "true",
		"WERK_IDENTITY_MFA_KEY":     "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY",
		"WERK_ALLOWED_ORIGINS":      "https://werk.example",
	}))
	if err != nil || !cfg.IdentityMFAEnabled || len(cfg.IdentityMFAKey) != 32 {
		t.Fatalf("valid MFA configuration rejected: %v", err)
	}
}

func TestLoadAcceptsMFAKeyringForRotation(t *testing.T) {
	cfg, err := load(environment(map[string]string{
		"WERK_IDENTITY_MFA_ENABLED":        "true",
		"WERK_IDENTITY_MFA_KEYS":           "current:MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY,old:MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY",
		"WERK_IDENTITY_MFA_CURRENT_KEY_ID": "current",
		"WERK_ALLOWED_ORIGINS":             "https://example.test",
	}))
	if err != nil {
		t.Fatalf("load keyring: %v", err)
	}
	if cfg.IdentityMFACurrentKeyID != "current" || len(cfg.IdentityMFAKeys) != 2 || len(cfg.IdentityMFAKey) != 32 {
		t.Fatalf("unexpected keyring config: current=%q keys=%d", cfg.IdentityMFACurrentKeyID, len(cfg.IdentityMFAKeys))
	}
}

func TestLoadRequiresExplicitDatabaseURLInProduction(t *testing.T) {
	_, err := load(environment(map[string]string{"WERK_ENV": "production"}))
	if err == nil {
		t.Fatal("expected production configuration without DATABASE_URL to fail")
	}
}

func TestLoadRejectsUnknownEnvironment(t *testing.T) {
	_, err := load(environment(map[string]string{"WERK_ENV": "staging"}))
	if err == nil {
		t.Fatal("expected unknown environment to fail")
	}
}

func TestLoadRejectsInvalidHTTPAddress(t *testing.T) {
	_, err := load(environment(map[string]string{"WERK_HTTP_ADDRESS": "8080"}))
	if err == nil {
		t.Fatal("expected address without host separator to fail")
	}
}

func TestLoadAcceptsProductionConfiguration(t *testing.T) {
	cfg, err := load(environment(map[string]string{
		"WERK_ENV":                  "production",
		"WERK_HTTP_ADDRESS":         "127.0.0.1:8081",
		"DATABASE_URL":              "postgresql://runtime:secret@database:5432/werk?sslmode=require",
		"IDENTITY_DATABASE_URL":     "postgresql://identity:other-secret@database:5432/werk?sslmode=require",
		"ADMIN_DATABASE_URL":        "postgresql://admin:admin-secret@database:5432/werk?sslmode=require",
		"WERK_IDENTITY_MFA_ENABLED": "true",
		"WERK_IDENTITY_MFA_KEY":     "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY",
		"WERK_ALLOWED_ORIGINS":      "https://werk.example",
	}))
	if err != nil {
		t.Fatalf("load production config: %v", err)
	}
	if cfg.Environment != "production" {
		t.Fatalf("environment = %q, want production", cfg.Environment)
	}
}

func TestLoadRejectsInsecureProductionIdentityTransport(t *testing.T) {
	base := map[string]string{
		"WERK_ENV":                  "production",
		"DATABASE_URL":              "postgresql://runtime:secret@database:5432/werk?sslmode=require",
		"IDENTITY_DATABASE_URL":     "postgresql://identity:other-secret@database:5432/werk?sslmode=require",
		"ADMIN_DATABASE_URL":        "postgresql://admin:admin-secret@database:5432/werk?sslmode=require",
		"WERK_IDENTITY_MFA_ENABLED": "true",
		"WERK_IDENTITY_MFA_KEY":     "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY",
		"WERK_ALLOWED_ORIGINS":      "http://example.test",
	}
	if _, err := load(environment(base)); err == nil {
		t.Fatal("insecure production origin was accepted")
	}
	base["WERK_ALLOWED_ORIGINS"] = "https://example.test"
	base["IDENTITY_DATABASE_URL"] = "postgresql://identity:other-secret@database:5432/werk?sslmode=disable"
	if _, err := load(environment(base)); err == nil {
		t.Fatal("insecure production identity database URL was accepted")
	}
}

func TestLoadRejectsDevelopmentDatabaseCredentialsInProduction(t *testing.T) {
	_, err := load(environment(map[string]string{
		"WERK_ENV":     "production",
		"DATABASE_URL": "postgresql://werk_work_runtime:werk-work-dev@database:5432/werk",
	}))
	if err == nil {
		t.Fatal("expected production configuration with development password to fail")
	}
}

func TestLoadRejectsBackupDevelopmentCredentialInProduction(t *testing.T) {
	_, err := load(environment(map[string]string{
		"WERK_ENV":     "production",
		"DATABASE_URL": "postgresql://werk_backup:werk-backup-dev@database:5432/werk",
	}))
	if err == nil {
		t.Fatal("expected production configuration with backup development password to fail")
	}
}

func TestLoadRejectsBootstrapRoleInProduction(t *testing.T) {
	_, err := load(environment(map[string]string{
		"WERK_ENV":     "production",
		"DATABASE_URL": "postgresql://werk:strong-secret@database:5432/werk",
	}))
	if err == nil {
		t.Fatal("expected production configuration with bootstrap role to fail")
	}
}

func environment(values map[string]string) lookupEnvironment {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}
