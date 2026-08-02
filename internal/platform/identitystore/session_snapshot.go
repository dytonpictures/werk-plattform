package identitystore

import (
	"context"
	"crypto/sha256"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/tenancy"
	"github.com/dytonpictures/werk/internal/platform/database"
)

// sessionSnapshot is the single authoritative read model used by profile,
// access-plane resolution and later session-policy evaluation. Keeping one SQL
// predicate prevents a session from appearing valid in the UI after its tenant
// or security generation has already made it unusable at an API boundary.
type sessionSnapshot struct {
	tokenHash      [sha256.Size]byte
	sessionID      identity.SessionID
	actor          identity.AuthenticatedActor
	expiresAt      time.Time
	lastSeenAt     time.Time
	loginName      string
	displayName    string
	mustChange     bool
	navigationMode string
}

func (service *Service) sessionLifetime(audience identity.Audience) time.Duration {
	if audience == identity.AudienceAdmin {
		return service.adminSessionTTL
	}
	return service.workSessionTTL
}

func (service *Service) sessionIdleLifetime(audience identity.Audience) time.Duration {
	if audience == identity.AudienceAdmin {
		return service.adminSessionIdleTTL
	}
	return service.workSessionIdleTTL
}

func (service *Service) loadSessionSnapshot(ctx context.Context, token string) (sessionSnapshot, error) {
	if token == "" {
		return sessionSnapshot{}, identity.ErrSessionInvalid
	}
	snapshot := sessionSnapshot{tokenHash: sha256.Sum256([]byte(token))}
	if service.sessionKnownInvalid(ctx, snapshot.tokenHash[:]) {
		return sessionSnapshot{}, identity.ErrSessionInvalid
	}
	var sessionID, accountID [16]byte
	var accountClass, audience, assurance, authenticationKind string
	var tenantValue pgtype.UUID
	now := service.now()
	err := service.database.WithinRead(ctx, func(ctx context.Context, tx database.TenantTx) error {
		return tx.QueryRow(ctx, `
			SELECT session.id, account.id, account.account_class, session.audience,
			       session.authentication_assurance, session.authentication_kind,
			       session.tenant_id, session.expires_at, session.last_seen_at, account.must_change_password,
			       account.login_name, COALESCE(admin_subject.display_name, party.display_name),
			       COALESCE(preference.navigation_mode, 'bar')
			FROM werk_core.sessions AS session
			JOIN werk_core.accounts AS account ON account.id=session.account_id
			LEFT JOIN werk_core.tenants AS tenant ON tenant.id=account.tenant_id
			LEFT JOIN werk_core.admin_subjects AS admin_subject ON admin_subject.id=account.admin_subject_id
			LEFT JOIN werk_core.parties AS party
			  ON party.tenant_id=account.tenant_id AND party.id=account.person_party_id
			LEFT JOIN werk_core.account_ui_preferences AS preference ON preference.account_id=account.id
			WHERE session.token_hash=$1 AND session.revoked_at IS NULL
			  AND session.expires_at>$2 AND account.status='active'
			  AND session.last_seen_at > $2 - CASE session.audience
			      WHEN 'admin' THEN $3 * interval '1 second'
			      ELSE $4 * interval '1 second'
			  END
			  AND session.session_generation=account.session_generation
			  AND (account.tenant_id IS NULL OR tenant.status='active')
		`, snapshot.tokenHash[:], now, int64(service.adminSessionIdleTTL/time.Second), int64(service.workSessionIdleTTL/time.Second)).Scan(
			&sessionID, &accountID, &accountClass, &audience, &assurance,
			&authenticationKind, &tenantValue, &snapshot.expiresAt, &snapshot.lastSeenAt, &snapshot.mustChange,
			&snapshot.loginName, &snapshot.displayName, &snapshot.navigationMode,
		)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		service.markSessionInvalid(ctx, snapshot.tokenHash[:])
		return sessionSnapshot{}, identity.ErrSessionInvalid
	}
	if err != nil {
		return sessionSnapshot{}, identity.ErrSessionInvalid
	}
	snapshot.sessionID = identity.SessionID(sessionID)
	snapshot.actor = identity.AuthenticatedActor{
		AccountID: identity.AccountID(accountID), AccountClass: identity.AccountClass(accountClass),
		Audience: identity.Audience(audience), Kind: identity.AuthenticationKind(authenticationKind),
		Assurance: identity.AuthenticationAssurance(assurance),
	}
	if tenantValue.Valid {
		tenantID := tenancy.TenantID(tenantValue.Bytes)
		snapshot.actor.TenantID = &tenantID
	}
	if _, err := identity.ValidateSessionRecord(identity.SessionRecord{
		ID: snapshot.sessionID, Account: snapshot.actor, Audience: snapshot.actor.Audience,
		TenantID: snapshot.actor.TenantID, ExpiresAt: snapshot.expiresAt,
	}, now); err != nil {
		service.markSessionInvalid(ctx, snapshot.tokenHash[:])
		return sessionSnapshot{}, identity.ErrSessionInvalid
	}
	if now.Sub(snapshot.lastSeenAt) >= sessionActivityWriteInterval {
		err := service.database.WithinWrite(ctx, func(ctx context.Context, tx database.TenantTx) error {
			command, err := tx.Exec(ctx, `
				UPDATE werk_core.sessions AS session
				SET last_seen_at=$3
				WHERE session.id=$1 AND session.last_seen_at=$2 AND session.revoked_at IS NULL AND session.expires_at>$3
				  AND EXISTS (
				    SELECT 1 FROM werk_core.accounts AS account
				    LEFT JOIN werk_core.tenants AS tenant ON tenant.id=account.tenant_id
				    WHERE account.id=session.account_id AND account.status='active'
				      AND account.session_generation=session.session_generation
				      AND (account.tenant_id IS NULL OR tenant.status='active')
				  )
			`, formatUUID(identity.SessionID(sessionID)), snapshot.lastSeenAt, now)
			if err != nil {
				return err
			}
			if command.RowsAffected() == 1 {
				return nil
			}
			var stillValid bool
			if err := tx.QueryRow(ctx, `
				SELECT EXISTS (
				  SELECT 1 FROM werk_core.sessions AS session
				  JOIN werk_core.accounts AS account ON account.id=session.account_id
				  LEFT JOIN werk_core.tenants AS tenant ON tenant.id=account.tenant_id
				  WHERE session.id=$1::uuid AND session.revoked_at IS NULL AND session.expires_at>$2
				    AND session.last_seen_at > $2 - CASE session.audience WHEN 'admin' THEN $3 * interval '1 second' ELSE $4 * interval '1 second' END
				    AND account.status='active' AND account.session_generation=session.session_generation
				    AND (account.tenant_id IS NULL OR tenant.status='active')
				)
			`, formatUUID(identity.SessionID(sessionID)), now, int64(service.adminSessionIdleTTL/time.Second), int64(service.workSessionIdleTTL/time.Second)).Scan(&stillValid); err != nil {
				return err
			}
			if !stillValid {
				return identity.ErrSessionInvalid
			}
			return nil
		})
		if err != nil {
			return sessionSnapshot{}, identity.ErrSessionInvalid
		}
		snapshot.lastSeenAt = now
	}
	return snapshot, nil
}
