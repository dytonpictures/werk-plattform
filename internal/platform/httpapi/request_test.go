package httpapi

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

type preferenceAuthStub struct {
	mode string
}

func (*preferenceAuthStub) Login(context.Context, string, string) (string, string, error) {
	return "", "", nil
}

func (*preferenceAuthStub) Session(context.Context, string) (any, error) { return nil, nil }
func (*preferenceAuthStub) Logout(context.Context, string) error         { return nil }
func (stub *preferenceAuthStub) UpdateNavigationPreference(_ context.Context, _, mode, requestID, correlationID string) error {
	if !validUUID(requestID) || !validUUID(correlationID) {
		return context.Canceled
	}
	stub.mode = mode
	return nil
}

func TestDecodeJSONAcceptsOneKnownDocument(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"WERK"}`))
	response := httptest.NewRecorder()
	var input struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(response, request, &input); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if input.Name != "WERK" {
		t.Fatalf("name = %q, want WERK", input.Name)
	}
}

func TestDecodeJSONRejectsUnknownFieldsAndMultipleDocuments(t *testing.T) {
	tests := []string{
		`{"name":"WERK","account_class":"admin"}`,
		`{"name":"WERK"} {"name":"other"}`,
	}
	for _, body := range tests {
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		response := httptest.NewRecorder()
		var input struct {
			Name string `json:"name"`
		}
		if err := decodeJSON(response, request, &input); err == nil {
			t.Fatalf("decodeJSON accepted %s", body)
		}
	}
}

func TestSessionCookieUsesSecureFlagWithNativeTLS(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	request.TLS = &tls.ConnectionState{HandshakeComplete: true}
	response := httptest.NewRecorder()
	setSessionCookie(response, request, "opaque", 0)
	assertSecureSessionCookies(t, response)
}

func TestSessionCookieTrustsHTTPSOnlyFromConfiguredProxy(t *testing.T) {
	handler := transportSecurityMiddleware([]netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")})(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		setSessionCookie(writer, request, "opaque", 0)
	}))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	request.RemoteAddr = "192.0.2.10:4242"
	request.Header.Set("X-Forwarded-Proto", "https")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	assertSecureSessionCookies(t, response)

	untrustedRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	untrustedRequest.RemoteAddr = "198.51.100.20:4242"
	untrustedRequest.Header.Set("X-Forwarded-Proto", "https")
	untrustedResponse := httptest.NewRecorder()
	handler.ServeHTTP(untrustedResponse, untrustedRequest)
	if untrustedResponse.Result().Cookies()[0].Secure {
		t.Fatal("untrusted proxy marked the session cookie as secure")
	}
}

func assertSecureSessionCookies(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	cookies := response.Result().Cookies()
	if len(cookies) != 2 || cookies[0].Name != "werk_session" || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("unexpected session cookie: %#v", cookies)
	}
	if cookies[1].Name != "werk_csrf" || !cookies[1].Secure || cookies[1].HttpOnly || cookies[1].SameSite != http.SameSiteStrictMode || len(cookies[1].Value) < 32 {
		t.Fatalf("unexpected CSRF cookie: %#v", cookies[1])
	}
}

func TestTOTPQRCodeDataURLContainsPNG(t *testing.T) {
	value, err := totpQRCodeDataURL("otpauth://totp/WERK:admin@werk.local?issuer=WERK&secret=TESTSECRET")
	if err != nil {
		t.Fatalf("generate QR code: %v", err)
	}
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(value, prefix) {
		t.Fatalf("QR code URL prefix = %q", value[:min(len(value), len(prefix))])
	}
	png, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, prefix))
	if err != nil || len(png) < 8 || string(png[:8]) != "\x89PNG\r\n\x1a\n" {
		t.Fatalf("QR code is not a valid PNG envelope: %v", err)
	}
}

func TestSessionRouteRepairsMissingCSRFCookieAfterValidation(t *testing.T) {
	service := &preferenceAuthStub{}
	handler := authRoutes(service)
	request := httptest.NewRequest(http.MethodGet, "/session", nil)
	request.AddCookie(&http.Cookie{Name: "werk_session", Value: "opaque"})
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("session response = %d, want 200", response.Code)
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "werk_csrf" || cookies[0].HttpOnly || len(cookies[0].Value) < 32 {
		t.Fatalf("unexpected repaired CSRF cookie: %#v", cookies)
	}
}

func TestSessionRouteKeepsExistingCSRFCookie(t *testing.T) {
	service := &preferenceAuthStub{}
	handler := authRoutes(service)
	request := httptest.NewRequest(http.MethodGet, "/session", nil)
	request.AddCookie(&http.Cookie{Name: "werk_session", Value: "opaque"})
	request.AddCookie(&http.Cookie{Name: "werk_csrf", Value: "0123456789abcdef0123456789abcdef"})
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || len(response.Result().Cookies()) != 0 {
		t.Fatalf("session response = %d, cookies = %#v", response.Code, response.Result().Cookies())
	}
}

func TestPreferenceRouteStoresValidatedNavigationMode(t *testing.T) {
	service := &preferenceAuthStub{}
	handler := requestIdentityMiddleware(authRoutes(service))
	request := httptest.NewRequest(http.MethodPatch, "/preferences", strings.NewReader(`{"navigation_mode":"collapsed"}`))
	request.AddCookie(&http.Cookie{Name: "werk_session", Value: "opaque"})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || service.mode != "collapsed" {
		t.Fatalf("preference response = %d, stored mode = %q", response.Code, service.mode)
	}
}

func TestPreferenceRouteRejectsUnknownMode(t *testing.T) {
	service := &preferenceAuthStub{}
	handler := requestIdentityMiddleware(authRoutes(service))
	request := httptest.NewRequest(http.MethodPatch, "/preferences", strings.NewReader(`{"navigation_mode":"grid"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || service.mode != "" {
		t.Fatalf("preference response = %d, stored mode = %q", response.Code, service.mode)
	}
}
