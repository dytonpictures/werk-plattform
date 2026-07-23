package tenancy

import (
	"strings"
	"testing"
)

func TestNewTenantCreatesValidUUIDv7Model(t *testing.T) {
	tenant, err := NewTenant(" WERK GmbH ", "de-DE", "Europe/Berlin")
	if err != nil {
		t.Fatalf("new tenant: %v", err)
	}
	if tenant.Name != "WERK GmbH" {
		t.Fatalf("name = %q, want trimmed name", tenant.Name)
	}
	if tenant.ID[6]>>4 != 7 {
		t.Fatalf("UUID version = %d, want 7", tenant.ID[6]>>4)
	}
	if tenant.Version != 1 || tenant.Status != TenantStatusActive {
		t.Fatalf("unexpected defaults: version=%d status=%q", tenant.Version, tenant.Status)
	}
}

func TestTenantValidationRejectsInvalidLocaleAndTimezone(t *testing.T) {
	tenant, err := NewTenant("WERK", "de-DE", "Europe/Berlin")
	if err != nil {
		t.Fatalf("new tenant: %v", err)
	}
	tenant.DefaultLocale = "not_a_locale"
	if err := tenant.Validate(); err == nil {
		t.Fatal("expected invalid locale to fail")
	}
	tenant.DefaultLocale = "de-DE"
	tenant.DefaultTimezone = "Local"
	if err := tenant.Validate(); err == nil {
		t.Fatal("expected implicit Local timezone to fail")
	}
}

func TestOrganizationalUnitValidationRejectsUnstableTypeAndSelfParent(t *testing.T) {
	tenantID, err := NewTenantID()
	if err != nil {
		t.Fatalf("new tenant ID: %v", err)
	}
	if _, err := NewOrganizationalUnit(tenantID, nil, "Sales Team", "Vertrieb"); err == nil {
		t.Fatal("expected unstable unit type to fail")
	}
	unit, err := NewOrganizationalUnit(tenantID, nil, "department.sales", "Vertrieb")
	if err != nil {
		t.Fatalf("new organizational unit: %v", err)
	}
	unit.ParentID = &unit.ID
	if err := unit.Validate(); err == nil {
		t.Fatal("expected self-parent to fail")
	}
}

func TestNameLengthCountsUnicodeCharacters(t *testing.T) {
	tenant, err := NewTenant(strings.Repeat("ä", maximumNameLength), "de-DE", "Europe/Berlin")
	if err != nil {
		t.Fatalf("unicode name at limit: %v", err)
	}
	tenant.Name += "x"
	if err := tenant.Validate(); err == nil {
		t.Fatal("expected name over character limit to fail")
	}
}

func TestTenantIDRoundTripAcceptsImportedUUID(t *testing.T) {
	const imported = "550e8400-e29b-41d4-a716-446655440000"
	id, err := ParseTenantID(imported)
	if err != nil {
		t.Fatalf("parse imported ID: %v", err)
	}
	if id.String() != imported {
		t.Fatalf("round trip = %q, want %q", id.String(), imported)
	}
}
