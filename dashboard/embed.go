// Package webui exposes the static WERK web client from the native API server.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed public/*
var embeddedFiles embed.FS

var pageRoutes = map[string]string{
	"/":                "index.html",
	"/activate":        "activate.html",
	"/change-password": "change-password.html",
	"/mfa":             "mfa.html",
	"/mfa-setup":       "mfa-setup.html",
	"/admin":           "admin.html",
	"/app":             "work.html",
	"/documents":       "documents.html",
	"/profile":         "profile.html",
}

// NewHandler serves the embedded web client and dispatches versioned API,
// admin, service, health and metadata paths to api. Metrics remain hidden from
// this public combined listener until they have a separate operator boundary.
func NewHandler(api http.Handler) http.Handler {
	publicFiles, err := fs.Sub(embeddedFiles, "public")
	if err != nil {
		panic("embedded web UI is unavailable: " + err.Error())
	}
	if api == nil {
		api = http.NotFoundHandler()
	}

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestPath := request.URL.Path
		if requestPath == "/metrics" || strings.HasPrefix(requestPath, "/metrics/") {
			http.NotFound(writer, request)
			return
		}
		if isAPIPath(requestPath) {
			api.ServeHTTP(writer, request)
			return
		}

		setWebSecurityHeaders(writer.Header())
		writer.Header().Set("Cache-Control", "no-store")
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			writer.Header().Set("Allow", "GET, HEAD")
			http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		filename := pageRoutes[requestPath]
		if filename == "" {
			filename = strings.TrimPrefix(path.Clean(requestPath), "/")
		}
		if filename == "" || filename == "." || !fs.ValidPath(filename) || strings.Contains(filename, "/") {
			http.NotFound(writer, request)
			return
		}
		information, statErr := fs.Stat(publicFiles, filename)
		if statErr != nil || information.IsDir() {
			http.NotFound(writer, request)
			return
		}
		http.ServeFileFS(writer, request, publicFiles, filename)
	})
}

func isAPIPath(requestPath string) bool {
	if requestPath == "/meta" {
		return true
	}
	for _, prefix := range []string{"/api", "/service", "/health"} {
		if requestPath == prefix || strings.HasPrefix(requestPath, prefix+"/") {
			return true
		}
	}
	// /admin is a UI route; only descendants belong to the admin API.
	return strings.HasPrefix(requestPath, "/admin/")
}

func setWebSecurityHeaders(headers http.Header) {
	headers.Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; font-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	headers.Set("Cross-Origin-Opener-Policy", "same-origin")
	headers.Set("Cross-Origin-Resource-Policy", "same-origin")
	headers.Set("Permissions-Policy", "camera=(), geolocation=(), microphone=()")
	headers.Set("Referrer-Policy", "no-referrer")
	headers.Set("X-Content-Type-Options", "nosniff")
	headers.Set("X-Frame-Options", "DENY")
}
