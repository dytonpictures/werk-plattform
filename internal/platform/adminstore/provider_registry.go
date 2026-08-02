package adminstore

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/platform/database"
)

const (
	maximumProviderRegistryServices     = 100
	maximumProviderRegistryCapabilities = 500
	maximumProviderRegistrations        = 200
	maximumProviderCapabilityBindings   = 1000
)

// ProviderRegistryCatalog is a bounded, non-secret administration projection
// of the global service/provider registry. It intentionally contains no
// provider endpoints, configuration, credentials, keys, health or routing
// preference.
type ProviderRegistryCatalog struct {
	ObservedAt time.Time                     `json:"observed_at"`
	Services   []ProviderRegistryServiceView `json:"services"`
	Providers  []ProviderRegistrationView    `json:"providers"`
	Truncated  bool                          `json:"truncated"`
}

type ProviderRegistryServiceView struct {
	ServiceKey      string                           `json:"service_key"`
	OwnerModule     string                           `json:"owner_module"`
	ContractVersion uint64                           `json:"contract_version"`
	Lifecycle       string                           `json:"lifecycle"`
	Capabilities    []ProviderRegistryCapabilityView `json:"capabilities"`
}

type ProviderRegistryCapabilityView struct {
	CapabilityKey     string `json:"capability_key"`
	CapabilityVersion uint64 `json:"capability_version"`
	OperationBoundary string `json:"operation_boundary"`
	Lifecycle         string `json:"lifecycle"`
}

type ProviderRegistrationView struct {
	ID                      string                          `json:"id"`
	ServiceKey              string                          `json:"service_key"`
	ServiceContractVersion  uint64                          `json:"service_contract_version"`
	ProviderKey             string                          `json:"provider_key"`
	AdapterKey              string                          `json:"adapter_key"`
	ConfigScope             string                          `json:"config_scope"`
	TenantID                string                          `json:"tenant_id,omitempty"`
	RegistryContractVersion uint64                          `json:"registry_contract_version"`
	Lifecycle               string                          `json:"lifecycle"`
	Revision                uint64                          `json:"revision"`
	Bindings                []ProviderCapabilityBindingView `json:"bindings"`
}

type ProviderCapabilityBindingView struct {
	CapabilityKey     string `json:"capability_key"`
	CapabilityVersion uint64 `json:"capability_version"`
	Lifecycle         string `json:"lifecycle"`
	Revision          uint64 `json:"revision"`
}

// ListProviderRegistry returns the declarative registry and atomically records
// the installation-wide observation. Missing provider registrations are a
// valid state: contracts may be present before an operator configures an
// executable provider.
func (service *Service) ListProviderRegistry(
	ctx context.Context,
	actor identity.AuthenticatedActor,
	requestID string,
	correlationID string,
) (ProviderRegistryCatalog, error) {
	if service == nil || service.database == nil {
		return ProviderRegistryCatalog{}, errors.New("provider registry catalog is not configured")
	}
	auditID, err := randomUUID()
	if err != nil {
		return ProviderRegistryCatalog{}, err
	}
	catalog := ProviderRegistryCatalog{
		Services:  make([]ProviderRegistryServiceView, 0),
		Providers: make([]ProviderRegistrationView, 0),
	}
	err = service.database.WithinInstallationAuditRead(ctx, func(ctx context.Context, tx database.TenantTx) error {
		if err := tx.QueryRow(ctx, `SELECT statement_timestamp()`).Scan(&catalog.ObservedAt); err != nil {
			return err
		}
		catalog.ObservedAt = catalog.ObservedAt.UTC()

		serviceIndexes, truncated, err := loadProviderRegistryServices(ctx, tx, &catalog)
		if err != nil {
			return err
		}
		catalog.Truncated = catalog.Truncated || truncated
		truncated, err = loadProviderRegistryCapabilities(ctx, tx, &catalog, serviceIndexes)
		if err != nil {
			return err
		}
		catalog.Truncated = catalog.Truncated || truncated
		providerIndexes, truncated, err := loadProviderRegistrations(ctx, tx, &catalog)
		if err != nil {
			return err
		}
		catalog.Truncated = catalog.Truncated || truncated
		truncated, err = loadProviderCapabilityBindings(ctx, tx, &catalog, providerIndexes)
		if err != nil {
			return err
		}
		catalog.Truncated = catalog.Truncated || truncated

		_, err = tx.Exec(ctx, `
			INSERT INTO werk_core.security_audit_events (
				id, occurred_at, event_type, outcome, account_id, tenant_id,
				request_id, correlation_id, details
			) VALUES (
				$1::uuid, $2, 'core.platform.provider-registry-listed.v1',
				'succeeded', $3::uuid, NULL, $4::uuid, $5::uuid,
				jsonb_build_object(
					'service_count', $6::integer,
					'provider_count', $7::integer,
					'truncated', $8::boolean
				)
			)
		`, auditID, service.now(), formatUUID(actor.AccountID), requestID, correlationID,
			len(catalog.Services), len(catalog.Providers), catalog.Truncated)
		return err
	})
	if err != nil {
		return ProviderRegistryCatalog{}, err
	}
	return catalog, nil
}

func loadProviderRegistryServices(
	ctx context.Context,
	tx database.TenantTx,
	catalog *ProviderRegistryCatalog,
) (map[string]int, bool, error) {
	rows, err := tx.Query(ctx, `
		SELECT service_key, owner_module, contract_version, lifecycle
		FROM werk_core.platform_service_contracts
		ORDER BY service_key, contract_version
		LIMIT $1
	`, maximumProviderRegistryServices+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	indexes := make(map[string]int)
	truncated := false
	for rows.Next() {
		var view ProviderRegistryServiceView
		if err := rows.Scan(&view.ServiceKey, &view.OwnerModule, &view.ContractVersion, &view.Lifecycle); err != nil {
			return nil, false, err
		}
		if len(catalog.Services) == maximumProviderRegistryServices {
			truncated = true
			continue
		}
		view.Capabilities = make([]ProviderRegistryCapabilityView, 0)
		indexes[providerRegistryServiceIndex(view.ServiceKey, view.ContractVersion)] = len(catalog.Services)
		catalog.Services = append(catalog.Services, view)
	}
	return indexes, truncated, rows.Err()
}

func loadProviderRegistryCapabilities(
	ctx context.Context,
	tx database.TenantTx,
	catalog *ProviderRegistryCatalog,
	serviceIndexes map[string]int,
) (bool, error) {
	rows, err := tx.Query(ctx, `
		SELECT service_key, service_contract_version, capability_key,
		       capability_version, operation_boundary, lifecycle
		FROM werk_core.platform_service_capability_contracts
		ORDER BY service_key, service_contract_version, capability_key, capability_version
		LIMIT $1
	`, maximumProviderRegistryCapabilities+1)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	count := 0
	truncated := false
	for rows.Next() {
		var serviceKey string
		var serviceVersion uint64
		var view ProviderRegistryCapabilityView
		if err := rows.Scan(
			&serviceKey, &serviceVersion, &view.CapabilityKey,
			&view.CapabilityVersion, &view.OperationBoundary, &view.Lifecycle,
		); err != nil {
			return false, err
		}
		if count == maximumProviderRegistryCapabilities {
			truncated = true
			continue
		}
		count++
		index, ok := serviceIndexes[providerRegistryServiceIndex(serviceKey, serviceVersion)]
		if !ok {
			truncated = true
			continue
		}
		catalog.Services[index].Capabilities = append(catalog.Services[index].Capabilities, view)
	}
	return truncated, rows.Err()
}

func loadProviderRegistrations(
	ctx context.Context,
	tx database.TenantTx,
	catalog *ProviderRegistryCatalog,
) (map[string]int, bool, error) {
	rows, err := tx.Query(ctx, `
		SELECT id::text, service_key, service_contract_version, provider_key,
		       adapter_key, config_scope, tenant_id::text,
		       registry_contract_version, lifecycle, revision
		FROM werk_core.platform_provider_registrations
		ORDER BY service_key, service_contract_version, provider_key,
		         registry_contract_version, id
		LIMIT $1
	`, maximumProviderRegistrations+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	indexes := make(map[string]int)
	truncated := false
	for rows.Next() {
		var view ProviderRegistrationView
		var tenantID pgtype.Text
		if err := rows.Scan(
			&view.ID, &view.ServiceKey, &view.ServiceContractVersion,
			&view.ProviderKey, &view.AdapterKey, &view.ConfigScope, &tenantID,
			&view.RegistryContractVersion, &view.Lifecycle, &view.Revision,
		); err != nil {
			return nil, false, err
		}
		if len(catalog.Providers) == maximumProviderRegistrations {
			truncated = true
			continue
		}
		if tenantID.Valid {
			view.TenantID = tenantID.String
		}
		view.Bindings = make([]ProviderCapabilityBindingView, 0)
		indexes[view.ID] = len(catalog.Providers)
		catalog.Providers = append(catalog.Providers, view)
	}
	return indexes, truncated, rows.Err()
}

func loadProviderCapabilityBindings(
	ctx context.Context,
	tx database.TenantTx,
	catalog *ProviderRegistryCatalog,
	providerIndexes map[string]int,
) (bool, error) {
	rows, err := tx.Query(ctx, `
		SELECT provider_id::text, capability_key, capability_version,
		       lifecycle, revision
		FROM werk_core.platform_provider_capability_bindings
		ORDER BY provider_id, capability_key, capability_version
		LIMIT $1
	`, maximumProviderCapabilityBindings+1)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	count := 0
	truncated := false
	for rows.Next() {
		var providerID string
		var view ProviderCapabilityBindingView
		if err := rows.Scan(
			&providerID, &view.CapabilityKey, &view.CapabilityVersion,
			&view.Lifecycle, &view.Revision,
		); err != nil {
			return false, err
		}
		if count == maximumProviderCapabilityBindings {
			truncated = true
			continue
		}
		count++
		index, ok := providerIndexes[providerID]
		if !ok {
			truncated = true
			continue
		}
		catalog.Providers[index].Bindings = append(catalog.Providers[index].Bindings, view)
	}
	return truncated, rows.Err()
}

func providerRegistryServiceIndex(serviceKey string, version uint64) string {
	return serviceKey + "\x00" + strconv.FormatUint(version, 10)
}
