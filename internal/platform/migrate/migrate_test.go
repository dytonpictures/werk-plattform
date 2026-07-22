package migrate

import (
	"strings"
	"testing"
)

func TestEmbeddedMigrationsAreOrdered(t *testing.T) {
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	if len(migrations) < 2 {
		t.Fatalf("migration count = %d, want at least 2", len(migrations))
	}
	for index := 1; index < len(migrations); index++ {
		if migrations[index-1].name >= migrations[index].name {
			t.Fatalf("migrations are not strictly ordered: %q before %q", migrations[index-1].name, migrations[index].name)
		}
	}
}

func TestOrganizationalParentMigrationUsesTenantBoundary(t *testing.T) {
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	for _, migration := range migrations {
		if migration.name != "000002_organizational_unit_tenant_boundary.sql" {
			continue
		}
		if !strings.Contains(migration.contents, "FOREIGN KEY (tenant_id, parent_id)") ||
			!strings.Contains(migration.contents, "REFERENCES werk_core.organizational_units (tenant_id, id)") {
			t.Fatal("organization parent migration does not enforce the tenant boundary")
		}
		return
	}
	t.Fatal("tenant boundary migration is not embedded")
}

func TestTenantRLSMigrationIsFailClosed(t *testing.T) {
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	for _, migration := range migrations {
		if migration.name != "000003_tenant_rls.sql" {
			continue
		}
		requiredFragments := []string{
			"NULLIF(pg_catalog.current_setting('werk.tenant_id', true), '')::uuid",
			"ENABLE ROW LEVEL SECURITY",
			"FORCE ROW LEVEL SECURITY",
			"AS RESTRICTIVE",
			"TO werk_work_runtime, werk_service_runtime, werk_worker_runtime",
		}
		for _, fragment := range requiredFragments {
			if !strings.Contains(migration.contents, fragment) {
				t.Errorf("tenant RLS migration is missing %q", fragment)
			}
		}
		return
	}
	t.Fatal("tenant RLS migration is not embedded")
}

func TestBackupReaderMigrationCoversCurrentAndFutureObjects(t *testing.T) {
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	for _, migration := range migrations {
		if migration.name != "000004_backup_reader_grants.sql" {
			continue
		}
		requiredFragments := []string{
			"GRANT USAGE ON SCHEMA werk_core, werk_security TO werk_backup_reader",
			"GRANT SELECT ON ALL TABLES IN SCHEMA werk_core, werk_security",
			"GRANT SELECT ON ALL SEQUENCES IN SCHEMA werk_core, werk_security",
			"ALTER DEFAULT PRIVILEGES FOR ROLE werk_owner IN SCHEMA werk_core",
			"ALTER DEFAULT PRIVILEGES FOR ROLE werk_owner IN SCHEMA werk_security",
		}
		for _, fragment := range requiredFragments {
			if !strings.Contains(migration.contents, fragment) {
				t.Errorf("backup reader migration is missing %q", fragment)
			}
		}
		return
	}
	t.Fatal("backup reader migration is not embedded")
}

func TestTenantAdministrationMigrationKeepsMutationsTenantBound(t *testing.T) {
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	for _, migration := range migrations {
		if migration.name != "000018_tenant_administration.sql" {
			continue
		}
		for _, fragment := range []string{
			"REVOKE DELETE ON werk_core.tenants, werk_core.organizational_units FROM werk_admin_runtime",
			"WITH CHECK (id = werk_security.current_tenant_id())",
			"USING (tenant_id = werk_security.current_tenant_id())",
			"core.tenancy.tenant.create",
			"core.tenancy.organizational-unit.create",
		} {
			if !strings.Contains(migration.contents, fragment) {
				t.Errorf("tenant administration migration is missing %q", fragment)
			}
		}
		return
	}
	t.Fatal("tenant administration migration is not embedded")
}

func TestAdminEntityUpdateMigrationProtectsSystemRoles(t *testing.T) {
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	for _, migration := range migrations {
		if migration.name != "000021_admin_entity_updates.sql" {
			continue
		}
		for _, fragment := range []string{
			"GRANT DELETE ON werk_core.role_permissions TO werk_admin_runtime",
			"GRANT SELECT (id, status) ON werk_core.tenants TO werk_identity_runtime",
			"AND NOT role.system_role",
			"roles_protect_system_update",
			"core.tenancy.tenant.update",
			"core.tenancy.organizational-unit.update",
			"core.authorization.work-role.update",
		} {
			if !strings.Contains(migration.contents, fragment) {
				t.Errorf("admin entity update migration is missing %q", fragment)
			}
		}
		return
	}
	t.Fatal("admin entity update migration is not embedded")
}

func TestIdentityProviderMigrationKeepsDimensionsSeparate(t *testing.T) {
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	for _, migration := range migrations {
		if migration.name != "000023_identity_provider_and_credential_contract.sql" {
			continue
		}
		for _, fragment := range []string{
			"CREATE TABLE werk_core.identity_account_classes",
			"CREATE TABLE werk_core.identity_audiences",
			"CREATE TABLE werk_core.identity_account_class_audiences",
			"CREATE TABLE werk_core.identity_agents",
			"CREATE TABLE werk_core.identity_providers",
			"CREATE TABLE werk_core.account_identity_bindings",
			"ADD CONSTRAINT account_credentials_pkey PRIMARY KEY (id)",
			"sessions_validate_identity_boundary",
			"accounts_protect_identity_boundary",
			"account_class_value = 'agent' AND NEW.access_plane = 'service'",
		} {
			if !strings.Contains(migration.contents, fragment) {
				t.Errorf("identity provider migration is missing %q", fragment)
			}
		}
		return
	}
	t.Fatal("identity provider migration is not embedded")
}

func TestPlatformResourceRegistryBindsPermissionsToResourceTypes(t *testing.T) {
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	for _, migration := range migrations {
		if migration.name != "000024_platform_resource_registry.sql" {
			continue
		}
		for _, fragment := range []string{
			"CREATE TABLE werk_core.platform_modules",
			"CREATE TABLE werk_core.resource_type_registrations",
			"CREATE TABLE werk_core.permission_resource_types",
			"ADD COLUMN contract_version bigint NOT NULL DEFAULT 1",
			"FOREIGN KEY (owning_module) REFERENCES werk_core.platform_modules(module_key)",
			"CHECK (resource_kind LIKE owner_module || '.%')",
			"core.workspace.workspace",
			"permission_resource_types_identity_read",
			"FORCE ROW LEVEL SECURITY",
		} {
			if !strings.Contains(migration.contents, fragment) {
				t.Errorf("platform resource registry migration is missing %q", fragment)
			}
		}
		return
	}
	t.Fatal("platform resource registry migration is not embedded")
}

func TestResourceDataComplianceProfilesAreMandatoryAndFailClosed(t *testing.T) {
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	for _, migration := range migrations {
		if migration.name != "000025_resource_data_compliance_profiles.sql" {
			continue
		}
		for _, fragment := range []string{
			"CREATE TABLE werk_core.resource_data_profiles",
			"personal_data_category IN ('none', 'personal', 'special-category', 'criminal-offence')",
			"CHECK (personal_data_category = 'none' OR processing_activity_required)",
			"resource_data_profiles_identity_read",
			"FORCE ROW LEVEL SECURITY",
			"does not establish a legal basis for processing",
		} {
			if !strings.Contains(migration.contents, fragment) {
				t.Errorf("resource data compliance migration is missing %q", fragment)
			}
		}
		return
	}
	t.Fatal("resource data compliance migration is not embedded")
}

func TestPermissionProcessingPoliciesAreMandatoryAndServerControlled(t *testing.T) {
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	for _, migration := range migrations {
		if migration.name != "000026_permission_processing_policies.sql" {
			continue
		}
		for _, fragment := range []string{
			"CREATE TABLE werk_core.permission_processing_policies",
			"REFERENCES werk_core.permission_resource_types(permission_id, resource_kind)",
			"processing_required",
			"operator.processing-register.identity-access",
			"permission_processing_policies_identity_read",
			"FORCE ROW LEVEL SECURITY",
			"never supplied by a client request",
		} {
			if !strings.Contains(migration.contents, fragment) {
				t.Errorf("permission processing policy migration is missing %q", fragment)
			}
		}
		return
	}
	t.Fatal("permission processing policy migration is not embedded")
}

func TestOrganizationalAppAccessFoundationIsTenantBoundAndExplicit(t *testing.T) {
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	for _, migration := range migrations {
		if migration.name != "000027_organizational_app_access_foundation.sql" {
			continue
		}
		for _, fragment := range []string{
			"CREATE TABLE werk_core.tenant_app_installations",
			"CREATE TABLE werk_core.access_groups",
			"CREATE TABLE werk_core.access_group_memberships",
			"CREATE TABLE werk_core.app_entitlements",
			"num_nonnulls(account_id, organizational_unit_id, access_group_id) = 1",
			"FOREIGN KEY (tenant_id, access_group_id)",
			"validate_human_app_access_subject",
			"tenant_id = werk_security.current_tenant_id()",
			"nested access groups are intentionally unsupported",
			"permissions remain separate",
			"FORCE ROW LEVEL SECURITY",
		} {
			if !strings.Contains(migration.contents, fragment) {
				t.Errorf("organizational app access migration is missing %q", fragment)
			}
		}
		return
	}
	t.Fatal("organizational app access migration is not embedded")
}

func TestDocumentStorageFoundationIsTenantBoundAndInactive(t *testing.T) {
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	for _, migration := range migrations {
		if migration.name != "000029_document_storage_foundation.sql" {
			continue
		}
		for _, fragment := range []string{
			"('core.documents', 'core', 'Dokumente')",
			"('core.storage', 'core', 'Objektspeicher')",
			"core.documents.document-version",
			"CREATE TABLE werk_core.storage_blobs",
			"CREATE TABLE werk_core.storage_blob_locations",
			"CREATE TABLE werk_core.documents",
			"CREATE TABLE werk_core.document_versions",
			"CREATE TABLE werk_core.document_classification_revisions",
			"FOREIGN KEY (tenant_id, blob_id)",
			"document_versions_protect_immutable",
			"document_classification_revisions_protect_immutable",
			"documents_validate_initial_records",
			"documents_validate_insert",
			"storage_blob_locations_validate_blob_consistency",
			"FOR UPDATE;",
			"activated storage location verification is immutable",
			"state IN ('quarantined', 'available', 'rejected', 'missing', 'unknown')",
			"AS RESTRICTIVE TO werk_work_runtime, werk_service_runtime",
			"REVOKE ALL ON",
			"Physical deletion is intentionally not part of this foundation",
		} {
			if !strings.Contains(migration.contents, fragment) {
				t.Errorf("document/storage migration is missing %q", fragment)
			}
		}
		for _, forbidden := range []string{
			"CREATE TABLE werk_core.storage_transfer_tickets",
			"CREATE TABLE werk_core.collaboration",
			"GRANT DELETE ON",
		} {
			if strings.Contains(migration.contents, forbidden) {
				t.Errorf("document/storage foundation already contains deferred capability %q", forbidden)
			}
		}
		return
	}
	t.Fatal("document/storage foundation migration is not embedded")
}

func TestBusinessAuditContractKeepsDualActorsAndServerPolicy(t *testing.T) {
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	for _, migration := range migrations {
		if migration.name != "000030_business_audit_contract.sql" {
			continue
		}
		for _, fragment := range []string{
			"ADD COLUMN initiated_by_account_id uuid NULL",
			"ADD COLUMN executed_by_account_id uuid NULL",
			"ADD COLUMN subject_kind text NULL",
			"ADD COLUMN policy_contract_version bigint NULL",
			"security_audit_business_shape_check",
			"CREATE TABLE werk_core.audit_action_contracts",
			"core.documents.document-published.v1",
			"REFERENCES werk_core.permission_resource_types(permission_id, resource_kind)",
			"action_contract.event_type = NEW.event_type",
			"action_contract.resource_kind = NEW.subject_kind",
			"audit_action_contracts_protect_meaning",
			"OLD.status = 'active'",
			"NEW.status = 'retired'",
			"audit action contract meaning is immutable",
			"validate_business_audit_entry",
			"registration.boundary = NEW.subject_boundary",
			"business audit policy snapshot does not match active server policy",
			"security_audit_protect_immutable",
			"GRANT INSERT ON werk_core.security_audit_events TO werk_service_runtime",
			"DROP POLICY security_audit_identity_insert",
			"event_type LIKE 'identity.%'",
			"DROP POLICY security_audit_admin_insert",
			"TO werk_admin_runtime",
			"tenant_id = werk_security.current_tenant_id()",
			"subject_tenant_id = werk_security.current_tenant_id()",
			"event_type LIKE 'core.documents.%' AND action_key LIKE 'core.documents.%'",
			"Titles, object paths, transfer tickets, hashes, and credentials are forbidden",
		} {
			if !strings.Contains(migration.contents, fragment) {
				t.Errorf("business audit migration is missing %q", fragment)
			}
		}
		return
	}
	t.Fatal("business audit migration is not embedded")
}
