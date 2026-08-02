package main

import (
	"context"
	"fmt"
	"os"

	"github.com/dytonpictures/werk/internal/platform/config"
	"github.com/dytonpictures/werk/internal/platform/database"
	"github.com/dytonpictures/werk/internal/platform/envfile"
	"github.com/dytonpictures/werk/internal/platform/migrate"
)

func main() {
	if err := envfile.LoadProcess(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "invalid .env file: %v\n", err)
		os.Exit(1)
	}
	cfg, err := config.LoadMigration()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "invalid configuration: %v\n", err)
		os.Exit(1)
	}
	logger := config.NewComponentLogger(cfg.Environment, cfg.BuildVersion, "migrate")
	pool, err := database.NewMigrationPool(context.Background(), cfg.DatabaseURL)
	if err != nil {
		logger.Error("database pool could not be created", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := migrate.Apply(context.Background(), pool); err != nil {
		logger.Error("migration failed", "error", err)
		os.Exit(1)
	}
	logger.Info("migrations complete")
}
