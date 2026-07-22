package identitystore

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/platform/database"
)

const (
	mfaChallengeTTL   = 5 * time.Minute
	recoveryCodeCount = 10
)

func (service *Service) beginAdminMFA(ctx context.Context, record accountRecord, requestID, correlationID string) (identity.LoginResult, error) {
	var factorID string
	err := service.database.WithinRead(ctx, func(ctx context.Context, tx database.TenantTx) error {
		return tx.QueryRow(ctx, `
			SELECT id::text FROM werk_core.identity_mfa_factors
			WHERE account_id = $1::uuid AND factor_kind = 'totp' AND status = 'active'
		`, formatUUID(record.actor.AccountID)).Scan(&factorID)
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return identity.LoginResult{}, identity.ErrInvalidCredentials
	}
	if errors.Is(err, pgx.ErrNoRows) {
		// No active factor is an enrollment state, not an authentication
		// bypass. The resulting single-factor session is accepted only by the
		// identity setup endpoints and cannot authorize the admin access plane.
		result, issueErr := service.issueLoginSession(ctx, record, identity.AssuranceSingleFactor, requestID, correlationID)
		if issueErr != nil {
			return identity.LoginResult{}, issueErr
		}
		if !record.mustChangePassword {
			result.Redirect = "/mfa-setup"
		}
		return result, nil
	}

	challengeToken, challengeHash, err := newSessionToken()
	if err != nil {
		return identity.LoginResult{}, identity.ErrInvalidCredentials
	}
	challengeID, err := randomUUID()
	if err != nil {
		return identity.LoginResult{}, identity.ErrInvalidCredentials
	}
	expiresAt := service.now().Add(mfaChallengeTTL)
	err = service.database.WithinWrite(ctx, func(ctx context.Context, tx database.TenantTx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO werk_core.identity_mfa_challenges (
				id, account_id, factor_id, purpose, challenge_hash, expires_at
			) VALUES ($1::uuid, $2::uuid, $3::uuid, 'authentication', $4, $5)
		`, challengeID, formatUUID(record.actor.AccountID), factorID, challengeHash[:], expiresAt); err != nil {
			return err
		}
		if err := service.insertSecurityAudit(ctx, tx, "identity.login.second-factor-required.v1", "succeeded", formatUUID(record.actor.AccountID), "", requestID, correlationID, `{}`); err != nil {
			return err
		}
		return service.insertSecurityAudit(ctx, tx, "identity.mfa.challenge-issued.v1", "succeeded", formatUUID(record.actor.AccountID), "", requestID, correlationID, "{}")
	})
	if err != nil {
		return identity.LoginResult{}, identity.ErrInvalidCredentials
	}
	return identity.LoginResult{ChallengeToken: challengeToken, Redirect: "/mfa", MFARequired: true}, nil
}

func (service *Service) StartTOTPEnrollment(ctx context.Context, sessionToken, currentPassword, displayName, requestID, correlationID string) (identity.TOTPEnrollment, error) {
	if !service.mfaEnabled || strings.TrimSpace(displayName) == "" {
		return identity.TOTPEnrollment{}, identity.ErrMFAInvalid
	}
	secret, err := identity.NewTOTPSecret()
	if err != nil {
		return identity.TOTPEnrollment{}, err
	}
	factorID, err := randomUUID()
	if err != nil {
		return identity.TOTPEnrollment{}, err
	}
	var loginName string
	err = service.database.WithinWrite(ctx, func(ctx context.Context, tx database.TenantTx) error {
		accountID, _, passwordHash, err := lockAdminSession(ctx, tx, sessionToken, service.now())
		if err != nil || !identity.VerifyPassword(passwordHash, currentPassword) {
			return identity.ErrInvalidCredentials
		}
		var activeCount int
		if err := tx.QueryRow(ctx, `
			SELECT login_name,
			       (SELECT count(*) FROM werk_core.identity_mfa_factors
			        WHERE account_id = account.id AND factor_kind = 'totp' AND status = 'active')
			FROM werk_core.accounts AS account WHERE id = $1::uuid
		`, accountID).Scan(&loginName, &activeCount); err != nil || activeCount != 0 {
			return identity.ErrMFAInvalid
		}
		encrypted, err := service.encryptMFASecret(accountID, factorID, secret)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE werk_core.identity_mfa_factors SET status = 'revoked', revoked_at = $2
			WHERE account_id = $1::uuid AND factor_kind = 'totp' AND status = 'pending'
		`, accountID, service.now()); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO werk_core.identity_mfa_factors (
				id, account_id, factor_kind, status, display_name, secret_reference
			) VALUES ($1::uuid, $2::uuid, 'totp', 'pending', $3, $4)
		`, factorID, accountID, strings.TrimSpace(displayName), encrypted); err != nil {
			return err
		}
		return service.insertSecurityAudit(ctx, tx, "identity.mfa.enrollment-started.v1", "succeeded", accountID, "", requestID, correlationID, "{}")
	})
	if err != nil {
		return identity.TOTPEnrollment{}, err
	}
	uri, err := identity.TOTPUri("WERK", loginName, secret)
	if err != nil {
		return identity.TOTPEnrollment{}, err
	}
	return identity.TOTPEnrollment{FactorID: factorID, Secret: secret, OTPAuthURI: uri}, nil
}

func (service *Service) ConfirmTOTPEnrollment(ctx context.Context, sessionToken, factorID, code, requestID, correlationID string) (identity.TOTPActivation, error) {
	if !service.mfaEnabled || factorID == "" {
		return identity.TOTPActivation{}, identity.ErrMFAInvalid
	}
	var recoveryCodes []string
	verificationFailed := false
	err := service.database.WithinWrite(ctx, func(ctx context.Context, tx database.TenantTx) error {
		accountID, sessionID, _, err := lockAdminSession(ctx, tx, sessionToken, service.now())
		if err != nil {
			return err
		}
		var encrypted string
		var failedAttempts int
		if err := tx.QueryRow(ctx, `
			SELECT secret_reference, failed_attempts FROM werk_core.identity_mfa_factors
			WHERE id = $1::uuid AND account_id = $2::uuid AND factor_kind = 'totp' AND status = 'pending'
			FOR UPDATE
		`, factorID, accountID).Scan(&encrypted, &failedAttempts); err != nil {
			return identity.ErrMFAInvalid
		}
		secret, err := service.decryptMFASecret(accountID, factorID, encrypted)
		if err != nil {
			return identity.ErrMFAInvalid
		}
		if !identity.VerifyTOTP(secret, code, service.now()) {
			failedAttempts++
			if _, err := tx.Exec(ctx, `
				UPDATE werk_core.identity_mfa_factors
				SET failed_attempts = $3,
				    status = CASE WHEN $3 >= 5 THEN 'revoked' ELSE status END,
				    revoked_at = CASE WHEN $3 >= 5 THEN $4 ELSE revoked_at END
				WHERE id = $1::uuid AND account_id = $2::uuid
			`, factorID, accountID, failedAttempts, service.now()); err != nil {
				return err
			}
			verificationFailed = true
			return service.insertSecurityAudit(ctx, tx, "identity.mfa.enrollment-denied.v1", "denied", accountID, sessionID, requestID, correlationID, "{}")
		}
		recoveryCodes, err = newRecoveryCodes(recoveryCodeCount)
		if err != nil {
			return err
		}
		for _, recoveryCode := range recoveryCodes {
			codeID, err := randomUUID()
			if err != nil {
				return err
			}
			hash := recoveryCodeHash(recoveryCode)
			if _, err := tx.Exec(ctx, `
				INSERT INTO werk_core.identity_mfa_recovery_codes (id, account_id, factor_id, code_hash)
				VALUES ($1::uuid, $2::uuid, $3::uuid, $4)
			`, codeID, accountID, factorID, hash[:]); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `
			UPDATE werk_core.identity_mfa_factors
			SET status = 'active', activated_at = $3, last_used_at = $3
			WHERE id = $1::uuid AND account_id = $2::uuid AND status = 'pending'
		`, factorID, accountID, service.now()); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE werk_core.sessions SET authentication_assurance = 'multi-factor'
			WHERE id = $1::uuid AND account_id = $2::uuid
		`, sessionID, accountID); err != nil {
			return err
		}
		return service.insertSecurityAudit(ctx, tx, "identity.mfa.enrollment-completed.v1", "succeeded", accountID, sessionID, requestID, correlationID, "{}")
	})
	if err != nil {
		return identity.TOTPActivation{}, err
	}
	if verificationFailed {
		return identity.TOTPActivation{}, identity.ErrMFAInvalid
	}
	return identity.TOTPActivation{RecoveryCodes: recoveryCodes}, nil
}

func (service *Service) CompleteMFAChallenge(ctx context.Context, challengeToken, code, requestID, correlationID string) (identity.LoginResult, error) {
	if !service.mfaEnabled || challengeToken == "" || strings.TrimSpace(code) == "" {
		return identity.LoginResult{}, identity.ErrMFAInvalid
	}
	challengeHash := sha256.Sum256([]byte(challengeToken))
	sessionToken, sessionHash, err := newSessionToken()
	if err != nil {
		return identity.LoginResult{}, err
	}
	sessionID, err := randomUUID()
	if err != nil {
		return identity.LoginResult{}, err
	}
	redirect := "/admin"
	verificationFailed := false
	err = service.database.WithinWrite(ctx, func(ctx context.Context, tx database.TenantTx) error {
		var challengeID, accountID, factorID, encrypted, loginName, audience string
		var mustChangePassword bool
		var failedAttempts int
		var expiresAt time.Time
		if err := tx.QueryRow(ctx, `
			SELECT challenge.id::text, account.id::text, factor.id::text,
			       factor.secret_reference, account.login_name, account.must_change_password,
			       $2::text,
			       challenge.expires_at, challenge.failed_attempts
			FROM werk_core.identity_mfa_challenges AS challenge
			JOIN werk_core.accounts AS account ON account.id = challenge.account_id
			JOIN werk_core.identity_mfa_factors AS factor ON factor.id = challenge.factor_id
			WHERE challenge.challenge_hash = $1 AND challenge.purpose = 'authentication'
			  AND challenge.used_at IS NULL AND account.account_class = 'admin'
			  AND account.status = 'active' AND factor.status = 'active'
			FOR UPDATE OF challenge, factor
		`, challengeHash[:], identity.AudienceAdmin).Scan(&challengeID, &accountID, &factorID, &encrypted, &loginName, &mustChangePassword, &audience, &expiresAt, &failedAttempts); err != nil || !service.now().Before(expiresAt) {
			return identity.ErrMFAInvalid
		}
		if mustChangePassword {
			redirect = "/change-password"
		}
		secret, err := service.decryptMFASecret(accountID, factorID, encrypted)
		if err != nil {
			return identity.ErrMFAInvalid
		}
		accepted := identity.VerifyTOTP(secret, code, service.now())
		if !accepted {
			hash := recoveryCodeHash(code)
			command, err := tx.Exec(ctx, `
				UPDATE werk_core.identity_mfa_recovery_codes SET used_at = $3
				WHERE account_id = $1::uuid AND factor_id = $2::uuid
				  AND code_hash = $4 AND used_at IS NULL
			`, accountID, factorID, service.now(), hash[:])
			accepted = err == nil && command.RowsAffected() == 1
		}
		if !accepted {
			failedAttempts++
			if _, err := tx.Exec(ctx, `
				UPDATE werk_core.identity_mfa_challenges
				SET failed_attempts = $2, used_at = CASE WHEN $2 >= 5 THEN $3 ELSE used_at END
				WHERE id = $1::uuid
			`, challengeID, failedAttempts, service.now()); err != nil {
				return err
			}
			verificationFailed = true
			return service.insertSecurityAudit(ctx, tx, "identity.mfa.authentication-denied.v1", "denied", accountID, "", requestID, correlationID, "{}")
		}
		if _, err := tx.Exec(ctx, `UPDATE werk_core.identity_mfa_challenges SET used_at = $2 WHERE id = $1::uuid`, challengeID, service.now()); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE werk_core.identity_mfa_factors SET last_used_at = $2 WHERE id = $1::uuid`, factorID, service.now()); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO werk_core.sessions (
				id, account_id, token_hash, audience, expires_at,
				authentication_assurance, authentication_kind
			) VALUES ($1::uuid, $2::uuid, $3, $4, $5, 'multi-factor', 'interactive')
		`, sessionID, accountID, sessionHash[:], audience, service.now().Add(sessionTTL)); err != nil {
			return err
		}
		if err := service.insertSecurityAudit(ctx, tx, "identity.mfa.authentication-succeeded.v1", "succeeded", accountID, sessionID, requestID, correlationID, "{}"); err != nil {
			return err
		}
		return service.insertSecurityAudit(ctx, tx, "identity.login.succeeded.v1", "succeeded", accountID, sessionID, requestID, correlationID, `{"authentication_assurance":"multi-factor","audience":"admin"}`)
	})
	if err != nil {
		return identity.LoginResult{}, identity.ErrInvalidCredentials
	}
	if verificationFailed {
		return identity.LoginResult{}, identity.ErrInvalidCredentials
	}
	return identity.LoginResult{SessionToken: sessionToken, Redirect: redirect}, nil
}

func lockAdminSession(ctx context.Context, tx database.TenantTx, token string, now time.Time) (accountID, sessionID string, passwordHash []byte, err error) {
	if token == "" {
		return "", "", nil, identity.ErrSessionInvalid
	}
	hash := sha256.Sum256([]byte(token))
	err = tx.QueryRow(ctx, `
		SELECT account.id::text, session.id::text, credential.secret_hash
		FROM werk_core.sessions AS session
		JOIN werk_core.accounts AS account ON account.id = session.account_id
		JOIN werk_core.account_credentials AS credential
		  ON credential.account_id = account.id
		 AND credential.credential_kind = 'password'
		 AND credential.status = 'active'
		 AND (credential.expires_at IS NULL OR credential.expires_at > $2)
		WHERE session.token_hash = $1 AND session.revoked_at IS NULL
		  AND session.expires_at > $2 AND account.status = 'active'
		  AND account.account_class = 'admin' AND session.audience = $3
		FOR UPDATE OF session, account, credential
	`, hash[:], now, identity.AudienceAdmin).Scan(&accountID, &sessionID, &passwordHash)
	if err != nil {
		return "", "", nil, identity.ErrSessionInvalid
	}
	return accountID, sessionID, passwordHash, nil
}

func (service *Service) encryptMFASecret(accountID, factorID, secret string) (string, error) {
	key := service.mfaKeys[service.mfaCurrentKeyID]
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	aad := []byte(accountID + ":" + factorID)
	ciphertext := gcm.Seal(nil, nonce, []byte(secret), aad)
	return "enc:v2:" + service.mfaCurrentKeyID + ":" + base64.RawURLEncoding.EncodeToString(nonce) + ":" + base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

func (service *Service) decryptMFASecret(accountID, factorID, reference string) (string, error) {
	parts := strings.Split(reference, ":")
	if len(parts) < 4 || parts[0] != "enc" {
		return "", identity.ErrMFAInvalid
	}
	if parts[1] == "v1" && len(parts) == 4 {
		for _, key := range service.mfaKeys {
			plaintext, err := decryptMFASecretWithKey(key, accountID, factorID, parts[2], parts[3])
			if err == nil {
				return plaintext, nil
			}
		}
		return "", identity.ErrMFAInvalid
	}
	if parts[1] != "v2" || len(parts) != 5 || !stableMFAKeyID(parts[2]) {
		return "", identity.ErrMFAInvalid
	}
	key, ok := service.mfaKeys[parts[2]]
	if !ok {
		return "", identity.ErrMFAInvalid
	}
	return decryptMFASecretWithKey(key, accountID, factorID, parts[3], parts[4])
}

func decryptMFASecretWithKey(key []byte, accountID, factorID, encodedNonce, encodedCiphertext string) (string, error) {
	nonce, err := base64.RawURLEncoding.DecodeString(encodedNonce)
	if err != nil {
		return "", identity.ErrMFAInvalid
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(encodedCiphertext)
	if err != nil {
		return "", identity.ErrMFAInvalid
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(nonce) != gcm.NonceSize() {
		return "", identity.ErrMFAInvalid
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, []byte(accountID+":"+factorID))
	if err != nil {
		return "", identity.ErrMFAInvalid
	}
	return string(plaintext), nil
}

func newRecoveryCodes(count int) ([]string, error) {
	codes := make([]string, count)
	for index := range codes {
		value := make([]byte, 10)
		if _, err := rand.Read(value); err != nil {
			return nil, err
		}
		encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(value)
		codes[index] = encoded[:4] + "-" + encoded[4:8] + "-" + encoded[8:12] + "-" + encoded[12:]
	}
	return codes, nil
}

func recoveryCodeHash(code string) [32]byte {
	normalized := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), "-", ""))
	return sha256.Sum256([]byte(normalized))
}

func (service *Service) insertSecurityAudit(ctx context.Context, tx database.TenantTx, eventType, outcome, accountID, sessionID, requestID, correlationID, details string) error {
	return service.insertSecurityAuditForTenant(ctx, tx, eventType, outcome, accountID, sessionID, "", requestID, correlationID, details)
}

func (service *Service) insertSecurityAuditForTenant(ctx context.Context, tx database.TenantTx, eventType, outcome, accountID, sessionID, tenantID, requestID, correlationID, details string) error {
	if requestID == "" {
		requestID, _ = randomUUID()
	}
	if correlationID == "" {
		correlationID, _ = randomUUID()
	}
	auditID, err := randomUUID()
	if err != nil {
		return err
	}
	var session any
	if sessionID != "" {
		session = sessionID
	}
	var account any
	if accountID != "" {
		account = accountID
	}
	var tenant any
	if tenantID != "" {
		tenant = tenantID
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO werk_core.security_audit_events (
			id, event_type, outcome, account_id, session_id, tenant_id,
			request_id, correlation_id, details
		) VALUES ($1::uuid, $2, $3, $4::uuid, $5::uuid, $6::uuid, $7::uuid, $8::uuid, $9::jsonb)
	`, auditID, eventType, outcome, account, session, tenant, requestID, correlationID, details)
	return err
}

func (service *Service) auditSecurityEvent(ctx context.Context, eventType, outcome, accountID, tenantID, requestID, correlationID, details string) error {
	return service.database.WithinWrite(ctx, func(ctx context.Context, tx database.TenantTx) error {
		return service.insertSecurityAuditForTenant(ctx, tx, eventType, outcome, accountID, "", tenantID, requestID, correlationID, details)
	})
}
