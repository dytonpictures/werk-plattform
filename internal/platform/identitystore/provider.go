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
		var bindingID string
		if err := tx.QueryRow(ctx, `
			SELECT binding.id::text, provider.provider_kind,
			       account.id, account.account_class, account.tenant_id
			FROM werk_core.account_identity_bindings AS binding
			JOIN werk_core.identity_providers AS provider
			  ON provider.provider_key = binding.provider_key AND provider.status = 'active'
			JOIN werk_core.accounts AS account
			  ON account.id = binding.account_id AND account.status = 'active'
			LEFT JOIN werk_core.tenants AS tenant ON tenant.id = account.tenant_id
			LEFT JOIN werk_core.identity_agents AS agent ON agent.id = account.agent_subject_id
			WHERE binding.provider_key = $1 AND binding.provider_subject = $2
			  AND binding.status = 'active'
			  AND (account.tenant_id IS NULL OR tenant.status = 'active')
			  AND (account.account_class <> 'agent' OR agent.status = 'active')
			FOR UPDATE OF binding
		`, proof.ProviderKey, proof.ProviderSubject).Scan(&bindingID, &providerKind, &accountID, &accountClass, &tenantValue); err != nil {
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
