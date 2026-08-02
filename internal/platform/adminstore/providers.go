package adminstore

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/platform/database"
)

type IdentityProviderView struct {
	ProviderKey         string `json:"provider_key"`
	ProviderKind        string `json:"provider_kind"`
	DisplayName         string `json:"display_name"`
	Issuer              string `json:"issuer,omitempty"`
	Status              string `json:"status"`
	SignInState         string `json:"sign_in_state"`
	ConfigurationSource string `json:"configuration_source"`
}

type LocalIdentityConfigurationView struct {
	ConfigurationSource string   `json:"configuration_source"`
	PasswordEnabled     bool     `json:"password_enabled"`
	PasskeyEnabled      bool     `json:"passkey_enabled"`
	MFAEnabled          bool     `json:"mfa_enabled"`
	WebAuthnRPID        string   `json:"webauthn_rp_id"`
	WebAuthnRPName      string   `json:"webauthn_rp_name"`
	AllowedOriginCount  int      `json:"allowed_origin_count"`
	Methods             []string `json:"methods"`
	SecretsExposed      bool     `json:"secrets_exposed"`
}

type IdentityProviderAdapterView struct {
	ProviderKind string `json:"provider_kind"`
	Implemented  bool   `json:"implemented"`
	State        string `json:"state"`
}

type IdentityProviderManagementView struct {
	ExternalConfigurationAvailable bool   `json:"external_configuration_available"`
	Reason                         string `json:"reason"`
}

type IdentityProviderCatalog struct {
	ObservedAt time.Time                      `json:"observed_at"`
	Local      LocalIdentityConfigurationView `json:"local"`
	Items      []IdentityProviderView         `json:"items"`
	Truncated  bool                           `json:"truncated"`
	Adapters   []IdentityProviderAdapterView  `json:"adapters"`
	Management IdentityProviderManagementView `json:"management"`
}

const maximumIdentityProviders = 100

func (service *Service) ListIdentityProviders(
	ctx context.Context,
	actor identity.AuthenticatedActor,
	requestID string,
	correlationID string,
) (IdentityProviderCatalog, error) {
	if service == nil || service.database == nil || !service.runtimeConfigured {
		return IdentityProviderCatalog{}, errors.New("identity provider catalog is not configured")
	}
	auditID, err := randomUUID()
	if err != nil {
		return IdentityProviderCatalog{}, err
	}
	catalog := IdentityProviderCatalog{
		Items: make([]IdentityProviderView, 0),
		Local: localIdentityConfiguration(service.runtime),
		Adapters: []IdentityProviderAdapterView{
			{ProviderKind: "oidc", Implemented: true, State: "protocol-implemented"},
			{ProviderKind: "saml", Implemented: false, State: "adapter-unavailable"},
			{ProviderKind: "ldap", Implemented: false, State: "adapter-unavailable"},
		},
		Management: IdentityProviderManagementView{
			ExternalConfigurationAvailable: false,
			Reason:                         "external-provider-runtime-incomplete",
		},
	}
	err = service.database.WithinInstallationAuditRead(ctx, func(ctx context.Context, tx database.TenantTx) error {
		if err := tx.QueryRow(ctx, `SELECT statement_timestamp()`).Scan(&catalog.ObservedAt); err != nil {
			return err
		}
		catalog.ObservedAt = catalog.ObservedAt.UTC()
		rows, err := tx.Query(ctx, `
			SELECT provider_key, provider_kind, display_name, issuer, status
			FROM werk_core.identity_providers
			ORDER BY CASE WHEN provider_kind='local' THEN 0 ELSE 1 END,
			         display_name, provider_key
			LIMIT 101
		`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var provider IdentityProviderView
			var issuer pgtype.Text
			if err := rows.Scan(
				&provider.ProviderKey, &provider.ProviderKind, &provider.DisplayName,
				&issuer, &provider.Status,
			); err != nil {
				return err
			}
			provider.Issuer = textValue(issuer)
			provider.ConfigurationSource = "registry-metadata"
			provider.SignInState = providerSignInState(provider.ProviderKind, provider.Status)
			if provider.ProviderKind == "local" {
				provider.ConfigurationSource = "runtime-environment"
			}
			catalog.Items = append(catalog.Items, provider)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(catalog.Items) > maximumIdentityProviders {
			catalog.Items = catalog.Items[:maximumIdentityProviders]
			catalog.Truncated = true
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO werk_core.security_audit_events (
				id, occurred_at, event_type, outcome, account_id, tenant_id,
				request_id, correlation_id, details
			) VALUES (
				$1::uuid, $2, 'core.identity.providers-listed.v1', 'succeeded',
				$3::uuid, NULL, $4::uuid, $5::uuid,
				jsonb_build_object(
					'result_count', $6::integer,
					'truncated', $7::boolean
				)
			)
		`, auditID, service.now(), formatUUID(actor.AccountID), requestID, correlationID,
			len(catalog.Items), catalog.Truncated)
		return err
	})
	if err != nil {
		return IdentityProviderCatalog{}, err
	}
	return catalog, nil
}

func localIdentityConfiguration(runtime RuntimeConfiguration) LocalIdentityConfigurationView {
	passkeyEnabled := runtime.IdentityMFAEnabled && runtime.WebAuthnOriginCount > 0
	methods := []string{"password"}
	if passkeyEnabled {
		methods = append(methods, "passkey")
	}
	return LocalIdentityConfigurationView{
		ConfigurationSource: "runtime-environment",
		PasswordEnabled:     true,
		PasskeyEnabled:      passkeyEnabled,
		MFAEnabled:          runtime.IdentityMFAEnabled,
		WebAuthnRPID:        runtime.WebAuthnRPID,
		WebAuthnRPName:      runtime.WebAuthnRPName,
		AllowedOriginCount:  runtime.WebAuthnOriginCount,
		Methods:             methods,
		SecretsExposed:      false,
	}
}

func providerSignInState(providerKind, status string) string {
	if status != "active" {
		return "disabled"
	}
	if providerKind == "local" {
		return "available"
	}
	return "not-ready"
}
