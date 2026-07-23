package httpapi

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

func browserMutationProtectionMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[strings.TrimSuffix(origin, "/")] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if !isUnsafeMethod(request.Method) {
				next.ServeHTTP(writer, request)
				return
			}
			fetchSite := strings.ToLower(strings.TrimSpace(request.Header.Get("Sec-Fetch-Site")))
			if fetchSite != "" && fetchSite != "same-origin" && fetchSite != "none" {
				writeProblem(writer, request, http.StatusForbidden, "cross-origin-request", "Cross-origin request rejected", "The request origin is not allowed.")
				return
			}
			origin := strings.TrimSuffix(strings.TrimSpace(request.Header.Get("Origin")), "/")
			if origin != "" {
				if _, ok := allowed[origin]; !ok {
					writeProblem(writer, request, http.StatusForbidden, "cross-origin-request", "Cross-origin request rejected", "The request origin is not allowed.")
					return
				}
			}

			hasCookieCredential := request.URL.Path != "/api/v1/auth/login" &&
				(cookieValue(request, "werk_session") != "" || cookieValue(request, "werk_mfa_challenge") != "")
			if hasCookieCredential {
				// Cookie-authenticated browser mutations require both an explicit
				// allowed Origin and a double-submit token. SameSite remains an
				// additional browser control, not the sole CSRF defense.
				if origin == "" || !equalCSRFToken(cookieValue(request, "werk_csrf"), request.Header.Get("X-CSRF-Token")) {
					writeProblem(writer, request, http.StatusForbidden, "csrf-verification-failed", "CSRF verification failed", "The request could not be bound to this browser session.")
					return
				}
			}
			next.ServeHTTP(writer, request)
		})
	}
}

func isUnsafeMethod(method string) bool {
	return method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions && method != http.MethodTrace
}

func equalCSRFToken(cookieToken, headerToken string) bool {
	if len(cookieToken) < 32 || len(cookieToken) != len(headerToken) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cookieToken), []byte(headerToken)) == 1
}
