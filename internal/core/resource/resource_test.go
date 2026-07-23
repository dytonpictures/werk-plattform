package resource

import (
	"testing"

	"github.com/dytonpictures/werk/internal/core/tenancy"
)

func TestResourceReferenceRequiresExplicitBoundary(t *testing.T) {
	tenant, _ := tenancy.NewTenantID()
	if err := InstallationRef(KindPlatformInstallation, RootID).Validate(); err != nil {
		t.Fatalf("installation reference denied: %v", err)
	}
	if err := TenantRef(tenant, KindWorkspace, tenant.String()).Validate(); err != nil {
		t.Fatalf("tenant reference denied: %v", err)
	}
	invalid := InstallationRef(KindWorkspace, tenant.String())
	invalid.TenantID = &tenant
	if err := invalid.Validate(); err == nil {
		t.Fatal("installation reference accepted a tenant")
	}
}

func TestRegistrationOwnsItsNamespace(t *testing.T) {
	module := ModuleRegistration{Key: "app.documents", Kind: ModuleKindApp, Status: RegistrationActive, Version: 1}
	if err := module.Validate(); err != nil {
		t.Fatalf("module registration denied: %v", err)
	}
	resourceType := TypeRegistration{Kind: "app.documents.file", OwnerModule: module.Key, Boundary: BoundaryTenant, Status: RegistrationActive, Version: 1}
	if err := resourceType.Validate(); err != nil {
		t.Fatalf("resource type denied: %v", err)
	}
	resourceType.OwnerModule = "app.workflow"
	if err := resourceType.Validate(); err == nil {
		t.Fatal("foreign resource namespace accepted")
	}
}

func TestDocumentResourceKindsAreTenantAddressable(t *testing.T) {
	tenant, _ := tenancy.NewTenantID()
	if err := TenantRef(tenant, KindDocumentCollection, RootID).Validate(); err != nil {
		t.Fatalf("tenant document collection root denied: %v", err)
	}
	for _, kind := range []Kind{KindDocument, KindDocumentVersion} {
		if err := TenantRef(tenant, kind, "019f8514-7086-7752-8ac3-5edecdbc4afd").Validate(); err != nil {
			t.Errorf("tenant document reference %q denied: %v", kind, err)
		}
	}
}
