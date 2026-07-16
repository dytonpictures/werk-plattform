package config

import (
	"errors"
	"os"
	"strings"
)

type Config struct {
	Environment string
	Version     string
	HTTPAddr    string
	DatabaseURL string
}

func Load() (Config, error) {
	cfg := Config{
		Environment: valueOrDefault("WERK_ENV", "development"),
		Version:     valueOrDefault("WERK_VERSION", "0.1.0-dev"),
		HTTPAddr:    valueOrDefault("WERK_HTTP_ADDR", ":8080"),
		DatabaseURL: strings.TrimSpace(os.Getenv("WERK_DATABASE_URL")),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("WERK_DATABASE_URL is required")
	}
	return cfg, nil
}

func valueOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
