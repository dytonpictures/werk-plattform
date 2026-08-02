// Package providerregistrystore resolves one explicitly selected platform
// provider from the authoritative PostgreSQL registry metadata.
package providerregistrystore

import (
	"context"
	"math"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/dytonpictures/werk/internal/core/providerregistry"
	"github.com/dytonpictures/werk/internal/core/tenancy"
	"github.com/dytonpictures/werk/internal/platform/database"
)

// The security-definer function returns one row containing the four exact
// registry records in this order:
//
//   - service_owner_module text, service_key text,
//     service_contract_version bigint, service_lifecycle text
//   - capability_service_key text, capability_service_contract_version bigint,
//     capability_key text, capability_version bigint,
//     capability_operation_boundary text, capability_lifecycle text
//   - provider_id uuid, provider_service_key text,
//     provider_service_contract_version bigint, provider_key text,
//     adapter_key text, provider_config_scope text, provider_tenant_id uuid,
//     provider_lifecycle text, provider_revision bigint,
//     provider_registry_contract_version bigint
//   - binding_provider_id uuid, binding_service_key text,
//     binding_service_contract_version bigint, binding_capability_key text,
//     binding_capability_version bigint, binding_lifecycle text,
//     binding_revision bigint
//
// provider_tenant_id is the only nullable result. The runtime receives no
// direct SELECT authority on the underlying registry tables.
const resolveQuery = `
	SELECT *
	FROM werk_security.resolve_platform_provider_registry(
		$1::uuid,
		$2::bigint,
		$3::text,
		$4::bigint,
		$5::text,
		$6::bigint,
		$7::text,
		$8::uuid
	)
`

type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// Resolve loads the exact service, capability, provider registration and
// capability binding named by request through the narrow database function.
// It never chooses a provider and never returns partially mapped registry
// data. Every invalid request, absent function or row, scan failure, malformed
// persisted value or final contract mismatch fails closed as ErrUnresolved.
func Resolve(
	ctx context.Context,
	tx database.TenantTx,
	request providerregistry.ResolveRequest,
) (providerregistry.Resolution, error) {
	if tx == nil {
		return providerregistry.Resolution{}, providerregistry.ErrUnresolved
	}
	return resolve(ctx, tx, request)
}

func resolve(
	ctx context.Context,
	rows rowQuerier,
	request providerregistry.ResolveRequest,
) (providerregistry.Resolution, error) {
	serviceVersion, serviceVersionOK := postgresVersion(request.ServiceVersion)
	capabilityVersion, capabilityVersionOK := postgresVersion(request.CapabilityVersion)
	registryContractVersion, registryVersionOK := postgresVersion(request.RegistryContractVersion)
	if rows == nil || ctx == nil || request.Validate() != nil ||
		!serviceVersionOK || !capabilityVersionOK || !registryVersionOK {
		return providerregistry.Resolution{}, providerregistry.ErrUnresolved
	}

	providerID := pgtype.UUID{Bytes: [16]byte(request.ProviderID), Valid: true}
	operationTenantID := pgtype.UUID{}
	if request.TenantID != nil {
		operationTenantID = pgtype.UUID{Bytes: [16]byte(*request.TenantID), Valid: true}
	}

	service, capability, provider, binding, ok := scanRegistryTuple(rows.QueryRow(
		ctx,
		resolveQuery,
		providerID,
		registryContractVersion,
		request.ServiceKey,
		serviceVersion,
		request.CapabilityKey,
		capabilityVersion,
		string(request.Boundary),
		operationTenantID,
	))
	if !ok {
		return providerregistry.Resolution{}, providerregistry.ErrUnresolved
	}

	resolution, err := providerregistry.Resolve(request, service, capability, provider, binding)
	if err != nil {
		return providerregistry.Resolution{}, providerregistry.ErrUnresolved
	}
	return resolution, nil
}

func scanRegistryTuple(row pgx.Row) (
	providerregistry.ServiceContract,
	providerregistry.CapabilityContract,
	providerregistry.ProviderRegistration,
	providerregistry.ProviderCapabilityBinding,
	bool,
) {
	if row == nil {
		return unresolvedTuple()
	}

	var serviceOwnerModule string
	var serviceKey string
	var serviceVersion int64
	var serviceLifecycle string

	var capabilityServiceKey string
	var capabilityServiceVersion int64
	var capabilityKey string
	var capabilityVersion int64
	var capabilityBoundary string
	var capabilityLifecycle string

	var providerID pgtype.UUID
	var providerServiceKey string
	var providerServiceVersion int64
	var providerKey string
	var adapterKey string
	var providerConfigScope string
	var providerTenantID pgtype.UUID
	var providerLifecycle string
	var providerRevision int64
	var providerRegistryVersion int64

	var bindingProviderID pgtype.UUID
	var bindingServiceKey string
	var bindingServiceVersion int64
	var bindingCapabilityKey string
	var bindingCapabilityVersion int64
	var bindingLifecycle string
	var bindingRevision int64

	if err := row.Scan(
		&serviceOwnerModule,
		&serviceKey,
		&serviceVersion,
		&serviceLifecycle,
		&capabilityServiceKey,
		&capabilityServiceVersion,
		&capabilityKey,
		&capabilityVersion,
		&capabilityBoundary,
		&capabilityLifecycle,
		&providerID,
		&providerServiceKey,
		&providerServiceVersion,
		&providerKey,
		&adapterKey,
		&providerConfigScope,
		&providerTenantID,
		&providerLifecycle,
		&providerRevision,
		&providerRegistryVersion,
		&bindingProviderID,
		&bindingServiceKey,
		&bindingServiceVersion,
		&bindingCapabilityKey,
		&bindingCapabilityVersion,
		&bindingLifecycle,
		&bindingRevision,
	); err != nil {
		return unresolvedTuple()
	}

	mappedServiceVersion, serviceVersionOK := registryVersion(serviceVersion)
	mappedCapabilityServiceVersion, capabilityServiceVersionOK := registryVersion(capabilityServiceVersion)
	mappedCapabilityVersion, capabilityVersionOK := registryVersion(capabilityVersion)
	mappedProviderID, providerIDOK := registryProviderID(providerID)
	mappedProviderServiceVersion, providerServiceVersionOK := registryVersion(providerServiceVersion)
	mappedProviderTenantID, providerTenantIDOK := registryTenantID(providerTenantID)
	mappedProviderRevision, providerRevisionOK := registryVersion(providerRevision)
	mappedProviderRegistryVersion, providerRegistryVersionOK := registryVersion(providerRegistryVersion)
	mappedBindingProviderID, bindingProviderIDOK := registryProviderID(bindingProviderID)
	mappedBindingServiceVersion, bindingServiceVersionOK := registryVersion(bindingServiceVersion)
	mappedBindingCapabilityVersion, bindingCapabilityVersionOK := registryVersion(bindingCapabilityVersion)
	mappedBindingRevision, bindingRevisionOK := registryVersion(bindingRevision)
	if !serviceVersionOK || !capabilityServiceVersionOK || !capabilityVersionOK ||
		!providerIDOK || !providerServiceVersionOK || !providerTenantIDOK ||
		!providerRevisionOK || !providerRegistryVersionOK || !bindingProviderIDOK ||
		!bindingServiceVersionOK || !bindingCapabilityVersionOK || !bindingRevisionOK {
		return unresolvedTuple()
	}

	service := providerregistry.ServiceContract{
		OwnerModule: serviceOwnerModule,
		ServiceKey:  serviceKey,
		Version:     mappedServiceVersion,
		Lifecycle:   providerregistry.Lifecycle(serviceLifecycle),
	}
	capability := providerregistry.CapabilityContract{
		ServiceKey:        capabilityServiceKey,
		ServiceVersion:    mappedCapabilityServiceVersion,
		CapabilityKey:     capabilityKey,
		Version:           mappedCapabilityVersion,
		OperationBoundary: providerregistry.OperationBoundary(capabilityBoundary),
		Lifecycle:         providerregistry.Lifecycle(capabilityLifecycle),
	}
	provider := providerregistry.ProviderRegistration{
		ID:                      mappedProviderID,
		ServiceKey:              providerServiceKey,
		ServiceVersion:          mappedProviderServiceVersion,
		ProviderKey:             providerKey,
		AdapterKey:              adapterKey,
		ConfigScope:             providerregistry.ConfigScope(providerConfigScope),
		TenantID:                mappedProviderTenantID,
		Lifecycle:               providerregistry.Lifecycle(providerLifecycle),
		Revision:                mappedProviderRevision,
		RegistryContractVersion: mappedProviderRegistryVersion,
	}
	binding := providerregistry.ProviderCapabilityBinding{
		ProviderID:        mappedBindingProviderID,
		ServiceKey:        bindingServiceKey,
		ServiceVersion:    mappedBindingServiceVersion,
		CapabilityKey:     bindingCapabilityKey,
		CapabilityVersion: mappedBindingCapabilityVersion,
		Lifecycle:         providerregistry.Lifecycle(bindingLifecycle),
		Revision:          mappedBindingRevision,
	}
	if service.Validate() != nil || capability.Validate() != nil ||
		provider.Validate() != nil || binding.Validate() != nil {
		return unresolvedTuple()
	}
	return service, capability, provider, binding, true
}

func unresolvedTuple() (
	providerregistry.ServiceContract,
	providerregistry.CapabilityContract,
	providerregistry.ProviderRegistration,
	providerregistry.ProviderCapabilityBinding,
	bool,
) {
	return providerregistry.ServiceContract{}, providerregistry.CapabilityContract{},
		providerregistry.ProviderRegistration{}, providerregistry.ProviderCapabilityBinding{}, false
}

func postgresVersion(value uint64) (int64, bool) {
	if value == 0 || value > math.MaxInt64 {
		return 0, false
	}
	return int64(value), true
}

func registryVersion(value int64) (uint64, bool) {
	if value <= 0 {
		return 0, false
	}
	return uint64(value), true
}

func registryProviderID(value pgtype.UUID) (providerregistry.ProviderID, bool) {
	id := providerregistry.ProviderID(value.Bytes)
	return id, value.Valid && !id.IsZero()
}

func registryTenantID(value pgtype.UUID) (*tenancy.TenantID, bool) {
	if !value.Valid {
		return nil, true
	}
	id := tenancy.TenantID(value.Bytes)
	if id.IsZero() {
		return nil, false
	}
	return &id, true
}
