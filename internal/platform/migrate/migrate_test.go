package migrate

import (
	"strconv"
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

func TestEmbeddedMigrationsHaveUniqueNumericVersions(t *testing.T) {
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	seen := make(map[uint64]string, len(migrations))
	for _, migration := range migrations {
		versionText, _, found := strings.Cut(migration.name, "_")
		if !found || len(versionText) != 6 {
			t.Fatalf("migration %q has no six-digit numeric version", migration.name)
		}
		version, err := strconv.ParseUint(versionText, 10, 64)
		if err != nil || version == 0 {
			t.Fatalf("migration %q has invalid version %q", migration.name, versionText)
		}
		if existing, duplicate := seen[version]; duplicate {
			t.Fatalf("migration version %06d is shared by %q and %q", version, existing, migration.name)
		}
		seen[version] = migration.name
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

func TestDocumentReadContractUsesCollectionPermissionWithoutStorageAccess(t *testing.T) {
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	for _, migration := range migrations {
		if migration.name != "000032_document_read_contract.sql" {
			continue
		}
		for _, fragment := range []string{
			"core.documents.document.list",
			"core.documents.collection",
			"core.documents.document-use",
			"created-by-me visibility rule",
			"idx_documents_tenant_creator_updated",
		} {
			if !strings.Contains(migration.contents, fragment) {
				t.Errorf("document read migration is missing %q", fragment)
			}
		}
		for _, forbidden := range []string{"storage_blobs", "storage_blob_locations", "GRANT SELECT"} {
			if strings.Contains(migration.contents, forbidden) {
				t.Errorf("document read contract unexpectedly widens storage access with %q", forbidden)
			}
		}
		return
	}
	t.Fatal("document read contract migration is not embedded")
}

func TestDocumentDirectVisibilityContractKeepsBindingLocalAndRevocable(t *testing.T) {
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	for _, migration := range migrations {
		if migration.name != "000034_document_direct_visibility.sql" {
			continue
		}
		for _, fragment := range []string{
			"core.documents.document.visibility-manage",
			"document_account_visibility_bindings",
			"document-visibility-granted.v1",
			"document-visibility-revoked.v1",
			"document_account_visibility_active_idx",
			"AS RESTRICTIVE TO werk_work_runtime, werk_service_runtime",
			"document visibility bindings cannot be deleted",
		} {
			if !strings.Contains(migration.contents, fragment) {
				t.Errorf("document visibility migration is missing %q", fragment)
			}
		}
		for _, forbidden := range []string{
			"TO werk_admin_runtime;",
			"TO werk_worker_runtime;",
			"GRANT DELETE",
			"principal_kind",
			"allow_deny",
		} {
			if strings.Contains(migration.contents, forbidden) {
				t.Errorf("document visibility contract unexpectedly contains %q", forbidden)
			}
		}
		return
	}
	t.Fatal("document visibility migration is not embedded")
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

func TestIdentitySessionGenerationFailsClosedAcrossSecurityChanges(t *testing.T) {
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	for _, migration := range migrations {
		if migration.name != "000031_identity_session_generation.sql" {
			continue
		}
		for _, fragment := range []string{
			"ADD COLUMN session_generation bigint NOT NULL DEFAULT 1",
			"ALTER COLUMN session_generation DROP DEFAULT",
			"CREATE FUNCTION werk_security.validate_session_generation()",
			"NEW.session_generation <> current_generation",
			"CREATE TRIGGER sessions_validate_generation",
			"CREATE TRIGGER identity_mfa_challenges_validate_generation",
			"REVOKE ALL ON FUNCTION werk_security.validate_session_generation() FROM PUBLIC",
			"CREATE FUNCTION werk_security.protect_account_session_generation()",
			"NEW.session_generation < OLD.session_generation",
			"CREATE TRIGGER accounts_protect_session_generation",
		} {
			if !strings.Contains(migration.contents, fragment) {
				t.Errorf("identity session generation migration is missing %q", fragment)
			}
		}
		if count := strings.Count(migration.contents, "ALTER COLUMN session_generation DROP DEFAULT"); count != 2 {
			t.Errorf("identity session generation migration drops %d unsafe defaults, want 2", count)
		}
		return
	}
	t.Fatal("identity session generation migration is not embedded")
}

func TestPlatformServiceProviderRegistryKeepsMetadataAndRuntimeAuthoritySeparate(t *testing.T) {
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	for _, migration := range migrations {
		if migration.name != "000033_platform_service_provider_registry.sql" {
			continue
		}
		for _, fragment := range []string{
			"CREATE TABLE werk_core.platform_service_contracts",
			"CREATE TABLE werk_core.platform_service_capability_contracts",
			"CREATE TABLE werk_core.platform_provider_registrations",
			"CREATE TABLE werk_core.platform_provider_capability_bindings",
			"CHECK (service_key LIKE owner_module || '.service.%')",
			"CHECK (capability_key LIKE service_key || '.capability.%')",
			"CHECK (provider_key LIKE service_key || '.provider.%')",
			"UNIQUE NULLS NOT DISTINCT",
			"config_scope = 'installation' AND tenant_id IS NULL",
			"config_scope = 'tenant' AND tenant_id IS NOT NULL",
			"lifecycle text NOT NULL DEFAULT 'disabled'",
			"retired provider registry entries cannot be reactivated",
			"OLD.registry_contract_version <> NEW.registry_contract_version",
			"OLD.created_at IS DISTINCT FROM NEW.created_at",
			"tenant-scoped provider cannot implement an installation-bound capability",
			"platform_provider_capability_bindings_validate_scope",
			"provider registry revision must increase by one",
			"provider registry entries must be retired, not deleted",
			"ALTER TABLE werk_core.platform_provider_registrations FORCE ROW LEVEL SECURITY",
			"TO werk_admin_runtime, werk_backup_reader",
		} {
			if !strings.Contains(migration.contents, fragment) {
				t.Errorf("service/provider registry migration is missing %q", fragment)
			}
		}
		if count := strings.Count(migration.contents, "CREATE TABLE werk_core.platform_"); count != 4 {
			t.Errorf("service/provider registry creates %d platform tables, want 4", count)
		}
		if count := strings.Count(migration.contents, "OLD.created_at IS DISTINCT FROM NEW.created_at"); count != 4 {
			t.Errorf("service/provider registry protects created_at in %d tables, want 4", count)
		}
		for _, forbidden := range []string{
			"secret_value", "credential_value", "endpoint_url", "configuration jsonb",
			"health_status", "bucket_name", "object_path",
			"INSERT INTO werk_core.platform_provider_registrations",
		} {
			if strings.Contains(migration.contents, forbidden) {
				t.Errorf("service/provider registry contains forbidden runtime/configuration state %q", forbidden)
			}
		}
		return
	}
	t.Fatal("service/provider registry migration is not embedded")
}
