package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dytonpictures/werk/internal/core/identity"
)

type rotationAuthStub struct {
	passwordRotation identity.SessionRotation
	totpActivation   identity.TOTPActivation
	passwordToken    string
	totpToken        string
	passwordErr      error
	totpErr          error
}

func (*rotationAuthStub) Login(context.Context, string, string) (string, string, error) {
	return "", "", nil
}

func (*rotationAuthStub) Session(context.Context, string) (any, error) { return nil, nil }
func (*rotationAuthStub) Logout(context.Context, string) error         { return nil }

func (stub *rotationAuthStub) ChangePassword(_ context.Context, token, _, _ string) (identity.SessionRotation, error) {
	stub.passwordToken = token
	return stub.passwordRotation, stub.passwordErr
}

func (stub *rotationAuthStub) ChangePasswordWithAudit(_ context.Context, token, _, _, _, _ string) (identity.SessionRotation, error) {
	stub.passwordToken = token
	return stub.passwordRotation, stub.passwordErr
}

func (*rotationAuthStub) StartTOTPEnrollment(context.Context, string, string, string, string, string) (identity.TOTPEnrollment, error) {
	return identity.TOTPEnrollment{}, nil
}

func (stub *rotationAuthStub) ConfirmTOTPEnrollment(_ context.Context, token, _, _, _, _ string) (identity.TOTPActivation, error) {
	stub.totpToken = token
	return stub.totpActivation, stub.totpErr
}

func (*rotationAuthStub) CompleteMFAChallenge(context.Context, string, string, string, string) (identity.LoginResult, error) {
	return identity.LoginResult{}, nil
}

func TestPasswordChangeReplacesSessionCookieWithoutLeakingToken(t *testing.T) {
	const oldToken = "old-session-token"
	const newToken = "rotated-password-session-token"
	service := &rotationAuthStub{passwordRotation: identity.SessionRotation{
		SessionToken: newToken,
		ExpiresAt:    time.Now().Add(12 * time.Hour),
	}}
	request := httptest.NewRequest(http.MethodPost, "/password", strings.NewReader(`{
		"current_password":"old-password",
		"new_password":"new-password-long-enough"
	}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: "werk_session", Value: oldToken})
	response := httptest.NewRecorder()

	requestIdentityMiddleware(authRoutes(service)).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("password response = %d %s", response.Code, response.Body.String())
	}
	if service.passwordToken != oldToken {
		t.Fatalf("password changer received token %q, want old session", service.passwordToken)
	}
	if body := response.Body.String(); body != "{\"password_changed\":true}\n" || strings.Contains(body, newToken) {
		t.Fatalf("password response exposed rotation state: %s", body)
	}
	assertRotatedSessionCookies(t, response, newToken)
}

func TestTOTPConfirmationReplacesSessionCookieWithoutLeakingRotation(t *testing.T) {
	const oldToken = "single-factor-session-token"
	const newToken = "multi-factor-session-token"
	service := &rotationAuthStub{totpActivation: identity.TOTPActivation{
		RecoveryCodes: []string{"recovery-one", "recovery-two"},
		Rotation: identity.SessionRotation{
			SessionToken: newToken,
			ExpiresAt:    time.Now().Add(12 * time.Hour),
		},
	}}
	request := httptest.NewRequest(http.MethodPost, "/mfa/totp/confirmation", strings.NewReader(`{
		"factor_id":"0190f2ac-7b6f-7cc0-8a1d-7f56b6d1a103",
		"code":"123456"
	}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: "werk_session", Value: oldToken})
	response := httptest.NewRecorder()

	requestIdentityMiddleware(authRoutes(service)).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("TOTP confirmation response = %d %s", response.Code, response.Body.String())
	}
	if service.totpToken != oldToken {
		t.Fatalf("TOTP manager received token %q, want single-factor session", service.totpToken)
	}
	body := response.Body.String()
	if !strings.Contains(body, `"recovery_codes":["recovery-one","recovery-two"]`) || strings.Contains(body, newToken) || strings.Contains(body, "rotation") || strings.Contains(body, "expires_at") {
		t.Fatalf("TOTP confirmation exposed internal rotation state: %s", body)
	}
	assertRotatedSessionCookies(t, response, newToken)
}

func TestPasswordChangeFailsClosedForInvalidRotation(t *testing.T) {
	service := &rotationAuthStub{}
	request := httptest.NewRequest(http.MethodPost, "/password", strings.NewReader(`{
		"current_password":"old-password",
		"new_password":"new-password-long-enough"
	}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: "werk_session", Value: "old-session-token"})
	response := httptest.NewRecorder()

	requestIdentityMiddleware(authRoutes(service)).ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("password response = %d, want 500", response.Code)
	}
	if len(response.Result().Cookies()) != 0 {
		t.Fatalf("invalid rotation installed cookies: %#v", response.Result().Cookies())
	}
	assertProblem(t, response, "session-rotation-failed")
}

func TestRotationInfrastructureErrorsAreNotReportedAsCredentialFailures(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		body        string
		service     *rotationAuthStub
		problemType string
	}{
		{
			name: "password", path: "/password",
			body:        `{"current_password":"old-password","new_password":"new-password-long-enough"}`,
			service:     &rotationAuthStub{passwordErr: errors.New("database unavailable")},
			problemType: "password-change-processing-failed",
		},
		{
			name: "totp", path: "/mfa/totp/confirmation",
			body:        `{"factor_id":"0190f2ac-7b6f-7cc0-8a1d-7f56b6d1a103","code":"123456"}`,
			service:     &rotationAuthStub{totpErr: errors.New("database unavailable")},
			problemType: "mfa-processing-failed",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			request.AddCookie(&http.Cookie{Name: "werk_session", Value: "old-session-token"})
			response := httptest.NewRecorder()

			requestIdentityMiddleware(authRoutes(test.service)).ServeHTTP(response, request)

			if response.Code != http.StatusInternalServerError {
				t.Fatalf("response = %d, want 500", response.Code)
			}
			if len(response.Result().Cookies()) != 0 {
				t.Fatalf("failed rotation installed cookies: %#v", response.Result().Cookies())
			}
			assertProblem(t, response, test.problemType)
		})
	}
}

func assertRotatedSessionCookies(t *testing.T, response *httptest.ResponseRecorder, sessionToken string) {
	t.Helper()
	cookies := response.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatalf("rotation returned %d cookies, want session and CSRF: %#v", len(cookies), cookies)
	}
	if session := cookies[0]; session.Name != "werk_session" || session.Value != sessionToken || !session.HttpOnly || session.MaxAge <= 0 {
		t.Fatalf("unexpected rotated session cookie: %#v", session)
	}
	if csrf := cookies[1]; csrf.Name != "werk_csrf" || csrf.HttpOnly || len(csrf.Value) < 32 || csrf.MaxAge <= 0 {
		t.Fatalf("unexpected rotated CSRF cookie: %#v", csrf)
	}
}
