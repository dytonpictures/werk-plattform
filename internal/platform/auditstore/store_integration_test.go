package auditstore

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	coreaudit "github.com/dytonpictures/werk/internal/core/audit"
	"github.com/dytonpictures/werk/internal/core/compliance"
	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/resource"
	"github.com/dytonpictures/werk/internal/core/tenancy"
	"github.com/dytonpictures/werk/internal/platform/database"
)

func TestBusinessAuditIntegration(t *testing.T) {
	migratorURL := auditIntegrationEnvironment(t, "WERK_TEST_MIGRATOR_DATABASE_URL")
	serviceURL := auditIntegrationEnvironment(t, "WERK_TEST_SERVICE_DATABASE_URL")
	workURL := auditIntegrationEnvironment(t, "WERK_TEST_WORK_DATABASE_URL")
	identityURL := auditIntegrationEnvironment(t, "WERK_TEST_IDENTITY_DATABASE_URL")
	adminURL := auditIntegrationEnvironment(t, "WERK_TEST_ADMIN_DATABASE_URL")
	workerURL := auditIntegrationEnvironment(t, "WERK_TEST_WORKER_DATABASE_URL")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ownerPool, err := pgxpool.New(ctx, migratorURL)
	if err != nil {
		t.Fatalf("open migrator pool: %v", err)
	}
	defer ownerPool.Close()
	owner, err := ownerPool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire migrator connection: %v", err)
	}
	defer owner.Release()
	if _, err := owner.Exec(ctx, `SET ROLE werk_owner`); err != nil {
		t.Fatalf("assume owner role for audit fixtures: %v", err)
	}

	tenantA := freshAuditTenantID(t)
	tenantB := freshAuditTenantID(t)
	partyA := freshAuditTenantID(t)
	partyB := freshAuditTenantID(t)
	workA := identity.AccountID(freshAuditTenantID(t))
	workB := identity.AccountID(freshAuditTenantID(t))
	serviceA := identity.AccountID(freshAuditTenantID(t))
	serviceB := identity.AccountID(freshAuditTenantID(t))

	if _, err := owner.Exec(ctx, `
		INSERT INTO werk_core.tenants (id, name, status, default_locale, default_timezone)
		VALUES ($1::uuid, 'Audit tenant A', 'active', 'de-DE', 'Europe/Berlin'),
		       ($2::uuid, 'Audit tenant B', 'active', 'de-DE', 'Europe/Berlin')
	`, tenantA.String(), tenantB.String()); err != nil {
		t.Fatalf("insert audit tenants: %v", err)
	}
	if _, err := owner.Exec(ctx, `
		INSERT INTO werk_core.parties (id, tenant_id, party_type, display_name, status)
		VALUES ($1::uuid, $2::uuid, 'person', 'Audit initiator A', 'active'),
		       ($3::uuid, $4::uuid, 'person', 'Audit initiator B', 'active')
	`, partyA.String(), tenantA.String(), partyB.String(), tenantB.String()); err != nil {
		t.Fatalf("insert audit parties: %v", err)
	}
	if _, err := owner.Exec(ctx, `
		INSERT INTO werk_core.persons (party_id, tenant_id, given_name, family_name)
		VALUES ($1::uuid, $2::uuid, 'Audit', 'A'),
		       ($3::uuid, $4::uuid, 'Audit', 'B')
	`, partyA.String(), tenantA.String(), partyB.String(), tenantB.String()); err != nil {
		t.Fatalf("insert audit persons: %v", err)
	}
	if _, err := owner.Exec(ctx, `
		INSERT INTO werk_core.accounts (
			id, account_class, tenant_id, person_party_id, login_name, status
		) VALUES
			($1::uuid, 'work', $2::uuid, $3::uuid, $4, 'active'),
			($5::uuid, 'work', $6::uuid, $7::uuid, $8, 'active')
	`, uuidString(workA), tenantA.String(), partyA.String(), auditLogin(workA),
		uuidString(workB), tenantB.String(), partyB.String(), auditLogin(workB)); err != nil {
		t.Fatalf("insert audit work actors: %v", err)
	}
	if _, err := owner.Exec(ctx, `
		INSERT INTO werk_core.accounts (
			id, account_class, tenant_id, login_name, status
		) VALUES
			($1::uuid, 'service', $2::uuid, $3, 'active'),
			($4::uuid, 'service', $5::uuid, $6, 'active')
	`, uuidString(serviceA), tenantA.String(), auditLogin(serviceA),
		uuidString(serviceB), tenantB.String(), auditLogin(serviceB)); err != nil {
		t.Fatalf("insert audit service actors: %v", err)
	}

	serviceDatabase, err := database.NewService(ctx, serviceURL, "werk-business-audit-integration")
	if err != nil {
		t.Fatalf("open service database: %v", err)
	}
	defer serviceDatabase.Close()
	executorA := newIntegrationExecutor(t, tenantA, serviceA)
	executorB := newIntegrationExecutor(t, tenantB, serviceB)

	entry := newIntegrationAuditEntry(t, tenantA, workA, serviceA)
	if err := serviceDatabase.WithinTenantWrite(ctx, tenantA, func(ctx context.Context, tx database.TenantTx) error {
		return Append(ctx, tx, executorA, entry)
	}); err != nil {
		t.Fatalf("append tenant business audit: %v", err)
	}

	foreignEntry := newIntegrationAuditEntry(t, tenantB, workB, serviceB)
	if err := serviceDatabase.WithinTenantWrite(ctx, tenantA, func(ctx context.Context, tx database.TenantTx) error {
		return Append(ctx, tx, executorB, foreignEntry)
	}); err == nil {
		t.Fatal("service tenant A wrote a tenant B audit entry")
	}

	crossTenantExecutor := newIntegrationAuditEntry(t, tenantA, workA, serviceA)
	if err := serviceDatabase.WithinTenantWrite(ctx, tenantA, func(ctx context.Context, tx database.TenantTx) error {
		return Append(ctx, tx, executorB, crossTenantExecutor)
	}); err == nil {
		t.Fatal("business audit accepted an authenticated executor from another tenant")
	}

	wrongPolicy := newIntegrationAuditEntry(t, tenantA, workA, serviceA)
	wrongPolicy.Policy.Processing.PurposeKey = "core.documents.unregistered-purpose"
	if err := serviceDatabase.WithinTenantWrite(ctx, tenantA, func(ctx context.Context, tx database.TenantTx) error {
		return Append(ctx, tx, executorA, wrongPolicy)
	}); err == nil {
		t.Fatal("business audit accepted a client-selected policy snapshot")
	}
	wrongPermission := newIntegrationAuditEntry(t, tenantA, workA, serviceA)
	wrongPermission.Policy.Permission = "core.documents.document.read"
	if err := serviceDatabase.WithinTenantWrite(ctx, tenantA, func(ctx context.Context, tx database.TenantTx) error {
		return Append(ctx, tx, executorA, wrongPermission)
	}); err == nil {
		t.Fatal("business audit accepted a permission not registered for its action")
	}

	servicePool, err := pgxpool.New(ctx, serviceURL)
	if err != nil {
		t.Fatalf("open raw service pool: %v", err)
	}
	defer servicePool.Close()
	withoutTenant := newIntegrationAuditEntry(t, tenantA, workA, serviceA)
	if err := Append(ctx, servicePool, executorA, withoutTenant); err == nil {
		t.Fatal("service wrote a business audit entry without transaction tenant")
	}
	adminDatabase, err := database.NewAdmin(ctx, adminURL, "werk-business-audit-admin-negative")
	if err != nil {
		t.Fatalf("open admin database for audit negative test: %v", err)
	}
	if err := adminDatabase.WithinTenantWrite(ctx, tenantA, func(ctx context.Context, tx database.TenantTx) error {
		return Append(ctx, tx, executorA, newIntegrationAuditEntry(t, tenantA, workA, serviceA))
	}); err == nil {
		adminDatabase.Close()
		t.Fatal("admin tenant transaction wrote a structured business audit entry")
	}
	adminDatabase.Close()
	for _, unauthorized := range []struct {
		name string
		url  string
	}{
		{name: "work", url: workURL},
		{name: "identity", url: identityURL},
		{name: "admin", url: adminURL},
		{name: "worker", url: workerURL},
	} {
		rolePool, err := pgxpool.New(ctx, unauthorized.url)
		if err != nil {
			t.Fatalf("open %s audit role pool: %v", unauthorized.name, err)
		}
		err = Append(ctx, rolePool, executorA, newIntegrationAuditEntry(t, tenantA, workA, serviceA))
		rolePool.Close()
		if err == nil {
			t.Fatalf("%s runtime wrote a structured business audit entry", unauthorized.name)
		}
	}
	if err := serviceDatabase.WithinTenantRead(ctx, tenantA, func(ctx context.Context, tx database.TenantTx) error {
		var count int
		return tx.QueryRow(ctx, `SELECT count(*) FROM werk_core.security_audit_events`).Scan(&count)
	}); err == nil {
		t.Fatal("service runtime can read the authoritative audit log")
	}

	var initiatedBy string
	var executedBy string
	var subjectKind string
	var purposeKey string
	if err := owner.QueryRow(ctx, `
		SELECT initiated_by_account_id::text, executed_by_account_id::text,
		       subject_kind, processing_purpose_key
		FROM werk_core.security_audit_events
		WHERE id = $1::uuid
	`, uuidString(entry.ID)).Scan(&initiatedBy, &executedBy, &subjectKind, &purposeKey); err != nil {
		t.Fatalf("read persisted business audit: %v", err)
	}
	if initiatedBy != uuidString(workA) || executedBy != uuidString(serviceA) ||
		subjectKind != string(resource.KindDocumentCollection) ||
		purposeKey != "core.documents.document-creation" {
		t.Fatalf("unexpected business audit projection: %s %s %s %s", initiatedBy, executedBy, subjectKind, purposeKey)
	}
	if _, err := owner.Exec(ctx, `
		UPDATE werk_core.security_audit_events SET outcome = 'failed' WHERE id = $1::uuid
	`, uuidString(entry.ID)); err == nil {
		t.Fatal("owner mutated an append-only audit entry")
	}
	if _, err := owner.Exec(ctx, `
		UPDATE werk_core.audit_action_contracts
		SET contract_version = contract_version + 1
		WHERE event_type = 'core.documents.document-published.v1'
		  AND action_key = 'core.documents.document.publish'
	`); err == nil {
		t.Fatal("owner changed the meaning of a versioned audit action contract")
	}
	retirement, err := owner.Begin(ctx)
	if err != nil {
		t.Fatalf("begin audit action retirement check: %v", err)
	}
	if _, err := retirement.Exec(ctx, `
		UPDATE werk_core.audit_action_contracts
		SET status = 'retired'
		WHERE event_type = 'core.documents.document-published.v1'
		  AND action_key = 'core.documents.document.publish'
	`); err != nil {
		_ = retirement.Rollback(ctx)
		t.Fatalf("retire audit action contract: %v", err)
	}
	if err := retirement.Rollback(ctx); err != nil {
		t.Fatalf("rollback audit action retirement check: %v", err)
	}
	var queued int
	if err := owner.QueryRow(ctx, `
		SELECT count(*) FROM werk_core.security_audit_export_queue WHERE audit_event_id = $1::uuid
	`, uuidString(entry.ID)).Scan(&queued); err != nil || queued != 1 {
		t.Fatalf("business audit export queue count = %d, err = %v", queued, err)
	}
}

func newIntegrationExecutor(t *testing.T, tenantID tenancy.TenantID, accountID identity.AccountID) Executor {
	t.Helper()
	executor, err := NewExecutor(identity.AuthenticatedActor{
		AccountID: accountID, AccountClass: identity.AccountClassService,
		Audience: identity.AudienceService, Kind: identity.AuthenticationWorkload,
		Assurance: identity.AssuranceUnknown, TenantID: &tenantID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return executor
}

func newIntegrationAuditEntry(t *testing.T, tenantID tenancy.TenantID, initiatedBy, executedBy identity.AccountID) coreaudit.Entry {
	t.Helper()
	return coreaudit.Entry{
		ID:         [16]byte(freshAuditTenantID(t)),
		TenantID:   tenantID,
		OccurredAt: time.Now().UTC(),
		EventType:  "core.documents.document-published.v1",
		Action:     "core.documents.document.publish",
		Outcome:    coreaudit.OutcomeSucceeded,
		InitiatedBy: coreaudit.ActorRef{
			AccountID: initiatedBy, AccountClass: identity.AccountClassWork, TenantID: tenantID,
		},
		ExecutedBy: coreaudit.ActorRef{
			AccountID: executedBy, AccountClass: identity.AccountClassService, TenantID: tenantID,
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
		RequestID:     [16]byte(freshAuditTenantID(t)),
		CorrelationID: [16]byte(freshAuditTenantID(t)),
	}
}

func freshAuditTenantID(t *testing.T) tenancy.TenantID {
	t.Helper()
	id, err := tenancy.NewTenantID()
	if err != nil {
		t.Fatalf("generate audit UUID: %v", err)
	}
	return id
}

func auditLogin(accountID identity.AccountID) string {
	return "audit-" + uuidString(accountID) + "@example.invalid"
}

func auditIntegrationEnvironment(t *testing.T, key string) string {
	t.Helper()
	value := os.Getenv(key)
	if value == "" {
		t.Skip("PostgreSQL integration environment is not configured")
	}
	return value
}

func TestServiceDatabaseRejectsZeroTenantBeforeConnection(t *testing.T) {
	if os.Getenv("WERK_TEST_SERVICE_DATABASE_URL") == "" {
		t.Skip("PostgreSQL integration environment is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	databaseURL := os.Getenv("WERK_TEST_SERVICE_DATABASE_URL")
	serviceDatabase, err := database.NewService(ctx, databaseURL, "werk-service-zero-tenant-test")
	if err != nil {
		t.Fatal(err)
	}
	defer serviceDatabase.Close()
	if err := serviceDatabase.WithinTenantWrite(ctx, tenancy.TenantID{}, func(context.Context, database.TenantTx) error {
		return fmt.Errorf("operation should not execute")
	}); err == nil {
		t.Fatal("service database accepted a zero tenant")
	}
}
