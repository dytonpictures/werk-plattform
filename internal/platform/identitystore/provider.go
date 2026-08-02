package identitystore

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/tenancy"
	"github.com/dytonpictures/werk/internal/platform/database"
)

const localProviderKey = "local"

// requireActiveProviderBinding is the shared runtime gate for credentials that
// Core Identity verifies itself. The provider is selected from trusted stored
// credential metadata (or the fixed local passkey provider), never from login
// request data. Provider and binding lifecycle are checked together so either
// side can disable authentication without changing account, tenant or audience.
func requireActiveProviderBinding(ctx context.Context, tx database.TenantTx, accountID, providerKey string, method identity.AuthenticationMethod) error {
	if tx == nil || strings.TrimSpace(accountID) == "" || strings.TrimSpace(providerKey) != providerKey || providerKey == "" {
		return identity.ErrInvalidCredentials
	}
	var providerKind string
	var providerStatus string
	err := tx.QueryRow(ctx, `
		SELECT provider.provider_kind, provider.status
		FROM werk_core.identity_providers AS provider
		JOIN werk_core.account_identity_bindings AS binding
		  ON binding.provider_key = provider.provider_key
		 AND binding.account_id = $1::uuid
		 AND binding.status = 'active'
		WHERE provider.provider_key = $2
		  AND ($2 <> 'local' OR binding.provider_subject = $1::uuid::text)
	`, accountID, providerKey).Scan(&providerKind, &providerStatus)
	if err != nil || !providerBindingAllowsAuthentication(providerKind, providerStatus, true, method) {
		return identity.ErrInvalidCredentials
	}
	return nil
}

// CompleteFederatedLogin performs subject resolution, lifecycle locking,
// session issuance and audit in one transaction. The expected audience comes
// from the server-owned login surface, never from provider claims.
func (service *Service) CompleteFederatedLogin(ctx context.Context, proof identity.VerifiedIdentity, expectedAudience identity.Audience, requestID, correlationID string) (identity.LoginResult, error) {
	now := service.now()
	if proof.Validate() != nil || (proof.Method != identity.AuthenticationMethodOIDC && proof.Method != identity.AuthenticationMethodSAML) ||
		proof.AuthenticatedAt.After(now.Add(time.Minute)) || proof.AuthenticatedAt.Before(now.Add(-10*time.Minute)) ||
		(expectedAudience != identity.AudienceWork && expectedAudience != identity.AudienceAdmin) {
		return identity.LoginResult{}, identity.ErrInvalidCredentials
	}
	token, tokenHash, err := newSessionToken()
	if err != nil {
		return identity.LoginResult{}, identity.ErrInvalidCredentials
	}
	sessionID, err := randomUUID()
	if err != nil {
		return identity.LoginResult{}, identity.ErrInvalidCredentials
	}
	expiresAt := now.Add(service.sessionLifetime(expectedAudience))
	redirect := "/app"
	err = service.database.WithinWrite(ctx, func(ctx context.Context, tx database.TenantTx) error {
		var mappedAccountID string
		if err := tx.QueryRow(ctx, `SELECT account_id::text FROM werk_core.account_identity_bindings WHERE provider_key=$1 AND provider_subject=$2`, proof.ProviderKey, proof.ProviderSubject).Scan(&mappedAccountID); err != nil {
			return identity.ErrInvalidCredentials
		}
		var accountID [16]byte
		var accountClass string
		var tenantValue pgtype.UUID
		var generation int64
		var mustChangePassword bool
		if err := tx.QueryRow(ctx, `SELECT account.id,account.account_class,account.tenant_id,account.session_generation,account.must_change_password
			FROM werk_core.accounts AS account LEFT JOIN werk_core.tenants AS tenant ON tenant.id=account.tenant_id
			WHERE account.id=$1::uuid AND account.status='active' AND (account.tenant_id IS NULL OR tenant.status='active')
			FOR UPDATE OF account`, mappedAccountID).Scan(&accountID, &accountClass, &tenantValue, &generation, &mustChangePassword); err != nil {
			return identity.ErrInvalidCredentials
		}
		var bindingID, lockedAccountID, providerKind string
		if err := tx.QueryRow(ctx, `SELECT binding_id::text,account_id::text,provider_kind FROM werk_security.lock_verified_identity_binding($1,$2)`, proof.ProviderKey, proof.ProviderSubject).Scan(&bindingID, &lockedAccountID, &providerKind); err != nil || lockedAccountID != mappedAccountID || !providerAcceptsMethod(providerKind, proof.Method) {
			return identity.ErrInvalidCredentials
		}
		actor, err := actorForStoredAccount(accountID, accountClass, tenantValue, identity.AuthenticationInteractive, proof.Assurance)
		if err != nil || actor.Audience != expectedAudience {
			return identity.ErrInvalidCredentials
		}
		command, err := tx.Exec(ctx, `INSERT INTO werk_core.sessions
			(id,account_id,token_hash,audience,tenant_id,expires_at,authentication_assurance,authentication_kind,session_generation)
			VALUES ($1::uuid,$2::uuid,$3,$4,$5::uuid,$6,$7,'interactive',$8)`, sessionID, mappedAccountID, tokenHash[:], actor.Audience, nullableTenant(actor.TenantID), expiresAt, proof.Assurance, generation)
		if err != nil || command.RowsAffected() != 1 {
			return identity.ErrInvalidCredentials
		}
		command, err = tx.Exec(ctx, `UPDATE werk_core.account_identity_bindings SET last_authenticated_at=$2 WHERE id=$1::uuid AND status='active'`, bindingID, proof.AuthenticatedAt)
		if err != nil || command.RowsAffected() != 1 {
			return identity.ErrInvalidCredentials
		}
		tenantID := ""
		if actor.TenantID != nil {
			tenantID = formatUUID(*actor.TenantID)
		}
		if mustChangePassword {
			redirect = "/change-password"
		} else if actor.AccountClass == identity.AccountClassAdmin {
			redirect = "/admin"
		}
		return service.insertSecurityAuditForTenant(ctx, tx, "identity.login.succeeded.v1", "succeeded", mappedAccountID, sessionID, tenantID, requestID, correlationID, `{"authentication_method":"`+string(proof.Method)+`","audience":"`+string(actor.Audience)+`"}`)
	})
	if err != nil {
		return identity.LoginResult{}, identity.ErrInvalidCredentials
	}
	return identity.LoginResult{SessionToken: token, Redirect: redirect}, nil
}

// lockActiveProviderBinding repeats the eligibility decision in the final
// write transaction and holds the exact provider/binding lifecycle rows until
// it commits. Provider disablement and session issuance are therefore ordered,
// rather than racing between a check and a new security artifact.
func lockActiveProviderBinding(ctx context.Context, tx database.TenantTx, accountID, providerKey string, method identity.AuthenticationMethod) error {
	if tx == nil || strings.TrimSpace(accountID) == "" || strings.TrimSpace(providerKey) != providerKey || providerKey == "" {
		return identity.ErrInvalidCredentials
	}
	var providerKind string
	if err := tx.QueryRow(ctx, `
		SELECT provider_kind
		FROM werk_security.lock_active_identity_provider_binding($1::uuid, $2)
	`, accountID, providerKey).Scan(&providerKind); err != nil || !providerAcceptsMethod(providerKind, method) {
		return identity.ErrInvalidCredentials
	}
	return nil
}

func providerBindingAllowsAuthentication(providerKind, providerStatus string, activeBinding bool, method identity.AuthenticationMethod) bool {
	return providerStatus == "active" && activeBinding && providerAcceptsMethod(providerKind, method)
}

// ResolveVerifiedIdentity is the Core-owned half of an external-provider flow.
// The provider proof contains no account class, tenant, audience or grants.
func (service *Service) ResolveVerifiedIdentity(ctx context.Context, proof identity.VerifiedIdentity) (identity.AuthenticatedActor, error) {
	now := service.now()
	if proof.Validate() != nil || proof.AuthenticatedAt.After(now.Add(time.Minute)) || proof.AuthenticatedAt.Before(now.Add(-10*time.Minute)) {
		return identity.AuthenticatedActor{}, identity.ErrInvalidCredentials
	}
	authenticationKind, ok := authenticationKindForMethod(proof.Method)
	if !ok {
		return identity.AuthenticatedActor{}, identity.ErrInvalidCredentials
	}
	var accountID [16]byte
	var accountClass string
	var providerKind string
	var tenantValue pgtype.UUID
	var actor identity.AuthenticatedActor
	err := service.database.WithinWrite(ctx, func(ctx context.Context, tx database.TenantTx) error {
		var mappedAccountID string
		if err := tx.QueryRow(ctx, `
			SELECT account_id::text
			FROM werk_core.account_identity_bindings
			WHERE provider_key = $1 AND provider_subject = $2
		`, proof.ProviderKey, proof.ProviderSubject).Scan(&mappedAccountID); err != nil {
			return identity.ErrInvalidCredentials
		}
		if err := tx.QueryRow(ctx, `
			SELECT account.id, account.account_class, account.tenant_id
			FROM werk_core.accounts AS account
			LEFT JOIN werk_core.tenants AS tenant ON tenant.id = account.tenant_id
			LEFT JOIN werk_core.identity_agents AS agent ON agent.id = account.agent_subject_id
			WHERE account.id = $1::uuid AND account.status = 'active'
			  AND (account.tenant_id IS NULL OR tenant.status = 'active')
			  AND (account.account_class <> 'agent' OR agent.status = 'active')
			FOR UPDATE OF account
		`, mappedAccountID).Scan(&accountID, &accountClass, &tenantValue); err != nil {
			return identity.ErrInvalidCredentials
		}
		var bindingID string
		var lockedAccountID string
		if err := tx.QueryRow(ctx, `
			SELECT binding_id::text, account_id::text, provider_kind
			FROM werk_security.lock_verified_identity_binding($1, $2)
		`, proof.ProviderKey, proof.ProviderSubject).Scan(
			&bindingID, &lockedAccountID, &providerKind,
		); err != nil || lockedAccountID != mappedAccountID {
			return identity.ErrInvalidCredentials
		}
		if !providerAcceptsMethod(providerKind, proof.Method) {
			return identity.ErrInvalidCredentials
		}
		resolved, err := actorForStoredAccount(accountID, accountClass, tenantValue, authenticationKind, proof.Assurance)
		if err != nil {
			return err
		}
		actor = resolved
		command, err := tx.Exec(ctx, `
			UPDATE werk_core.account_identity_bindings
			SET last_authenticated_at = $2
			WHERE id = $1::uuid AND status = 'active'
		`, bindingID, proof.AuthenticatedAt)
		if err != nil || command.RowsAffected() != 1 {
			return identity.ErrInvalidCredentials
		}
		return nil
	})
	if err != nil {
		return identity.AuthenticatedActor{}, identity.ErrInvalidCredentials
	}
	return actor, nil
}

func providerAcceptsMethod(providerKind string, method identity.AuthenticationMethod) bool {
	switch providerKind {
	case "local":
		return method == identity.AuthenticationMethodPassword || method == identity.AuthenticationMethodPasskey || method == identity.AuthenticationMethodAPIKey
	case "oidc":
		return method == identity.AuthenticationMethodOIDC
	case "saml":
		return method == identity.AuthenticationMethodSAML
	case "ldap":
		return method == identity.AuthenticationMethodLDAPPassword
	default:
		return false
	}
}

func authenticationKindForMethod(method identity.AuthenticationMethod) (identity.AuthenticationKind, bool) {
	switch method {
	case identity.AuthenticationMethodPassword, identity.AuthenticationMethodPasskey,
		identity.AuthenticationMethodOIDC, identity.AuthenticationMethodSAML,
		identity.AuthenticationMethodLDAPPassword:
		return identity.AuthenticationInteractive, true
	case identity.AuthenticationMethodAPIKey:
		return identity.AuthenticationWorkload, true
	default:
		return "", false
	}
}

func actorForStoredAccount(accountID [16]byte, accountClass string, tenantValue pgtype.UUID, authenticationKind identity.AuthenticationKind, assurance identity.AuthenticationAssurance) (identity.AuthenticatedActor, error) {
	actor := identity.AuthenticatedActor{
		AccountID: identity.AccountID(accountID), AccountClass: identity.AccountClass(accountClass),
		Kind: authenticationKind, Assurance: assurance,
	}
	if tenantValue.Valid {
		tenantID := tenancy.TenantID(tenantValue.Bytes)
		actor.TenantID = &tenantID
	}
	switch actor.AccountClass {
	case identity.AccountClassWork:
		actor.Audience = identity.AudienceWork
	case identity.AccountClassAdmin:
		actor.Audience = identity.AudienceAdmin
	case identity.AccountClassService, identity.AccountClassAgent:
		actor.Audience = identity.AudienceService
	default:
		return identity.AuthenticatedActor{}, identity.ErrInvalidCredentials
	}
	if strings.TrimSpace(accountClass) != accountClass || identity.ValidateActorBoundary(actor) != nil {
		return identity.AuthenticatedActor{}, identity.ErrInvalidCredentials
	}
	return actor, nil
}
