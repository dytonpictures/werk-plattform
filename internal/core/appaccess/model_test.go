package appaccess

import (
	"testing"
	"time"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/tenancy"
)

func TestDirectAccountEntitlementOpensOnlyTheAppGate(t *testing.T) {
	now := time.Now().UTC()
	tenantID, _ := tenancy.ParseTenantID("0196f000-0000-7000-8000-000000000701")
	accountID := identity.AccountID{1}
	installation := Installation{
		TenantID: tenantID, AppModule: "app.crm", Status: InstallationStatusActive, ContractVersion: 1,
	}
	entitlement := Entitlement{
		ID: EntitlementID{1}, TenantID: tenantID, AppModule: installation.AppModule,
		Subject:   SubjectRef{TenantID: tenantID, AccountID: &accountID},
		ValidFrom: now.Add(-time.Minute), Status: EntitlementStatusActive, ContractVersion: 1,
	}
	coordinates := ActorCoordinates{TenantID: tenantID, AccountID: accountID}
	if err := Authorize(installation, coordinates, []Entitlement{entitlement}, now); err != nil {
		t.Fatalf("direct app entitlement denied: %v", err)
	}
	otherAccount := coordinates
	otherAccount.AccountID = identity.AccountID{2}
	if err := Authorize(installation, otherAccount, []Entitlement{entitlement}, now); err == nil {
		t.Fatal("direct app entitlement opened the app for another account")
	}
}

func TestDepartmentEntitlementCanIncludeDescendants(t *testing.T) {
	now := time.Now().UTC()
	tenantID, _ := tenancy.ParseTenantID("0196f000-0000-7000-8000-000000000702")
	departmentID, _ := tenancy.ParseUnitID("0196f000-0000-7000-8000-000000000711")
	teamID, _ := tenancy.ParseUnitID("0196f000-0000-7000-8000-000000000712")
	installation := Installation{
		TenantID: tenantID, AppModule: "app.documents", Status: InstallationStatusActive, ContractVersion: 1,
	}
	entitlement := Entitlement{
		ID: EntitlementID{2}, TenantID: tenantID, AppModule: installation.AppModule,
		Subject: SubjectRef{
			TenantID: tenantID, OrganizationalUnitID: &departmentID, IncludeDescendants: true,
		},
		ValidFrom: now.Add(-time.Minute), Status: EntitlementStatusActive, ContractVersion: 1,
	}
	coordinates := ActorCoordinates{
		TenantID: tenantID, AccountID: identity.AccountID{3},
		DirectOrganizationalUnitIDs: []tenancy.UnitID{teamID}, AncestorUnitIDs: []tenancy.UnitID{departmentID},
	}
	if err := Authorize(installation, coordinates, []Entitlement{entitlement}, now); err != nil {
		t.Fatalf("department descendant entitlement denied: %v", err)
	}
	entitlement.Subject.IncludeDescendants = false
	if err := Authorize(installation, coordinates, []Entitlement{entitlement}, now); err == nil {
		t.Fatal("exact department entitlement included a descendant")
	}
}

func TestAccessGroupEntitlementUsesResolvedGroupCoordinate(t *testing.T) {
	now := time.Now().UTC()
	tenantID, _ := tenancy.ParseTenantID("0196f000-0000-7000-8000-000000000703")
	groupID := GroupID{1}
	installation := Installation{
		TenantID: tenantID, AppModule: "app.workflow", Status: InstallationStatusActive, ContractVersion: 1,
	}
	entitlement := Entitlement{
		ID: EntitlementID{3}, TenantID: tenantID, AppModule: installation.AppModule,
		Subject:   SubjectRef{TenantID: tenantID, AccessGroupID: &groupID},
		ValidFrom: now.Add(-time.Minute), Status: EntitlementStatusActive, ContractVersion: 1,
	}
	coordinates := ActorCoordinates{
		TenantID: tenantID, AccountID: identity.AccountID{4}, AccessGroupIDs: []GroupID{groupID},
	}
	if !Evaluate(installation, coordinates, []Entitlement{entitlement}, now).Allowed() {
		t.Fatal("resolved access group entitlement was denied")
	}
}

func TestAppAccessFailsClosedAcrossTenantAndUnavailableInstallation(t *testing.T) {
	now := time.Now().UTC()
	tenantA, _ := tenancy.ParseTenantID("0196f000-0000-7000-8000-000000000704")
	tenantB, _ := tenancy.ParseTenantID("0196f000-0000-7000-8000-000000000705")
	accountID := identity.AccountID{5}
	installation := Installation{
		TenantID: tenantA, AppModule: "app.crm", Status: InstallationStatusActive, ContractVersion: 1,
	}
	entitlement := Entitlement{
		ID: EntitlementID{4}, TenantID: tenantB, AppModule: installation.AppModule,
		Subject:   SubjectRef{TenantID: tenantB, AccountID: &accountID},
		ValidFrom: now.Add(-time.Minute), Status: EntitlementStatusActive, ContractVersion: 1,
	}
	coordinates := ActorCoordinates{TenantID: tenantA, AccountID: accountID}
	decision := Evaluate(installation, coordinates, []Entitlement{entitlement}, now)
	if decision.Allowed() || decision.Reason != ReasonInvalidContract {
		t.Fatalf("cross-tenant decision = %#v", decision)
	}
	installation.Status = InstallationStatusDisabled
	decision = Evaluate(installation, coordinates, nil, now)
	if decision.Allowed() || decision.Reason != ReasonAppUnavailable {
		t.Fatalf("disabled installation decision = %#v", decision)
	}
}

func TestGroupMembershipRejectsNestedGroups(t *testing.T) {
	tenantID, _ := tenancy.ParseTenantID("0196f000-0000-7000-8000-000000000706")
	groupID := GroupID{1}
	membership := GroupMembership{
		ID: GroupMembershipID{1}, TenantID: tenantID, AccessGroupID: groupID,
		Subject:   SubjectRef{TenantID: tenantID, AccessGroupID: &groupID},
		ValidFrom: time.Now().UTC(), Status: EntitlementStatusActive, ContractVersion: 1,
	}
	if err := membership.Validate(); err == nil {
		t.Fatal("nested access group membership was accepted")
	}
}
