package audit

import (
	"testing"
	"time"

	"github.com/dytonpictures/werk/internal/core/compliance"
	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/resource"
	"github.com/dytonpictures/werk/internal/core/tenancy"
)

func validEntry() Entry {
	tenantID := tenancy.TenantID{1}
	return Entry{
		ID:         [16]byte{1},
		TenantID:   tenantID,
		OccurredAt: time.Date(2026, 7, 22, 14, 0, 0, 0, time.UTC),
		EventType:  "core.documents.document-published.v1",
		Action:     "core.documents.document.publish",
		Outcome:    OutcomeSucceeded,
		InitiatedBy: ActorRef{
			AccountID: identity.AccountID{2}, AccountClass: identity.AccountClassWork, TenantID: tenantID,
		},
		ExecutedBy: ActorRef{
			AccountID: identity.AccountID{3}, AccountClass: identity.AccountClassService, TenantID: tenantID,
		},
		Subject: resource.TenantRef(tenantID, resource.KindDocumentCollection, resource.RootID),
		Policy: PolicySnapshot{
			Permission: "core.documents.document.create", ContractVersion: 1, ProcessingRequired: true,
			Processing: compliance.ProcessingContext{
				ActivityKey:   "core.documents.document-management",
				PurposeKey:    "core.documents.document-creation",
				LegalBasisRef: "operator.processing-register.documents",
			},
		},
		RequestID:     [16]byte{4},
		CorrelationID: [16]byte{5},
	}
}

func TestEntryValidatesDualActorAndPolicySnapshot(t *testing.T) {
	entry := validEntry()
	if err := entry.Validate(); err != nil {
		t.Fatal(err)
	}
	entry.ExecutedBy.AccountClass = identity.AccountClassWork
	if err := entry.Validate(); err == nil {
		t.Fatal("work account was accepted as technical executor")
	}
}

func TestEntryRejectsTenantBoundaryMismatch(t *testing.T) {
	entry := validEntry()
	entry.Subject = resource.TenantRef(tenancy.TenantID{9}, resource.KindDocument, "document-1")
	if err := entry.Validate(); err == nil {
		t.Fatal("foreign-tenant subject was accepted")
	}
	entry = validEntry()
	entry.ExecutedBy.TenantID = tenancy.TenantID{9}
	if err := entry.Validate(); err == nil {
		t.Fatal("foreign-tenant executor was accepted")
	}
}

func TestEntryRejectsIncompleteTraceAndPolicyContext(t *testing.T) {
	entry := validEntry()
	entry.CorrelationID = [16]byte{}
	if err := entry.Validate(); err == nil {
		t.Fatal("entry without correlation ID was accepted")
	}
	entry = validEntry()
	entry.Policy.Processing.PurposeKey = ""
	if err := entry.Validate(); err == nil {
		t.Fatal("entry without required processing purpose was accepted")
	}
	entry = validEntry()
	entry.Action = "Documents Publish"
	if err := entry.Validate(); err == nil {
		t.Fatal("unstable action key was accepted")
	}
	entry = validEntry()
	entry.Action = "core.storage.blob.seal"
	if err := entry.Validate(); err == nil {
		t.Fatal("action from another module namespace was accepted")
	}
}

func TestEntryAllowsAutomatedServiceInitiator(t *testing.T) {
	entry := validEntry()
	entry.InitiatedBy = entry.ExecutedBy
	if err := entry.Validate(); err != nil {
		t.Fatal(err)
	}
}
