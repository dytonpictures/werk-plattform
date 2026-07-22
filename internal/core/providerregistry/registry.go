// Package providerregistry defines the provider-neutral contracts used to
// bind an explicitly selected provider to a versioned platform capability.
// It deliberately contains no provider configuration, secrets, endpoints or
// health state.
package providerregistry

import (
	"errors"
	"strings"

	"github.com/dytonpictures/werk/internal/core/resource"
	"github.com/dytonpictures/werk/internal/core/tenancy"
)

var (
	// ErrInvalid indicates that a registry value violates the structural
	// contract. Persisted values may be disabled or retired and still valid.
	ErrInvalid = errors.New("invalid provider registry contract")
	// ErrUnresolved is returned for every failed resolution. Callers must not
	// infer a usable provider from a partially matching registry entry.
	ErrUnresolved = errors.New("provider could not be resolved")
)

// ContractVersionV1 is the only registry contract this package can resolve.
// PostgreSQL may retain metadata for other versions during an upgrade, but an
// older runtime must fail closed instead of interpreting it optimistically.
const ContractVersionV1 uint64 = 1

// ProviderID is the opaque persistent identity of a provider registration.
type ProviderID [16]byte

func (id ProviderID) IsZero() bool {
	return id == ProviderID{}
}

// Lifecycle is shared by all registry entries. Disabled entries may be
// re-enabled; retired entries remain addressable for history but not use.
type Lifecycle string

const (
	LifecycleActive   Lifecycle = "active"
	LifecycleDisabled Lifecycle = "disabled"
	LifecycleRetired  Lifecycle = "retired"
)

func (lifecycle Lifecycle) Valid() bool {
	return lifecycle == LifecycleActive || lifecycle == LifecycleDisabled || lifecycle == LifecycleRetired
}

// ConfigScope describes ownership of a provider's configuration. It is kept
// separate from the boundary of any individual operation.
type ConfigScope string

const (
	ConfigScopeInstallation ConfigScope = "installation"
	ConfigScopeTenant       ConfigScope = "tenant"
)

func (scope ConfigScope) Valid() bool {
	return scope == ConfigScopeInstallation || scope == ConfigScopeTenant
}

// OperationBoundary describes the data boundary of a single resolution
// request; it does not imply where provider configuration is stored.
type OperationBoundary string

const (
	OperationBoundaryInstallation OperationBoundary = "installation"
	OperationBoundaryTenant       OperationBoundary = "tenant"
)

func (boundary OperationBoundary) Valid() bool {
	return boundary == OperationBoundaryInstallation || boundary == OperationBoundaryTenant
}

type ServiceContract struct {
	OwnerModule string
	ServiceKey  string
	Version     uint64
	Lifecycle   Lifecycle
}

func (contract ServiceContract) Validate() error {
	prefix := contract.OwnerModule + ".service."
	if !resource.ValidKey(contract.OwnerModule) || !resource.ValidKey(contract.ServiceKey) ||
		!strings.HasPrefix(contract.ServiceKey, prefix) || len(contract.ServiceKey) == len(prefix) ||
		contract.Version == 0 || !contract.Lifecycle.Valid() {
		return ErrInvalid
	}
	return nil
}

type CapabilityContract struct {
	ServiceKey        string
	ServiceVersion    uint64
	CapabilityKey     string
	Version           uint64
	OperationBoundary OperationBoundary
	Lifecycle         Lifecycle
}

func (contract CapabilityContract) Validate() error {
	prefix := contract.ServiceKey + ".capability."
	if !resource.ValidKey(contract.ServiceKey) || !resource.ValidKey(contract.CapabilityKey) ||
		!strings.HasPrefix(contract.CapabilityKey, prefix) || len(contract.CapabilityKey) == len(prefix) ||
		contract.ServiceVersion == 0 || contract.Version == 0 || !contract.OperationBoundary.Valid() ||
		!contract.Lifecycle.Valid() {
		return ErrInvalid
	}
	return nil
}

// ProviderRegistration selects an adapter without containing its
// configuration. A tenant-scoped registration owns configuration for exactly
// one tenant; installation-scoped registrations have no TenantID.
type ProviderRegistration struct {
	ID                      ProviderID
	ServiceKey              string
	ServiceVersion          uint64
	ProviderKey             string
	AdapterKey              string
	ConfigScope             ConfigScope
	TenantID                *tenancy.TenantID
	Lifecycle               Lifecycle
	Revision                uint64
	RegistryContractVersion uint64
}

func (registration ProviderRegistration) Validate() error {
	prefix := registration.ServiceKey + ".provider."
	if registration.ID.IsZero() || !resource.ValidKey(registration.ServiceKey) ||
		!resource.ValidKey(registration.ProviderKey) ||
		!strings.HasPrefix(registration.ProviderKey, prefix) || len(registration.ProviderKey) == len(prefix) ||
		!resource.ValidKey(registration.AdapterKey) || registration.ServiceVersion == 0 ||
		registration.Revision == 0 || registration.RegistryContractVersion != ContractVersionV1 ||
		!registration.ConfigScope.Valid() || !registration.Lifecycle.Valid() {
		return ErrInvalid
	}
	switch registration.ConfigScope {
	case ConfigScopeInstallation:
		if registration.TenantID != nil {
			return ErrInvalid
		}
	case ConfigScopeTenant:
		if registration.TenantID == nil || registration.TenantID.IsZero() {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

// ProviderCapabilityBinding explicitly permits one provider registration to
// implement one exact service/capability contract pair.
type ProviderCapabilityBinding struct {
	ProviderID        ProviderID
	ServiceKey        string
	ServiceVersion    uint64
	CapabilityKey     string
	CapabilityVersion uint64
	Lifecycle         Lifecycle
	Revision          uint64
}

func (binding ProviderCapabilityBinding) Validate() error {
	prefix := binding.ServiceKey + ".capability."
	if binding.ProviderID.IsZero() || !resource.ValidKey(binding.ServiceKey) ||
		!resource.ValidKey(binding.CapabilityKey) || !strings.HasPrefix(binding.CapabilityKey, prefix) ||
		len(binding.CapabilityKey) == len(prefix) || binding.ServiceVersion == 0 ||
		binding.CapabilityVersion == 0 || binding.Revision == 0 || !binding.Lifecycle.Valid() {
		return ErrInvalid
	}
	return nil
}

// ResolveRequest always names a provider. The registry never chooses a
// default, first or otherwise preferred registration.
type ResolveRequest struct {
	ProviderID              ProviderID
	RegistryContractVersion uint64
	ServiceKey              string
	ServiceVersion          uint64
	CapabilityKey           string
	CapabilityVersion       uint64
	Boundary                OperationBoundary
	TenantID                *tenancy.TenantID
}

func (request ResolveRequest) Validate() error {
	prefix := request.ServiceKey + ".capability."
	if request.ProviderID.IsZero() || !resource.ValidKey(request.ServiceKey) ||
		!resource.ValidKey(request.CapabilityKey) || !strings.HasPrefix(request.CapabilityKey, prefix) ||
		len(request.CapabilityKey) == len(prefix) || request.RegistryContractVersion != ContractVersionV1 ||
		request.ServiceVersion == 0 ||
		request.CapabilityVersion == 0 || !request.Boundary.Valid() {
		return ErrInvalid
	}
	switch request.Boundary {
	case OperationBoundaryInstallation:
		if request.TenantID != nil {
			return ErrInvalid
		}
	case OperationBoundaryTenant:
		if request.TenantID == nil || request.TenantID.IsZero() {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

// Resolution is the immutable, operation-bound result of a successful lookup.
// Zero tenant IDs are meaningful only together with their installation scope or
// boundary. Keeping both tenant coordinates prevents an installation-scoped
// provider from erasing the tenant boundary of the operation it serves.
type Resolution struct {
	ProviderID              ProviderID
	ProviderKey             string
	AdapterKey              string
	ProviderConfigScope     ConfigScope
	ProviderTenantID        tenancy.TenantID
	RegistryContractVersion uint64
	ServiceKey              string
	ServiceVersion          uint64
	CapabilityKey           string
	CapabilityVersion       uint64
	OperationBoundary       OperationBoundary
	OperationTenantID       tenancy.TenantID
}

// Resolve validates one explicit registration and binding against exact
// contract versions. Every malformed, inactive or mismatched input fails
// closed with ErrUnresolved.
func Resolve(
	request ResolveRequest,
	service ServiceContract,
	capability CapabilityContract,
	provider ProviderRegistration,
	binding ProviderCapabilityBinding,
) (Resolution, error) {
	if request.Validate() != nil || service.Validate() != nil || capability.Validate() != nil ||
		provider.Validate() != nil || binding.Validate() != nil {
		return Resolution{}, ErrUnresolved
	}
	if service.Lifecycle != LifecycleActive || capability.Lifecycle != LifecycleActive ||
		provider.Lifecycle != LifecycleActive || binding.Lifecycle != LifecycleActive {
		return Resolution{}, ErrUnresolved
	}
	if request.ServiceKey != service.ServiceKey || request.ServiceVersion != service.Version ||
		capability.ServiceKey != service.ServiceKey || capability.ServiceVersion != service.Version ||
		request.CapabilityKey != capability.CapabilityKey || request.CapabilityVersion != capability.Version ||
		request.Boundary != capability.OperationBoundary ||
		provider.ID != request.ProviderID || provider.ServiceKey != service.ServiceKey ||
		provider.ServiceVersion != service.Version ||
		provider.RegistryContractVersion != request.RegistryContractVersion ||
		binding.ProviderID != provider.ID ||
		binding.ServiceKey != service.ServiceKey || binding.ServiceVersion != service.Version ||
		binding.CapabilityKey != capability.CapabilityKey || binding.CapabilityVersion != capability.Version {
		return Resolution{}, ErrUnresolved
	}
	if !providerSupportsBoundary(provider, request) {
		return Resolution{}, ErrUnresolved
	}
	resolution := Resolution{
		ProviderID:              provider.ID,
		ProviderKey:             provider.ProviderKey,
		AdapterKey:              provider.AdapterKey,
		ProviderConfigScope:     provider.ConfigScope,
		RegistryContractVersion: provider.RegistryContractVersion,
		ServiceKey:              service.ServiceKey,
		ServiceVersion:          service.Version,
		CapabilityKey:           capability.CapabilityKey,
		CapabilityVersion:       capability.Version,
		OperationBoundary:       request.Boundary,
	}
	if provider.TenantID != nil {
		resolution.ProviderTenantID = *provider.TenantID
	}
	if request.TenantID != nil {
		resolution.OperationTenantID = *request.TenantID
	}
	return resolution, nil
}

func providerSupportsBoundary(provider ProviderRegistration, request ResolveRequest) bool {
	switch request.Boundary {
	case OperationBoundaryInstallation:
		return provider.ConfigScope == ConfigScopeInstallation
	case OperationBoundaryTenant:
		if provider.ConfigScope == ConfigScopeInstallation {
			return true
		}
		return provider.ConfigScope == ConfigScopeTenant && provider.TenantID != nil &&
			request.TenantID != nil && *provider.TenantID == *request.TenantID
	default:
		return false
	}
}
