package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBrowserMutationProtectionRejectsCrossOrigin(t *testing.T) {
	handler := browserMutationProtectionMiddleware([]string{"https://werk.example"})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	request.Header.Set("Origin", "https://attacker.example")
	request.Header.Set("Sec-Fetch-Site", "cross-site")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.Code)
	}
}

func TestBrowserMutationProtectionRequiresCSRFForCookieSession(t *testing.T) {
	handler := browserMutationProtectionMiddleware([]string{"https://werk.example"})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password", nil)
	request.Header.Set("Origin", "https://werk.example")
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.AddCookie(&http.Cookie{Name: "werk_session", Value: "opaque"})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.Code)
	}
}

func TestBrowserMutationProtectionAcceptsBoundCSRFToken(t *testing.T) {
	handler := browserMutationProtectionMiddleware([]string{"https://werk.example"})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	token := "0123456789abcdef0123456789abcdef" // gitleaks:allow -- deterministic CSRF test fixture
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password", nil)
	request.Header.Set("Origin", "https://werk.example")
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.Header.Set("X-CSRF-Token", token)
	request.AddCookie(&http.Cookie{Name: "werk_session", Value: "opaque"})
	request.AddCookie(&http.Cookie{Name: "werk_csrf", Value: token})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", response.Code)
	}
}
