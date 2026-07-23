package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/skip2/go-qrcode"

	"github.com/dytonpictures/werk/internal/core/identity"
)

// AuthService is intentionally an adapter boundary. Implementations own
// credential hashing and session persistence; the HTTP layer never contains
// demo credentials or provider-specific logic.
type AuthService interface {
	Login(context.Context, string, string) (token string, redirect string, err error)
	Session(context.Context, string) (any, error)
	Logout(context.Context, string) error
}

type passwordChanger interface {
	ChangePassword(context.Context, string, string, string) (identity.SessionRotation, error)
}

type auditedPasswordChanger interface {
	ChangePasswordWithAudit(context.Context, string, string, string, string, string) (identity.SessionRotation, error)
}

type auditedLogoutService interface {
	LogoutWithAudit(context.Context, string, string, string) error
}

type preferenceUpdater interface {
	UpdateNavigationPreference(context.Context, string, string, string, string) error
}

type mfaLoginService interface {
	LoginWithMFA(context.Context, string, string, string, string) (identity.LoginResult, error)
}

type mfaManager interface {
	StartTOTPEnrollment(context.Context, string, string, string, string, string) (identity.TOTPEnrollment, error)
	ConfirmTOTPEnrollment(context.Context, string, string, string, string, string) (identity.TOTPActivation, error)
	CompleteMFAChallenge(context.Context, string, string, string, string) (identity.LoginResult, error)
}

func authRoutes(service AuthService) http.Handler {
	r := chi.NewRouter()
	r.Post("/login", func(w http.ResponseWriter, req *http.Request) {
		if service == nil {
			writeProblem(w, req, http.StatusNotImplemented, "auth-unavailable", "Authentication unavailable", "Authentication is not configured.")
			return
		}
		var input struct {
			LoginName string `json:"login_name"`
			Password  string `json:"password"`
		}
		if decodeJSON(w, req, &input) != nil || input.LoginName == "" || input.Password == "" {
			writeProblem(w, req, http.StatusBadRequest, "invalid-login", "Invalid login", "Login data is invalid.")
			return
		}
		var token, redirect string
		if mfaService, ok := service.(mfaLoginService); ok {
			result, err := mfaService.LoginWithMFA(
				req.Context(), input.LoginName, input.Password,
				requestIDFromContext(req.Context()), correlationIDFromContext(req.Context()),
			)
			if err != nil {
				writeProblem(w, req, http.StatusUnauthorized, "invalid-credentials", "Authentication failed", "The credentials are invalid.")
				return
			}
			if result.MFARequired {
				setPrivateCookie(w, req, "werk_mfa_challenge", result.ChallengeToken, int((5*time.Minute)/time.Second))
				setCSRFCookie(w, req, newCSRFToken(), int((5*time.Minute)/time.Second))
				writeJSON(w, http.StatusOK, map[string]any{"redirect": result.Redirect, "mfa_required": true})
				return
			}
			token, redirect = result.SessionToken, result.Redirect
		} else {
			var err error
			token, redirect, err = service.Login(req.Context(), input.LoginName, input.Password)
			if err != nil {
				writeProblem(w, req, http.StatusUnauthorized, "invalid-credentials", "Authentication failed", "The credentials are invalid.")
				return
			}
		}
		// The opaque token is transported only as an HttpOnly same-origin cookie;
		// it is never exposed to dashboard JavaScript.
		setSessionCookie(w, req, token, 0)
		writeJSON(w, http.StatusOK, map[string]string{"redirect": redirect})
	})
	r.Post("/mfa/challenge", func(w http.ResponseWriter, req *http.Request) {
		manager, ok := service.(mfaManager)
		if !ok {
			writeProblem(w, req, http.StatusNotImplemented, "mfa-unavailable", "MFA unavailable", "Multi-factor authentication is not configured.")
			return
		}
		var input struct {
			Code string `json:"code"`
		}
		if decodeJSON(w, req, &input) != nil || input.Code == "" {
			writeProblem(w, req, http.StatusBadRequest, "invalid-mfa-code", "Invalid MFA code", "The verification code is invalid.")
			return
		}
		result, err := manager.CompleteMFAChallenge(
			req.Context(), cookieValue(req, "werk_mfa_challenge"), input.Code,
			requestIDFromContext(req.Context()), correlationIDFromContext(req.Context()),
		)
		if err != nil {
			if !isAuthenticationRejection(err) {
				writeProblem(w, req, http.StatusInternalServerError, "mfa-processing-failed", "MFA processing failed", "The MFA request could not be processed.")
				return
			}
			writeProblem(w, req, http.StatusUnauthorized, "mfa-verification-failed", "MFA verification failed", "The verification code was rejected.")
			return
		}
		setPrivateCookie(w, req, "werk_mfa_challenge", "", -1)
		setSessionCookie(w, req, result.SessionToken, 0)
		writeJSON(w, http.StatusOK, map[string]string{"redirect": result.Redirect})
	})
	r.Post("/mfa/totp/enrollment", func(w http.ResponseWriter, req *http.Request) {
		manager, ok := service.(mfaManager)
		if !ok {
			writeProblem(w, req, http.StatusNotImplemented, "mfa-unavailable", "MFA unavailable", "Multi-factor authentication is not configured.")
			return
		}
		var input struct {
			CurrentPassword string `json:"current_password"`
			DisplayName     string `json:"display_name"`
		}
		if decodeJSON(w, req, &input) != nil || input.CurrentPassword == "" || input.DisplayName == "" {
			writeProblem(w, req, http.StatusBadRequest, "invalid-mfa-enrollment", "Invalid MFA enrollment", "Enrollment data is invalid.")
			return
		}
		result, err := manager.StartTOTPEnrollment(
			req.Context(), cookieValue(req, "werk_session"), input.CurrentPassword, input.DisplayName,
			requestIDFromContext(req.Context()), correlationIDFromContext(req.Context()),
		)
		if err != nil {
			if !isAuthenticationRejection(err) {
				writeProblem(w, req, http.StatusInternalServerError, "mfa-processing-failed", "MFA processing failed", "The MFA request could not be processed.")
				return
			}
			writeProblem(w, req, http.StatusUnauthorized, "mfa-enrollment-failed", "MFA enrollment failed", "Enrollment could not be started.")
			return
		}
		qrCode, err := totpQRCodeDataURL(result.OTPAuthURI)
		if err != nil {
			writeProblem(w, req, http.StatusInternalServerError, "mfa-qr-code-failed", "MFA QR code unavailable", "The authenticator QR code could not be generated.")
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{
			"factor_id":        result.FactorID,
			"secret":           result.Secret,
			"otpauth_uri":      result.OTPAuthURI,
			"qr_code_data_url": qrCode,
		})
	})
	r.Post("/mfa/totp/confirmation", func(w http.ResponseWriter, req *http.Request) {
		manager, ok := service.(mfaManager)
		if !ok {
			writeProblem(w, req, http.StatusNotImplemented, "mfa-unavailable", "MFA unavailable", "Multi-factor authentication is not configured.")
			return
		}
		var input struct {
			FactorID string `json:"factor_id"`
			Code     string `json:"code"`
		}
		if decodeJSON(w, req, &input) != nil || input.FactorID == "" || input.Code == "" {
			writeProblem(w, req, http.StatusBadRequest, "invalid-mfa-code", "Invalid MFA code", "The verification code is invalid.")
			return
		}
		result, err := manager.ConfirmTOTPEnrollment(
			req.Context(), cookieValue(req, "werk_session"), input.FactorID, input.Code,
			requestIDFromContext(req.Context()), correlationIDFromContext(req.Context()),
		)
		if err != nil {
			if !isAuthenticationRejection(err) {
				writeProblem(w, req, http.StatusInternalServerError, "mfa-processing-failed", "MFA processing failed", "The MFA request could not be processed.")
				return
			}
			writeProblem(w, req, http.StatusUnauthorized, "mfa-verification-failed", "MFA verification failed", "The verification code was rejected.")
			return
		}
		if err := result.Rotation.Validate(time.Now()); err != nil {
			writeProblem(w, req, http.StatusInternalServerError, "session-rotation-failed", "Session rotation failed", "The replacement session could not be installed.")
			return
		}
		setSessionCookie(w, req, result.Rotation.SessionToken, sessionCookieMaxAge(result.Rotation.ExpiresAt))
		writeJSON(w, http.StatusOK, result)
	})
	r.Get("/session", func(w http.ResponseWriter, req *http.Request) {
		if service == nil {
			writeProblem(w, req, http.StatusNotImplemented, "auth-unavailable", "Authentication unavailable", "Authentication is not configured.")
			return
		}
		value, err := service.Session(req.Context(), cookieValue(req, "werk_session"))
		if err != nil {
			writeProblem(w, req, http.StatusUnauthorized, "invalid-session", "Authentication required", "No valid session exists.")
			return
		}
		// Sessions created before CSRF protection was enabled can remain valid in
		// PostgreSQL while their browser has no double-submit cookie yet. Repair
		// that browser-side companion only after the session itself was validated.
		// The token remains unreadable cross-origin and unsafe requests still need
		// both an allowed Origin and the matching explicit request header.
		if len(cookieValue(req, "werk_csrf")) < 32 {
			setCSRFCookie(w, req, newCSRFToken(), 0)
		}
		writeJSON(w, http.StatusOK, value)
	})
	r.Post("/logout", func(w http.ResponseWriter, req *http.Request) {
		if service == nil {
			writeProblem(w, req, http.StatusNotImplemented, "auth-unavailable", "Authentication unavailable", "Authentication is not configured.")
			return
		}
		var err error
		if audited, ok := service.(auditedLogoutService); ok {
			err = audited.LogoutWithAudit(req.Context(), cookieValue(req, "werk_session"), requestIDFromContext(req.Context()), correlationIDFromContext(req.Context()))
		} else {
			err = service.Logout(req.Context(), cookieValue(req, "werk_session"))
		}
		if err != nil {
			writeProblem(w, req, http.StatusUnauthorized, "invalid-session", "Authentication required", "No valid session exists.")
			return
		}
		setSessionCookie(w, req, "", -1)
		w.WriteHeader(http.StatusNoContent)
	})
	r.Post("/password", func(w http.ResponseWriter, req *http.Request) {
		changer, ok := service.(passwordChanger)
		if !ok {
			writeProblem(w, req, http.StatusNotImplemented, "password-change-unavailable", "Password change unavailable", "Password change is not configured.")
			return
		}
		var input struct {
			CurrentPassword string `json:"current_password"`
			NewPassword     string `json:"new_password"`
		}
		if decodeJSON(w, req, &input) != nil || len(input.NewPassword) < 12 {
			writeProblem(w, req, http.StatusBadRequest, "invalid-password", "Invalid password", "The new password does not meet the requirements.")
			return
		}
		var rotation identity.SessionRotation
		var err error
		if audited, ok := service.(auditedPasswordChanger); ok {
			rotation, err = audited.ChangePasswordWithAudit(req.Context(), cookieValue(req, "werk_session"), input.CurrentPassword, input.NewPassword, requestIDFromContext(req.Context()), correlationIDFromContext(req.Context()))
		} else {
			rotation, err = changer.ChangePassword(req.Context(), cookieValue(req, "werk_session"), input.CurrentPassword, input.NewPassword)
		}
		if err != nil {
			if !isAuthenticationRejection(err) {
				writeProblem(w, req, http.StatusInternalServerError, "password-change-processing-failed", "Password change failed", "The password change could not be processed.")
				return
			}
			writeProblem(w, req, http.StatusUnauthorized, "password-change-failed", "Password change failed", "The password could not be changed.")
			return
		}
		if err := rotation.Validate(time.Now()); err != nil {
			writeProblem(w, req, http.StatusInternalServerError, "session-rotation-failed", "Session rotation failed", "The replacement session could not be installed.")
			return
		}
		setSessionCookie(w, req, rotation.SessionToken, sessionCookieMaxAge(rotation.ExpiresAt))
		writeJSON(w, http.StatusOK, map[string]bool{"password_changed": true})
	})
	r.Patch("/preferences", func(w http.ResponseWriter, req *http.Request) {
		updater, ok := service.(preferenceUpdater)
		if !ok {
			writeProblem(w, req, http.StatusNotImplemented, "preferences-unavailable", "Preferences unavailable", "Account preferences are not configured.")
			return
		}
		var input struct {
			NavigationMode string `json:"navigation_mode"`
		}
		if decodeJSON(w, req, &input) != nil || (input.NavigationMode != "bar" && input.NavigationMode != "collapsed") {
			writeProblem(w, req, http.StatusBadRequest, "invalid-preferences", "Invalid preferences", "The account preferences are invalid.")
			return
		}
		if err := updater.UpdateNavigationPreference(
			req.Context(), cookieValue(req, "werk_session"), input.NavigationMode,
			requestIDFromContext(req.Context()), correlationIDFromContext(req.Context()),
		); err != nil {
			writeProblem(w, req, http.StatusUnauthorized, "preferences-update-failed", "Preferences update failed", "The account preferences could not be changed.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"navigation_mode": input.NavigationMode})
	})
	return r
}

func isAuthenticationRejection(err error) bool {
	return errors.Is(err, identity.ErrInvalidCredentials) ||
		errors.Is(err, identity.ErrSessionInvalid) ||
		errors.Is(err, identity.ErrSessionExpired) ||
		errors.Is(err, identity.ErrMFAInvalid) ||
		errors.Is(err, identity.ErrMFARequired) ||
		errors.Is(err, identity.ErrMFAEnrollment) ||
		errors.Is(err, identity.ErrMFAChallengeUsed) ||
		errors.Is(err, identity.ErrAccessDenied)
}

func setSessionCookie(writer http.ResponseWriter, request *http.Request, value string, maxAge int) {
	setPrivateCookie(writer, request, "werk_session", value, maxAge)
	csrfValue := newCSRFToken()
	if maxAge < 0 {
		csrfValue = ""
	}
	setCSRFCookie(writer, request, csrfValue, maxAge)
}

func sessionCookieMaxAge(expiresAt time.Time) int {
	if expiresAt.IsZero() {
		return 0
	}
	remaining := time.Until(expiresAt)
	if remaining <= 0 {
		return -1
	}
	seconds := int64(remaining / time.Second)
	if remaining%time.Second != 0 {
		seconds++
	}
	maxInt := int64(^uint(0) >> 1)
	if seconds > maxInt {
		return int(maxInt)
	}
	return int(seconds)
}

func setPrivateCookie(writer http.ResponseWriter, request *http.Request, name, value string, maxAge int) {
	secure := requestUsesSecureTransport(request)
	http.SetCookie(writer, &http.Cookie{
		Name: name, Value: value, Path: "/", MaxAge: maxAge,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode,
	})
}

func setCSRFCookie(writer http.ResponseWriter, request *http.Request, value string, maxAge int) {
	secure := requestUsesSecureTransport(request)
	http.SetCookie(writer, &http.Cookie{
		Name: "werk_csrf", Value: value, Path: "/", MaxAge: maxAge,
		HttpOnly: false, Secure: secure, SameSite: http.SameSiteStrictMode,
	})
}

func newCSRFToken() string {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(value)
}

func totpQRCodeDataURL(uri string) (string, error) {
	png, err := qrcode.Encode(uri, qrcode.Medium, 256)
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), nil
}

func cookieValue(r *http.Request, name string) string {
	c, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return c.Value
}
