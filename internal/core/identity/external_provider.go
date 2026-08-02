package identity

import (
	"errors"

	"github.com/dytonpictures/werk/internal/core/providerregistry"
	"github.com/dytonpictures/werk/internal/core/tenancy"
)

var ErrInvalidExternalProviderBinding = errors.New("invalid external provider binding")

type ExternalProviderKind string

const (
	ExternalProviderOIDC ExternalProviderKind = "oidc"
	ExternalProviderSAML ExternalProviderKind = "saml"
	ExternalProviderLDAP ExternalProviderKind = "ldap"
)

type ProviderConfigurationKind string

const (
	ProviderConfigurationOIDCClient    ProviderConfigurationKind = "oidc-client"
	ProviderConfigurationSAMLService   ProviderConfigurationKind = "saml-service-provider"
	ProviderConfigurationLDAPDirectory ProviderConfigurationKind = "ldap-directory"
)

type ExternalProviderBindingStatus string

const (
	ExternalProviderBindingActive   ExternalProviderBindingStatus = "active"
	ExternalProviderBindingDisabled ExternalProviderBindingStatus = "disabled"
	ExternalProviderBindingRetired  ExternalProviderBindingStatus = "retired"
)

type ProviderConfigurationID [16]byte

// ProviderConfigurationRef addresses one immutable revision of a typed,
// Identity-owned configuration. It is not a secret handle and contains no
// free-form provider configuration.
type ProviderConfigurationRef struct {
	ID      ProviderConfigurationID
	Kind    ProviderConfigurationKind
	Version uint64
}

func (ref ProviderConfigurationRef) Validate() error {
	if ref.ID == (ProviderConfigurationID{}) || ref.Version == 0 {
		return ErrInvalidExternalProviderBinding
	}
	switch ref.Kind {
	case ProviderConfigurationOIDCClient, ProviderConfigurationSAMLService, ProviderConfigurationLDAPDirectory:
		return nil
	default:
		return ErrInvalidExternalProviderBinding
	}
}

// FederatedLoginProviderBinding couples one Core Identity login provider to
// one exact installation-bound login-federation capability. The global
// registry remains unaware of Identity subjects and configuration.
type FederatedLoginProviderBinding struct {
	IdentityProviderKey     string
	ProviderKind            ExternalProviderKind
	RegistryProviderID      providerregistry.ProviderID
	RegistryContractVersion uint64
	ServiceContractVersion  uint64
	CapabilityKey           string
	CapabilityVersion       uint64
	Configuration           ProviderConfigurationRef
	Status                  ExternalProviderBindingStatus
	Revision                uint64
}

func (binding FederatedLoginProviderBinding) Validate() error {
	if !stableIdentityKey(binding.IdentityProviderKey) || binding.RegistryProviderID.IsZero() ||
		binding.RegistryContractVersion != providerregistry.ContractVersionV1 ||
		binding.ServiceContractVersion != providerregistry.IdentityServiceContractVersionV1 ||
		binding.CapabilityVersion != providerregistry.IdentityCapabilityContractVersionV1 ||
		binding.Configuration.Validate() != nil || !validExternalProviderBindingStatus(binding.Status) ||
		binding.Revision == 0 {
		return ErrInvalidExternalProviderBinding
	}
	switch binding.ProviderKind {
	case ExternalProviderOIDC:
		if binding.CapabilityKey != providerregistry.IdentityOIDCLoginCapability ||
			binding.Configuration.Kind != ProviderConfigurationOIDCClient {
			return ErrInvalidExternalProviderBinding
		}
	case ExternalProviderSAML:
		if binding.CapabilityKey != providerregistry.IdentitySAMLLoginCapability ||
			binding.Configuration.Kind != ProviderConfigurationSAMLService {
			return ErrInvalidExternalProviderBinding
		}
	default:
		return ErrInvalidExternalProviderBinding
	}
	return nil
}

type DirectorySourceID [16]byte

// DirectorySourceBinding binds a separately configured directory source to
// exactly one tenant. Each operation must additionally resolve the concrete
// read capability it needs; this value is never an all-capabilities token.
type DirectorySourceBinding struct {
	ID                      DirectorySourceID
	TenantID                tenancy.TenantID
	RegistryProviderID      providerregistry.ProviderID
	RegistryContractVersion uint64
	ServiceContractVersion  uint64
	Configuration           ProviderConfigurationRef
	MappingContractVersion  uint64
	Status                  ExternalProviderBindingStatus
	Revision                uint64
}

func (binding DirectorySourceBinding) Validate() error {
	if binding.ID == (DirectorySourceID{}) || binding.TenantID.IsZero() ||
		binding.RegistryProviderID.IsZero() ||
		binding.RegistryContractVersion != providerregistry.ContractVersionV1 ||
		binding.ServiceContractVersion != providerregistry.IdentityServiceContractVersionV1 ||
		binding.Configuration.Validate() != nil ||
		binding.Configuration.Kind != ProviderConfigurationLDAPDirectory ||
		binding.MappingContractVersion == 0 ||
		!validExternalProviderBindingStatus(binding.Status) || binding.Revision == 0 {
		return ErrInvalidExternalProviderBinding
	}
	return nil
}

func validExternalProviderBindingStatus(status ExternalProviderBindingStatus) bool {
	return status == ExternalProviderBindingActive || status == ExternalProviderBindingDisabled || status == ExternalProviderBindingRetired
}
