package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dytonpictures/werk/internal/platform/config"
)

type readinessChecker interface {
	Ping(context.Context) error
}

func NewRouter(cfg config.Config, readiness readinessChecker, logger *slog.Logger, auth ...AuthService) http.Handler {
	var authService AuthService
	if len(auth) > 0 {
		authService = auth[0]
	}
	return NewRouterWithServices(cfg, readiness, logger, authService, nil, nil)
}

func NewRouterWithAdmin(cfg config.Config, readiness readinessChecker, logger *slog.Logger, authService AuthService, adminService AdminService) http.Handler {
	return NewRouterWithServices(cfg, readiness, logger, authService, nil, adminService)
}

func NewRouterWithServices(cfg config.Config, readiness readinessChecker, logger *slog.Logger, authService AuthService, workspaceService WorkspaceService, adminService AdminService) http.Handler {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	metrics := newHTTPMetrics(cfg.BuildVersion)
	router := chi.NewRouter()
	router.Use(requestIdentityMiddleware)
	router.Use(securityHeadersMiddleware)
	router.Use(metrics.middleware)
	router.Use(accessLogMiddleware(logger))
	router.Use(recoveryMiddleware(logger))
	router.Use(correlationValidationMiddleware)
	router.Use(browserMutationProtectionMiddleware(cfg.AllowedOrigins))
	router.Mount("/api/v1/auth", authRoutes(authService))
	router.Mount("/api/v1", workRoutes(authService, workspaceService))
	router.Mount("/admin/v1", adminRoutes(authService, adminService))

	router.Get("/health/live", func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
	})
	router.Get("/health/ready", func(writer http.ResponseWriter, request *http.Request) {
		if readiness == nil {
			writeProblem(writer, request, http.StatusServiceUnavailable, "not-ready", "Service not ready", "A required dependency is unavailable.")
			return
		}

		checkContext, cancel := context.WithTimeout(request.Context(), 2*time.Second)
		defer cancel()
		if err := readiness.Ping(checkContext); err != nil {
			logger.WarnContext(request.Context(), "readiness check failed", "dependency", "postgresql", "error", err)
			writeProblem(writer, request, http.StatusServiceUnavailable, "not-ready", "Service not ready", "A required dependency is unavailable.")
			return
		}
		writeJSON(writer, http.StatusOK, map[string]string{"status": "ready"})
	})

	// Operational metadata does not belong to the work or admin API namespace.
	// strings are using in a environment variable to avoid hardcoding them in the codebase. This allows for easier configuration and flexibility in different deployment environments.
	router.Get("/meta", func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusOK, map[string]string{
			"product":     "WERK",
			"service":     "werk-api",
			"version":     cfg.BuildVersion,
			"api_version": "v1",
		})
	})

	// This endpoint is intended for internal scraping and is not exposed by Caddy.
	router.Get("/metrics", metrics.serveHTTP)

	router.NotFound(func(writer http.ResponseWriter, request *http.Request) {
		writeProblem(writer, request, http.StatusNotFound, "not-found", "Resource not found", "The requested resource does not exist.")
	})
	router.MethodNotAllowed(func(writer http.ResponseWriter, request *http.Request) {
		writeProblem(writer, request, http.StatusMethodNotAllowed, "method-not-allowed", "Method not allowed", "The requested method is not supported for this resource.")
	})

	return router
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
