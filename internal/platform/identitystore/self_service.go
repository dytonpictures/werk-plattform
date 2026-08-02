package identitystore

import (
	"context"
	"crypto/sha256"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/platform/database"
)

const maximumSessionLifecycleViews = 101

func (service *Service) ListOwnPasskeys(ctx context.Context, token string) ([]identity.PasskeyView, error) {
	accountID, _, err := service.resolveSelfServiceSession(ctx, token)
	if err != nil {
		return nil, err
	}
	views := []identity.PasskeyView{}
	err = service.database.WithinRead(ctx, func(ctx context.Context, tx database.TenantTx) error {
		rows, err := tx.Query(ctx, `
			SELECT id::text, display_name, created_at, activated_at, last_used_at
			FROM werk_core.identity_mfa_factors
			WHERE account_id = $1::uuid AND factor_kind = 'webauthn' AND status = 'active'
			ORDER BY created_at, id
		`, accountID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var view identity.PasskeyView
			var activated time.Time
			var last pgtype.Timestamptz
			if err := rows.Scan(&view.ID, &view.DisplayName, &view.CreatedAt, &activated, &last); err != nil {
				return err
			}
			view.ActivatedAt = activated
			if last.Valid {
				value := last.Time
				view.LastUsedAt = &value
			}
			views = append(views, view)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, identity.ErrSessionInvalid
	}
	return views, nil
}

func (service *Service) ListOwnSessions(ctx context.Context, token string) ([]identity.SessionLifecycleView, error) {
	accountID, currentID, err := service.resolveSelfServiceSession(ctx, token)
	if err != nil {
		return nil, err
	}
	views := []identity.SessionLifecycleView{}
	err = service.database.WithinRead(ctx, func(ctx context.Context, tx database.TenantTx) error {
		rows, err := tx.Query(ctx, `
			SELECT session.id::text, session.created_at, session.expires_at, session.authentication_kind, session.authentication_assurance
			FROM werk_core.sessions AS session
			JOIN werk_core.accounts AS account ON account.id = session.account_id
			WHERE session.account_id = $1::uuid AND session.revoked_at IS NULL AND session.expires_at > $2
			  AND session.session_generation = account.session_generation
			  AND session.last_seen_at > $2 - CASE session.audience
			      WHEN 'admin' THEN $4 * interval '1 second'
			      ELSE $5 * interval '1 second'
			  END
			ORDER BY session.created_at DESC, session.id
			LIMIT $3
		`, accountID, service.now(), maximumSessionLifecycleViews, int64(service.adminSessionIdleTTL/time.Second), int64(service.workSessionIdleTTL/time.Second))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var view identity.SessionLifecycleView
			if err := rows.Scan(&view.ID, &view.CreatedAt, &view.ExpiresAt, &view.AuthenticationKind, &view.AuthenticationAssurance); err != nil {
				return err
			}
			view.Current = view.ID == currentID
			views = append(views, view)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, identity.ErrSessionInvalid
	}
	return views, nil
}

func (service *Service) RevokeOwnSession(ctx context.Context, token, targetID, requestID, correlationID string) (bool, error) {
	if _, err := uuid.Parse(targetID); err != nil {
		return false, identity.ErrSessionInvalid
	}
	tokenHash := sha256.Sum256([]byte(token))
	current := false
	var targetTokenHash []byte
	err := service.database.WithinWrite(ctx, func(ctx context.Context, tx database.TenantTx) error {
		accountID, currentID, tenantID, err := resolveSelfServiceSessionInTx(ctx, tx, tokenHash[:], service.now(), service.adminSessionIdleTTL, service.workSessionIdleTTL)
		if err != nil {
			return err
		}
		var generation int64
		if err := tx.QueryRow(ctx, `SELECT id::text, session_generation FROM werk_core.accounts WHERE id=$1::uuid AND status='active' FOR UPDATE`, accountID).Scan(&accountID, &generation); err != nil {
			return identity.ErrSessionInvalid
		}
		rows, err := tx.Query(ctx, `
			SELECT id::text, token_hash FROM werk_core.sessions
			WHERE id IN ($1::uuid, $2::uuid) AND account_id=$3::uuid
			  AND revoked_at IS NULL AND expires_at>$4
			  AND session_generation=$5
			ORDER BY id FOR UPDATE
		`, currentID, targetID, accountID, service.now(), generation)
		if err != nil {
			return err
		}
		defer rows.Close()
		seenCurrent, seenTarget := false, false
		for rows.Next() {
			var id string
			var selectedTokenHash []byte
			if err := rows.Scan(&id, &selectedTokenHash); err != nil {
				return err
			}
			seenCurrent = seenCurrent || id == currentID
			seenTarget = seenTarget || id == targetID
			if id == targetID {
				targetTokenHash = append([]byte(nil), selectedTokenHash...)
			}
		}
		if err := rows.Err(); err != nil || !seenCurrent || !seenTarget {
			return identity.ErrSessionInvalid
		}
		command, err := tx.Exec(ctx, `UPDATE werk_core.sessions SET revoked_at = $2 WHERE id = $1::uuid AND revoked_at IS NULL`, targetID, service.now())
		if err != nil || command.RowsAffected() != 1 {
			return identity.ErrSessionInvalid
		}
		current = targetID == currentID
		return service.insertSecurityAuditForTenant(ctx, tx, "identity.session.revoked.v1", "succeeded", accountID, currentID, tenantID, requestID, correlationID, `{"target_session_id":"`+targetID+`"}`)
	})
	if err == nil {
		service.markSessionInvalid(ctx, targetTokenHash)
	}
	return current, err
}

func (service *Service) RevokeOwnPasskey(ctx context.Context, token, factorID, currentPassword, requestID, correlationID string) (identity.SessionRotation, error) {
	if _, err := uuid.Parse(factorID); err != nil || currentPassword == "" {
		return identity.SessionRotation{}, identity.ErrInvalidCredentials
	}
	snapshot, err := service.loadSessionPasswordSnapshot(ctx, token, false)
	if err != nil || !identity.VerifyPassword(snapshot.passwordHash, currentPassword) {
		return identity.SessionRotation{}, identity.ErrInvalidCredentials
	}
	rotation, err := service.prepareSessionRotation()
	if err != nil {
		return identity.SessionRotation{}, err
	}
	err = service.database.WithinWrite(ctx, func(ctx context.Context, tx database.TenantTx) error {
		accountID, sessionID, credentialID, providerKey, passwordHash, err := lockPasskeyEnrollmentSession(ctx, tx, token, service.now())
		if err != nil || accountID != snapshot.accountID || credentialID != snapshot.credentialID || providerKey != snapshot.providerKey || !samePasswordHash(passwordHash, snapshot.passwordHash) {
			return identity.ErrInvalidCredentials
		}
		if err := lockActiveProviderBinding(ctx, tx, accountID, providerKey, identity.AuthenticationMethodPassword); err != nil {
			return identity.ErrInvalidCredentials
		}
		var currentAssurance identity.AuthenticationAssurance
		if err := tx.QueryRow(ctx, `SELECT authentication_assurance FROM werk_core.sessions WHERE id=$1::uuid`, sessionID).Scan(&currentAssurance); err != nil {
			return identity.ErrSessionInvalid
		}
		command, err := tx.Exec(ctx, `UPDATE werk_core.identity_mfa_factors SET status='revoked', revoked_at=$3 WHERE id=$1::uuid AND account_id=$2::uuid AND factor_kind='webauthn' AND status='active'`, factorID, accountID, service.now())
		if err != nil || command.RowsAffected() != 1 {
			return identity.ErrInvalidCredentials
		}
		user, err := service.loadPasskeyUser(ctx, tx, accountID, true)
		if err != nil {
			return err
		}
		if err := service.rotateAccountSessions(ctx, tx, sessionRotationSubject{accountID: accountID, previousSessionID: sessionID, tenantID: snapshot.tenantID, audience: user.actor.Audience, assurance: currentAssurance, kind: identity.AuthenticationInteractive}, rotation, sessionRotationPasskeyRevocation, requestID, correlationID); err != nil {
			return err
		}
		return service.insertSecurityAuditForTenant(ctx, tx, "identity.passkey.revoked.v1", "succeeded", accountID, rotation.sessionID, snapshot.tenantID, requestID, correlationID, `{"factor_id":"`+factorID+`"}`)
	})
	if err != nil {
		return identity.SessionRotation{}, err
	}
	return rotation.result, nil
}

func (service *Service) resolveSelfServiceSession(ctx context.Context, token string) (accountID, sessionID string, err error) {
	if token == "" {
		return "", "", identity.ErrSessionInvalid
	}
	hash := sha256.Sum256([]byte(token))
	err = service.database.WithinRead(ctx, func(ctx context.Context, tx database.TenantTx) error {
		var tenant pgtype.UUID
		accountID, sessionID, _, err = readSelfServiceSession(ctx, tx, hash[:], service.now(), service.adminSessionIdleTTL, service.workSessionIdleTTL, false, &tenant)
		return err
	})
	return
}

func readSelfServiceSession(ctx context.Context, tx database.TenantTx, hash []byte, now time.Time, adminIdleTTL, workIdleTTL time.Duration, lock bool, tenant *pgtype.UUID) (accountID, sessionID, tenantTextValue string, err error) {
	query := `SELECT account.id::text, session.id::text, session.tenant_id FROM werk_core.sessions session JOIN werk_core.accounts account ON account.id=session.account_id LEFT JOIN werk_core.tenants tenant ON tenant.id=account.tenant_id WHERE session.token_hash=$1 AND session.revoked_at IS NULL AND session.expires_at>$2 AND session.last_seen_at > $2 - CASE session.audience WHEN 'admin' THEN $3 * interval '1 second' ELSE $4 * interval '1 second' END AND account.status='active' AND session.session_generation=account.session_generation AND ((account.account_class='admin' AND session.audience='admin') OR (account.account_class='work' AND session.audience='work' AND session.tenant_id=account.tenant_id)) AND (account.tenant_id IS NULL OR tenant.status='active')`
	if lock {
		query += ` FOR UPDATE OF account, session`
	}
	err = tx.QueryRow(ctx, query, hash, now, int64(adminIdleTTL/time.Second), int64(workIdleTTL/time.Second)).Scan(&accountID, &sessionID, tenant)
	if err != nil {
		return "", "", "", identity.ErrSessionInvalid
	}
	if tenant.Valid {
		tenantTextValue = formatUUID(tenant.Bytes)
	}
	return
}

func lockSelfServiceSession(ctx context.Context, tx database.TenantTx, hash []byte, now time.Time, adminIdleTTL, workIdleTTL time.Duration) (accountID, sessionID, tenantID string, err error) {
	var tenant pgtype.UUID
	return readSelfServiceSession(ctx, tx, hash, now, adminIdleTTL, workIdleTTL, true, &tenant)
}

func resolveSelfServiceSessionInTx(ctx context.Context, tx database.TenantTx, hash []byte, now time.Time, adminIdleTTL, workIdleTTL time.Duration) (accountID, sessionID, tenantID string, err error) {
	var tenant pgtype.UUID
	return readSelfServiceSession(ctx, tx, hash, now, adminIdleTTL, workIdleTTL, false, &tenant)
}
