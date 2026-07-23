package identitystore

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/platform/database"
)

type sessionPasswordSnapshot struct {
	accountID    string
	tenantID     string
	passwordHash []byte
}

// loadSessionPasswordSnapshot performs the expensive password verification
// precondition without retaining PostgreSQL row locks. The caller must compare
// the observed hash again after locking the session, account and credential;
// a concurrent credential/security-generation change then fails closed.
func (service *Service) loadSessionPasswordSnapshot(ctx context.Context, token string, adminOnly bool) (sessionPasswordSnapshot, error) {
	if token == "" {
		return sessionPasswordSnapshot{}, identity.ErrInvalidCredentials
	}
	tokenHash := sha256.Sum256([]byte(token))
	var snapshot sessionPasswordSnapshot
	err := service.database.WithinRead(ctx, func(ctx context.Context, tx database.TenantTx) error {
		return tx.QueryRow(ctx, `
			SELECT account.id::text, COALESCE(account.tenant_id::text, ''), credential.secret_hash
			FROM werk_core.sessions AS session
			JOIN werk_core.accounts AS account ON account.id = session.account_id
			JOIN werk_core.account_credentials AS credential
			  ON credential.account_id = account.id
			 AND credential.credential_kind = 'password'
			 AND credential.status = 'active'
			 AND (credential.expires_at IS NULL OR credential.expires_at > $2)
			WHERE session.token_hash = $1 AND session.revoked_at IS NULL
			  AND session.expires_at > $2 AND account.status = 'active'
			  AND session.session_generation = account.session_generation
			  AND (NOT $3 OR (account.account_class = 'admin' AND session.audience = 'admin'))
		`, tokenHash[:], service.now(), adminOnly).Scan(&snapshot.accountID, &snapshot.tenantID, &snapshot.passwordHash)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return sessionPasswordSnapshot{}, identity.ErrInvalidCredentials
	}
	if err != nil {
		return sessionPasswordSnapshot{}, err
	}
	return snapshot, nil
}

func samePasswordHash(left, right []byte) bool {
	return len(left) == len(right) && subtle.ConstantTimeCompare(left, right) == 1
}
