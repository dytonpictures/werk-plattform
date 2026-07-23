package documents

import (
	"testing"
	"time"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/tenancy"
)

func visibilityDocument(tenantID tenancy.TenantID, creator identity.AccountID, createdAt time.Time) Document {
	return Document{
		ID: DocumentID{1}, TenantID: tenantID, Title: "Freigabe", Status: StatusActive,
		SourceModule: "core.documents", CreatedBy: creator, CreatedAt: createdAt.UTC(),
		UpdatedAt: createdAt.UTC(), Version: 1,
	}
}

func TestCreateDirectGrantBindsActiveGrantToDocument(t *testing.T) {
	tenantID := tenancy.TenantID{1}
	creator := identity.AccountID{2}
	recipient := identity.AccountID{3}
	createdAt := time.Date(2026, 7, 22, 10, 0, 0, 0, time.FixedZone("CEST", 2*60*60))
	document := visibilityDocument(tenantID, creator, createdAt)

	grant, err := CreateDirectGrant(document, tenantID, recipient, creator, createdAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if grant.ID.IsZero() || grant.TenantID != tenantID || grant.DocumentID != document.ID || grant.GrantedTo != recipient ||
		grant.GrantedBy != creator || grant.Status != GrantStatusActive || grant.Version != 1 || grant.RevokedAt != nil {
		t.Fatalf("unexpected grant: %#v", grant)
	}
	if grant.GrantedAt.Location() != time.UTC {
		t.Fatalf("grant time is not UTC: %v", grant.GrantedAt.Location())
	}
	if AccessReasonCreatedByMe != "created-by-me" || AccessReasonSharedDirectlyWithMe != "shared-directly-with-me" {
		t.Fatal("access reasons changed their public values")
	}
}

func TestCreateDirectGrantRequiresCreatorDistinctRecipientAndMatchingTenant(t *testing.T) {
	tenantID := tenancy.TenantID{1}
	creator := identity.AccountID{2}
	document := visibilityDocument(tenantID, creator, time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC))
	at := document.CreatedAt.Add(time.Minute)

	tests := []struct {
		name      string
		tenantID  tenancy.TenantID
		recipient identity.AccountID
		actor     identity.AccountID
	}{
		{name: "foreign tenant", tenantID: tenancy.TenantID{9}, recipient: identity.AccountID{3}, actor: creator},
		{name: "empty recipient", tenantID: tenantID, actor: creator},
		{name: "creator as recipient", tenantID: tenantID, recipient: creator, actor: creator},
		{name: "not creator", tenantID: tenantID, recipient: identity.AccountID{3}, actor: identity.AccountID{4}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := CreateDirectGrant(document, test.tenantID, test.recipient, test.actor, at); err == nil {
				t.Fatal("invalid direct grant was accepted")
			}
		})
	}

	archived := document
	archived.Status = StatusArchived
	if _, err := CreateDirectGrant(archived, tenantID, identity.AccountID{3}, creator, at); err == nil {
		t.Fatal("archived document was shared")
	}
}

func TestRevokeDirectGrantIsCreatorOnlyAndOneTime(t *testing.T) {
	tenantID := tenancy.TenantID{1}
	creator := identity.AccountID{2}
	document := visibilityDocument(tenantID, creator, time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC))
	grant, err := CreateDirectGrant(document, tenantID, identity.AccountID{3}, creator, document.CreatedAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := RevokeDirectGrant(document, grant, tenantID, identity.AccountID{4}, document.CreatedAt.Add(2*time.Minute)); err == nil {
		t.Fatal("non-creator revoked grant")
	}
	revoked, err := RevokeDirectGrant(document, grant, tenantID, creator, document.CreatedAt.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if revoked.Status != GrantStatusRevoked || revoked.RevokedBy != creator || revoked.RevokedAt == nil ||
		revoked.RevokedAt.Location() != time.UTC || revoked.Version != grant.Version+1 {
		t.Fatalf("unexpected revoked grant: %#v", revoked)
	}
	if _, err := RevokeDirectGrant(document, revoked, tenantID, creator, document.CreatedAt.Add(3*time.Minute)); err == nil {
		t.Fatal("grant was revoked twice")
	}
}

func TestDirectGrantValidationRejectsDetachedCoordinates(t *testing.T) {
	tenantID := tenancy.TenantID{1}
	creator := identity.AccountID{2}
	document := visibilityDocument(tenantID, creator, time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC))
	grant, err := CreateDirectGrant(document, tenantID, identity.AccountID{3}, creator, document.CreatedAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	foreignTenant := document
	foreignTenant.TenantID = tenancy.TenantID{9}
	if grant.Validate(foreignTenant) == nil {
		t.Fatal("grant validated against a foreign tenant")
	}
	foreignDocument := document
	foreignDocument.ID = DocumentID{9}
	if grant.Validate(foreignDocument) == nil {
		t.Fatal("grant validated against another document")
	}
	nonMonotone := grant
	nonMonotone.Version = 2
	if nonMonotone.Validate(document) == nil {
		t.Fatal("active grant accepted a post-revocation version")
	}
}
