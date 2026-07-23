package party

import (
	"strings"
	"testing"
	"time"

	"github.com/dytonpictures/werk/internal/core/tenancy"
)

func TestNewPersonCreatesSharedPartyIdentity(t *testing.T) {
	tenant := tenancy.TenantID{1}
	party, person, err := NewPerson(tenant, " Ada ", " Lovelace ")
	if err != nil {
		t.Fatalf("new person: %v", err)
	}
	if party.Type != TypePerson || party.Status != StatusActive || party.Version != 1 {
		t.Fatalf("unexpected party defaults: %+v", party)
	}
	if party.TenantID != tenant || person.PartyID != party.ID {
		t.Fatalf("person is not attached to its tenant-owned party")
	}
	if person.GivenName != "Ada" || person.FamilyName != "Lovelace" || party.DisplayName != "Ada Lovelace" {
		t.Fatalf("names were not normalized: party=%q person=%+v", party.DisplayName, person)
	}
	if party.ID[6]>>4 != 7 {
		t.Fatalf("party ID version = %d, want 7", party.ID[6]>>4)
	}
}

func TestPartyConstructorsRejectMissingTenantAndNames(t *testing.T) {
	if _, _, err := NewPerson(tenancy.TenantID{}, "Ada", "Lovelace"); err == nil {
		t.Fatal("expected missing person tenant to fail")
	}
	if _, _, err := NewOrganization(tenancy.TenantID{1}, " "); err == nil {
		t.Fatal("expected missing organization name to fail")
	}
	if _, _, err := NewPerson(tenancy.TenantID{1}, "", ""); err == nil {
		t.Fatal("expected missing person name to fail")
	}
}

func TestMembershipValidatesStableTypeAndTimeWindow(t *testing.T) {
	from := time.Date(2026, 7, 19, 12, 0, 0, 0, time.FixedZone("test", 3600))
	until := from.Add(-time.Hour)
	if _, err := NewMembership(tenancy.TenantID{1}, ID{1}, tenancy.UnitID{2}, "Sales Lead", from, nil); err == nil {
		t.Fatal("expected unstable membership type to fail")
	}
	if _, err := NewMembership(tenancy.TenantID{1}, ID{1}, tenancy.UnitID{2}, "team.member", from, &until); err == nil {
		t.Fatal("expected reversed membership window to fail")
	}
	valid, err := NewMembership(tenancy.TenantID{1}, ID{1}, tenancy.UnitID{2}, "team.member", from, nil)
	if err != nil {
		t.Fatalf("new membership: %v", err)
	}
	if !valid.ValidFrom.Equal(from.UTC()) || valid.TenantID != (tenancy.TenantID{1}) {
		t.Fatalf("membership was not normalized: %+v", valid)
	}
}

func TestPartyNameLimitsCountUnicodeCharacters(t *testing.T) {
	party, _, err := NewPerson(tenancy.TenantID{1}, strings.Repeat("ä", maximumNamePartLength+1), "Test")
	if err == nil {
		t.Fatalf("expected overlong person name to fail, got %+v", party)
	}
}
