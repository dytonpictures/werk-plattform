package documents

import (
	"testing"
	"time"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/storage"
	"github.com/dytonpictures/werk/internal/core/tenancy"
)

func availableBlob(t *testing.T, tenantID tenancy.TenantID, at time.Time) storage.Blob {
	t.Helper()
	blob, err := storage.NewQuarantinedBlob(tenantID, identity.AccountID{2}, at)
	if err != nil {
		t.Fatal(err)
	}
	blob, err = blob.Seal(storage.ContentDescriptor{SizeBytes: 12, SHA256: storage.Digest{3}, MediaType: "application/pdf"}, at.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	return blob
}

func TestPublishNewRequiresAvailableTenantMatchingBlob(t *testing.T) {
	at := time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC)
	tenantID := tenancy.TenantID{1}
	blob := availableBlob(t, tenantID, at)
	document, version, classification, err := PublishNew(
		tenantID, "Vertrag", "core.documents", blob, SourceUpload,
		ClassificationConfidential, "business.standard", nil, false, identity.AccountID{2}, at.Add(2*time.Minute),
	)
	if err != nil || document.Status != StatusActive || version.VersionNumber != 1 || classification.Revision != 1 || version.BlobID != blob.ID {
		t.Fatalf("document=%#v version=%#v classification=%#v err=%v", document, version, classification, err)
	}
	foreignBlob := availableBlob(t, tenancy.TenantID{9}, at)
	if _, _, _, err := PublishNew(
		tenantID, "Fremd", "core.documents", foreignBlob, SourceUpload,
		ClassificationInternal, "business.standard", nil, false, identity.AccountID{2}, at.Add(2*time.Minute),
	); err == nil {
		t.Fatal("document accepted a foreign-tenant blob")
	}
}

func TestPendingBlobCannotBecomeDocumentVersion(t *testing.T) {
	at := time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC)
	tenantID := tenancy.TenantID{1}
	pending, _ := storage.NewQuarantinedBlob(tenantID, identity.AccountID{2}, at)
	if _, _, _, err := PublishNew(
		tenantID, "Unvollständig", "core.documents", pending, SourceUpload,
		ClassificationInternal, "business.standard", nil, false, identity.AccountID{2}, at.Add(time.Minute),
	); err == nil {
		t.Fatal("document accepted a quarantined blob")
	}
}

func TestPublishedVersionsAndClassificationsAdvanceMonotonically(t *testing.T) {
	at := time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC)
	tenantID := tenancy.TenantID{1}
	document, _, _, err := PublishNew(
		tenantID, "Akte", "app.case", availableBlob(t, tenantID, at), SourceImport,
		ClassificationInternal, "business.standard", nil, false, identity.AccountID{2}, at.Add(2*time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	version, err := PublishNextVersion(document, 1, availableBlob(t, tenantID, at.Add(3*time.Minute)), SourceSignature, identity.AccountID{2}, at.Add(5*time.Minute))
	if err != nil || version.VersionNumber != 2 {
		t.Fatalf("version=%#v err=%v", version, err)
	}
	revision, err := RecordClassification(document, 1, ClassificationRestricted, "legal.long-term", nil, true, identity.AccountID{2}, at.Add(6*time.Minute))
	if err != nil || revision.Revision != 2 || !revision.LegalHold {
		t.Fatalf("revision=%#v err=%v", revision, err)
	}
}
