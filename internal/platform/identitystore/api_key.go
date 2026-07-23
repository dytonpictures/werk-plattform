package identitystore

import (
	"context"
	"crypto/sha256"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/platform/database"
)

// AuthenticateAPIKey resolves a technical credential directly to a Core-owned
// actor. It does not create a browser session or cookie. The use counter is
// exact across instances because verification and increment share one row lock.
func (service *Service) AuthenticateAPIKey(ctx context.Context, token, requestID, correlationID string) (identity.AuthenticatedActor, error) {
	digest, err := identity.DigestAPIKey(token)
	if err != nil {
		return identity.AuthenticatedActor{}, identity.ErrInvalidCredentials
	}
	var actor identity.AuthenticatedActor
	verificationFailed := false
	err = service.database.WithinWrite(ctx, func(ctx context.Context, tx database.TenantTx) error {
		var credentialID string
		var accountID [16]byte
		var accountClass string
		var tenantValue pgtype.UUID
		var expectedSecret []byte
		var useCount int64
		var useLimit *int64
		if err := tx.QueryRow(ctx, `
			SELECT credential.id::text, account.id, account.account_class, account.tenant_id,
			       credential.secret_hash, credential.use_count, credential.use_limit
			FROM werk_core.account_credentials AS credential
			JOIN werk_core.accounts AS account
			  ON account.id = credential.account_id AND account.status = 'active'
			LEFT JOIN werk_core.tenants AS tenant ON tenant.id = account.tenant_id
			LEFT JOIN werk_core.identity_agents AS agent ON agent.id = account.agent_subject_id
			WHERE credential.public_id_hash = $1 AND credential.credential_kind = 'api-key'
			  AND credential.status = 'active'
			  AND (credential.expires_at IS NULL OR credential.expires_at > $2)
			  AND account.account_class IN ('service', 'agent')
			  AND (account.tenant_id IS NULL OR tenant.status = 'active')
			  AND (account.account_class <> 'agent' OR agent.status = 'active')
			FOR UPDATE OF credential
		`, digest.PublicIDHash[:], service.now()).Scan(
			&credentialID, &accountID, &accountClass, &tenantValue,
			&expectedSecret, &useCount, &useLimit,
		); err != nil {
			return identity.ErrInvalidCredentials
		}
		var expected [sha256.Size]byte
		if len(expectedSecret) != sha256.Size {
			verificationFailed = true
		} else {
			copy(expected[:], expectedSecret)
			verificationFailed = !identity.VerifyAPIKeySecret(expected, digest.SecretHash)
		}
		if useLimit != nil && useCount >= *useLimit {
			verificationFailed = true
		}
		resolved, actorErr := actorForStoredAccount(
			accountID, accountClass, tenantValue,
			identity.AuthenticationWorkload, identity.AssuranceSingleFactor,
		)
		if actorErr != nil {
			verificationFailed = true
		}
		actor = resolved
		if verificationFailed {
			tenantID := ""
			if tenantValue.Valid {
				tenantID = formatUUID(tenantValue.Bytes)
			}
			return service.insertSecurityAuditForTenant(
				ctx, tx, "identity.api-key.authentication-denied.v1", "denied",
				formatUUID(accountID), "", tenantID, requestID, correlationID, `{}`,
			)
		}
		command, err := tx.Exec(ctx, `
			UPDATE werk_core.account_credentials
			SET use_count = use_count + 1, last_used_at = $2
			WHERE id = $1::uuid AND status = 'active'
			  AND (use_limit IS NULL OR use_count < use_limit)
		`, credentialID, service.now())
		if err != nil || command.RowsAffected() != 1 {
			return identity.ErrInvalidCredentials
		}
		tenantID := ""
		if actor.TenantID != nil {
			tenantID = formatUUID(*actor.TenantID)
		}
		return service.insertSecurityAuditForTenant(
			ctx, tx, "identity.api-key.authentication-succeeded.v1", "succeeded",
			formatUUID(actor.AccountID), "", tenantID, requestID, correlationID,
			`{"authentication_kind":"workload","audience":"service"}`,
		)
	})
	if err != nil || verificationFailed {
		return identity.AuthenticatedActor{}, identity.ErrInvalidCredentials
	}
	return actor, nil
}

// APIKeyCredentialInput is consumed by an already-authorized provisioning
// command. It contains no plaintext token.
type APIKeyCredentialInput struct {
	AccountID    identity.AccountID
	DisplayName  string
	PublicIDHash [sha256.Size]byte
	SecretHash   [sha256.Size]byte
	ExpiresAt    *time.Time
	UseLimit     *int64
}

func (input APIKeyCredentialInput) Validate(now time.Time) error {
	if input.AccountID.IsZero() || strings.TrimSpace(input.DisplayName) == "" || strings.TrimSpace(input.DisplayName) != input.DisplayName || len(input.DisplayName) > 120 ||
		input.PublicIDHash == ([sha256.Size]byte{}) || input.SecretHash == ([sha256.Size]byte{}) ||
		(input.ExpiresAt != nil && !now.Before(*input.ExpiresAt)) ||
		(input.UseLimit != nil && *input.UseLimit <= 0) {
		return identity.ErrInvalidCredentials
	}
	return nil
}
