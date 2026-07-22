package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dytonpictures/werk/internal/platform/config"
)

type readinessStub struct {
	err error
}

func (stub readinessStub) Ping(context.Context) error {
	return stub.err
}

func TestLiveHealthcheckDoesNotNeedDatabase(t *testing.T) {
	response := request(t, NewRouter(config.Config{BuildVersion: "test"}, nil, testLogger()), http.MethodGet, "/health/live", "")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if !validUUID(response.Header().Get(requestIDHeader)) {
		t.Fatalf("X-Request-ID = %q, want UUID", response.Header().Get(requestIDHeader))
	}
	if response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("security headers are missing")
	}
	if response.Header().Get("Content-Security-Policy") == "" || response.Header().Get("Cross-Origin-Opener-Policy") != "same-origin" {
		t.Fatal("global browser security headers are missing")
	}
}

func TestReadinessDoesNotExposeDependencyError(t *testing.T) {
	router := NewRouter(config.Config{BuildVersion: "test"}, readinessStub{err: errors.New("secret connection detail")}, testLogger())
	response := request(t, router, http.MethodGet, "/health/ready", "")

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	if strings.Contains(response.Body.String(), "secret connection detail") {
		t.Fatal("readiness response exposed the dependency error")
	}
	assertProblem(t, response, "not-ready")
}

func TestReadinessSucceedsWhenDatabaseResponds(t *testing.T) {
	router := NewRouter(config.Config{BuildVersion: "test"}, readinessStub{}, testLogger())
	response := request(t, router, http.MethodGet, "/health/ready", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
}

func TestCorrelationIDIsPropagated(t *testing.T) {
	const correlationID = "0190f2ac-7b6f-7cc0-8a1d-7f56b6d1a103"
	response := request(t, NewRouter(config.Config{}, nil, testLogger()), http.MethodGet, "/health/live", correlationID)
	if response.Header().Get(correlationIDHeader) != correlationID {
		t.Fatalf("X-Correlation-ID = %q, want %q", response.Header().Get(correlationIDHeader), correlationID)
	}
}

func TestInvalidCorrelationIDReturnsProblem(t *testing.T) {
	response := request(t, NewRouter(config.Config{}, nil, testLogger()), http.MethodGet, "/health/live", "not-a-uuid")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	assertProblem(t, response, "invalid-correlation-id")
	if !validUUID(response.Header().Get(correlationIDHeader)) {
		t.Fatal("error response did not receive a valid correlation ID")
	}
}

func TestUnknownRouteUsesProblemDetails(t *testing.T) {
	response := request(t, NewRouter(config.Config{}, nil, testLogger()), http.MethodGet, "/api/v1/unknown", "")
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
	assertProblem(t, response, "not-found")
}

func TestMetadataIsOutsideWorkNamespace(t *testing.T) {
	router := NewRouter(config.Config{BuildVersion: "test-build"}, nil, testLogger())
	metadata := request(t, router, http.MethodGet, "/meta", "")
	if metadata.Code != http.StatusOK || !strings.Contains(metadata.Body.String(), "test-build") || !strings.Contains(metadata.Body.String(), `"api_version":"v1"`) {
		t.Fatalf("metadata response = %d %s", metadata.Code, metadata.Body.String())
	}
	legacy := request(t, router, http.MethodGet, "/api/v1/meta", "")
	if legacy.Code != http.StatusNotFound {
		t.Fatalf("legacy work metadata status = %d, want %d", legacy.Code, http.StatusNotFound)
	}
}

func TestMetricsHaveOnlyBoundedStatusLabels(t *testing.T) {
	router := NewRouter(config.Config{BuildVersion: "test\"build"}, nil, testLogger())
	response := request(t, router, http.MethodGet, "/metrics", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	body := response.Body.String()
	if !strings.Contains(body, "werk_http_requests_total") || !strings.Contains(body, `version="test\"build"`) {
		t.Fatalf("unexpected metrics body: %s", body)
	}
	if strings.Contains(body, "tenant") || strings.Contains(body, "http.route") {
		t.Fatal("metrics contain a high-cardinality or tenant label")
	}
}

func TestRecoveryReturnsProblemDetails(t *testing.T) {
	handler := requestIdentityMiddleware(recoveryMiddleware(testLogger())(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("test panic")
	})))
	response := request(t, handler, http.MethodGet, "/panic", "")
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	assertProblem(t, response, "internal-error")
}

func request(t *testing.T, handler http.Handler, method, path, correlationID string) *httptest.ResponseRecorder {
	t.Helper()
	httpRequest := httptest.NewRequest(method, path, nil)
	if correlationID != "" {
		httpRequest.Header.Set(correlationIDHeader, correlationID)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httpRequest)
	return response
}

func assertProblem(t *testing.T, response *httptest.ResponseRecorder, expectedCode string) {
	t.Helper()
	if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/problem+json") {
		t.Fatalf("Content-Type = %q, want application/problem+json", contentType)
	}
	var value problem
	if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if value.Code != expectedCode {
		t.Fatalf("problem code = %q, want %q", value.Code, expectedCode)
	}
	if value.RequestID == "" || value.CorrelationID == "" {
		t.Fatal("problem does not contain request identity")
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
