package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

const (
	requestIDHeader     = "X-Request-ID"
	correlationIDHeader = "X-Correlation-ID"
)

type requestContextKey uint8

const (
	requestIDContextKey requestContextKey = iota
	correlationIDContextKey
	correlationErrorContextKey
)

var fallbackIDCounter atomic.Uint64

func requestIdentityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestID := newUUIDv7()
		correlationID := newUUIDv7()
		correlationError := ""

		providedCorrelationIDs := request.Header.Values(correlationIDHeader)
		if len(providedCorrelationIDs) == 1 && strings.TrimSpace(providedCorrelationIDs[0]) != "" {
			provided := strings.TrimSpace(providedCorrelationIDs[0])
			if validUUID(provided) {
				correlationID = strings.ToLower(provided)
			} else {
				correlationError = "invalid"
			}
		} else if len(providedCorrelationIDs) > 1 {
			correlationError = "multiple"
		}

		request = requestWithIdentity(request, requestID, correlationID, correlationError)
		request.Header.Set(requestIDHeader, requestID)
		request.Header.Set(correlationIDHeader, correlationID)
		setIdentityHeaders(writer, requestID, correlationID)
		next.ServeHTTP(writer, request)
	})
}

func requestWithIdentity(request *http.Request, requestID, correlationID, correlationError string) *http.Request {
	ctx := context.WithValue(request.Context(), requestIDContextKey, requestID)
	ctx = context.WithValue(ctx, correlationIDContextKey, correlationID)
	ctx = context.WithValue(ctx, correlationErrorContextKey, correlationError)
	return request.WithContext(ctx)
}

func correlationValidationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		validationError, _ := request.Context().Value(correlationErrorContextKey).(string)
		switch validationError {
		case "invalid":
			writeProblem(writer, request, http.StatusBadRequest, "invalid-correlation-id", "Invalid correlation ID", "X-Correlation-ID must contain a valid UUID.")
			return
		case "multiple":
			writeProblem(writer, request, http.StatusBadRequest, "invalid-correlation-id", "Invalid correlation ID", "X-Correlation-ID must be provided at most once.")
			return
		default:
			next.ServeHTTP(writer, request)
		}
	})
}

func setIdentityHeaders(writer http.ResponseWriter, requestID, correlationID string) {
	writer.Header().Set(requestIDHeader, requestID)
	writer.Header().Set(correlationIDHeader, correlationID)
}

func requestIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(requestIDContextKey).(string)
	return value
}

func correlationIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(correlationIDContextKey).(string)
	return value
}

func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Cache-Control", "no-store")
		writer.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		writer.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		writer.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		writer.Header().Set("Permissions-Policy", "camera=(), geolocation=(), microphone=()")
		writer.Header().Set("Referrer-Policy", "no-referrer")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(writer, request)
	})
}

func accessLogMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			startedAt := time.Now()
			wrapped := chimiddleware.NewWrapResponseWriter(writer, request.ProtoMajor)
			next.ServeHTTP(wrapped, request)

			status := wrapped.Status()
			if status == 0 {
				status = http.StatusOK
			}
			route := chi.RouteContext(request.Context()).RoutePattern()
			if route == "" {
				route = "unmatched"
			}
			attributes := []any{
				"http.method", request.Method,
				"http.route", route,
				"http.status", status,
				"http.response_bytes", wrapped.BytesWritten(),
				"duration_ms", time.Since(startedAt).Milliseconds(),
				"request_id", requestIDFromContext(request.Context()),
				"correlation_id", correlationIDFromContext(request.Context()),
			}
			if status >= http.StatusInternalServerError {
				logger.WarnContext(request.Context(), "http request completed", attributes...)
				return
			}
			logger.InfoContext(request.Context(), "http request completed", attributes...)
		})
	}
}

func recoveryMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			wrapped := chimiddleware.NewWrapResponseWriter(writer, request.ProtoMajor)
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.ErrorContext(request.Context(), "panic recovered while serving request",
						"panic", fmt.Sprint(recovered),
						"request_id", requestIDFromContext(request.Context()),
						"correlation_id", correlationIDFromContext(request.Context()),
						"stack", string(debug.Stack()),
					)
					if wrapped.Status() == 0 {
						writeProblem(wrapped, request, http.StatusInternalServerError, "internal-error", "Internal server error", "The request could not be completed.")
					}
				}
			}()
			next.ServeHTTP(wrapped, request)
		})
	}
}

func newUUIDv7() string {
	var value [16]byte
	milliseconds := uint64(time.Now().UnixMilli())
	value[0] = byte(milliseconds >> 40)
	value[1] = byte(milliseconds >> 32)
	value[2] = byte(milliseconds >> 24)
	value[3] = byte(milliseconds >> 16)
	value[4] = byte(milliseconds >> 8)
	value[5] = byte(milliseconds)
	if _, err := rand.Read(value[6:]); err != nil {
		fallback := uint64(time.Now().UnixNano()) ^ fallbackIDCounter.Add(1)
		for index := 6; index < len(value); index++ {
			value[index] = byte(fallback >> ((index - 6) * 8 % 64))
		}
	}
	value[6] = (value[6] & 0x0f) | 0x70
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
}

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	compact := strings.ReplaceAll(value, "-", "")
	decoded := make([]byte, 16)
	if _, err := hex.Decode(decoded, []byte(compact)); err != nil {
		return false
	}
	version := decoded[6] >> 4
	return version >= 1 && version <= 8 && decoded[8]&0xc0 == 0x80
}
