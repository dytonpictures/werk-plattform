package identitystore

import (
	"context"
	"crypto/sha256"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/platform/database"
)

const reauthenticationTTL = 5 * time.Minute

func reauthenticationThrottleKey(sessionID string) [sha256.Size]byte {
	return sha256.Sum256([]byte("identity-reauthentication-v1:" + sessionID))
}

func (service *Service) StartReauthentication(ctx context.Context, sessionToken, currentPassword string, binding identity.ReauthenticationBinding, requestID, correlationID string) (identity.ReauthenticationTicket, error) {
	if identity.ValidateReauthenticationBinding(binding) != nil {
		return identity.ReauthenticationTicket{}, identity.ErrReauthenticationRequired
	}
	snapshot, err := service.loadSessionPasswordSnapshot(ctx, sessionToken, true)
	if err != nil {
		return identity.ReauthenticationTicket{}, identity.ErrInvalidCredentials
	}
	throttleKey := reauthenticationThrottleKey(snapshot.sessionID)
	throttled, err := service.loginThrottled(ctx, throttleKey)
	if err != nil || throttled {
		return identity.ReauthenticationTicket{}, identity.ErrInvalidCredentials
	}
	if !identity.VerifyPassword(snapshot.passwordHash, currentPassword) {
		_ = service.recordReauthenticationFailure(ctx, throttleKey, snapshot.accountID, snapshot.sessionID, requestID, correlationID, "password")
		return identity.ReauthenticationTicket{}, identity.ErrInvalidCredentials
	}
	token, tokenHash, err := newSessionToken()
	if err != nil {
		return identity.ReauthenticationTicket{}, err
	}
	ticketID, err := randomUUID()
	if err != nil {
		return identity.ReauthenticationTicket{}, err
	}
	now := service.now()
	expiresAt := now.Add(reauthenticationTTL)
	sessionHash := sha256.Sum256([]byte(sessionToken))
	err = service.database.WithinWrite(ctx, func(ctx context.Context, tx database.TenantTx) error {
		if _, err := tx.Exec(ctx, `WITH expired AS (SELECT id FROM werk_core.identity_reauthentication_tickets WHERE expires_at<=$1 ORDER BY expires_at LIMIT 128 FOR UPDATE SKIP LOCKED) DELETE FROM werk_core.identity_reauthentication_tickets AS ticket USING expired WHERE ticket.id=expired.id`, now); err != nil {
			return err
		}
		var generation int64
		if err := tx.QueryRow(ctx, `
			SELECT account.session_generation
			FROM werk_core.sessions AS session
			JOIN werk_core.accounts AS account ON account.id=session.account_id
			WHERE session.id=$1::uuid AND session.token_hash=$2 AND session.account_id=$3::uuid
			  AND session.revoked_at IS NULL AND session.expires_at>$4
			  AND session.last_seen_at > $4 - $5 * interval '1 second'
			  AND session.audience='admin' AND account.account_class='admin'
			  AND account.status='active' AND session.session_generation=account.session_generation
			FOR UPDATE OF session, account
		`, snapshot.sessionID, sessionHash[:], snapshot.accountID, now, int64(service.adminSessionIdleTTL/time.Second)).Scan(&generation); err != nil {
			return identity.ErrInvalidCredentials
		}
		providerKey, currentHash, err := lockPasswordCredential(ctx, tx, snapshot.accountID, snapshot.credentialID, now)
		if err != nil || providerKey != snapshot.providerKey || !samePasswordHash(currentHash, snapshot.passwordHash) {
			return identity.ErrInvalidCredentials
		}
		if _, err := tx.Exec(ctx, `DELETE FROM werk_core.identity_auth_throttles WHERE subject_hash=$1`, throttleKey[:]); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO werk_core.identity_reauthentication_tickets
			(id,token_hash,account_id,session_id,session_generation,permission_key,resource_kind,resource_id,authentication_method,created_at,expires_at)
			VALUES ($1::uuid,$2,$3::uuid,$4::uuid,$5,$6,$7,$8,'password',$9,$10)`,
			ticketID, tokenHash[:], snapshot.accountID, snapshot.sessionID, generation, binding.PermissionKey, binding.ResourceKind, binding.ResourceID, now, expiresAt)
		if err != nil {
			return err
		}
		return service.insertSecurityAudit(ctx, tx, "identity.reauthentication.succeeded.v1", "succeeded", snapshot.accountID, snapshot.sessionID, requestID, correlationID, `{"method":"password"}`)
	})
	if err != nil {
		return identity.ReauthenticationTicket{}, err
	}
	return identity.ReauthenticationTicket{Token: token, ExpiresAt: expiresAt}, nil
}

func (service *Service) StartTOTPReauthentication(ctx context.Context, sessionToken, code string, binding identity.ReauthenticationBinding, requestID, correlationID string) (identity.ReauthenticationTicket, error) {
	if !service.mfaEnabled || sessionToken == "" || !validReauthenticationTOTPCode(code) || identity.ValidateReauthenticationBinding(binding) != nil {
		return identity.ReauthenticationTicket{}, identity.ErrInvalidCredentials
	}
	token, tokenHash, err := newSessionToken()
	if err != nil {
		return identity.ReauthenticationTicket{}, err
	}
	ticketID, err := randomUUID()
	if err != nil {
		return identity.ReauthenticationTicket{}, err
	}
	now := service.now()
	expiresAt := now.Add(reauthenticationTTL)
	sessionHash := sha256.Sum256([]byte(sessionToken))
	verificationFailed := false
	err = service.database.WithinWrite(ctx, func(ctx context.Context, tx database.TenantTx) error {
		var accountID, sessionID, factorID, encrypted string
		var generation int64
		if err := tx.QueryRow(ctx, `
			SELECT account.id::text,session.id::text,account.session_generation,factor.id::text,factor.secret_reference
			FROM werk_core.sessions AS session
			JOIN werk_core.accounts AS account ON account.id=session.account_id
			JOIN LATERAL (SELECT id,secret_reference FROM werk_core.identity_mfa_factors WHERE account_id=account.id AND factor_kind='totp' AND status='active' ORDER BY activated_at DESC,id LIMIT 1) AS factor ON true
			WHERE session.token_hash=$1 AND session.revoked_at IS NULL AND session.expires_at>$2
			  AND session.last_seen_at > $2 - $3 * interval '1 second'
			  AND session.audience='admin' AND account.account_class='admin' AND account.status='active'
			  AND session.session_generation=account.session_generation
			FOR UPDATE OF session,account,factor
		`, sessionHash[:], now, int64(service.adminSessionIdleTTL/time.Second)).Scan(&accountID, &sessionID, &generation, &factorID, &encrypted); err != nil {
			return identity.ErrInvalidCredentials
		}
		throttleKey := reauthenticationThrottleKey(sessionID)
		var locked bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM werk_core.identity_auth_throttles WHERE subject_hash=$1 AND locked_until>$2)`, throttleKey[:], now).Scan(&locked); err != nil {
			return err
		}
		if locked {
			return identity.ErrInvalidCredentials
		}
		secret, err := service.decryptMFASecret(accountID, factorID, encrypted)
		if err != nil || !identity.VerifyTOTP(secret, code, now) {
			verificationFailed = true
			if err := recordAuthenticationFailureTx(ctx, tx, throttleKey, now); err != nil {
				return err
			}
			return service.insertSecurityAudit(ctx, tx, "identity.reauthentication.denied.v1", "denied", accountID, sessionID, requestID, correlationID, `{"method":"totp"}`)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM werk_core.identity_auth_throttles WHERE subject_hash=$1`, throttleKey[:]); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE werk_core.identity_mfa_factors SET last_used_at=$2 WHERE id=$1::uuid`, factorID, now); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO werk_core.identity_reauthentication_tickets
			(id,token_hash,account_id,session_id,session_generation,permission_key,resource_kind,resource_id,authentication_method,created_at,expires_at)
			VALUES ($1::uuid,$2,$3::uuid,$4::uuid,$5,$6,$7,$8,'totp',$9,$10)`,
			ticketID, tokenHash[:], accountID, sessionID, generation, binding.PermissionKey, binding.ResourceKind, binding.ResourceID, now, expiresAt); err != nil {
			return err
		}
		return service.insertSecurityAudit(ctx, tx, "identity.reauthentication.succeeded.v1", "succeeded", accountID, sessionID, requestID, correlationID, `{"method":"totp"}`)
	})
	if err != nil {
		return identity.ReauthenticationTicket{}, err
	}
	if verificationFailed {
		return identity.ReauthenticationTicket{}, identity.ErrInvalidCredentials
	}
	return identity.ReauthenticationTicket{Token: token, ExpiresAt: expiresAt}, nil
}

func validReauthenticationTOTPCode(code string) bool {
	if len(code) != 6 {
		return false
	}
	for _, character := range code {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func (service *Service) ConsumeReauthentication(ctx context.Context, sessionToken, ticketToken string, binding identity.ReauthenticationBinding, requestID, correlationID string) error {
	if sessionToken == "" || ticketToken == "" || identity.ValidateReauthenticationBinding(binding) != nil {
		return identity.ErrReauthenticationRequired
	}
	sessionHash := sha256.Sum256([]byte(sessionToken))
	ticketHash := sha256.Sum256([]byte(ticketToken))
	now := service.now()
	err := service.database.WithinWrite(ctx, func(ctx context.Context, tx database.TenantTx) error {
		var accountID, sessionID string
		var generation int64
		if err := tx.QueryRow(ctx, `SELECT account.id::text,session.id::text,account.session_generation
			FROM werk_core.sessions AS session JOIN werk_core.accounts AS account ON account.id=session.account_id
			WHERE session.token_hash=$1 AND session.revoked_at IS NULL AND session.expires_at>$2
			AND session.last_seen_at > $2 - $3 * interval '1 second'
			AND session.audience='admin' AND account.account_class='admin' AND account.status='active'
			AND session.session_generation=account.session_generation FOR UPDATE OF session,account`, sessionHash[:], now, int64(service.adminSessionIdleTTL/time.Second)).Scan(&accountID, &sessionID, &generation); err != nil {
			return identity.ErrReauthenticationRequired
		}
		command, err := tx.Exec(ctx, `UPDATE werk_core.identity_reauthentication_tickets SET used_at=$1
			WHERE token_hash=$2 AND account_id=$3::uuid AND session_id=$4::uuid AND session_generation=$5
			AND permission_key=$6 AND resource_kind=$7 AND resource_id=$8 AND used_at IS NULL AND expires_at>$1`,
			now, ticketHash[:], accountID, sessionID, generation, binding.PermissionKey, binding.ResourceKind, binding.ResourceID)
		if err != nil || command.RowsAffected() != 1 {
			return identity.ErrReauthenticationRequired
		}
		return service.insertSecurityAudit(ctx, tx, "identity.reauthentication.consumed.v1", "succeeded", accountID, sessionID, requestID, correlationID, `{"binding":"action-bound"}`)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.ErrReauthenticationRequired
	}
	return err
}

func (service *Service) recordReauthenticationFailure(ctx context.Context, key [sha256.Size]byte, accountID, sessionID, requestID, correlationID, method string) error {
	now := service.now()
	return service.database.WithinWrite(ctx, func(ctx context.Context, tx database.TenantTx) error {
		if err := recordAuthenticationFailureTx(ctx, tx, key, now); err != nil {
			return err
		}
		return service.insertSecurityAudit(ctx, tx, "identity.reauthentication.denied.v1", "denied", accountID, sessionID, requestID, correlationID, `{"method":"`+method+`"}`)
	})
}
