package database

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPlatformServiceProviderRegistryIntegration(t *testing.T) {
	migratorURL := integrationEnvironment(t, "WERK_TEST_MIGRATOR_DATABASE_URL")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, migratorURL)
	if err != nil {
		t.Fatalf("open migrator pool: %v", err)
	}
	defer pool.Close()
	connection, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire migrator connection: %v", err)
	}
	defer connection.Release()
	if _, err := connection.Exec(ctx, `SET ROLE werk_owner`); err != nil {
		t.Fatalf("assume owner role for provider registry fixtures: %v", err)
	}

	transaction, err := connection.Begin(ctx)
	if err != nil {
		t.Fatalf("begin provider registry fixture transaction: %v", err)
	}
	defer func() { _ = transaction.Rollback(context.Background()) }()

	assertProviderRegistryCatalog(t, ctx, transaction)
	assertProviderRegistryRuntimeRules(t, ctx, transaction)
}

func assertProviderRegistryCatalog(t *testing.T, ctx context.Context, transaction pgx.Tx) {
	t.Helper()

	const expectedTableCount = 4
	var secureTableCount int
	if err := transaction.QueryRow(ctx, `
		SELECT count(*)
		FROM pg_catalog.pg_class AS relation
		JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid = relation.relnamespace
		WHERE namespace.nspname = 'werk_core'
		  AND relation.relname IN (
		    'platform_service_contracts',
		    'platform_service_capability_contracts',
		    'platform_provider_registrations',
		    'platform_provider_capability_bindings'
		  )
		  AND relation.relrowsecurity
		  AND relation.relforcerowsecurity
		  AND pg_catalog.pg_get_userbyid(relation.relowner) = 'werk_owner'
	`).Scan(&secureTableCount); err != nil {
		t.Fatalf("inspect provider registry table security: %v", err)
	}
	if secureTableCount != expectedTableCount {
		t.Fatalf("secure provider registry table count = %d, want %d", secureTableCount, expectedTableCount)
	}

	tables := []string{
		"platform_service_contracts",
		"platform_service_capability_contracts",
		"platform_provider_registrations",
		"platform_provider_capability_bindings",
	}
	for _, role := range []string{"werk_admin_runtime", "werk_backup_reader"} {
		for _, table := range tables {
			var allowed bool
			if err := transaction.QueryRow(ctx,
				`SELECT pg_catalog.has_table_privilege($1, $2, 'SELECT')`,
				role, "werk_core."+table,
			).Scan(&allowed); err != nil {
				t.Fatalf("inspect %s SELECT grant on %s: %v", role, table, err)
			}
			if !allowed {
				t.Fatalf("%s lacks SELECT on werk_core.%s", role, table)
			}
			for _, privilege := range []string{"INSERT", "UPDATE", "DELETE"} {
				if err := transaction.QueryRow(ctx,
					`SELECT pg_catalog.has_table_privilege($1, $2, $3)`,
					role, "werk_core."+table, privilege,
				).Scan(&allowed); err != nil {
					t.Fatalf("inspect %s %s grant on %s: %v", role, privilege, table, err)
				}
				if allowed {
					t.Fatalf("%s unexpectedly has %s on werk_core.%s", role, privilege, table)
				}
			}
		}
	}
	for _, role := range []string{
		"werk_work_runtime",
		"werk_identity_runtime",
		"werk_service_runtime",
		"werk_worker_runtime",
	} {
		for _, table := range tables {
			var allowed bool
			for _, privilege := range []string{"SELECT", "INSERT", "UPDATE", "DELETE"} {
				if err := transaction.QueryRow(ctx,
					`SELECT pg_catalog.has_table_privilege($1, $2, $3)`,
					role, "werk_core."+table, privilege,
				).Scan(&allowed); err != nil {
					t.Fatalf("inspect %s %s grant on %s: %v", role, privilege, table, err)
				}
				if allowed {
					t.Fatalf("%s unexpectedly has %s on werk_core.%s", role, privilege, table)
				}
			}
		}
	}

	var policyCount int
	if err := transaction.QueryRow(ctx, `
		WITH expected(table_name, policy_name, command_name, role_name) AS (
			VALUES
				('platform_service_contracts', 'platform_service_contracts_admin_read', 'SELECT', 'werk_admin_runtime'),
				('platform_service_contracts', 'platform_service_contracts_backup_read', 'SELECT', 'werk_backup_reader'),
				('platform_service_contracts', 'platform_service_contracts_owner_all', 'ALL', 'werk_owner'),
				('platform_service_capability_contracts', 'platform_service_capability_contracts_admin_read', 'SELECT', 'werk_admin_runtime'),
				('platform_service_capability_contracts', 'platform_service_capability_contracts_backup_read', 'SELECT', 'werk_backup_reader'),
				('platform_service_capability_contracts', 'platform_service_capability_contracts_owner_all', 'ALL', 'werk_owner'),
				('platform_provider_registrations', 'platform_provider_registrations_admin_read', 'SELECT', 'werk_admin_runtime'),
				('platform_provider_registrations', 'platform_provider_registrations_backup_read', 'SELECT', 'werk_backup_reader'),
				('platform_provider_registrations', 'platform_provider_registrations_owner_all', 'ALL', 'werk_owner'),
				('platform_provider_capability_bindings', 'platform_provider_capability_bindings_admin_read', 'SELECT', 'werk_admin_runtime'),
				('platform_provider_capability_bindings', 'platform_provider_capability_bindings_backup_read', 'SELECT', 'werk_backup_reader'),
				('platform_provider_capability_bindings', 'platform_provider_capability_bindings_owner_all', 'ALL', 'werk_owner')
		)
		SELECT count(*)
		FROM expected
		JOIN pg_catalog.pg_policies AS policy
		  ON policy.schemaname = 'werk_core'
		 AND policy.tablename = expected.table_name
		 AND policy.policyname = expected.policy_name
		 AND policy.cmd = expected.command_name
		 AND expected.role_name = ANY(policy.roles::text[])
	`).Scan(&policyCount); err != nil {
		t.Fatalf("inspect provider registry policies: %v", err)
	}
	if policyCount != 12 {
		t.Fatalf("bound provider registry policy count = %d, want 12", policyCount)
	}

	var triggerCount int
	if err := transaction.QueryRow(ctx, `
		WITH expected(table_name, trigger_name, function_name) AS (
			VALUES
				('platform_service_contracts', 'platform_service_contracts_protect_lifecycle', 'protect_platform_provider_registry_lifecycle'),
				('platform_service_contracts', 'platform_service_contracts_protect_identity', 'protect_platform_service_contract'),
				('platform_service_contracts', 'platform_service_contracts_reject_delete', 'reject_platform_provider_registry_delete'),
				('platform_service_capability_contracts', 'platform_service_capability_contracts_protect_lifecycle', 'protect_platform_provider_registry_lifecycle'),
				('platform_service_capability_contracts', 'platform_service_capability_contracts_protect_identity', 'protect_platform_service_capability_contract'),
				('platform_service_capability_contracts', 'platform_service_capability_contracts_reject_delete', 'reject_platform_provider_registry_delete'),
				('platform_provider_registrations', 'platform_provider_registrations_protect_lifecycle', 'protect_platform_provider_registry_lifecycle'),
				('platform_provider_registrations', 'platform_provider_registrations_protect_identity', 'protect_platform_provider_registration'),
				('platform_provider_registrations', 'platform_provider_registrations_reject_delete', 'reject_platform_provider_registry_delete'),
				('platform_provider_capability_bindings', 'platform_provider_capability_bindings_protect_lifecycle', 'protect_platform_provider_registry_lifecycle'),
				('platform_provider_capability_bindings', 'platform_provider_capability_bindings_protect_identity', 'protect_platform_provider_binding'),
				('platform_provider_capability_bindings', 'platform_provider_capability_bindings_validate_scope', 'validate_platform_provider_binding_scope'),
				('platform_provider_capability_bindings', 'platform_provider_capability_bindings_reject_delete', 'reject_platform_provider_registry_delete')
		)
		SELECT count(*)
		FROM expected
		JOIN pg_catalog.pg_class AS relation ON relation.relname = expected.table_name
		JOIN pg_catalog.pg_namespace AS relation_namespace
		  ON relation_namespace.oid = relation.relnamespace
		 AND relation_namespace.nspname = 'werk_core'
		JOIN pg_catalog.pg_trigger AS binding
		  ON binding.tgrelid = relation.oid
		 AND binding.tgname = expected.trigger_name
		 AND NOT binding.tgisinternal
		 AND binding.tgenabled <> 'D'
		JOIN pg_catalog.pg_proc AS function ON function.oid = binding.tgfoid
		JOIN pg_catalog.pg_namespace AS function_namespace
		  ON function_namespace.oid = function.pronamespace
		 AND function_namespace.nspname = 'werk_security'
		 AND function.proname = expected.function_name
	`).Scan(&triggerCount); err != nil {
		t.Fatalf("inspect provider registry triggers: %v", err)
	}
	if triggerCount != 13 {
		t.Fatalf("bound provider registry trigger count = %d, want 13", triggerCount)
	}
}

func assertProviderRegistryRuntimeRules(t *testing.T, ctx context.Context, transaction pgx.Tx) {
	t.Helper()

	const (
		tenantID           = "0196f000-0000-7000-8000-00000000f101"
		providerID         = "0196f000-0000-7000-8000-00000000f102"
		invalidProviderID  = "0196f000-0000-7000-8000-00000000f103"
		upgradedProviderID = "0196f000-0000-7000-8000-00000000f104"
		duplicateProvider  = "0196f000-0000-7000-8000-00000000f105"
		serviceKey         = "core.platform.service.integration-registry"
		otherServiceKey    = "core.platform.service.integration-registry-alt"
		capabilityKey      = serviceKey + ".capability.read"
		installCapability  = serviceKey + ".capability.install"
		otherCapabilityKey = otherServiceKey + ".capability.read"
	)

	if _, err := transaction.Exec(ctx, `
		INSERT INTO werk_core.tenants (id, name, status, default_locale, default_timezone)
		VALUES ($1::uuid, 'Provider registry integration', 'active', 'de-DE', 'Europe/Berlin')
	`, tenantID); err != nil {
		t.Fatalf("insert provider registry tenant fixture: %v", err)
	}
	if _, err := transaction.Exec(ctx, `
		INSERT INTO werk_core.platform_service_contracts (
			service_key, owner_module, contract_version
		) VALUES
			($1, 'core.platform', 1),
			($2, 'core.platform', 1)
	`, serviceKey, otherServiceKey); err != nil {
		t.Fatalf("insert provider registry service fixtures: %v", err)
	}
	if _, err := transaction.Exec(ctx, `
		INSERT INTO werk_core.platform_service_capability_contracts (
			service_key, service_contract_version,
			capability_key, capability_version, operation_boundary
		) VALUES
			($1, 1, $2, 1, 'tenant'),
			($1, 1, $3, 1, 'installation'),
			($4, 1, $5, 1, 'installation')
	`, serviceKey, capabilityKey, installCapability, otherServiceKey, otherCapabilityKey); err != nil {
		t.Fatalf("insert provider registry capability fixtures: %v", err)
	}
	expectProviderRegistryStatementRejected(t, ctx, transaction, "immutable service created_at", func(nested pgx.Tx) error {
		_, err := nested.Exec(ctx, `
			UPDATE werk_core.platform_service_contracts
			SET created_at = created_at - interval '1 second'
			WHERE service_key = $1 AND contract_version = 1
		`, serviceKey)
		return err
	})
	expectProviderRegistryStatementRejected(t, ctx, transaction, "immutable capability created_at", func(nested pgx.Tx) error {
		_, err := nested.Exec(ctx, `
			UPDATE werk_core.platform_service_capability_contracts
			SET created_at = created_at - interval '1 second'
			WHERE service_key = $1 AND service_contract_version = 1
			  AND capability_key = $2 AND capability_version = 1
		`, serviceKey, capabilityKey)
		return err
	})

	var providerLifecycle string
	var providerRevision int64
	if err := transaction.QueryRow(ctx, `
		INSERT INTO werk_core.platform_provider_registrations (
			id, service_key, service_contract_version, provider_key,
			adapter_key, config_scope, registry_contract_version
		) VALUES (
			$1::uuid, $2, 1, $2 || '.provider.primary',
			'core.platform.adapter.integration-registry', 'installation', 1
		)
		RETURNING lifecycle, revision
	`, providerID, serviceKey).Scan(&providerLifecycle, &providerRevision); err != nil {
		t.Fatalf("insert provider registration fixture: %v", err)
	}
	if providerLifecycle != "disabled" || providerRevision != 1 {
		t.Fatalf("provider defaults = lifecycle %q, revision %d; want disabled, 1", providerLifecycle, providerRevision)
	}
	if _, err := transaction.Exec(ctx, `
		INSERT INTO werk_core.platform_provider_registrations (
			id, service_key, service_contract_version, provider_key,
			adapter_key, config_scope, registry_contract_version
		) VALUES (
			$1::uuid, $2, 1, $2 || '.provider.primary',
			'core.platform.adapter.integration-registry-v2', 'installation', 2
		)
	`, upgradedProviderID, serviceKey); err != nil {
		t.Fatalf("insert independently versioned provider registration: %v", err)
	}
	expectProviderRegistryStatementRejected(t, ctx, transaction, "duplicate provider key and registry version", func(nested pgx.Tx) error {
		_, err := nested.Exec(ctx, `
			INSERT INTO werk_core.platform_provider_registrations (
				id, service_key, service_contract_version, provider_key,
				adapter_key, config_scope, registry_contract_version
			) VALUES (
				$1::uuid, $2, 1, $2 || '.provider.primary',
				'core.platform.adapter.integration-registry-duplicate', 'installation', 1
			)
		`, duplicateProvider, serviceKey)
		return err
	})

	var bindingLifecycle string
	var bindingRevision int64
	if err := transaction.QueryRow(ctx, `
		INSERT INTO werk_core.platform_provider_capability_bindings (
			provider_id, service_key, service_contract_version,
			capability_key, capability_version
		) VALUES ($1::uuid, $2, 1, $3, 1)
		RETURNING lifecycle, revision
	`, providerID, serviceKey, capabilityKey).Scan(&bindingLifecycle, &bindingRevision); err != nil {
		t.Fatalf("insert provider capability binding fixture: %v", err)
	}
	if bindingLifecycle != "disabled" || bindingRevision != 1 {
		t.Fatalf("binding defaults = lifecycle %q, revision %d; want disabled, 1", bindingLifecycle, bindingRevision)
	}

	expectProviderRegistryStatementRejected(t, ctx, transaction, "tenant scope without tenant", func(nested pgx.Tx) error {
		_, err := nested.Exec(ctx, `
			INSERT INTO werk_core.platform_provider_registrations (
				id, service_key, service_contract_version, provider_key,
				adapter_key, config_scope, registry_contract_version
			) VALUES (
				$1::uuid, $2, 1, $2 || '.provider.invalid-tenant-null',
				'core.platform.adapter.integration-registry', 'tenant', 1
			)
		`, invalidProviderID, serviceKey)
		return err
	})
	expectProviderRegistryStatementRejected(t, ctx, transaction, "installation scope with tenant", func(nested pgx.Tx) error {
		_, err := nested.Exec(ctx, `
			INSERT INTO werk_core.platform_provider_registrations (
				id, service_key, service_contract_version, provider_key,
				adapter_key, config_scope, tenant_id, registry_contract_version
			) VALUES (
				$1::uuid, $2, 1, $2 || '.provider.invalid-installation-tenant',
				'core.platform.adapter.integration-registry', 'installation', $3::uuid, 1
			)
		`, invalidProviderID, serviceKey, tenantID)
		return err
	})
	if _, err := transaction.Exec(ctx, `
		INSERT INTO werk_core.platform_provider_registrations (
			id, service_key, service_contract_version, provider_key,
			adapter_key, config_scope, tenant_id, registry_contract_version
		) VALUES (
			$1::uuid, $2, 1, $2 || '.provider.tenant',
			'core.platform.adapter.integration-registry', 'tenant', $3::uuid, 1
		)
	`, invalidProviderID, serviceKey, tenantID); err != nil {
		t.Fatalf("insert tenant provider fixture: %v", err)
	}
	expectProviderRegistryStatementRejected(t, ctx, transaction, "tenant provider for installation capability", func(nested pgx.Tx) error {
		_, err := nested.Exec(ctx, `
			INSERT INTO werk_core.platform_provider_capability_bindings (
				provider_id, service_key, service_contract_version,
				capability_key, capability_version
			) VALUES ($1::uuid, $2, 1, $3, 1)
		`, invalidProviderID, serviceKey, installCapability)
		return err
	})

	expectProviderRegistryStatementRejected(t, ctx, transaction, "provider revision jump", func(nested pgx.Tx) error {
		_, err := nested.Exec(ctx, `
			UPDATE werk_core.platform_provider_registrations
			SET lifecycle = 'active', revision = 3
			WHERE id = $1::uuid
		`, providerID)
		return err
	})
	if _, err := transaction.Exec(ctx, `
		UPDATE werk_core.platform_provider_registrations
		SET lifecycle = 'active', revision = 2
		WHERE id = $1::uuid
	`, providerID); err != nil {
		t.Fatalf("advance provider revision by one: %v", err)
	}
	expectProviderRegistryStatementRejected(t, ctx, transaction, "immutable registry contract version", func(nested pgx.Tx) error {
		_, err := nested.Exec(ctx, `
			UPDATE werk_core.platform_provider_registrations
			SET registry_contract_version = 2, revision = 3
			WHERE id = $1::uuid
		`, providerID)
		return err
	})
	expectProviderRegistryStatementRejected(t, ctx, transaction, "immutable provider created_at", func(nested pgx.Tx) error {
		_, err := nested.Exec(ctx, `
			UPDATE werk_core.platform_provider_registrations
			SET created_at = created_at - interval '1 second', revision = 3
			WHERE id = $1::uuid
		`, providerID)
		return err
	})

	expectProviderRegistryStatementRejected(t, ctx, transaction, "binding revision jump", func(nested pgx.Tx) error {
		_, err := nested.Exec(ctx, `
			UPDATE werk_core.platform_provider_capability_bindings
			SET lifecycle = 'active', revision = 3
			WHERE provider_id = $1::uuid
			  AND capability_key = $2
			  AND capability_version = 1
		`, providerID, capabilityKey)
		return err
	})
	if _, err := transaction.Exec(ctx, `
		UPDATE werk_core.platform_provider_capability_bindings
		SET lifecycle = 'active', revision = 2
		WHERE provider_id = $1::uuid
		  AND capability_key = $2
		  AND capability_version = 1
	`, providerID, capabilityKey); err != nil {
		t.Fatalf("advance provider binding revision by one: %v", err)
	}
	expectProviderRegistryStatementRejected(t, ctx, transaction, "immutable binding created_at", func(nested pgx.Tx) error {
		_, err := nested.Exec(ctx, `
			UPDATE werk_core.platform_provider_capability_bindings
			SET created_at = created_at - interval '1 second', revision = 3
			WHERE provider_id = $1::uuid
			  AND capability_key = $2
			  AND capability_version = 1
		`, providerID, capabilityKey)
		return err
	})

	expectProviderRegistryStatementRejected(t, ctx, transaction, "provider-service foreign key mismatch", func(nested pgx.Tx) error {
		_, err := nested.Exec(ctx, `
			INSERT INTO werk_core.platform_provider_capability_bindings (
				provider_id, service_key, service_contract_version,
				capability_key, capability_version
			) VALUES ($1::uuid, $2, 1, $3, 1)
		`, providerID, otherServiceKey, otherCapabilityKey)
		return err
	})
	expectProviderRegistryStatementRejected(t, ctx, transaction, "capability foreign key version mismatch", func(nested pgx.Tx) error {
		_, err := nested.Exec(ctx, `
			INSERT INTO werk_core.platform_provider_capability_bindings (
				provider_id, service_key, service_contract_version,
				capability_key, capability_version
			) VALUES ($1::uuid, $2, 1, $3, 2)
		`, providerID, serviceKey, capabilityKey)
		return err
	})

	if _, err := transaction.Exec(ctx, `
		UPDATE werk_core.platform_provider_registrations
		SET lifecycle = 'retired', revision = 3
		WHERE id = $1::uuid
	`, providerID); err != nil {
		t.Fatalf("retire provider registration: %v", err)
	}
	expectProviderRegistryStatementRejected(t, ctx, transaction, "reactivate retired provider", func(nested pgx.Tx) error {
		_, err := nested.Exec(ctx, `
			UPDATE werk_core.platform_provider_registrations
			SET lifecycle = 'active', revision = 4
			WHERE id = $1::uuid
		`, providerID)
		return err
	})
	expectProviderRegistryStatementRejected(t, ctx, transaction, "delete provider registry entry", func(nested pgx.Tx) error {
		_, err := nested.Exec(ctx, `
			DELETE FROM werk_core.platform_provider_registrations
			WHERE id = $1::uuid
		`, providerID)
		return err
	})
}

func expectProviderRegistryStatementRejected(
	t *testing.T,
	ctx context.Context,
	transaction pgx.Tx,
	operation string,
	statement func(pgx.Tx) error,
) {
	t.Helper()
	nested, err := transaction.Begin(ctx)
	if err != nil {
		t.Fatalf("begin savepoint for %s: %v", operation, err)
	}
	statementErr := statement(nested)
	rollbackErr := nested.Rollback(ctx)
	if rollbackErr != nil && rollbackErr != pgx.ErrTxClosed {
		t.Fatalf("rollback savepoint for %s: %v", operation, rollbackErr)
	}
	if statementErr == nil {
		t.Fatalf("%s was accepted", operation)
	}
}
