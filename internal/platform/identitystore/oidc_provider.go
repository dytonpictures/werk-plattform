package identitystore

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/providerregistry"
	"github.com/dytonpictures/werk/internal/core/securematerial"
	"github.com/dytonpictures/werk/internal/platform/database"
	"github.com/dytonpictures/werk/internal/platform/identityprotocol/oidc"
	"github.com/dytonpictures/werk/internal/platform/providerregistrystore"
)

type OIDCLoginProvider struct {
	Binding       identity.FederatedLoginProviderBinding
	Resolution    providerregistry.Resolution
	Configuration oidc.Configuration
	Secret        *securematerial.MaterialRef
}

// ResolveOIDCLoginProvider loads one explicitly selected Identity provider,
// then freshly resolves its exact Registry tuple in the same PostgreSQL
// snapshot. It never selects a default provider.
func (service *Service) ResolveOIDCLoginProvider(ctx context.Context, providerKey string) (OIDCLoginProvider, error) {
	if service == nil || !stableIdentityProviderKey(providerKey) {
		return OIDCLoginProvider{}, oidc.ErrLoginUnavailable
	}
	var result OIDCLoginProvider
	err := service.database.WithinRead(ctx, func(ctx context.Context, tx database.TenantTx) error {
		var configurationID pgtype.UUID
		var registryProviderID pgtype.UUID
		var providerKind, capabilityKey, status string
		var registryVersion, serviceVersion, capabilityVersion, configurationRevision, bindingRevision int64
		var authentication string
		var secretHandle pgtype.Text
		var secretVersion pgtype.Int8
		err := tx.QueryRow(ctx, `
			SELECT binding.provider_kind,binding.registry_provider_id,binding.registry_contract_version,
			       binding.service_contract_version,binding.capability_key,binding.capability_version,
			       binding.configuration_id,binding.configuration_revision,binding.status,binding.revision,
			       configuration.issuer_uri,configuration.client_id,configuration.redirect_uri,
			       configuration.audience,configuration.client_authentication,configuration.secret_material_handle,
			       configuration.secret_material_version
			FROM werk_core.identity_federated_login_provider_bindings AS binding
			JOIN werk_core.identity_oidc_client_configurations AS configuration
			  ON configuration.id=binding.configuration_id AND configuration.revision=binding.configuration_revision
			JOIN werk_core.identity_providers AS provider
			  ON provider.provider_key=binding.identity_provider_key
			WHERE binding.identity_provider_key=$1 AND binding.provider_kind='oidc'
			  AND binding.status='active' AND provider.provider_kind='oidc' AND provider.status='active'
		`, providerKey).Scan(&providerKind, &registryProviderID, &registryVersion, &serviceVersion,
			&capabilityKey, &capabilityVersion, &configurationID, &configurationRevision, &status,
			&bindingRevision, &result.Configuration.IssuerURL, &result.Configuration.ClientID,
			&result.Configuration.RedirectURI, &result.Configuration.Audience, &authentication, &secretHandle, &secretVersion)
		if err != nil || !registryProviderID.Valid || !configurationID.Valid || registryVersion < 1 ||
			serviceVersion < 1 || capabilityVersion < 1 || configurationRevision < 1 || bindingRevision < 1 {
			return oidc.ErrLoginUnavailable
		}
		result.Configuration.Revision = uint64(configurationRevision)
		result.Configuration.IdentityProviderKey = providerKey
		result.Configuration.ClientAuthentication = oidc.ClientAuthentication(authentication)
		result.Binding = identity.FederatedLoginProviderBinding{
			IdentityProviderKey: providerKey, ProviderKind: identity.ExternalProviderKind(providerKind),
			RegistryProviderID:      providerregistry.ProviderID(registryProviderID.Bytes),
			RegistryContractVersion: uint64(registryVersion), ServiceContractVersion: uint64(serviceVersion),
			CapabilityKey: capabilityKey, CapabilityVersion: uint64(capabilityVersion),
			Configuration: identity.ProviderConfigurationRef{ID: identity.ProviderConfigurationID(configurationID.Bytes), Kind: identity.ProviderConfigurationOIDCClient, Version: uint64(configurationRevision)},
			Status:        identity.ExternalProviderBindingStatus(status), Revision: uint64(bindingRevision),
		}
		if result.Binding.Validate() != nil {
			return oidc.ErrLoginUnavailable
		}
		request := providerregistry.ResolveRequest{
			ProviderID: result.Binding.RegistryProviderID, RegistryContractVersion: result.Binding.RegistryContractVersion,
			ServiceKey: providerregistry.IdentityLoginFederationServiceKey, ServiceVersion: result.Binding.ServiceContractVersion,
			CapabilityKey: result.Binding.CapabilityKey, CapabilityVersion: result.Binding.CapabilityVersion,
			Boundary: providerregistry.OperationBoundaryInstallation,
		}
		resolution, err := providerregistrystore.Resolve(ctx, tx, request)
		if err != nil || resolution.AdapterKey != providerregistry.IdentityOIDCLoginAdapterV1 {
			return oidc.ErrLoginUnavailable
		}
		result.Resolution = resolution
		if result.Configuration.ClientAuthentication == oidc.ClientAuthenticationSecretBasic {
			if !secretHandle.Valid || !secretVersion.Valid || secretVersion.Int64 < 1 {
				return oidc.ErrLoginUnavailable
			}
			material, err := securematerial.NewMaterialRef(secretHandle.String, uint64(secretVersion.Int64))
			if err != nil {
				return oidc.ErrLoginUnavailable
			}
			result.Secret = &material
		} else if result.Configuration.ClientAuthentication != oidc.ClientAuthenticationNone || secretHandle.Valid || secretVersion.Valid {
			return oidc.ErrLoginUnavailable
		}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) || err != nil {
		return OIDCLoginProvider{}, oidc.ErrLoginUnavailable
	}
	return result, nil
}

func stableIdentityProviderKey(value string) bool {
	if value == "" || len(value) > 120 {
		return false
	}
	for index, character := range value {
		if (character >= 'a' && character <= 'z') || (index > 0 && character >= '0' && character <= '9') || (index > 0 && (character == '-' || character == '.')) {
			continue
		}
		return false
	}
	return true
}
