package adminstore

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/tenancy"
	"github.com/dytonpictures/werk/internal/platform/database"
	"github.com/dytonpictures/werk/internal/platform/identitystore"
)

func TestCreateWorkUserIntegration(t *testing.T) {
	adminURL, migratorURL := os.Getenv("WERK_TEST_ADMIN_DATABASE_URL"), os.Getenv("WERK_TEST_MIGRATOR_DATABASE_URL")
	if adminURL == "" || migratorURL == "" {
		t.Skip("integration database URLs are not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	owner, err := pgxpool.New(ctx, migratorURL)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	conn, err := owner.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SET ROLE werk_owner`); err != nil {
		t.Fatal(err)
	}
	const tenantID = "0196f000-0000-7000-8000-000000000201"
	const unitID = "0196f000-0000-7000-8000-000000000202"
	const adminSubjectID = "0196f000-0000-7000-8000-000000000203"
	const adminAccountID = "0196f000-0000-7000-8000-000000000204"
	fixtures := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO werk_core.tenants (id,name,status,default_locale,default_timezone) VALUES ($1::uuid,'Provisioning Test','active','de-DE','Europe/Berlin')`, []any{tenantID}},
		{`INSERT INTO werk_core.organizational_units (id,tenant_id,unit_type,name,status) VALUES ($1::uuid,$2::uuid,'company','Test Company','active')`, []any{unitID, tenantID}},
		{`INSERT INTO werk_core.admin_subjects (id,display_name,status) VALUES ($1::uuid,'Provisioning Admin','active')`, []any{adminSubjectID}},
		{`INSERT INTO werk_core.accounts (id,account_class,admin_subject_id,login_name,status) VALUES ($1::uuid,'admin',$2::uuid,'provisioning-admin@werk.local','active')`, []any{adminAccountID, adminSubjectID}},
	}
	for _, fixture := range fixtures {
		if _, err := conn.Exec(ctx, fixture.query, fixture.args...); err != nil {
			t.Fatal(err)
		}
	}
	adminDB, err := database.NewAdmin(ctx, adminURL, "werk-adminstore-integration")
	if err != nil {
		t.Fatal(err)
	}
	defer adminDB.Close()
	service, _ := New(adminDB)
	actor := identity.AuthenticatedActor{AccountID: accountID(adminAccountID), AccountClass: identity.AccountClassAdmin, Audience: identity.AudienceAdmin, Kind: identity.AuthenticationInteractive, Assurance: identity.AssuranceMultiFactor}
	const developmentPassword = "Werk-Dev-Worker-Initial-2026!"
	const changedDevelopmentPassword = "Werk-Dev-Worker-Changed-2026!"
	if err := service.EnsureDevelopmentWorkAccount(ctx, developmentPassword); err != nil {
		t.Fatal(err)
	}
	changedHash, err := identity.HashPassword(changedDevelopmentPassword)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `
		UPDATE werk_core.account_credentials
		SET secret_hash=$2
		WHERE account_id=$1::uuid
		  AND credential_kind='password'
		  AND status='active'
	`, developmentAccountID, changedHash); err != nil {
		t.Fatal(err)
	}
	if err := service.EnsureDevelopmentWorkAccount(ctx, developmentPassword); err != nil {
		t.Fatal(err)
	}
	var developmentHash []byte
	var developmentCount int
	if err := conn.QueryRow(ctx, `
		SELECT credential.secret_hash, count(*) OVER ()
		FROM werk_core.accounts AS account
		JOIN werk_core.account_credentials AS credential
		  ON credential.account_id=account.id
		 AND credential.credential_kind='password'
		 AND credential.status='active'
		JOIN werk_core.account_identity_bindings AS binding
		  ON binding.account_id=account.id
		 AND binding.provider_key='local'
		 AND binding.provider_subject=account.id::text
		 AND binding.status='active'
		JOIN werk_core.role_assignments AS assignment ON assignment.account_id=account.id
		JOIN werk_core.roles AS role ON role.id=assignment.role_id
		WHERE account.login_name=$1 AND account.account_class='work'
		  AND account.must_change_password AND role.role_key='workspace-member'
	`, developmentLoginName).Scan(&developmentHash, &developmentCount); err != nil || developmentCount != 1 {
		t.Fatalf("development account count=%d err=%v", developmentCount, err)
	}
	if !identity.VerifyPassword(developmentHash, changedDevelopmentPassword) || identity.VerifyPassword(developmentHash, developmentPassword) {
		t.Fatal("idempotent development bootstrap reset an existing password")
	}
	createdTenant, err := service.CreateTenant(ctx, CreateTenantInput{Name: "Managed Tenant", DefaultLocale: "de-DE", DefaultTimezone: "Europe/Berlin"}, actor, "0196f000-0000-7000-8000-000000000221", "0196f000-0000-7000-8000-000000000222")
	if err != nil {
		t.Fatal(err)
	}
	tenants, err := service.ListTenants(ctx)
	if err != nil {
		t.Fatal(err)
	}
	foundTenant := false
	for _, tenant := range tenants {
		foundTenant = foundTenant || tenant.ID == createdTenant.ID
	}
	if !foundTenant {
		t.Fatalf("created tenant %s is missing from list", createdTenant.ID)
	}
	rootUnit, err := service.CreateOrganizationalUnit(ctx, createdTenant.ID, CreateOrganizationalUnitInput{UnitType: "company", Name: "Managed Company"}, actor, "0196f000-0000-7000-8000-000000000223", "0196f000-0000-7000-8000-000000000224")
	if err != nil {
		t.Fatal(err)
	}
	childUnit, err := service.CreateOrganizationalUnit(ctx, createdTenant.ID, CreateOrganizationalUnitInput{ParentID: rootUnit.ID, UnitType: "team", Name: "Managed Team"}, actor, "0196f000-0000-7000-8000-000000000225", "0196f000-0000-7000-8000-000000000226")
	if err != nil || childUnit.ParentID == nil || *childUnit.ParentID != rootUnit.ID {
		t.Fatalf("child unit = %#v, err = %v", childUnit, err)
	}
	units, err := service.ListOrganizationalUnits(ctx, createdTenant.ID)
	if err != nil || len(units) != 2 {
		t.Fatalf("organizational units = %#v, err = %v", units, err)
	}
	if _, err := service.UpdateOrganizationalUnit(ctx, createdTenant.ID, rootUnit.ID, rootUnit.Version, UpdateOrganizationalUnitInput{
		ParentID: childUnit.ID, UnitType: rootUnit.UnitType, Name: rootUnit.Name, Status: rootUnit.Status,
	}, actor, "0196f000-0000-7000-8000-000000000261", "0196f000-0000-7000-8000-000000000262"); err == nil {
		t.Fatal("organizational unit hierarchy cycle was accepted")
	}
	updatedChild, err := service.UpdateOrganizationalUnit(ctx, createdTenant.ID, childUnit.ID, childUnit.Version, UpdateOrganizationalUnitInput{
		ParentID: rootUnit.ID, UnitType: "department", Name: "Managed Department", Status: "active",
	}, actor, "0196f000-0000-7000-8000-000000000263", "0196f000-0000-7000-8000-000000000264")
	if err != nil || updatedChild.Version != childUnit.Version+1 || updatedChild.Name != "Managed Department" {
		t.Fatalf("updated child unit = %#v, err = %v", updatedChild, err)
	}
	if _, err := service.UpdateOrganizationalUnit(ctx, createdTenant.ID, childUnit.ID, childUnit.Version, UpdateOrganizationalUnitInput{
		ParentID: rootUnit.ID, UnitType: "department", Name: "Stale Department", Status: "active",
	}, actor, "0196f000-0000-7000-8000-000000000265", "0196f000-0000-7000-8000-000000000266"); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("stale organizational unit update error = %v, want version conflict", err)
	}
	if _, err := service.UpdateOrganizationalUnit(ctx, createdTenant.ID, rootUnit.ID, rootUnit.Version, UpdateOrganizationalUnitInput{
		UnitType: rootUnit.UnitType, Name: rootUnit.Name, Status: "archived",
	}, actor, "0196f000-0000-7000-8000-000000000267", "0196f000-0000-7000-8000-000000000268"); err == nil {
		t.Fatal("organizational unit with an active child was archived")
	}
	updatedTenant, err := service.UpdateTenant(ctx, createdTenant.ID, createdTenant.Version, UpdateTenantInput{
		Name: "Managed Tenant Updated", Status: "active", DefaultLocale: "de-DE", DefaultTimezone: "Europe/Berlin",
	}, actor, "0196f000-0000-7000-8000-000000000269", "0196f000-0000-7000-8000-00000000026a")
	if err != nil || updatedTenant.Version != createdTenant.Version+1 || updatedTenant.Name != "Managed Tenant Updated" {
		t.Fatalf("updated tenant = %#v, err = %v", updatedTenant, err)
	}
	if _, err := service.UpdateTenant(ctx, createdTenant.ID, createdTenant.Version, UpdateTenantInput{
		Name: "Stale Tenant", Status: "active", DefaultLocale: "de-DE", DefaultTimezone: "Europe/Berlin",
	}, actor, "0196f000-0000-7000-8000-00000000026b", "0196f000-0000-7000-8000-00000000026c"); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("stale tenant update error = %v, want version conflict", err)
	}
	var tenancyEventCount int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM werk_core.outbox_events WHERE tenant_id=$1::uuid AND event_type IN ('core.tenancy.tenant-created.v1','core.tenancy.organizational-unit-created.v1','core.tenancy.tenant-updated.v1','core.tenancy.organizational-unit-updated.v1')`, createdTenant.ID).Scan(&tenancyEventCount); err != nil || tenancyEventCount != 5 {
		t.Fatalf("tenancy outbox count=%d err=%v", tenancyEventCount, err)
	}
	parsedCreatedTenant, _ := tenancy.ParseTenantID(createdTenant.ID)
	if err := adminDB.WithinTenantWrite(ctx, parsedCreatedTenant, func(ctx context.Context, tx database.TenantTx) error {
		_, err := tx.Exec(ctx, `INSERT INTO werk_core.organizational_units (id,tenant_id,unit_type,name,status) VALUES ('0196f000-0000-7000-8000-000000000229',$1::uuid,'team','Cross Tenant','active')`, tenantID)
		return err
	}); err == nil {
		t.Fatal("admin runtime wrote an organizational unit outside its transaction tenant")
	}
	const temporaryWorkPassword = "Temporary-Password-2026!"
	view, err := service.CreateWorkUser(ctx, CreateWorkUserInput{TenantID: tenantID, OrganizationalUnitID: unitID, GivenName: "New", FamilyName: "User", LoginName: "new.user@werk.local", TemporaryPassword: temporaryWorkPassword, MembershipType: "team.member"}, actor, "0196f000-0000-7000-8000-000000000205", "0196f000-0000-7000-8000-000000000206")
	if err != nil {
		t.Fatal(err)
	}
	identityURL := os.Getenv("WERK_TEST_IDENTITY_DATABASE_URL")
	identityDatabase, err := database.NewIdentity(ctx, identityURL, "werk-adminstore-password-gate")
	if err != nil {
		t.Fatal(err)
	}
	defer identityDatabase.Close()
	identityService, err := identitystore.New(identityDatabase)
	if err != nil {
		t.Fatal(err)
	}
	initialWorkLogin, err := identityService.LoginWithMFA(ctx, view.LoginName, temporaryWorkPassword, "0196f000-0000-7000-8000-00000000020a", "0196f000-0000-7000-8000-00000000020b")
	if err != nil || initialWorkLogin.Redirect != "/change-password" || initialWorkLogin.SessionToken == "" {
		t.Fatalf("initial work login = %#v, err = %v", initialWorkLogin, err)
	}
	if _, err := identityService.ResolveActor(ctx, initialWorkLogin.SessionToken, identity.AccessPlaneWork); err == nil {
		t.Fatal("work session requiring a password change was accepted for an authorized API")
	}
	directory, err := service.ListWorkUsers(ctx, tenantID, actor, "0196f000-0000-7000-8000-000000000231", "0196f000-0000-7000-8000-000000000232")
	if err != nil || len(directory) != 1 {
		t.Fatalf("work user directory = %#v, err = %v", directory, err)
	}
	if directory[0].AccountID != view.AccountID || directory[0].DisplayName != "New User" || directory[0].OrganizationalUnitID != unitID || len(directory[0].Roles) != 1 || directory[0].Roles[0] != "workspace-member" {
		t.Fatalf("unexpected directory entry: %#v", directory[0])
	}
	var directoryAuditCount int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM werk_core.security_audit_events WHERE correlation_id='0196f000-0000-7000-8000-000000000232'::uuid AND event_type='identity.work-account.listed.v1' AND tenant_id=$1::uuid`, tenantID).Scan(&directoryAuditCount); err != nil || directoryAuditCount != 1 {
		t.Fatalf("directory audit count=%d err=%v", directoryAuditCount, err)
	}
	var count int
	if err := conn.QueryRow(ctx, `
		SELECT count(*)
		FROM werk_core.accounts AS account
		JOIN werk_core.role_assignments AS assignment ON assignment.account_id=account.id
		JOIN werk_core.roles AS role ON role.id=assignment.role_id
		JOIN werk_core.account_identity_bindings AS binding
		  ON binding.account_id=account.id
		 AND binding.provider_key='local'
		 AND binding.provider_subject=account.id::text
		 AND binding.status='active'
		WHERE account.id=$1::uuid
		  AND account.account_class='work'
		  AND account.tenant_id=$2::uuid
		  AND account.must_change_password
		  AND role.role_key='workspace-member'
	`, view.AccountID, tenantID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("provisioned account count=%d err=%v", count, err)
	}
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM werk_core.outbox_events WHERE tenant_id=$1::uuid AND event_type='core.identity.work-account-created.v1'`, tenantID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("outbox count=%d err=%v", count, err)
	}
	const permanentWorkPassword = "Permanent-Password-2026!"
	workPasswordRotation, err := identityService.ChangePassword(ctx, initialWorkLogin.SessionToken, temporaryWorkPassword, permanentWorkPassword)
	if err != nil {
		t.Fatalf("change work password: %v", err)
	}
	if _, err := identityService.ResolveActor(ctx, initialWorkLogin.SessionToken, identity.AccessPlaneWork); err == nil {
		t.Fatal("pre-password-change work session remained valid")
	}
	if _, err := identityService.ResolveActor(ctx, workPasswordRotation.SessionToken, identity.AccessPlaneWork); err != nil {
		t.Fatalf("resolve work actor after password change: %v", err)
	}
	catalog, err := service.ListWorkRoles(ctx, tenantID, actor, "0196f000-0000-7000-8000-000000000233", "0196f000-0000-7000-8000-000000000234")
	if err != nil {
		t.Fatal(err)
	}
	var workspaceRoleID string
	for _, role := range catalog.Roles {
		if role.RoleKey == "workspace-member" {
			workspaceRoleID = role.ID
			if !role.SystemRole || role.AssignmentCount != 1 || len(role.Permissions) != 1 || role.Permissions[0].PermissionKey != "core.workspace.access" {
				t.Fatalf("unexpected workspace role: %#v", role)
			}
		}
	}
	permissionKeys := make([]string, 0, len(catalog.Permissions))
	for _, permission := range catalog.Permissions {
		permissionKeys = append(permissionKeys, permission.PermissionKey)
	}
	wantPermissionKeys := "core.documents.content.download,core.documents.document.create,core.documents.document.list,core.documents.document.read,core.documents.document.update,core.documents.version.create,core.workspace.access"
	if workspaceRoleID == "" || strings.Join(permissionKeys, ",") != wantPermissionKeys {
		t.Fatalf("unexpected role catalog: %#v", catalog)
	}
	customRole, err := service.CreateWorkRole(ctx, CreateWorkRoleInput{
		TenantID: tenantID, RoleKey: "team-viewer", DisplayName: "Team-Lesezugriff", PermissionKeys: []string{"core.workspace.access"},
	}, actor, "0196f000-0000-7000-8000-000000000235", "0196f000-0000-7000-8000-000000000236")
	if err != nil || customRole.SystemRole || len(customRole.Permissions) != 1 {
		t.Fatalf("custom role = %#v, err = %v", customRole, err)
	}
	customRole, err = service.UpdateWorkRole(ctx, customRole.ID, uint64(customRole.Version), UpdateWorkRoleInput{
		TenantID: tenantID, DisplayName: "Team-Zugriff", Status: "active", PermissionKeys: []string{"core.workspace.access"},
	}, actor, "0196f000-0000-7000-8000-00000000026d", "0196f000-0000-7000-8000-00000000026e")
	if err != nil || customRole.Version != 2 || customRole.DisplayName != "Team-Zugriff" {
		t.Fatalf("updated custom role = %#v, err = %v", customRole, err)
	}
	if _, err := service.UpdateWorkRole(ctx, customRole.ID, 1, UpdateWorkRoleInput{
		TenantID: tenantID, DisplayName: "Stale Role", Status: "active", PermissionKeys: []string{"core.workspace.access"},
	}, actor, "0196f000-0000-7000-8000-00000000026f", "0196f000-0000-7000-8000-000000000270"); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("stale work role update error = %v, want version conflict", err)
	}
	if _, err := service.UpdateWorkRole(ctx, workspaceRoleID, 1, UpdateWorkRoleInput{
		TenantID: tenantID, DisplayName: "Changed System Role", Status: "active", PermissionKeys: []string{"core.workspace.access"},
	}, actor, "0196f000-0000-7000-8000-000000000271", "0196f000-0000-7000-8000-000000000272"); !errors.Is(err, ErrImmutable) {
		t.Fatalf("system role update error = %v, want immutable", err)
	}
	if _, err := service.CreateWorkRole(ctx, CreateWorkRoleInput{
		TenantID: tenantID, RoleKey: "invalid-admin-permission", DisplayName: "Ungültige Rolle", PermissionKeys: []string{"core.identity.work-account.create"},
	}, actor, "0196f000-0000-7000-8000-000000000237", "0196f000-0000-7000-8000-000000000238"); err == nil {
		t.Fatal("admin permission was accepted in a work role")
	}
	assignedRoleKeys, err := service.ReplaceWorkUserRoles(ctx, view.AccountID, ReplaceWorkUserRolesInput{
		TenantID: tenantID, RoleIDs: []string{workspaceRoleID, customRole.ID},
	}, actor, "0196f000-0000-7000-8000-000000000239", "0196f000-0000-7000-8000-000000000240")
	if err != nil || len(assignedRoleKeys) != 2 {
		t.Fatalf("assigned roles = %#v, err = %v", assignedRoleKeys, err)
	}
	assignedRoleKeys, err = service.ReplaceWorkUserRoles(ctx, view.AccountID, ReplaceWorkUserRolesInput{
		TenantID: tenantID, RoleIDs: []string{customRole.ID},
	}, actor, "0196f000-0000-7000-8000-000000000241", "0196f000-0000-7000-8000-000000000242")
	if err != nil || len(assignedRoleKeys) != 1 || assignedRoleKeys[0] != "team-viewer" {
		t.Fatalf("replaced roles = %#v, err = %v", assignedRoleKeys, err)
	}
	directory, err = service.ListWorkUsers(ctx, tenantID, actor, "0196f000-0000-7000-8000-000000000243", "0196f000-0000-7000-8000-000000000244")
	if err != nil || len(directory) != 1 || len(directory[0].Roles) != 1 || directory[0].Roles[0] != "team-viewer" {
		t.Fatalf("directory after role replacement = %#v, err = %v", directory, err)
	}
	crossTenantRole, err := service.CreateWorkRole(ctx, CreateWorkRoleInput{
		TenantID: createdTenant.ID, RoleKey: "cross-tenant-role", DisplayName: "Fremde Rolle", PermissionKeys: []string{"core.workspace.access"},
	}, actor, "0196f000-0000-7000-8000-000000000245", "0196f000-0000-7000-8000-000000000246")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReplaceWorkUserRoles(ctx, view.AccountID, ReplaceWorkUserRolesInput{
		TenantID: tenantID, RoleIDs: []string{crossTenantRole.ID},
	}, actor, "0196f000-0000-7000-8000-000000000247", "0196f000-0000-7000-8000-000000000248"); err == nil {
		t.Fatal("cross-tenant role was assigned to a work account")
	}
	if err := conn.QueryRow(ctx, `
		SELECT count(*) FROM werk_core.outbox_events
		WHERE tenant_id=$1::uuid AND event_type IN (
			'core.authorization.work-role-created.v1',
			'core.authorization.work-role-updated.v1',
			'core.authorization.work-role-assignments-replaced.v1'
		)
	`, tenantID).Scan(&count); err != nil || count != 4 {
		t.Fatalf("authorization outbox count=%d err=%v", count, err)
	}
	firstAuditPage, err := service.ListSecurityAuditEvents(ctx, SecurityAuditQuery{
		TenantID: tenantID,
		Outcome:  "succeeded",
		Limit:    2,
	}, actor, "0196f000-0000-7000-8000-000000000275", "0196f000-0000-7000-8000-000000000276")
	if err != nil || len(firstAuditPage.Items) != 2 || firstAuditPage.NextCursor == nil {
		t.Fatalf("first security audit page = %#v, err = %v", firstAuditPage, err)
	}
	for _, item := range firstAuditPage.Items {
		if item.TenantID != tenantID || item.Outcome != "succeeded" || item.TenantName != "Provisioning Test" || item.ActorAccountClass != "admin" {
			t.Fatalf("unexpected tenant audit item: %#v", item)
		}
	}
	secondAuditPage, err := service.ListSecurityAuditEvents(ctx, SecurityAuditQuery{
		TenantID: tenantID,
		Outcome:  "succeeded",
		Limit:    2,
		Cursor:   firstAuditPage.NextCursor,
	}, actor, "0196f000-0000-7000-8000-000000000277", "0196f000-0000-7000-8000-000000000278")
	if err != nil || len(secondAuditPage.Items) == 0 {
		t.Fatalf("second security audit page = %#v, err = %v", secondAuditPage, err)
	}
	if secondAuditPage.Items[0].ID == firstAuditPage.Items[0].ID || secondAuditPage.Items[0].ID == firstAuditPage.Items[1].ID {
		t.Fatal("security audit cursor repeated an item from the first page")
	}
	if err := conn.QueryRow(ctx, `
		SELECT count(*)
		FROM werk_core.security_audit_events
		WHERE tenant_id IS NULL
		  AND event_type = 'core.audit.security-events-listed.v1'
		  AND correlation_id IN (
		    '0196f000-0000-7000-8000-000000000276'::uuid,
		    '0196f000-0000-7000-8000-000000000278'::uuid
		  )
	`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("security audit read audit count=%d err=%v", count, err)
	}
	if err := adminDB.WithinInstallationAuditRead(ctx, func(ctx context.Context, tx database.TenantTx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO werk_core.security_audit_events (
				id,event_type,outcome,account_id,tenant_id,request_id,correlation_id
			) VALUES (
				'0196f000-0000-7000-8000-000000000279'::uuid,
				'core.audit.unapproved-write.v1','succeeded',$1::uuid,NULL,
				'0196f000-0000-7000-8000-00000000027a'::uuid,
				'0196f000-0000-7000-8000-00000000027b'::uuid
			)
		`, adminAccountID)
		return err
	}); err == nil {
		t.Fatal("installation audit boundary accepted an unrelated global audit write")
	}
	parsedTenant, _ := tenancy.ParseTenantID(tenantID)
	if err := adminDB.WithinTenantWrite(ctx, parsedTenant, func(ctx context.Context, tx database.TenantTx) error {
		_, err := tx.Exec(ctx, `INSERT INTO werk_core.role_assignments (id,account_id,role_id,access_plane,scope_type,valid_from) VALUES ('0196f000-0000-7000-8000-000000000209',$1::uuid,'0196f000-0000-7000-8000-000000000111','admin','installation',now())`, view.AccountID)
		return err
	}); err == nil {
		t.Fatal("admin role was assigned to a work account")
	}
	if err := adminDB.WithinTenantWrite(ctx, parsedTenant, func(ctx context.Context, tx database.TenantTx) error {
		_, err := tx.Exec(ctx, `UPDATE werk_core.roles SET display_name='Mutated System Role' WHERE id=$1::uuid`, workspaceRoleID)
		return err
	}); err == nil {
		t.Fatal("admin runtime changed a protected system role directly")
	}
	if _, err := service.UpdateTenant(ctx, tenantID, 1, UpdateTenantInput{
		Name: "Provisioning Test", Status: "suspended", DefaultLocale: "de-DE", DefaultTimezone: "Europe/Berlin",
	}, actor, "0196f000-0000-7000-8000-000000000273", "0196f000-0000-7000-8000-000000000274"); err != nil {
		t.Fatalf("suspend tenant: %v", err)
	}
	if _, err := identityService.ResolveActor(ctx, initialWorkLogin.SessionToken, identity.AccessPlaneWork); err == nil {
		t.Fatal("work session for a suspended tenant remained authorized")
	}
}

func accountID(value string) identity.AccountID {
	raw, _ := hex.DecodeString(strings.ReplaceAll(value, "-", ""))
	var id identity.AccountID
	copy(id[:], raw)
	return id
}
