// passende dokumentation fehlt noch
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/platform/adminstore"
	"github.com/dytonpictures/werk/internal/platform/config"
	"github.com/dytonpictures/werk/internal/platform/database"
	"github.com/dytonpictures/werk/internal/platform/httpapi"
	"github.com/dytonpictures/werk/internal/platform/identitystore"
	"github.com/dytonpictures/werk/internal/platform/workspacestore"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "invalid configuration: %v\n", err)
		os.Exit(1)
	}
	logger := config.NewLogger(cfg)
	workDatabase, err := database.NewWork(context.Background(), cfg.DatabaseURL, "werk-api-work")
	if err != nil {
		logger.Error("work database could not be created", "error", err)
		os.Exit(1)
	}
	defer workDatabase.Close()
	workspaceService, err := workspacestore.New(workDatabase)
	if err != nil {
		logger.Error("workspace service could not be created", "error", err)
		os.Exit(1)
	}
	identityDatabase, err := database.NewIdentity(context.Background(), cfg.IdentityDatabaseURL, "werk-api-identity")
	if err != nil {
		logger.Error("identity database could not be created", "error", err)
		os.Exit(1)
	}
	defer identityDatabase.Close()
	authService, err := identitystore.New(identityDatabase, identitystore.WithMFAKeyring(
		cfg.IdentityMFAEnabled, cfg.IdentityMFACurrentKeyID, cfg.IdentityMFAKeys,
	))
	if err != nil {
		logger.Error("identity service could not be created", "error", err)
		os.Exit(1)
	}
	adminDatabase, err := database.NewAdmin(context.Background(), cfg.AdminDatabaseURL, "werk-api-admin")
	if err != nil {
		logger.Error("admin database could not be created", "error", err)
		os.Exit(1)
	}
	defer adminDatabase.Close()
	adminService, err := adminstore.New(adminDatabase)
	if err != nil {
		logger.Error("admin service could not be created", "error", err)
		os.Exit(1)
	}
	if cfg.BootstrapAdminPassword != "" {
		if err := authService.BootstrapAdmin(context.Background(), "admin@werk.local", "Initial Administrator", cfg.BootstrapAdminPassword); err != nil && !errors.Is(err, identity.ErrBootstrapAlreadyUsed) {
			logger.Error("initial administrator could not be bootstrapped", "error", err)
			os.Exit(1)
		}
	}
	if cfg.DevelopmentWorkPassword != "" {
		if err := adminService.EnsureDevelopmentWorkAccount(context.Background(), cfg.DevelopmentWorkPassword); err != nil {
			logger.Error("development work account could not be bootstrapped", "error", err)
			os.Exit(1)
		}
	}

	server := &http.Server{
		Addr:              cfg.HTTPAddress,
		Handler:           httpapi.NewRouterWithServices(cfg, workDatabase, logger, authService, workspaceService, adminService),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		logger.Info("api server started", "address", cfg.HTTPAddress, "environment", cfg.Environment)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("api server stopped unexpectedly", "error", err)
			os.Exit(1)
		}
	}()

	signalContext, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-signalContext.Done()

	shutdownContext, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.Info("api server stopped")
}
