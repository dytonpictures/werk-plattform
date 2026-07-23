package main

import (
	"context"
	"fmt"
	"os"

	"github.com/dytonpictures/werk/internal/platform/config"
	"github.com/dytonpictures/werk/internal/platform/database"
	"github.com/dytonpictures/werk/internal/platform/migrate"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "invalid configuration: %v\n", err)
		os.Exit(1)
	}
	logger := config.NewLogger(cfg, "migrate")
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
