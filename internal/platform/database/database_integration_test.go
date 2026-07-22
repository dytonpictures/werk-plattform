package database

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dytonpictures/werk/internal/core/tenancy"
)

func TestTenantIsolationIntegration(t *testing.T) {
	migratorURL := integrationEnvironment(t, "WERK_TEST_MIGRATOR_DATABASE_URL")
	workURL := integrationEnvironment(t, "WERK_TEST_WORK_DATABASE_URL")
	workerURL := integrationEnvironment(t, "WERK_TEST_WORKER_DATABASE_URL")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ownerPool, err := pgxpool.New(ctx, migratorURL)
	if err != nil {
		t.Fatalf("open migrator pool: %v", err)
	}
	defer ownerPool.Close()
	ownerConnection, err := ownerPool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire migrator connection: %v", err)
	}
	defer ownerConnection.Release()
	if _, err := ownerConnection.Exec(ctx, `SET ROLE werk_owner`); err != nil {
		t.Fatalf("assume owner role for fixtures: %v", err)
	}

	tenantA, err := tenancy.NewTenant("Isolation A", "de-DE", "Europe/Berlin")
	if err != nil {
		t.Fatalf("create tenant A fixture: %v", err)
	}
	tenantB, err := tenancy.NewTenant("Isolation B", "de-DE", "Europe/Berlin")
	if err != nil {
		t.Fatalf("create tenant B fixture: %v", err)
	}
	unitA, err := tenancy.NewOrganizationalUnit(tenantA.ID, nil, "company", "Company A")
	if err != nil {
		t.Fatalf("create unit A fixture: %v", err)
	}
	unitB, err := tenancy.NewOrganizationalUnit(tenantB.ID, nil, "company", "Company B")
	if err != nil {
		t.Fatalf("create unit B fixture: %v", err)
	}

	if _, err := ownerConnection.Exec(ctx, `
		INSERT INTO werk_core.tenants (id, name, status, default_locale, default_timezone)
		VALUES ($1::uuid, $2, 'active', 'de-DE', 'Europe/Berlin'),
		       ($3::uuid, $4, 'active', 'de-DE', 'Europe/Berlin')
	`, tenantA.ID.String(), tenantA.Name, tenantB.ID.String(), tenantB.Name); err != nil {
		t.Fatalf("insert tenant fixtures: %v", err)
	}
	if _, err := ownerConnection.Exec(ctx, `
		INSERT INTO werk_core.organizational_units (id, tenant_id, unit_type, name, status)
		VALUES ($1::uuid, $2::uuid, 'company', $3, 'active'),
		       ($4::uuid, $5::uuid, 'company', $6, 'active')
	`, unitA.ID.String(), tenantA.ID.String(), unitA.Name, unitB.ID.String(), tenantB.ID.String(), unitB.Name); err != nil {
		t.Fatalf("insert organizational unit fixtures: %v", err)
	}
	const (
		partyA      = "0196f000-0000-7000-8000-000000000031"
		partyB      = "0196f000-0000-7000-8000-000000000032"
		membershipA = "0196f000-0000-7000-8000-000000000041"
		membershipB = "0196f000-0000-7000-8000-000000000042"
	)
	if _, err := ownerConnection.Exec(ctx, `
		INSERT INTO werk_core.parties (id, tenant_id, party_type, display_name, status)
		VALUES ($1::uuid, $3::uuid, 'person', 'Person A', 'active'),
		       ($2::uuid, $4::uuid, 'organization', 'Organisation B', 'active')
	`, partyA, partyB, tenantA.ID.String(), tenantB.ID.String()); err != nil {
		t.Fatalf("insert party fixtures: %v", err)
	}
	if _, err := ownerConnection.Exec(ctx, `
		INSERT INTO werk_core.persons (party_id, tenant_id, given_name, family_name)
		VALUES ($1::uuid, $2::uuid, 'Person', 'A')
	`, partyA, tenantA.ID.String()); err != nil {
		t.Fatalf("insert person fixture: %v", err)
	}
	if _, err := ownerConnection.Exec(ctx, `
		INSERT INTO werk_core.organizations (party_id, tenant_id, legal_name)
		VALUES ($1::uuid, $2::uuid, 'Organisation B')
	`, partyB, tenantB.ID.String()); err != nil {
		t.Fatalf("insert organization fixture: %v", err)
	}
	if _, err := ownerConnection.Exec(ctx, `
		INSERT INTO werk_core.memberships (
			id, tenant_id, party_id, organizational_unit_id, membership_type, valid_from
		) VALUES
			($1::uuid, $3::uuid, $5::uuid, $7::uuid, 'team.member', '2026-07-19T00:00:00Z'),
			($2::uuid, $4::uuid, $6::uuid, $8::uuid, 'team.member', '2026-07-19T00:00:00Z')
	`, membershipA, membershipB, tenantA.ID.String(), tenantB.ID.String(), partyA, partyB, unitA.ID.String(), unitB.ID.String()); err != nil {
		t.Fatalf("insert membership fixtures: %v", err)
	}
	defer func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = ownerConnection.Exec(cleanupContext, `DELETE FROM werk_core.outbox_events WHERE tenant_id IN ($1::uuid, $2::uuid)`, tenantA.ID.String(), tenantB.ID.String())
		_, _ = ownerConnection.Exec(cleanupContext, `DELETE FROM werk_core.memberships WHERE id IN ($1::uuid, $2::uuid)`, membershipA, membershipB)
		_, _ = ownerConnection.Exec(cleanupContext, `DELETE FROM werk_core.persons WHERE party_id = $1::uuid`, partyA)
		_, _ = ownerConnection.Exec(cleanupContext, `DELETE FROM werk_core.organizations WHERE party_id = $1::uuid`, partyB)
		_, _ = ownerConnection.Exec(cleanupContext, `DELETE FROM werk_core.parties WHERE id IN ($1::uuid, $2::uuid)`, partyA, partyB)
		_, _ = ownerConnection.Exec(cleanupContext, `DELETE FROM werk_core.organizational_units WHERE id IN ($1::uuid, $2::uuid)`, unitA.ID.String(), unitB.ID.String())
		_, _ = ownerConnection.Exec(cleanupContext, `DELETE FROM werk_core.tenants WHERE id IN ($1::uuid, $2::uuid)`, tenantA.ID.String(), tenantB.ID.String())
	}()

	assertDatabaseSecurityCatalog(t, ctx, ownerConnection)
	assertPlatformResourceRegistry(t, ctx, ownerConnection)
	assertOrganizationalAppAccessFoundation(t, ctx, ownerConnection, tenantA.ID, tenantB.ID, unitA.ID, unitB.ID)
	assertCrossTenantParentRejected(t, ctx, ownerConnection, tenantA.ID, unitB.ID)
	assertCrossTenantMembershipRejected(t, ctx, ownerConnection, tenantA.ID, partyA, unitB.ID)

	workDatabase, err := NewRuntime(ctx, workURL, RuntimeOptions{
		ExpectedRole:    WorkRuntimeRole,
		ApplicationName: "werk-integration-work",
		MaxConnections:  1,
	})
	if err != nil {
		t.Fatalf("open work runtime: %v", err)
	}
	defer workDatabase.Close()

	assertNoTenantRowsVisible(t, ctx, workDatabase, unitA.ID, unitB.ID)
	assertOnlyTenantVisible(t, ctx, workDatabase, tenantA.ID, unitA.ID, unitB.ID)
	assertOnlyPartyVisible(t, ctx, workDatabase, tenantA.ID, partyA, partyB)
	assertNoTenantRowsVisible(t, ctx, workDatabase, unitA.ID, unitB.ID)

	sentinel := errors.New("force rollback")
	err = workDatabase.WithinTenantRead(ctx, tenantA.ID, func(operationContext context.Context, transaction TenantTx) error {
		var count int
		if err := transaction.QueryRow(operationContext, `SELECT count(*) FROM werk_core.organizational_units`).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("tenant A row count = %d, want 1", count)
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("rollback operation error = %v, want sentinel", err)
	}
	assertOnlyTenantVisible(t, ctx, workDatabase, tenantB.ID, unitB.ID, unitA.ID)

	if recovered := tenantPanic(workDatabase, tenantA.ID); recovered == nil {
		t.Fatal("tenant operation panic was not propagated")
	}
	assertOnlyTenantVisible(t, ctx, workDatabase, tenantB.ID, unitB.ID, unitA.ID)

	if err := workDatabase.WithinTenantWrite(ctx, tenantA.ID, func(operationContext context.Context, transaction TenantTx) error {
		_, err := transaction.Exec(operationContext, `SELECT pg_catalog.set_config('werk.tenant_id', $1, false)`, tenantB.ID.String())
		return err
	}); err != nil {
		t.Fatalf("poison runtime session: %v", err)
	}
	assertNoTenantRowsVisible(t, ctx, workDatabase, unitA.ID, unitB.ID)
	assertRuntimeCannotEscalate(t, ctx, workDatabase)

	workerDatabase, err := NewWorker(ctx, workerURL, "werk-integration-worker")
	if err != nil {
		t.Fatalf("open worker runtime: %v", err)
	}
	defer workerDatabase.Close()
	assertOnlyTenantVisible(t, ctx, workerDatabase.runtime, tenantA.ID, unitA.ID, unitB.ID)
	assertOutboxBoundary(t, ctx, workDatabase, workerDatabase, tenantA.ID, tenantB.ID)
}

func assertOutboxBoundary(t *testing.T, ctx context.Context, workDatabase *RuntimeDB, workerDatabase *WorkerDB, tenantA, tenantB tenancy.TenantID) {
	t.Helper()
	const eventID = "0196f000-0000-7000-8000-000000000091"
	err := workDatabase.WithinTenantWrite(ctx, tenantA, func(ctx context.Context, tx TenantTx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO werk_core.outbox_events (
				id, tenant_id, event_type, producer, subject_kind, subject_id,
				partition_key, occurred_at, correlation_id, payload
			) VALUES (
				$1::uuid, $2::uuid, 'core.integration.created.v1', 'core.integration',
				'integration.item', $1::uuid, 'integration:91', now(), $1::uuid, '{}'::jsonb
			)
		`, eventID, tenantA.String())
		return err
	})
	if err != nil {
		t.Fatalf("tenant producer cannot enqueue outbox event: %v", err)
	}
	err = workDatabase.WithinTenantWrite(ctx, tenantA, func(ctx context.Context, tx TenantTx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO werk_core.outbox_events (
				id, tenant_id, event_type, producer, subject_kind, subject_id,
				partition_key, occurred_at, correlation_id, payload
			) VALUES (
				'0196f000-0000-7000-8000-000000000092', $1::uuid,
				'core.integration.created.v1', 'core.integration', 'integration.item',
				'0196f000-0000-7000-8000-000000000092', 'integration:92', now(),
				'0196f000-0000-7000-8000-000000000092', '{}'::jsonb
			)
		`, tenantB.String())
		return err
	})
	if err == nil {
		t.Fatal("tenant producer enqueued an event for another tenant")
	}
	err = workerDatabase.WithinGlobalWrite(ctx, func(ctx context.Context, tx TenantTx) error {
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM werk_core.outbox_events WHERE id = $1::uuid`, eventID).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("worker outbox count = %d, want 1", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("worker cannot read global outbox infrastructure: %v", err)
	}
}

func TestPrivilegedLoginIsRejectedAsRuntimeIntegration(t *testing.T) {
	migratorURL := integrationEnvironment(t, "WERK_TEST_MIGRATOR_DATABASE_URL")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	database, err := NewRuntime(ctx, migratorURL, RuntimeOptions{ExpectedRole: "werk_migrator"})
	if err == nil {
		database.Close()
		t.Fatal("migrator login was accepted as a runtime database role")
	}
}

func TestBackupLoginIsReadOnlyAndRequiresExplicitReaderRoleIntegration(t *testing.T) {
	backupURL := integrationEnvironment(t, "WERK_TEST_BACKUP_DATABASE_URL")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	backupPool, err := pgxpool.New(ctx, backupURL)
	if err != nil {
		t.Fatalf("open backup pool: %v", err)
	}
	defer backupPool.Close()

	var sessionUser string
	var currentUser string
	var defaultReadOnly string
	if err := backupPool.QueryRow(ctx, `
		SELECT session_user, current_user, pg_catalog.current_setting('default_transaction_read_only')
	`).Scan(&sessionUser, &currentUser, &defaultReadOnly); err != nil {
		t.Fatalf("inspect backup login: %v", err)
	}
	if sessionUser != "werk_backup" || currentUser != "werk_backup" || defaultReadOnly != "on" {
		t.Fatalf("unexpected backup login state: session=%q current=%q read_only=%q", sessionUser, currentUser, defaultReadOnly)
	}
	if err := backupPool.QueryRow(ctx, `SELECT count(*) FROM werk_core.tenants`).Scan(new(int)); err == nil {
		t.Fatal("backup login can read tenant data without assuming its capability role")
	}
	if _, err := backupPool.Exec(ctx, `SET ROLE werk_backup_reader`); err != nil {
		t.Fatalf("assume backup reader role: %v", err)
	}
	var tenantCount int
	if err := backupPool.QueryRow(ctx, `SELECT count(*) FROM werk_core.tenants`).Scan(&tenantCount); err != nil {
		t.Fatalf("backup reader cannot read complete tenant table: %v", err)
	}
	if _, err := backupPool.Exec(ctx, `UPDATE werk_core.tenants SET updated_at = updated_at`); err == nil {
		t.Fatal("backup reader can update tenant data")
	}
	if _, err := backupPool.Exec(ctx, `SET ROLE werk_owner`); err == nil {
		t.Fatal("backup login can assume werk_owner")
	}
}

func assertOnlyTenantVisible(t *testing.T, ctx context.Context, database *RuntimeDB, tenantID tenancy.TenantID, expected, forbidden tenancy.UnitID) {
	t.Helper()
	err := database.WithinTenantRead(ctx, tenantID, func(operationContext context.Context, transaction TenantTx) error {
		rows, err := transaction.Query(operationContext, `
			SELECT id::text
			FROM werk_core.organizational_units
			WHERE id IN ($1::uuid, $2::uuid)
		`, expected.String(), forbidden.String())
		if err != nil {
			return err
		}
		defer rows.Close()
		var visible []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return err
			}
			visible = append(visible, id)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		sort.Strings(visible)
		if len(visible) != 1 || visible[0] != expected.String() {
			return fmt.Errorf("visible organizational units = %v, want only %s", visible, expected.String())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read tenant %s: %v", tenantID.String(), err)
	}
}

func assertNoTenantRowsVisible(t *testing.T, ctx context.Context, database *RuntimeDB, first, second tenancy.UnitID) {
	t.Helper()
	var count int
	if err := database.pool.QueryRow(ctx, `
		SELECT count(*)
		FROM werk_core.organizational_units
		WHERE id IN ($1::uuid, $2::uuid)
	`, first.String(), second.String()).Scan(&count); err != nil {
		t.Fatalf("query without tenant context: %v", err)
	}
	if count != 0 {
		t.Fatalf("rows visible without tenant context = %d, want 0", count)
	}
}

func assertOnlyPartyVisible(t *testing.T, ctx context.Context, database *RuntimeDB, tenantID tenancy.TenantID, expected, forbidden string) {
	t.Helper()
	err := database.WithinTenantRead(ctx, tenantID, func(operationContext context.Context, transaction TenantTx) error {
		var count int
		if err := transaction.QueryRow(operationContext, `
			SELECT count(*)
			FROM werk_core.parties
			WHERE id IN ($1::uuid, $2::uuid)
		`, expected, forbidden).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("visible parties = %d, want one", count)
		}
		var visibleID string
		if err := transaction.QueryRow(operationContext, `
			SELECT id::text
			FROM werk_core.parties
			WHERE id = $1::uuid
		`, expected).Scan(&visibleID); err != nil {
			return err
		}
		if visibleID != expected {
			return fmt.Errorf("visible party = %s, want %s", visibleID, expected)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read parties for tenant %s: %v", tenantID.String(), err)
	}
	var count int
	if err := database.pool.QueryRow(ctx, `
		SELECT count(*) FROM werk_core.parties WHERE id IN ($1::uuid, $2::uuid)
	`, expected, forbidden).Scan(&count); err != nil {
		t.Fatalf("query parties without tenant context: %v", err)
	}
	if count != 0 {
		t.Fatalf("parties visible without tenant context = %d, want 0", count)
	}
}

func assertRuntimeCannotEscalate(t *testing.T, ctx context.Context, database *RuntimeDB) {
	t.Helper()
	if _, err := database.pool.Exec(ctx, `SET ROLE werk_owner`); err == nil {
		_, _ = database.pool.Exec(ctx, `RESET ROLE`)
		t.Fatal("work runtime can assume werk_owner")
	}
	if _, err := database.pool.Exec(ctx, `SELECT * FROM werk_core.schema_migrations`); err == nil {
		t.Fatal("work runtime can read schema migrations")
	}
	if _, err := database.pool.Exec(ctx, `SELECT * FROM werk_core.admin_subjects`); err == nil {
		t.Fatal("work runtime can read admin subjects")
	}
	if _, err := database.pool.Exec(ctx, `SELECT * FROM werk_core.identity_auth_throttles`); err == nil {
		t.Fatal("work runtime can read authentication throttle state")
	}
	if _, err := database.pool.Exec(ctx, `SELECT * FROM werk_core.identity_mfa_recovery_codes`); err == nil {
		t.Fatal("work runtime can read MFA recovery codes")
	}
	if _, err := database.pool.Exec(ctx, `SELECT * FROM werk_core.app_entitlements`); err == nil {
		t.Fatal("work runtime can read raw app entitlements")
	}
	if _, err := database.pool.Exec(ctx, `TRUNCATE werk_core.organizational_units`); err == nil {
		t.Fatal("work runtime can truncate tenant data")
	}
}

func assertDatabaseSecurityCatalog(t *testing.T, ctx context.Context, ownerConnection *pgxpool.Conn) {
	t.Helper()
	var secureTables int
	if err := ownerConnection.QueryRow(ctx, `
		SELECT count(*)
		FROM pg_catalog.pg_class AS relation
		JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid = relation.relnamespace
		WHERE namespace.nspname = 'werk_core'
		  AND relation.relname IN ('tenants', 'organizational_units')
		  AND relation.relrowsecurity
		  AND relation.relforcerowsecurity
		  AND pg_catalog.pg_get_userbyid(relation.relowner) = 'werk_owner'
	`).Scan(&secureTables); err != nil {
		t.Fatalf("inspect tenant table security: %v", err)
	}
	if secureTables != 2 {
		t.Fatalf("secure tenant table count = %d, want 2", secureTables)
	}
	var secureIdentityTables int
	if err := ownerConnection.QueryRow(ctx, `
		SELECT count(*)
		FROM pg_catalog.pg_class AS relation
		JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid = relation.relnamespace
		WHERE namespace.nspname = 'werk_core'
		  AND relation.relname IN (
		    'identity_mfa_factors', 'identity_mfa_challenges',
		    'identity_mfa_recovery_codes', 'identity_auth_throttles',
		    'identity_account_classes', 'identity_audiences',
		    'identity_account_class_audiences', 'identity_agents',
		    'identity_providers', 'account_identity_bindings'
		  )
		  AND relation.relrowsecurity
		  AND relation.relforcerowsecurity
		  AND pg_catalog.pg_get_userbyid(relation.relowner) = 'werk_owner'
	`).Scan(&secureIdentityTables); err != nil {
		t.Fatalf("inspect identity table security: %v", err)
	}
	if secureIdentityTables != 10 {
		t.Fatalf("secure identity table count = %d, want 10", secureIdentityTables)
	}
	var securePlatformRegistryTables int
	if err := ownerConnection.QueryRow(ctx, `
		SELECT count(*)
		FROM pg_catalog.pg_class AS relation
		JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid = relation.relnamespace
		WHERE namespace.nspname = 'werk_core'
		  AND relation.relname IN (
		    'platform_modules', 'resource_type_registrations', 'permission_resource_types',
		    'resource_data_profiles', 'permission_processing_policies'
		  )
		  AND relation.relrowsecurity
		  AND relation.relforcerowsecurity
		  AND pg_catalog.pg_get_userbyid(relation.relowner) = 'werk_owner'
	`).Scan(&securePlatformRegistryTables); err != nil {
		t.Fatalf("inspect platform registry security: %v", err)
	}
	if securePlatformRegistryTables != 5 {
		t.Fatalf("secure platform registry table count = %d, want 5", securePlatformRegistryTables)
	}
	var secureAppAccessTables int
	if err := ownerConnection.QueryRow(ctx, `
		SELECT count(*)
		FROM pg_catalog.pg_class AS relation
		JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid = relation.relnamespace
		WHERE namespace.nspname = 'werk_core'
		  AND relation.relname IN (
		    'tenant_app_installations', 'access_groups',
		    'access_group_memberships', 'app_entitlements'
		  )
		  AND relation.relrowsecurity
		  AND relation.relforcerowsecurity
		  AND pg_catalog.pg_get_userbyid(relation.relowner) = 'werk_owner'
	`).Scan(&secureAppAccessTables); err != nil {
		t.Fatalf("inspect app access table security: %v", err)
	}
	if secureAppAccessTables != 4 {
		t.Fatalf("secure app access table count = %d, want 4", secureAppAccessTables)
	}

	var backupRolesValid bool
	if err := ownerConnection.QueryRow(ctx, `
		SELECT
			EXISTS (
				SELECT 1 FROM pg_catalog.pg_roles
				WHERE rolname = 'werk_backup_reader'
				  AND NOT rolcanlogin AND NOT rolsuper AND NOT rolcreatedb
				  AND NOT rolcreaterole AND NOT rolinherit AND NOT rolreplication
				  AND rolbypassrls
			)
			AND EXISTS (
				SELECT 1 FROM pg_catalog.pg_roles
				WHERE rolname = 'werk_backup'
				  AND rolcanlogin AND NOT rolsuper AND NOT rolcreatedb
				  AND NOT rolcreaterole AND NOT rolinherit AND NOT rolreplication
				  AND NOT rolbypassrls AND rolconnlimit = 1
			)
			AND EXISTS (
				SELECT 1
				FROM pg_catalog.pg_auth_members AS membership
				JOIN pg_catalog.pg_roles AS granted_role ON granted_role.oid = membership.roleid
				JOIN pg_catalog.pg_roles AS member_role ON member_role.oid = membership.member
				WHERE granted_role.rolname = 'werk_backup_reader'
				  AND member_role.rolname = 'werk_backup'
				  AND NOT membership.inherit_option
				  AND membership.set_option
			)
	`).Scan(&backupRolesValid); err != nil {
		t.Fatalf("inspect backup roles: %v", err)
	}
	if !backupRolesValid {
		t.Fatal("backup roles do not satisfy the security contract")
	}

	var missingBackupGrants int
	if err := ownerConnection.QueryRow(ctx, `
		SELECT count(*)
		FROM pg_catalog.pg_class AS relation
		JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid = relation.relnamespace
		WHERE namespace.nspname IN ('werk_core', 'werk_security')
		  AND relation.relkind IN ('r', 'p', 'S', 'v', 'm', 'f')
		  AND pg_catalog.pg_get_userbyid(relation.relowner) = 'werk_owner'
		  AND NOT pg_catalog.has_table_privilege('werk_backup_reader', relation.oid, 'SELECT')
	`).Scan(&missingBackupGrants); err != nil {
		t.Fatalf("inspect backup reader grants: %v", err)
	}
	if missingBackupGrants != 0 {
		t.Fatalf("WERK relations without backup SELECT grant = %d, want 0", missingBackupGrants)
	}
}

func assertOrganizationalAppAccessFoundation(
	t *testing.T,
	ctx context.Context,
	ownerConnection *pgxpool.Conn,
	tenantA tenancy.TenantID,
	tenantB tenancy.TenantID,
	unitA tenancy.UnitID,
	unitB tenancy.UnitID,
) {
	t.Helper()
	const (
		appModule     = "app.integration-access"
		groupID       = "0196f000-0000-7000-8000-000000000051"
		membershipID  = "0196f000-0000-7000-8000-000000000052"
		entitlementID = "0196f000-0000-7000-8000-000000000053"
	)
	defer func() {
		cleanupContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = ownerConnection.Exec(cleanupContext, `DELETE FROM werk_core.app_entitlements WHERE id = $1::uuid`, entitlementID)
		_, _ = ownerConnection.Exec(cleanupContext, `DELETE FROM werk_core.access_group_memberships WHERE id = $1::uuid`, membershipID)
		_, _ = ownerConnection.Exec(cleanupContext, `DELETE FROM werk_core.access_groups WHERE id = $1::uuid`, groupID)
		_, _ = ownerConnection.Exec(cleanupContext, `DELETE FROM werk_core.tenant_app_installations WHERE tenant_id = $1::uuid AND app_module = $2`, tenantA.String(), appModule)
		_, _ = ownerConnection.Exec(cleanupContext, `DELETE FROM werk_core.platform_modules WHERE module_key = $1`, appModule)
	}()

	if _, err := ownerConnection.Exec(ctx, `
		INSERT INTO werk_core.platform_modules (module_key, module_kind, display_name)
		VALUES ($1, 'app', 'Integration Access App')
	`, appModule); err != nil {
		t.Fatalf("insert app module fixture: %v", err)
	}
	if _, err := ownerConnection.Exec(ctx, `
		INSERT INTO werk_core.tenant_app_installations (tenant_id, app_module)
		VALUES ($1::uuid, $2)
	`, tenantA.String(), appModule); err != nil {
		t.Fatalf("insert tenant app installation: %v", err)
	}
	if _, err := ownerConnection.Exec(ctx, `
		INSERT INTO werk_core.access_groups (id, tenant_id, group_key, display_name, governing_unit_id)
		VALUES ($1::uuid, $2::uuid, 'integration.reviewers', 'Integration Reviewers', $3::uuid)
	`, groupID, tenantA.String(), unitA.String()); err != nil {
		t.Fatalf("insert access group: %v", err)
	}
	if _, err := ownerConnection.Exec(ctx, `
		INSERT INTO werk_core.access_group_memberships (
			id, tenant_id, access_group_id, organizational_unit_id, include_descendants
		) VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, true)
	`, membershipID, tenantA.String(), groupID, unitA.String()); err != nil {
		t.Fatalf("insert organizational access group edge: %v", err)
	}
	if _, err := ownerConnection.Exec(ctx, `
		INSERT INTO werk_core.app_entitlements (id, tenant_id, app_module, access_group_id)
		VALUES ($1::uuid, $2::uuid, $3, $4::uuid)
	`, entitlementID, tenantA.String(), appModule, groupID); err != nil {
		t.Fatalf("insert app entitlement: %v", err)
	}

	var entitlementCount int
	if err := ownerConnection.QueryRow(ctx, `
		SELECT count(*) FROM werk_core.app_entitlements
		WHERE tenant_id = $1::uuid AND app_module = $2 AND status = 'active'
	`, tenantA.String(), appModule).Scan(&entitlementCount); err != nil {
		t.Fatalf("read app entitlement fixture: %v", err)
	}
	if entitlementCount != 1 {
		t.Fatalf("active app entitlement count = %d, want 1", entitlementCount)
	}

	if _, err := ownerConnection.Exec(ctx, `
		INSERT INTO werk_core.tenant_app_installations (tenant_id, app_module)
		VALUES ($1::uuid, 'core.workspace')
	`, tenantB.String()); err == nil {
		t.Fatal("core module was accepted as a tenant app installation")
	}
	if _, err := ownerConnection.Exec(ctx, `
		INSERT INTO werk_core.access_groups (id, tenant_id, group_key, display_name, governing_unit_id)
		VALUES ('0196f000-0000-7000-8000-000000000054', $1::uuid, 'invalid.cross-tenant', 'Invalid', $2::uuid)
	`, tenantA.String(), unitB.String()); err == nil {
		t.Fatal("access group accepted a governing unit from another tenant")
	}
	if _, err := ownerConnection.Exec(ctx, `
		INSERT INTO werk_core.app_entitlements (
			id, tenant_id, app_module, organizational_unit_id
		) VALUES ('0196f000-0000-7000-8000-000000000055', $1::uuid, $2, $3::uuid)
	`, tenantA.String(), appModule, unitB.String()); err == nil {
		t.Fatal("app entitlement accepted an organizational unit from another tenant")
	}
}

func assertPlatformResourceRegistry(t *testing.T, ctx context.Context, ownerConnection *pgxpool.Conn) {
	t.Helper()
	var unmappedPermissions int
	if err := ownerConnection.QueryRow(ctx, `
		SELECT count(*)
		FROM werk_core.permissions AS permission
		LEFT JOIN werk_core.permission_resource_types AS target
		  ON target.permission_id = permission.id
		WHERE permission.status = 'active' AND target.permission_id IS NULL
	`).Scan(&unmappedPermissions); err != nil {
		t.Fatalf("inspect permission resource mappings: %v", err)
	}
	if unmappedPermissions != 0 {
		t.Fatalf("active permissions without resource mapping = %d, want 0", unmappedPermissions)
	}
	var unclassifiedResourceTypes int
	if err := ownerConnection.QueryRow(ctx, `
		SELECT count(*)
		FROM werk_core.resource_type_registrations AS resource_type
		LEFT JOIN werk_core.resource_data_profiles AS data_profile
		  ON data_profile.resource_kind = resource_type.resource_kind
		 AND data_profile.status = 'active'
		WHERE resource_type.status = 'active' AND data_profile.resource_kind IS NULL
	`).Scan(&unclassifiedResourceTypes); err != nil {
		t.Fatalf("inspect resource data profiles: %v", err)
	}
	if unclassifiedResourceTypes != 0 {
		t.Fatalf("active resource types without active data profile = %d, want 0", unclassifiedResourceTypes)
	}
	var unclassifiedPermissionTargets int
	if err := ownerConnection.QueryRow(ctx, `
		SELECT count(*)
		FROM werk_core.permission_resource_types AS target
		JOIN werk_core.permissions AS permission
		  ON permission.id = target.permission_id AND permission.status = 'active'
		LEFT JOIN werk_core.permission_processing_policies AS processing
		  ON processing.permission_id = target.permission_id
		 AND processing.resource_kind = target.resource_kind
		 AND processing.status = 'active'
		WHERE processing.permission_id IS NULL
	`).Scan(&unclassifiedPermissionTargets); err != nil {
		t.Fatalf("inspect permission processing policies: %v", err)
	}
	if unclassifiedPermissionTargets != 0 {
		t.Fatalf("active permission targets without processing policy = %d, want 0", unclassifiedPermissionTargets)
	}

	var boundary string
	if err := ownerConnection.QueryRow(ctx, `
		SELECT boundary
		FROM werk_core.resource_type_registrations
		WHERE resource_kind = 'core.workspace.workspace' AND status = 'active'
	`).Scan(&boundary); err != nil {
		t.Fatalf("read workspace resource registration: %v", err)
	}
	if boundary != "tenant" {
		t.Fatalf("workspace boundary = %q, want tenant", boundary)
	}

	transaction, err := ownerConnection.Begin(ctx)
	if err != nil {
		t.Fatalf("begin invalid resource registration check: %v", err)
	}
	if _, err := transaction.Exec(ctx, `
		INSERT INTO werk_core.resource_type_registrations (
			resource_kind, owner_module, display_name, boundary
		) VALUES ('core.identity.foreign-test', 'core.tenancy', 'Invalid foreign namespace', 'installation')
	`); err == nil {
		_ = transaction.Rollback(ctx)
		t.Fatal("resource registration outside its owner namespace was accepted")
	}
	if err := transaction.Rollback(ctx); err != nil {
		t.Fatalf("rollback invalid resource registration check: %v", err)
	}

	transaction, err = ownerConnection.Begin(ctx)
	if err != nil {
		t.Fatalf("begin invalid data profile check: %v", err)
	}
	if _, err := transaction.Exec(ctx, `
		UPDATE werk_core.resource_data_profiles
		SET processing_activity_required = false
		WHERE resource_kind = 'core.identity.work-account'
	`); err == nil {
		_ = transaction.Rollback(ctx)
		t.Fatal("personal resource profile accepted without processing activity requirement")
	}
	if err := transaction.Rollback(ctx); err != nil {
		t.Fatalf("rollback invalid data profile check: %v", err)
	}

	transaction, err = ownerConnection.Begin(ctx)
	if err != nil {
		t.Fatalf("begin invalid processing policy check: %v", err)
	}
	if _, err := transaction.Exec(ctx, `
		UPDATE werk_core.permission_processing_policies
		SET processing_required = false
		WHERE activity_key IS NOT NULL
	`); err == nil {
		_ = transaction.Rollback(ctx)
		t.Fatal("processing policy accepted a context while processing was declared unnecessary")
	}
	if err := transaction.Rollback(ctx); err != nil {
		t.Fatalf("rollback invalid processing policy check: %v", err)
	}

	transaction, err = ownerConnection.Begin(ctx)
	if err != nil {
		t.Fatalf("begin incomplete processing policy check: %v", err)
	}
	if _, err := transaction.Exec(ctx, `
		UPDATE werk_core.permission_processing_policies
		SET activity_key = NULL
		WHERE processing_required
	`); err == nil {
		_ = transaction.Rollback(ctx)
		t.Fatal("required processing policy accepted an incomplete context")
	}
	if err := transaction.Rollback(ctx); err != nil {
		t.Fatalf("rollback incomplete processing policy check: %v", err)
	}
}

func assertCrossTenantParentRejected(t *testing.T, ctx context.Context, ownerConnection *pgxpool.Conn, tenantID tenancy.TenantID, foreignParent tenancy.UnitID) {
	t.Helper()
	unitID, err := tenancy.NewUnitID()
	if err != nil {
		t.Fatalf("new cross-tenant unit ID: %v", err)
	}
	if _, err := ownerConnection.Exec(ctx, `
		INSERT INTO werk_core.organizational_units (id, tenant_id, parent_id, unit_type, name, status)
		VALUES ($1::uuid, $2::uuid, $3::uuid, 'team', 'Invalid cross tenant parent', 'active')
	`, unitID.String(), tenantID.String(), foreignParent.String()); err == nil {
		t.Fatal("cross-tenant organizational parent was accepted")
	}
}

func assertCrossTenantMembershipRejected(t *testing.T, ctx context.Context, ownerConnection *pgxpool.Conn, tenantID tenancy.TenantID, partyID string, foreignUnit tenancy.UnitID) {
	t.Helper()
	membershipID := "0196f000-0000-7000-8000-000000000049"
	if _, err := ownerConnection.Exec(ctx, `
		INSERT INTO werk_core.memberships (
			id, tenant_id, party_id, organizational_unit_id, membership_type, valid_from
		) VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, 'team.member', '2026-07-19T00:00:00Z')
	`, membershipID, tenantID.String(), partyID, foreignUnit.String()); err == nil {
		t.Fatal("cross-tenant membership was accepted")
	}
}

func tenantPanic(database *RuntimeDB, tenantID tenancy.TenantID) (recovered any) {
	defer func() { recovered = recover() }()
	_ = database.WithinTenantRead(context.Background(), tenantID, func(context.Context, TenantTx) error {
		panic("integration panic")
	})
	return nil
}

func integrationEnvironment(t *testing.T, key string) string {
	t.Helper()
	value := os.Getenv(key)
	if value == "" {
		t.Skip("PostgreSQL integration environment is not configured")
	}
	return value
}
