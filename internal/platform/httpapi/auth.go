package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/skip2/go-qrcode"

	corecache "github.com/dytonpictures/werk/internal/core/cache"
	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/platform/identityprotocol/oidc"
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

type passkeyManager interface {
	StartPasskeyRegistration(context.Context, string, string, string, string, string) (identity.PasskeyCeremony, error)
	FinishPasskeyRegistration(context.Context, string, string, string, identity.PasskeyRegistrationCredential, string, string) (identity.PasskeyActivation, error)
	StartPasskeyLogin(context.Context, string, string) (identity.PasskeyCeremony, error)
	FinishPasskeyLogin(context.Context, string, identity.PasskeyAuthenticationCredential, string, string) (identity.LoginResult, error)
}

type identitySelfService interface {
	ListOwnPasskeys(context.Context, string) ([]identity.PasskeyView, error)
	RevokeOwnPasskey(context.Context, string, string, string, string, string) (identity.SessionRotation, error)
	ListOwnSessions(context.Context, string) ([]identity.SessionLifecycleView, error)
	RevokeOwnSession(context.Context, string, string, string, string) (bool, error)
}

type initialWorkAccountInvitationAccepter interface {
	AcceptInitialWorkAccountInvitation(context.Context, string, string, string, string) error
}

type OIDCLoginService interface {
	Begin(context.Context, string) (string, error)
	Complete(context.Context, string, oidc.Callback, string, string) (identity.LoginResult, error)
}

func authRoutes(service AuthService, options ...any) http.Handler {
	var rateLimitCounter corecache.CounterPort
	var oidcLogin OIDCLoginService
	for _, option := range options {
		switch value := option.(type) {
		case corecache.CounterPort:
			rateLimitCounter = value
		case OIDCLoginService:
			oidcLogin = value
		}
	}
	r := chi.NewRouter()
	invitationActivationLimiter := newSourceWindowLimiter(
		invitationActivationAttemptsPerWindow,
		invitationActivationWindow,
		invitationActivationMaximumSources,
	).withCounter(rateLimitCounter, "werk:v1:rate:invitation-activation")
	passkeyStartLimiter := newSourceWindowLimiter(30, time.Minute, 4096).withCounter(rateLimitCounter, "werk:v1:rate:passkey-start")
	passkeyVerificationLimiter := newSourceWindowLimiter(30, time.Minute, 4096).withCounter(rateLimitCounter, "werk:v1:rate:passkey-verify")
	r.Get("/oidc/{providerKey}/start", func(w http.ResponseWriter, req *http.Request) {
		if oidcLogin == nil {
			writeProblem(w, req, http.StatusNotImplemented, "oidc-login-unavailable", "OIDC login unavailable", "External OIDC login is not configured.")
			return
		}
		providerKey := strings.TrimSpace(chi.URLParam(req, "providerKey"))
		location, err := oidcLogin.Begin(req.Context(), providerKey)
		if err != nil {
			writeProblem(w, req, http.StatusBadRequest, "oidc-login-unavailable", "OIDC login unavailable", "The selected identity provider is unavailable.")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, req, location, http.StatusSeeOther)
	})
	r.Get("/oidc/{providerKey}/callback", func(w http.ResponseWriter, req *http.Request) {
		if oidcLogin == nil {
			writeProblem(w, req, http.StatusNotImplemented, "oidc-login-unavailable", "OIDC login unavailable", "External OIDC login is not configured.")
			return
		}
		query := req.URL.Query()
		for _, key := range []string{"state", "code", "error"} {
			if len(query[key]) > 1 {
				writeProblem(w, req, http.StatusBadRequest, "oidc-callback-rejected", "OIDC login failed", "The identity provider response was rejected.")
				return
			}
		}
		result, err := oidcLogin.Complete(req.Context(), strings.TrimSpace(chi.URLParam(req, "providerKey")), oidc.Callback{State: query.Get("state"), Code: query.Get("code"), Error: query.Get("error")}, requestIDFromContext(req.Context()), correlationIDFromContext(req.Context()))
		if err != nil {
			writeProblem(w, req, http.StatusUnauthorized, "oidc-callback-rejected", "OIDC login failed", "The identity provider response was rejected.")
			return
		}
		setSessionCookie(w, req, result.SessionToken, 0)
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, req, result.Redirect, http.StatusSeeOther)
	})
	r.Post("/invitations/initial/accept", func(w http.ResponseWriter, req *http.Request) {
		if !invitationActivationLimiter.allowContext(req.Context(), req.RemoteAddr, time.Now().UTC()) {
			writeInvitationActivationRateLimit(w, req)
			return
		}
		accepter, ok := service.(initialWorkAccountInvitationAccepter)
		if !ok {
			writeProblem(w, req, http.StatusNotImplemented, "invitation-activation-unavailable", "Invitation activation unavailable", "Invitation activation is not configured.")
			return
		}
		var input struct {
			Token       string `json:"token"`
			NewPassword string `json:"new_password"`
		}
		if decodeJSON(w, req, &input) != nil {
			writeInvitationActivationRejection(w, req)
			return
		}
		if _, err := identity.HashInitialWorkAccountInvitationToken(input.Token); err != nil ||
			len(input.NewPassword) < 12 || len(input.NewPassword) > 1024 {
			writeInvitationActivationRejection(w, req)
			return
		}
		if err := accepter.AcceptInitialWorkAccountInvitation(
			req.Context(), input.Token, input.NewPassword,
			requestIDFromContext(req.Context()), correlationIDFromContext(req.Context()),
		); err != nil {
			if errors.Is(err, identity.ErrInitialWorkAccountInvitationBusy) {
				writeInvitationActivationRateLimit(w, req)
				return
			}
			if errors.Is(err, identity.ErrInitialWorkAccountInvitationInvalid) || errors.Is(err, identity.ErrPasswordInvalid) {
				writeInvitationActivationRejection(w, req)
				return
			}
			writeProblem(w, req, http.StatusInternalServerError, "invitation-activation-processing-failed", "Invitation activation failed", "The invitation could not be processed.")
			return
		}
		// Activation never authenticates implicitly. The user signs in through
		// the normal provider flow after this one-time ceremony completes.
		w.WriteHeader(http.StatusNoContent)
	})
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
	r.Post("/passkeys/registration/options", func(w http.ResponseWriter, req *http.Request) {
		manager, ok := service.(passkeyManager)
		if !ok {
			writeProblem(w, req, http.StatusNotImplemented, "passkeys-unavailable", "Passkeys unavailable", "Passkey authentication is not configured.")
			return
		}
		var input struct {
			CurrentPassword string `json:"current_password"`
			DisplayName     string `json:"display_name"`
		}
		if decodeJSON(w, req, &input) != nil || input.CurrentPassword == "" || input.DisplayName == "" {
			writeProblem(w, req, http.StatusBadRequest, "invalid-passkey-enrollment", "Invalid passkey enrollment", "Enrollment data is invalid.")
			return
		}
		result, err := manager.StartPasskeyRegistration(
			req.Context(), cookieValue(req, "werk_session"), input.CurrentPassword, input.DisplayName,
			requestIDFromContext(req.Context()), correlationIDFromContext(req.Context()),
		)
		if err != nil {
			if !isAuthenticationRejection(err) {
				writeProblem(w, req, http.StatusInternalServerError, "passkey-processing-failed", "Passkey processing failed", "The passkey request could not be processed.")
				return
			}
			writeProblem(w, req, http.StatusUnauthorized, "passkey-enrollment-failed", "Passkey enrollment failed", "Enrollment could not be started.")
			return
		}
		setPrivateCookie(w, req, "webauthn_ceremony", result.Token, int((5*time.Minute)/time.Second))
		writeJSON(w, http.StatusOK, result.PublicKey)
	})
	r.Post("/passkeys/registration/verification", func(w http.ResponseWriter, req *http.Request) {
		manager, ok := service.(passkeyManager)
		if !ok {
			writeProblem(w, req, http.StatusNotImplemented, "passkeys-unavailable", "Passkeys unavailable", "Passkey authentication is not configured.")
			return
		}
		var input struct {
			DisplayName string                                 `json:"display_name"`
			Credential  identity.PasskeyRegistrationCredential `json:"credential"`
		}
		if decodeJSON(w, req, &input) != nil || input.DisplayName == "" {
			writeProblem(w, req, http.StatusBadRequest, "invalid-passkey-enrollment", "Invalid passkey enrollment", "Enrollment data is invalid.")
			return
		}
		result, err := manager.FinishPasskeyRegistration(
			req.Context(), cookieValue(req, "werk_session"), cookieValue(req, "webauthn_ceremony"),
			input.DisplayName, input.Credential,
			requestIDFromContext(req.Context()), correlationIDFromContext(req.Context()),
		)
		if err != nil {
			if !isAuthenticationRejection(err) {
				writeProblem(w, req, http.StatusInternalServerError, "passkey-processing-failed", "Passkey processing failed", "The passkey request could not be processed.")
				return
			}
			writeProblem(w, req, http.StatusUnauthorized, "passkey-verification-failed", "Passkey verification failed", "The passkey response was rejected.")
			return
		}
		if err := result.Rotation.Validate(time.Now()); err != nil {
			writeProblem(w, req, http.StatusInternalServerError, "session-rotation-failed", "Session rotation failed", "The replacement session could not be installed.")
			return
		}
		setPrivateCookie(w, req, "webauthn_ceremony", "", -1)
		setSessionCookie(w, req, result.Rotation.SessionToken, sessionCookieMaxAge(result.Rotation.ExpiresAt))
		writeJSON(w, http.StatusCreated, map[string]any{"registered": true, "display_name": result.DisplayName})
	})
	r.Get("/passkeys", func(w http.ResponseWriter, req *http.Request) {
		manager, ok := service.(identitySelfService)
		if !ok {
			writeProblem(w, req, http.StatusNotImplemented, "identity-self-service-unavailable", "Identity self-service unavailable", "Identity self-service is not configured.")
			return
		}
		items, err := manager.ListOwnPasskeys(req.Context(), cookieValue(req, "werk_session"))
		if err != nil {
			writeProblem(w, req, http.StatusUnauthorized, "invalid-session", "Authentication required", "No valid session exists.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	})
	r.Delete("/passkeys/{factorID}", func(w http.ResponseWriter, req *http.Request) {
		manager, ok := service.(identitySelfService)
		if !ok {
			writeProblem(w, req, http.StatusNotImplemented, "identity-self-service-unavailable", "Identity self-service unavailable", "Identity self-service is not configured.")
			return
		}
		var input struct {
			CurrentPassword string `json:"current_password"`
		}
		if decodeJSON(w, req, &input) != nil || input.CurrentPassword == "" {
			writeProblem(w, req, http.StatusBadRequest, "invalid-passkey-revocation", "Invalid passkey revocation", "The revocation request is invalid.")
			return
		}
		rotation, err := manager.RevokeOwnPasskey(req.Context(), cookieValue(req, "werk_session"), chi.URLParam(req, "factorID"), input.CurrentPassword, requestIDFromContext(req.Context()), correlationIDFromContext(req.Context()))
		if err != nil || rotation.Validate(time.Now()) != nil {
			writeProblem(w, req, http.StatusUnauthorized, "passkey-revocation-failed", "Passkey revocation failed", "The passkey could not be revoked.")
			return
		}
		setSessionCookie(w, req, rotation.SessionToken, sessionCookieMaxAge(rotation.ExpiresAt))
		w.WriteHeader(http.StatusNoContent)
	})
	r.Get("/sessions", func(w http.ResponseWriter, req *http.Request) {
		manager, ok := service.(identitySelfService)
		if !ok {
			writeProblem(w, req, http.StatusNotImplemented, "identity-self-service-unavailable", "Identity self-service unavailable", "Identity self-service is not configured.")
			return
		}
		items, err := manager.ListOwnSessions(req.Context(), cookieValue(req, "werk_session"))
		if err != nil {
			writeProblem(w, req, http.StatusUnauthorized, "invalid-session", "Authentication required", "No valid session exists.")
			return
		}
		truncated := len(items) > 100
		if truncated {
			items = items[:100]
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items, "truncated": truncated})
	})
	r.Delete("/sessions/{sessionID}", func(w http.ResponseWriter, req *http.Request) {
		manager, ok := service.(identitySelfService)
		if !ok {
			writeProblem(w, req, http.StatusNotImplemented, "identity-self-service-unavailable", "Identity self-service unavailable", "Identity self-service is not configured.")
			return
		}
		current, err := manager.RevokeOwnSession(req.Context(), cookieValue(req, "werk_session"), chi.URLParam(req, "sessionID"), requestIDFromContext(req.Context()), correlationIDFromContext(req.Context()))
		if err != nil {
			writeProblem(w, req, http.StatusNotFound, "session-not-found", "Session not found", "The session does not exist or cannot be revoked.")
			return
		}
		if current {
			setSessionCookie(w, req, "", -1)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	r.Post("/passkeys/authentication/options", func(w http.ResponseWriter, req *http.Request) {
		manager, ok := service.(passkeyManager)
		if !ok {
			writeProblem(w, req, http.StatusNotImplemented, "passkeys-unavailable", "Passkeys unavailable", "Passkey authentication is not configured.")
			return
		}
		if !passkeyStartLimiter.allowContext(req.Context(), req.RemoteAddr, time.Now().UTC()) {
			w.Header().Set("Retry-After", "60")
			writeProblem(w, req, http.StatusTooManyRequests, "passkey-login-rate-limited", "Passkey login temporarily limited", "Too many passkey requests are in progress. Try again later.")
			return
		}
		var input struct {
			// Accepted temporarily for v1 wire compatibility and deliberately
			// ignored. Passkey account selection is performed by WebAuthn.
			LegacyLoginName string `json:"login_name,omitempty"`
		}
		if decodeJSON(w, req, &input) != nil {
			writeProblem(w, req, http.StatusBadRequest, "invalid-passkey-login", "Invalid passkey login", "The passkey request is invalid.")
			return
		}
		result, err := manager.StartPasskeyLogin(
			req.Context(), requestIDFromContext(req.Context()), correlationIDFromContext(req.Context()),
		)
		if err != nil {
			writeProblem(w, req, http.StatusUnauthorized, "passkey-login-failed", "Passkey login failed", "Passkey authentication could not be started.")
			return
		}
		setPrivateCookie(w, req, "webauthn_ceremony", result.Token, int((5*time.Minute)/time.Second))
		setCSRFCookie(w, req, newCSRFToken(), int((5*time.Minute)/time.Second))
		writeJSON(w, http.StatusOK, result.PublicKey)
	})
	r.Post("/passkeys/authentication/verification", func(w http.ResponseWriter, req *http.Request) {
		manager, ok := service.(passkeyManager)
		if !ok {
			writeProblem(w, req, http.StatusNotImplemented, "passkeys-unavailable", "Passkeys unavailable", "Passkey authentication is not configured.")
			return
		}
		if !passkeyVerificationLimiter.allowContext(req.Context(), req.RemoteAddr, time.Now().UTC()) {
			w.Header().Set("Retry-After", "60")
			writeProblem(w, req, http.StatusTooManyRequests, "passkey-verification-rate-limited", "Passkey verification temporarily limited", "Too many passkey responses are in progress. Try again later.")
			return
		}
		var credential identity.PasskeyAuthenticationCredential
		if decodeJSON(w, req, &credential) != nil {
			writeProblem(w, req, http.StatusBadRequest, "invalid-passkey-login", "Invalid passkey login", "The passkey response is invalid.")
			return
		}
		result, err := manager.FinishPasskeyLogin(
			req.Context(), cookieValue(req, "webauthn_ceremony"), credential,
			requestIDFromContext(req.Context()), correlationIDFromContext(req.Context()),
		)
		if err != nil {
			writeProblem(w, req, http.StatusUnauthorized, "passkey-verification-failed", "Passkey verification failed", "The passkey response was rejected.")
			return
		}
		setPrivateCookie(w, req, "webauthn_ceremony", "", -1)
		setSessionCookie(w, req, result.SessionToken, 0)
		writeJSON(w, http.StatusOK, map[string]string{"redirect": result.Redirect})
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

func writeInvitationActivationRejection(writer http.ResponseWriter, request *http.Request) {
	writeProblem(writer, request, http.StatusBadRequest, "invitation-activation-failed", "Invitation activation failed", "The invitation could not be accepted.")
}

func writeInvitationActivationRateLimit(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Retry-After", "60")
	writeProblem(writer, request, http.StatusTooManyRequests, "invitation-activation-rate-limited", "Invitation activation temporarily limited", "Too many activation attempts are in progress. Try again later.")
}

func isAuthenticationRejection(err error) bool {
	return errors.Is(err, identity.ErrInvalidCredentials) ||
		errors.Is(err, identity.ErrSessionInvalid) ||
		errors.Is(err, identity.ErrSessionExpired) ||
		errors.Is(err, identity.ErrMFAInvalid) ||
		errors.Is(err, identity.ErrMFARequired) ||
		errors.Is(err, identity.ErrMFAEnrollment) ||
		errors.Is(err, identity.ErrMFAChallengeUsed) ||
		errors.Is(err, identity.ErrPasskeyInvalid) ||
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
