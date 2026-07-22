// passende dokumentation fehlt noch
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/dytonpictures/werk/internal/platform/config"
	"github.com/dytonpictures/werk/internal/platform/database"
	"github.com/dytonpictures/werk/internal/platform/outbox"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "invalid configuration: %v\n", err)
		os.Exit(1)
	}
	logger := config.NewLogger(cfg)
	workerDatabase, err := database.NewWorker(context.Background(), cfg.DatabaseURL, "werk-worker")
	if err != nil {
		logger.Error("runtime database could not be created", "error", err)
		os.Exit(1)
	}
	defer workerDatabase.Close()
	store, err := outbox.NewStore(workerDatabase)
	if err != nil {
		logger.Error("outbox store could not be created", "error", err)
		os.Exit(1)
	}
	hostname, _ := os.Hostname()
	workerID := hostname + "-" + strconv.Itoa(os.Getpid())
	registry := outbox.NewRegistry()
	runtime, err := outbox.NewRuntime(store, registry, logger, workerID, cfg.WorkerConcurrency)
	if err != nil {
		logger.Error("outbox runtime could not be created", "error", err)
		os.Exit(1)
	}

	signalContext, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	logger.Info("worker started", "environment", cfg.Environment, "concurrency", cfg.WorkerConcurrency)
	runtime.Run(signalContext)
	logger.Info("worker stopped")
}
