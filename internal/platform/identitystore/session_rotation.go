package identitystore

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/platform/database"
)

const (
	sessionRotationPasswordChange = "password-change"
	sessionRotationMFAEnrollment  = "mfa-enrollment"
)

type pendingSessionRotation struct {
	result    identity.SessionRotation
	sessionID string
	tokenHash [32]byte
	createdAt time.Time
}

type sessionRotationSubject struct {
	accountID         string
	previousSessionID string
	tenantID          string
	audience          identity.Audience
	assurance         identity.AuthenticationAssurance
	kind              identity.AuthenticationKind
}

func (service *Service) prepareSessionRotation() (pendingSessionRotation, error) {
	token, tokenHash, err := newSessionToken()
	if err != nil {
		return pendingSessionRotation{}, err
	}
	sessionID, err := randomUUID()
	if err != nil {
		return pendingSessionRotation{}, err
	}
	createdAt := service.now()
	result := identity.SessionRotation{SessionToken: token, ExpiresAt: createdAt.Add(sessionTTL)}
	if err := result.Validate(createdAt); err != nil {
		return pendingSessionRotation{}, err
	}
	return pendingSessionRotation{
		result: result, sessionID: sessionID, tokenHash: tokenHash, createdAt: createdAt,
	}, nil
}

func (rotation *pendingSessionRotation) limitExpiresAt(expiresAt time.Time) error {
	if expiresAt.Before(rotation.result.ExpiresAt) {
		rotation.result.ExpiresAt = expiresAt
	}
	return rotation.result.Validate(rotation.createdAt)
}

// rotateAccountSessions replaces every non-revoked session for an account with
// exactly one new interactive session. Callers must first lock the current
// session and account row in the same transaction. Credential/factor changes,
// revocation, replacement and audit export therefore commit or roll back as a
// single PostgreSQL operation.
func (service *Service) rotateAccountSessions(
	ctx context.Context,
	tx database.TenantTx,
	subject sessionRotationSubject,
	rotation pendingSessionRotation,
	reason string,
	requestID string,
	correlationID string,
) error {
	if subject.accountID == "" || subject.previousSessionID == "" {
		return identity.ErrSessionInvalid
	}
	if subject.kind != identity.AuthenticationInteractive {
		return identity.ErrAccessDenied
	}
	if reason != sessionRotationPasswordChange && reason != sessionRotationMFAEnrollment {
		return errors.New("unsupported session rotation reason")
	}
	var sessionGeneration int64
	if err := tx.QueryRow(ctx, `
		UPDATE werk_core.accounts
		SET session_generation = session_generation + 1
		WHERE id = $1::uuid
		RETURNING session_generation
	`, subject.accountID).Scan(&sessionGeneration); err != nil {
		return err
	}

	command, err := tx.Exec(ctx, `
		UPDATE werk_core.sessions
		SET revoked_at = $2
		WHERE account_id = $1::uuid AND revoked_at IS NULL
	`, subject.accountID, rotation.createdAt)
	if err != nil {
		return err
	}
	if command.RowsAffected() < 1 {
		return identity.ErrSessionInvalid
	}
	revokedSessionCount := command.RowsAffected()

	var tenant any
	if subject.tenantID != "" {
		tenant = subject.tenantID
	}
	command, err = tx.Exec(ctx, `
		INSERT INTO werk_core.sessions (
			id, account_id, token_hash, audience, tenant_id, created_at, expires_at,
			authentication_assurance, authentication_kind, session_generation
		) VALUES ($1::uuid, $2::uuid, $3, $4, $5::uuid, $6, $7, $8, $9, $10)
	`, rotation.sessionID, subject.accountID, rotation.tokenHash[:], subject.audience, tenant,
		rotation.createdAt, rotation.result.ExpiresAt, subject.assurance, subject.kind, sessionGeneration)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return identity.ErrSessionInvalid
	}

	details, err := json.Marshal(map[string]any{
		"previous_session_id":   subject.previousSessionID,
		"reason":                reason,
		"revoked_session_count": revokedSessionCount,
		"session_generation":    sessionGeneration,
	})
	if err != nil {
		return err
	}
	return service.insertSecurityAuditForTenant(
		ctx, tx, "identity.session.rotated.v1", "succeeded",
		subject.accountID, rotation.sessionID, subject.tenantID,
		requestID, correlationID, string(details),
	)
}
