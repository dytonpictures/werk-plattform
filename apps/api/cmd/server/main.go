package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dytonpictures/werk-plattform/apps/api/internal/app"
	"github.com/dytonpictures/werk-plattform/apps/api/internal/auth"
	"github.com/dytonpictures/werk-plattform/apps/api/internal/config"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if len(os.Args) > 1 {
		runUtility(os.Args[1])
		return
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("create database pool", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           app.NewHandler(cfg, db, logger),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("server shutdown", "error", err)
		}
	}()

	logger.Info("api listening", "address", cfg.HTTPAddr, "environment", cfg.Environment)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func runUtility(command string) {
	switch command {
	case "healthcheck":
		client := http.Client{Timeout: 2 * time.Second}
		response, err := client.Get("http://127.0.0.1:8080/health")
		if err != nil || response.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		_ = response.Body.Close()
	case "migrate":
		cfg, err := config.Load()
		if err != nil {
			println(err.Error())
			os.Exit(1)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		db, err := pgxpool.New(ctx, cfg.DatabaseURL)
		if err != nil {
			println(err.Error())
			os.Exit(1)
		}
		defer db.Close()
		migration, err := os.ReadFile("/app/migrations/00001_platform.sql")
		if err != nil {
			println(err.Error())
			os.Exit(1)
		}
		if _, err := db.Exec(ctx, string(migration)); err != nil {
			println(err.Error())
			os.Exit(1)
		}
		println("migration 00001 applied")
	case "bootstrap-admin":
		cfg, err := config.Load()
		if err != nil {
			println(err.Error())
			os.Exit(1)
		}
		email, password := os.Getenv("WERK_BOOTSTRAP_EMAIL"), os.Getenv("WERK_BOOTSTRAP_PASSWORD")
		if email == "" || password == "" {
			println("WERK_BOOTSTRAP_EMAIL and WERK_BOOTSTRAP_PASSWORD are required")
			os.Exit(2)
		}
		hash, err := auth.HashPassword(password)
		if err != nil {
			println(err.Error())
			os.Exit(2)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		db, err := pgxpool.New(ctx, cfg.DatabaseURL)
		if err != nil {
			println(err.Error())
			os.Exit(1)
		}
		defer db.Close()
		userID, roleID := uuid.New(), uuid.New()
		_, err = db.Exec(ctx, `INSERT INTO roles (id,name,description) VALUES ($1,'system_admin','Full platform administration') ON CONFLICT (name) DO UPDATE SET description=EXCLUDED.description`, roleID)
		if err == nil {
			_, err = db.Exec(ctx, `INSERT INTO users (id,email,display_name,password_hash) VALUES ($1,lower($2),'System Administrator',$3) ON CONFLICT (email) DO UPDATE SET password_hash=EXCLUDED.password_hash,is_active=true`, userID, email, hash)
		}
		if err == nil {
			_, err = db.Exec(ctx, `INSERT INTO user_roles (user_id,role_id) SELECT u.id,r.id FROM users u CROSS JOIN roles r WHERE u.email=lower($1) AND r.name='system_admin' ON CONFLICT DO NOTHING`, email)
		}
		if err != nil {
			println(err.Error())
			os.Exit(1)
		}
		println("administrator bootstrapped")
	default:
		println("unknown command: " + command)
		os.Exit(2)
	}
}
