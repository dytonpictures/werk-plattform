package auditstore

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	coreaudit "github.com/dytonpictures/werk/internal/core/audit"
	"github.com/dytonpictures/werk/internal/core/compliance"
	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/resource"
	"github.com/dytonpictures/werk/internal/core/tenancy"
	"github.com/dytonpictures/werk/internal/platform/database"
)

type recordingTransaction struct {
	query string
	args  []any
	err   error
}

func (transaction *recordingTransaction) Exec(_ context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	transaction.query = query
	transaction.args = args
	return pgconn.CommandTag{}, transaction.err
}

func (*recordingTransaction) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("unexpected query")
}

func (*recordingTransaction) QueryRow(context.Context, string, ...any) pgx.Row {
	panic("unexpected query row")
}

var _ database.TenantTx = (*recordingTransaction)(nil)

func coreauditEntry() coreaudit.Entry {
	tenantID := tenancy.TenantID{1}
	return coreaudit.Entry{
		ID:         [16]byte{1},
		TenantID:   tenantID,
		OccurredAt: time.Date(2026, 7, 22, 14, 0, 0, 0, time.UTC),
		EventType:  "core.documents.document-published.v1",
		Action:     "core.documents.document.publish",
		Outcome:    coreaudit.OutcomeSucceeded,
		InitiatedBy: coreaudit.ActorRef{
			AccountID: identity.AccountID{2}, AccountClass: identity.AccountClassWork, TenantID: tenantID,
		},
		ExecutedBy: coreaudit.ActorRef{
			AccountID: identity.AccountID{3}, AccountClass: identity.AccountClassService, TenantID: tenantID,
		},
		Subject: resource.TenantRef(tenantID, resource.KindDocumentCollection, resource.RootID),
		Policy: coreaudit.PolicySnapshot{
			Permission: "core.documents.document.create", ContractVersion: 1, ProcessingRequired: true,
			Processing: compliance.ProcessingContext{
				ActivityKey:   "core.documents.document-management",
				PurposeKey:    "core.documents.document-creation",
				LegalBasisRef: "operator.processing-register.documents",
			},
		},
		RequestID: [16]byte{4}, CorrelationID: [16]byte{5},
	}
}

func coreauditExecutor(t *testing.T) Executor {
	t.Helper()
	tenantID := tenancy.TenantID{1}
	executor, err := NewExecutor(identity.AuthenticatedActor{
		AccountID: identity.AccountID{3}, AccountClass: identity.AccountClassService,
		Audience: identity.AudienceService, Kind: identity.AuthenticationWorkload,
		Assurance: identity.AssuranceUnknown, TenantID: &tenantID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return executor
}

func TestAppendPersistsStructuredActorsSubjectAndPolicy(t *testing.T) {
	entry := coreauditEntry()
	entry.ExecutedBy = coreaudit.ActorRef{
		AccountID: identity.AccountID{99}, AccountClass: identity.AccountClassService, TenantID: entry.TenantID,
	}
	transaction := &recordingTransaction{}
	if err := Append(context.Background(), transaction, coreauditExecutor(t), entry); err != nil {
		t.Fatal(err)
	}
	for _, column := range []string{
		"initiated_by_account_id", "executed_by_account_id", "subject_kind",
		"permission_key", "processing_purpose_key", "correlation_id",
	} {
		if !strings.Contains(transaction.query, column) {
			t.Fatalf("audit insert is missing %s", column)
		}
	}
	if len(transaction.args) != 19 {
		t.Fatalf("audit insert argument count = %d, want 19", len(transaction.args))
	}
	if transaction.args[4] != entry.Action || transaction.args[11] != entry.Policy.Permission {
		t.Fatalf("unexpected audit arguments: %#v", transaction.args)
	}
	if transaction.args[7] != uuidString(identity.AccountID{3}) {
		t.Fatalf("executor was taken from entry instead of authenticated context: %#v", transaction.args[7])
	}
}

func TestAppendRejectsInvalidEntryAndPropagatesDatabaseFailure(t *testing.T) {
	entry := coreauditEntry()
	entry.InitiatedBy.AccountID = identity.AccountID{}
	if err := Append(context.Background(), &recordingTransaction{}, coreauditExecutor(t), entry); !errors.Is(err, coreaudit.ErrInvalidEntry) {
		t.Fatalf("invalid entry error = %v", err)
	}
	entry = coreauditEntry()
	want := errors.New("insert failed")
	if err := Append(context.Background(), &recordingTransaction{err: want}, coreauditExecutor(t), entry); !errors.Is(err, want) {
		t.Fatalf("database error = %v", err)
	}
	if err := Append(context.Background(), nil, coreauditExecutor(t), entry); !errors.Is(err, coreaudit.ErrInvalidEntry) {
		t.Fatalf("nil transaction error = %v", err)
	}
}

func TestNewExecutorRequiresAuthenticatedTenantWorkload(t *testing.T) {
	tenantID := tenancy.TenantID{1}
	if _, err := NewExecutor(identity.AuthenticatedActor{
		AccountID: identity.AccountID{3}, AccountClass: identity.AccountClassWork,
		Audience: identity.AudienceWork, Kind: identity.AuthenticationInteractive,
		Assurance: identity.AssuranceSingleFactor, TenantID: &tenantID,
	}); !errors.Is(err, ErrInvalidExecutor) {
		t.Fatalf("work executor error = %v", err)
	}
	if _, err := NewExecutor(identity.AuthenticatedActor{
		AccountID: identity.AccountID{3}, AccountClass: identity.AccountClassService,
		Audience: identity.AudienceService, Kind: identity.AuthenticationWorkload,
		Assurance: identity.AssuranceUnknown,
	}); !errors.Is(err, ErrInvalidExecutor) {
		t.Fatalf("global executor error = %v", err)
	}
}
