package authorization

import (
	"testing"
	"time"

	"github.com/dytonpictures/werk/internal/core/compliance"
	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/resource"
	"github.com/dytonpictures/werk/internal/core/tenancy"
)

func policyRequest(actor identity.AuthenticatedActor, permission string, target Resource, grants []Grant, now time.Time) PolicyRequest {
	return PolicyRequest{
		Actor: actor, Permission: permission, Target: target, Grants: grants, EvaluatedAt: now,
		DataProfile: compliance.ResourceDataProfile{
			ResourceKind: target.Reference.Kind, PersonalData: compliance.PersonalDataNone,
			Confidentiality: compliance.ConfidentialityInternal,
			Status:          resource.RegistrationActive, Version: 1,
		},
		ProcessingPolicy: compliance.ProcessingPolicy{
			Permission: permission, ResourceKind: target.Reference.Kind,
			Status: resource.RegistrationActive, Version: 1,
		},
	}
}

func TestAdminInstallationGrantCannotAuthorizeWorkActor(t *testing.T) {
	now := time.Now().UTC()
	admin := identity.AuthenticatedActor{AccountID: identity.AccountID{1}, AccountClass: identity.AccountClassAdmin, Audience: identity.AudienceAdmin, Kind: identity.AuthenticationInteractive, Assurance: identity.AssuranceMultiFactor}
	grant := Grant{AccessPlane: identity.AccessPlaneAdmin, Permission: "core.identity.work-account.create", Scope: ScopeInstallation, ValidFrom: now.Add(-time.Hour)}
	adminTarget := InstallationResource(resource.KindPlatformInstallation, resource.RootID)
	if err := Authorize(policyRequest(admin, grant.Permission, adminTarget, []Grant{grant}, now)); err != nil {
		t.Fatalf("admin grant denied: %v", err)
	}
	tenant, _ := tenancy.NewTenantID()
	work := identity.AuthenticatedActor{AccountID: identity.AccountID{2}, AccountClass: identity.AccountClassWork, Audience: identity.AudienceWork, Kind: identity.AuthenticationInteractive, Assurance: identity.AssuranceSingleFactor, TenantID: &tenant}
	workTarget := TenantResource(tenant, resource.KindWorkspace, tenant.String(), ScopeTenant)
	if err := Authorize(policyRequest(work, grant.Permission, workTarget, []Grant{grant}, now)); err == nil {
		t.Fatal("admin grant authorized work actor")
	}
}

func TestTenantGrantDoesNotCrossTenant(t *testing.T) {
	now := time.Now().UTC()
	tenantA, _ := tenancy.NewTenantID()
	tenantB, _ := tenancy.NewTenantID()
	actor := identity.AuthenticatedActor{AccountID: identity.AccountID{2}, AccountClass: identity.AccountClassWork, Audience: identity.AudienceWork, Kind: identity.AuthenticationInteractive, Assurance: identity.AssuranceSingleFactor, TenantID: &tenantA}
	grant := Grant{AccessPlane: identity.AccessPlaneWork, Permission: "core.workspace.access", Scope: ScopeTenant, TenantID: &tenantA, ValidFrom: now.Add(-time.Hour)}
	target := TenantResource(tenantB, resource.KindWorkspace, tenantB.String(), ScopeTenant)
	if err := Authorize(policyRequest(actor, grant.Permission, target, []Grant{grant}, now)); err == nil {
		t.Fatal("tenant grant crossed tenant boundary")
	}
}

func TestTenantBoundAgentCanUseOnlyServiceGrant(t *testing.T) {
	now := time.Now().UTC()
	tenant, _ := tenancy.NewTenantID()
	agent := identity.AuthenticatedActor{
		AccountID: identity.AccountID{3}, AccountClass: identity.AccountClassAgent,
		Audience: identity.AudienceService, Kind: identity.AuthenticationWorkload,
		Assurance: identity.AssuranceSingleFactor, TenantID: &tenant,
	}
	target := TenantResource(tenant, resource.Kind("core.ai.model"), "model-1", ScopeTenant)
	serviceGrant := Grant{
		AccessPlane: identity.AccessPlaneService, Permission: "core.ai.model.invoke",
		Scope: ScopeTenant, TenantID: &tenant, ValidFrom: now.Add(-time.Minute),
	}
	if err := Authorize(policyRequest(agent, serviceGrant.Permission, target, []Grant{serviceGrant}, now)); err != nil {
		t.Fatalf("service grant denied agent: %v", err)
	}
	workGrant := serviceGrant
	workGrant.AccessPlane = identity.AccessPlaneWork
	if err := Authorize(policyRequest(agent, workGrant.Permission, target, []Grant{workGrant}, now)); err == nil {
		t.Fatal("work grant authorized agent")
	}
}

func TestDecisionRejectsBoundaryMismatchBeforeGrant(t *testing.T) {
	now := time.Now().UTC()
	tenant, _ := tenancy.NewTenantID()
	actor := identity.AuthenticatedActor{AccountID: identity.AccountID{2}, AccountClass: identity.AccountClassWork, Audience: identity.AudienceWork, Kind: identity.AuthenticationInteractive, Assurance: identity.AssuranceSingleFactor, TenantID: &tenant}
	grant := Grant{AccessPlane: identity.AccessPlaneWork, Permission: "core.workspace.access", Scope: ScopeTenant, TenantID: &tenant, ValidFrom: now.Add(-time.Hour)}
	target := InstallationResource(resource.KindPlatformInstallation, resource.RootID)
	decision := Evaluate(policyRequest(actor, grant.Permission, target, []Grant{grant}, now))
	if decision.Allowed() || decision.Reason != ReasonActorBoundary {
		t.Fatalf("decision = %#v, want actor boundary denial", decision)
	}
}

func TestPlatformContextIsDerivedFromActorBoundary(t *testing.T) {
	tenant, _ := tenancy.NewTenantID()
	work := identity.AuthenticatedActor{AccountID: identity.AccountID{4}, AccountClass: identity.AccountClassWork, Audience: identity.AudienceWork, Kind: identity.AuthenticationInteractive, Assurance: identity.AssuranceSingleFactor, TenantID: &tenant}
	platformContext, err := ResolvePlatformContext(work)
	if err != nil || platformContext.AccessPlane != identity.AccessPlaneWork || platformContext.TenantID == nil || *platformContext.TenantID != tenant {
		t.Fatalf("platform context = %#v, %v", platformContext, err)
	}
	adminWithTenant := identity.AuthenticatedActor{AccountID: identity.AccountID{5}, AccountClass: identity.AccountClassAdmin, Audience: identity.AudienceAdmin, Kind: identity.AuthenticationInteractive, Assurance: identity.AssuranceMultiFactor, TenantID: &tenant}
	if _, err := ResolvePlatformContext(adminWithTenant); err == nil {
		t.Fatal("admin platform context accepted a tenant")
	}
}

func TestInstallationServiceUsesOnlyServiceGrant(t *testing.T) {
	now := time.Now().UTC()
	serviceActor := identity.AuthenticatedActor{
		AccountID: identity.AccountID{6}, AccountClass: identity.AccountClassService,
		Audience: identity.AudienceService, Kind: identity.AuthenticationWorkload,
		Assurance: identity.AssuranceSingleFactor,
	}
	grant := Grant{
		AccessPlane: identity.AccessPlaneService, Permission: "core.platform.health.read",
		Scope: ScopeInstallation, ValidFrom: now.Add(-time.Minute),
	}
	target := InstallationResource(resource.KindPlatformInstallation, resource.RootID)
	if err := Authorize(policyRequest(serviceActor, grant.Permission, target, []Grant{grant}, now)); err != nil {
		t.Fatalf("installation service grant denied: %v", err)
	}
	adminGrant := grant
	adminGrant.AccessPlane = identity.AccessPlaneAdmin
	if err := Authorize(policyRequest(serviceActor, adminGrant.Permission, target, []Grant{adminGrant}, now)); err == nil {
		t.Fatal("admin grant authorized installation service")
	}
}

func TestGrantCannotBypassProcessingPolicy(t *testing.T) {
	now := time.Now().UTC()
	tenant, _ := tenancy.NewTenantID()
	actor := identity.AuthenticatedActor{
		AccountID: identity.AccountID{7}, AccountClass: identity.AccountClassWork,
		Audience: identity.AudienceWork, Kind: identity.AuthenticationInteractive,
		Assurance: identity.AssuranceSingleFactor, TenantID: &tenant,
	}
	permission := "core.workspace.access"
	target := TenantResource(tenant, resource.KindWorkspace, tenant.String(), ScopeTenant)
	grant := Grant{
		AccessPlane: identity.AccessPlaneWork, Permission: permission, Scope: ScopeTenant,
		TenantID: &tenant, ValidFrom: now.Add(-time.Minute),
	}
	request := policyRequest(actor, permission, target, []Grant{grant}, now)
	request.DataProfile.PersonalData = compliance.PersonalDataPersonal
	request.DataProfile.ProcessingActivityRequired = true
	decision := Evaluate(request)
	if decision.Allowed() || decision.Reason != ReasonProcessingDenied {
		t.Fatalf("decision = %#v, want processing denial", decision)
	}
	request.ProcessingPolicy.Required = true
	request.ProcessingPolicy.Context = compliance.ProcessingContext{
		ActivityKey: "core.workspace.context-access", PurposeKey: "core.workspace.work-delivery",
		LegalBasisRef: "operator.processing-register.workspace",
	}
	if err := Authorize(request); err != nil {
		t.Fatalf("complete access and processing policy denied: %v", err)
	}
}
