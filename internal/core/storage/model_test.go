package storage

import (
	"testing"
	"time"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/tenancy"
)

func TestBlobRequiresServerVerifiedContentBeforeAvailability(t *testing.T) {
	createdAt := time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC)
	blob, err := NewQuarantinedBlob(tenancy.TenantID{1}, identity.AccountID{2}, createdAt)
	if err != nil || blob.State != BlobQuarantined {
		t.Fatalf("quarantined blob = %#v, err = %v", blob, err)
	}
	if _, err := blob.Seal(ContentDescriptor{MediaType: "application/pdf"}, createdAt.Add(time.Minute)); err == nil {
		t.Fatal("blob accepted a missing server digest")
	}
	digest := Digest{1}
	sealed, err := blob.Seal(ContentDescriptor{SizeBytes: 128, SHA256: digest, MediaType: "application/pdf"}, createdAt.Add(time.Minute))
	if err != nil || sealed.State != BlobAvailable || sealed.Content.SHA256 != digest || sealed.Version != 2 {
		t.Fatalf("sealed blob = %#v, err = %v", sealed, err)
	}
	if _, err := sealed.Seal(sealed.Content, createdAt.Add(2*time.Minute)); err == nil {
		t.Fatal("available blob was sealed again")
	}
}

func TestBlobLocationUsesOpaqueTechnicalKey(t *testing.T) {
	createdAt := time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC)
	blob, _ := NewQuarantinedBlob(tenancy.TenantID{1}, identity.AccountID{2}, createdAt)
	location, err := NewQuarantinedLocation(blob, "internal.s3", createdAt)
	if err != nil || location.OpaqueKey.IsZero() || location.ProviderKey != "internal.s3" {
		t.Fatalf("location = %#v, err = %v", location, err)
	}
	active, err := location.Activate("provider-checksum", createdAt.Add(time.Minute))
	if err != nil || active.State != LocationAvailable || active.Version != 2 {
		t.Fatalf("active location = %#v, err = %v", active, err)
	}
	missing, err := active.MarkMissing()
	if err != nil || missing.State != LocationMissing || missing.Version != 3 {
		t.Fatalf("missing location = %#v, err = %v", missing, err)
	}
}

func TestQuarantinedBlobCanBecomeTerminallyRejected(t *testing.T) {
	createdAt := time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC)
	blob, _ := NewQuarantinedBlob(tenancy.TenantID{1}, identity.AccountID{2}, createdAt)
	rejected, err := blob.Reject()
	if err != nil || rejected.State != BlobRejected || rejected.Version != 2 {
		t.Fatalf("rejected blob = %#v, err = %v", rejected, err)
	}
	if _, err := rejected.Reject(); err == nil {
		t.Fatal("rejected blob accepted another transition")
	}
}

func TestAvailableBlobFailsClosedForUnknownAndMissingProviderState(t *testing.T) {
	createdAt := time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC)
	blob, _ := NewQuarantinedBlob(tenancy.TenantID{1}, identity.AccountID{2}, createdAt)
	available, _ := blob.Seal(
		ContentDescriptor{SizeBytes: 128, SHA256: Digest{1}, MediaType: "application/pdf"},
		createdAt.Add(time.Minute),
	)
	unknown, err := available.MarkUnknown()
	if err != nil || unknown.State != BlobUnknown || unknown.Version != 3 || unknown.Content != available.Content {
		t.Fatalf("unknown blob = %#v, err = %v", unknown, err)
	}
	restored, err := unknown.RestoreAvailability()
	if err != nil || restored.State != BlobAvailable || restored.Version != 4 || restored.Content != available.Content {
		t.Fatalf("restored blob = %#v, err = %v", restored, err)
	}
	missing, err := restored.MarkMissing()
	if err != nil || missing.State != BlobMissing || missing.Version != 5 || missing.Content != available.Content {
		t.Fatalf("missing blob = %#v, err = %v", missing, err)
	}
	if _, err := missing.MarkUnknown(); err == nil {
		t.Fatal("confirmed missing blob was downgraded to unknown")
	}
}

func TestStorageModelsRejectMissingTenantAndInvalidProvider(t *testing.T) {
	createdAt := time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC)
	if _, err := NewQuarantinedBlob(tenancy.TenantID{}, identity.AccountID{2}, createdAt); err == nil {
		t.Fatal("blob accepted a missing tenant")
	}
	blob, _ := NewQuarantinedBlob(tenancy.TenantID{1}, identity.AccountID{2}, createdAt)
	if _, err := NewQuarantinedLocation(blob, "Client selected/file.pdf", createdAt); err == nil {
		t.Fatal("location accepted an invalid provider key")
	}
}
