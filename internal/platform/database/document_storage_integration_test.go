package database

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dytonpictures/werk/internal/core/party"
	"github.com/dytonpictures/werk/internal/core/tenancy"
)

func TestDocumentStorageFoundationIntegration(t *testing.T) {
	migratorURL := integrationEnvironment(t, "WERK_TEST_MIGRATOR_DATABASE_URL")
	workURL := integrationEnvironment(t, "WERK_TEST_WORK_DATABASE_URL")
	serviceURL := integrationEnvironment(t, "WERK_TEST_SERVICE_DATABASE_URL")
	workerURL := integrationEnvironment(t, "WERK_TEST_WORKER_DATABASE_URL")
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
		t.Fatalf("assume owner role for document fixtures: %v", err)
	}

	tenantA, err := tenancy.NewTenant("Document tenant A", "de-DE", "Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	tenantB, err := tenancy.NewTenant("Document tenant B", "de-DE", "Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	partyA, personA, err := party.NewPerson(tenantA.ID, "Document", "Actor A")
	if err != nil {
		t.Fatal(err)
	}
	partyB, personB, err := party.NewPerson(tenantB.ID, "Document", "Actor B")
	if err != nil {
		t.Fatal(err)
	}
	accountA := freshDatabaseUUID(t)
	accountB := freshDatabaseUUID(t)
	blobA := freshDatabaseUUID(t)
	blobB := freshDatabaseUUID(t)
	locationA := freshDatabaseUUID(t)
	locationB := freshDatabaseUUID(t)
	locationBReplica := freshDatabaseUUID(t)
	documentA := freshDatabaseUUID(t)
	documentB := freshDatabaseUUID(t)
	versionA := freshDatabaseUUID(t)
	versionB := freshDatabaseUUID(t)
	classificationA := freshDatabaseUUID(t)
	classificationB := freshDatabaseUUID(t)
	createdAt := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)

	if _, err := owner.Exec(ctx, `
		INSERT INTO werk_core.tenants (id, name, status, default_locale, default_timezone)
		VALUES ($1::uuid, $2, 'active', 'de-DE', 'Europe/Berlin'),
		       ($3::uuid, $4, 'active', 'de-DE', 'Europe/Berlin')
	`, tenantA.ID.String(), tenantA.Name, tenantB.ID.String(), tenantB.Name); err != nil {
		t.Fatalf("insert document tenant fixtures: %v", err)
	}
	if _, err := owner.Exec(ctx, `
		INSERT INTO werk_core.parties (id, tenant_id, party_type, display_name, status)
		VALUES ($1::uuid, $2::uuid, 'person', $3, 'active'),
		       ($4::uuid, $5::uuid, 'person', $6, 'active')
	`, databaseUUID([16]byte(partyA.ID)), tenantA.ID.String(), partyA.DisplayName,
		databaseUUID([16]byte(partyB.ID)), tenantB.ID.String(), partyB.DisplayName); err != nil {
		t.Fatalf("insert document actor parties: %v", err)
	}
	if _, err := owner.Exec(ctx, `
		INSERT INTO werk_core.persons (party_id, tenant_id, given_name, family_name)
		VALUES ($1::uuid, $2::uuid, $3, $4),
		       ($5::uuid, $6::uuid, $7, $8)
	`, databaseUUID([16]byte(partyA.ID)), tenantA.ID.String(), personA.GivenName, personA.FamilyName,
		databaseUUID([16]byte(partyB.ID)), tenantB.ID.String(), personB.GivenName, personB.FamilyName); err != nil {
		t.Fatalf("insert document actor persons: %v", err)
	}
	if _, err := owner.Exec(ctx, `
		INSERT INTO werk_core.accounts (
			id, account_class, tenant_id, person_party_id, login_name, status
		) VALUES
			($1::uuid, 'work', $2::uuid, $3::uuid, $4, 'active'),
			($5::uuid, 'work', $6::uuid, $7::uuid, $8, 'active')
	`, accountA, tenantA.ID.String(), databaseUUID([16]byte(partyA.ID)), "document-"+accountA+"@example.invalid",
		accountB, tenantB.ID.String(), databaseUUID([16]byte(partyB.ID)), "document-"+accountB+"@example.invalid"); err != nil {
		t.Fatalf("insert document actor accounts: %v", err)
	}

	if _, err := owner.Exec(ctx, `
		INSERT INTO werk_core.storage_blobs (
			id, tenant_id, state, created_by_account_id, created_at, updated_at
		) VALUES
			($1::uuid, $2::uuid, 'quarantined', $3::uuid, $4, $4),
			($5::uuid, $6::uuid, 'quarantined', $7::uuid, $4, $4)
	`, blobA, tenantA.ID.String(), accountA, createdAt, blobB, tenantB.ID.String(), accountB); err != nil {
		t.Fatalf("insert quarantined blobs: %v", err)
	}
	if _, err := owner.Exec(ctx, `
		INSERT INTO werk_core.storage_blobs (
			id, tenant_id, state, size_bytes, sha256, media_type,
			created_by_account_id, created_at, verified_at, updated_at, version
		) VALUES (
			$1::uuid, $2::uuid, 'available', 1, decode(repeat('01', 32), 'hex'),
			'application/pdf', $3::uuid, $4, $4, $4, 2
		)
	`, freshDatabaseUUID(t), tenantA.ID.String(), accountA, createdAt); err == nil {
		t.Fatal("storage accepted a blob that skipped quarantine")
	}
	if _, err := owner.Exec(ctx, `
		INSERT INTO werk_core.storage_blob_locations (
			id, tenant_id, blob_id, provider_key, opaque_key, state, created_at, updated_at
		) VALUES
			($1::uuid, $2::uuid, $3::uuid, 'internal.s3', $4::uuid, 'quarantined', $5, $5),
			($6::uuid, $7::uuid, $8::uuid, 'internal.s3', $9::uuid, 'quarantined', $5, $5)
	`, locationA, tenantA.ID.String(), blobA, freshDatabaseUUID(t), createdAt,
		locationB, tenantB.ID.String(), blobB, freshDatabaseUUID(t)); err != nil {
		t.Fatalf("insert quarantined blob locations: %v", err)
	}
	if _, err := owner.Exec(ctx, `
		UPDATE werk_core.storage_blobs
		SET state = 'available', size_bytes = 128, sha256 = decode(repeat('02', 32), 'hex'),
		    media_type = 'application/pdf', verified_at = $3, updated_at = $3, version = 2
		WHERE tenant_id = $1::uuid AND id = $2::uuid
	`, tenantA.ID.String(), blobA, createdAt.Add(time.Second)); err == nil {
		t.Fatal("storage sealed a blob before its location became available")
	}
	if _, err := owner.Exec(ctx, `
		UPDATE werk_core.storage_blob_locations
		SET state = 'available', provider_checksum = 'provider-checksum',
		    activated_at = $5, updated_at = $5, version = 2
		WHERE (tenant_id = $1::uuid AND id = $2::uuid)
		   OR (tenant_id = $3::uuid AND id = $4::uuid)
	`, tenantA.ID.String(), locationA, tenantB.ID.String(), locationB, createdAt.Add(time.Second)); err != nil {
		t.Fatalf("activate blob locations: %v", err)
	}
	if _, err := owner.Exec(ctx, `
		UPDATE werk_core.storage_blobs
		SET state = 'available', size_bytes = 128, sha256 = decode(repeat('02', 32), 'hex'),
		    media_type = 'application/pdf', verified_at = $5, updated_at = $5, version = 2
		WHERE (tenant_id = $1::uuid AND id = $2::uuid)
		   OR (tenant_id = $3::uuid AND id = $4::uuid)
	`, tenantA.ID.String(), blobA, tenantB.ID.String(), blobB, createdAt.Add(2*time.Second)); err != nil {
		t.Fatalf("seal tenant blobs: %v", err)
	}
	if _, err := owner.Exec(ctx, `
		INSERT INTO werk_core.storage_blob_locations (
			id, tenant_id, blob_id, provider_key, opaque_key, state, created_at, updated_at
		) VALUES ($1::uuid, $2::uuid, $3::uuid, 'internal.s3', $4::uuid, 'quarantined', $5, $5)
	`, locationBReplica, tenantB.ID.String(), blobB, freshDatabaseUUID(t), createdAt.Add(3*time.Second)); err != nil {
		t.Fatalf("insert replicated blob location: %v", err)
	}
	if _, err := owner.Exec(ctx, `
		UPDATE werk_core.storage_blob_locations
		SET state = 'available', provider_checksum = 'replica-checksum',
		    activated_at = $2, updated_at = $2, version = 2
		WHERE id = $1::uuid
	`, locationBReplica, createdAt.Add(4*time.Second)); err != nil {
		t.Fatalf("activate replicated blob location: %v", err)
	}

	if _, err := owner.Exec(ctx, `
		INSERT INTO werk_core.documents (
			id, tenant_id, title, status, source_module, created_by_account_id,
			created_at, updated_at
		) VALUES ($1::uuid, $2::uuid, 'Archived initially', 'archived', 'core.documents', $3::uuid, $4, $4)
	`, freshDatabaseUUID(t), tenantA.ID.String(), accountA, createdAt.Add(3*time.Second)); err == nil {
		t.Fatal("database accepted an initially archived document")
	}

	documentTransaction, err := owner.Begin(ctx)
	if err != nil {
		t.Fatalf("begin atomic document publication: %v", err)
	}
	if _, err := documentTransaction.Exec(ctx, `
		INSERT INTO werk_core.documents (
			id, tenant_id, title, status, source_module, created_by_account_id,
			created_at, updated_at
		) VALUES
			($1::uuid, $2::uuid, 'Tenant A document', 'active', 'core.documents', $3::uuid, $4, $4),
			($5::uuid, $6::uuid, 'Tenant B document', 'active', 'core.documents', $7::uuid, $4, $4)
	`, documentA, tenantA.ID.String(), accountA, createdAt.Add(3*time.Second),
		documentB, tenantB.ID.String(), accountB); err != nil {
		_ = documentTransaction.Rollback(ctx)
		t.Fatalf("insert tenant documents: %v", err)
	}
	if _, err := documentTransaction.Exec(ctx, `
		INSERT INTO werk_core.document_versions (
			id, tenant_id, document_id, version_number, blob_id, source,
			created_by_account_id, published_at
		) VALUES
			($1::uuid, $2::uuid, $3::uuid, 1, $4::uuid, 'upload', $5::uuid, $6),
			($7::uuid, $8::uuid, $9::uuid, 1, $10::uuid, 'upload', $11::uuid, $6)
	`, versionA, tenantA.ID.String(), documentA, blobA, accountA, createdAt.Add(4*time.Second),
		versionB, tenantB.ID.String(), documentB, blobB, accountB); err != nil {
		_ = documentTransaction.Rollback(ctx)
		t.Fatalf("publish tenant document versions: %v", err)
	}
	if _, err := documentTransaction.Exec(ctx, `
		INSERT INTO werk_core.document_classification_revisions (
			id, tenant_id, document_id, revision, classification, retention_class,
			legal_hold, recorded_by_account_id, recorded_at
		) VALUES
			($1::uuid, $2::uuid, $3::uuid, 1, 'confidential', 'business.standard', false, $4::uuid, $5),
			($6::uuid, $7::uuid, $8::uuid, 1, 'restricted', 'business.standard', false, $9::uuid, $5)
	`, classificationA, tenantA.ID.String(), documentA, accountA, createdAt.Add(4*time.Second),
		classificationB, tenantB.ID.String(), documentB, accountB); err != nil {
		_ = documentTransaction.Rollback(ctx)
		t.Fatalf("classify tenant documents: %v", err)
	}
	if err := documentTransaction.Commit(ctx); err != nil {
		t.Fatalf("commit atomic document publication: %v", err)
	}

	incompleteDocumentTransaction, err := owner.Begin(ctx)
	if err != nil {
		t.Fatalf("begin incomplete document transaction: %v", err)
	}
	if _, err := incompleteDocumentTransaction.Exec(ctx, `
		INSERT INTO werk_core.documents (
			id, tenant_id, title, status, source_module, created_by_account_id,
			created_at, updated_at
		) VALUES ($1::uuid, $2::uuid, 'Incomplete', 'active', 'core.documents', $3::uuid, $4, $4)
	`, freshDatabaseUUID(t), tenantA.ID.String(), accountA, createdAt.Add(5*time.Second)); err != nil {
		_ = incompleteDocumentTransaction.Rollback(ctx)
		t.Fatalf("insert incomplete document fixture: %v", err)
	}
	if err := incompleteDocumentTransaction.Commit(ctx); err == nil {
		t.Fatal("document without initial version and classification was committed")
	}

	if _, err := owner.Exec(ctx, `
		INSERT INTO werk_core.document_versions (
			id, tenant_id, document_id, version_number, blob_id, source,
			created_by_account_id, published_at
		) VALUES ($1::uuid, $2::uuid, $3::uuid, 2, $4::uuid, 'upload', $5::uuid, $6)
	`, freshDatabaseUUID(t), tenantA.ID.String(), documentA, blobB, accountA, createdAt.Add(5*time.Second)); err == nil {
		t.Fatal("document version accepted a foreign-tenant blob")
	}
	if _, err := owner.Exec(ctx, `UPDATE werk_core.document_versions SET source = 'import' WHERE id = $1::uuid`, versionA); err == nil {
		t.Fatal("published document version was mutable")
	}
	if _, err := owner.Exec(ctx, `DELETE FROM werk_core.document_classification_revisions WHERE id = $1::uuid`, classificationA); err == nil {
		t.Fatal("published document classification was deletable")
	}
	if _, err := owner.Exec(ctx, `UPDATE werk_core.storage_blobs SET media_type = 'text/plain', version = 3, updated_at = $2 WHERE id = $1::uuid`, blobA, createdAt.Add(6*time.Second)); err == nil {
		t.Fatal("available blob content metadata was mutable")
	}
	if _, err := owner.Exec(ctx, `
		UPDATE werk_core.storage_blobs
		SET state = 'unknown', version = 3, updated_at = $2
		WHERE id = $1::uuid
	`, blobA, createdAt.Add(7*time.Second)); err != nil {
		t.Fatalf("mark provider result unknown: %v", err)
	}
	if _, err := owner.Exec(ctx, `
		UPDATE werk_core.storage_blobs
		SET state = 'available', version = 4, updated_at = $2
		WHERE id = $1::uuid
	`, blobA, createdAt.Add(8*time.Second)); err != nil {
		t.Fatalf("restore blob after successful provider check: %v", err)
	}
	if _, err := owner.Exec(ctx, `
		UPDATE werk_core.storage_blob_locations
		SET state = 'missing', provider_checksum = 'tampered',
		    activated_at = $2, updated_at = $2, version = 3
		WHERE id = $1::uuid
	`, locationA, createdAt.Add(9*time.Second)); err == nil {
		t.Fatal("missing transition rewrote historical location verification")
	}
	missingTransaction, err := owner.Begin(ctx)
	if err != nil {
		t.Fatalf("begin missing blob transition: %v", err)
	}
	if _, err := missingTransaction.Exec(ctx, `
		UPDATE werk_core.storage_blob_locations
		SET state = 'missing', version = 3, updated_at = $2
		WHERE id = $1::uuid
	`, locationA, createdAt.Add(9*time.Second)); err != nil {
		_ = missingTransaction.Rollback(ctx)
		t.Fatalf("mark blob location missing: %v", err)
	}
	if _, err := missingTransaction.Exec(ctx, `
		UPDATE werk_core.storage_blobs
		SET state = 'missing', version = 5, updated_at = $2
		WHERE id = $1::uuid
	`, blobA, createdAt.Add(9*time.Second)); err != nil {
		_ = missingTransaction.Rollback(ctx)
		t.Fatalf("mark blob missing: %v", err)
	}
	if err := missingTransaction.Commit(ctx); err != nil {
		t.Fatalf("commit missing blob transition: %v", err)
	}
	if _, err := owner.Exec(ctx, `
		UPDATE werk_core.storage_blobs
		SET state = 'available', version = 6, updated_at = $2
		WHERE id = $1::uuid
	`, blobA, createdAt.Add(10*time.Second)); err == nil {
		t.Fatal("missing blob became available without a repaired location")
	}
	repairedLocation := freshDatabaseUUID(t)
	repairTransaction, err := owner.Begin(ctx)
	if err != nil {
		t.Fatalf("begin blob repair: %v", err)
	}
	if _, err := repairTransaction.Exec(ctx, `
		INSERT INTO werk_core.storage_blob_locations (
			id, tenant_id, blob_id, provider_key, opaque_key, state, created_at, updated_at
		) VALUES ($1::uuid, $2::uuid, $3::uuid, 'internal.s3', $4::uuid, 'quarantined', $5, $5)
	`, repairedLocation, tenantA.ID.String(), blobA, freshDatabaseUUID(t), createdAt.Add(10*time.Second)); err != nil {
		_ = repairTransaction.Rollback(ctx)
		t.Fatalf("insert repaired blob location: %v", err)
	}
	if _, err := repairTransaction.Exec(ctx, `
		UPDATE werk_core.storage_blob_locations
		SET state = 'available', provider_checksum = 'repaired-checksum',
		    activated_at = $2, updated_at = $2, version = 2
		WHERE id = $1::uuid
	`, repairedLocation, createdAt.Add(11*time.Second)); err != nil {
		_ = repairTransaction.Rollback(ctx)
		t.Fatalf("activate repaired blob location: %v", err)
	}
	if _, err := repairTransaction.Exec(ctx, `
		UPDATE werk_core.storage_blobs
		SET state = 'available', version = 6, updated_at = $2
		WHERE id = $1::uuid
	`, blobA, createdAt.Add(11*time.Second)); err != nil {
		_ = repairTransaction.Rollback(ctx)
		t.Fatalf("restore repaired blob: %v", err)
	}
	if err := repairTransaction.Commit(ctx); err != nil {
		t.Fatalf("commit repaired blob: %v", err)
	}
	if _, err := owner.Exec(ctx, `
		UPDATE werk_core.documents
		SET status = 'archived', version = 2, updated_at = $2
		WHERE id = $1::uuid
	`, documentA, createdAt.Add(12*time.Second)); err != nil {
		t.Fatalf("archive document: %v", err)
	}
	if _, err := owner.Exec(ctx, `
		INSERT INTO werk_core.document_versions (
			id, tenant_id, document_id, version_number, blob_id, source,
			created_by_account_id, published_at
		) VALUES ($1::uuid, $2::uuid, $3::uuid, 2, $4::uuid, 'upload', $5::uuid, $6)
	`, freshDatabaseUUID(t), tenantA.ID.String(), documentA, blobA, accountA, createdAt.Add(13*time.Second)); err == nil {
		t.Fatal("archived document accepted a new version")
	}

	assertParallelLocationFailuresAreSerialized(
		t, ctx, ownerPool, tenantB.ID, blobB, locationB, locationBReplica, createdAt.Add(20*time.Second),
	)

	assertDocumentStorageCatalog(t, ctx, owner)

	workDatabase, err := NewWork(ctx, workURL, "werk-integration-document-work")
	if err != nil {
		t.Fatalf("open work runtime: %v", err)
	}
	defer workDatabase.Close()
	assertTenantDocumentVisibility(t, ctx, workDatabase, tenantA.ID, documentA)
	assertTenantDocumentVisibility(t, ctx, workDatabase, tenantB.ID, documentB)
	if err := workDatabase.runtime.WithinTenantRead(ctx, tenantA.ID, func(ctx context.Context, tx TenantTx) error {
		var count int
		return tx.QueryRow(ctx, `SELECT count(*) FROM werk_core.storage_blobs`).Scan(&count)
	}); err == nil {
		t.Fatal("work runtime can read raw storage metadata")
	}
	if err := workDatabase.runtime.WithinTenantWrite(ctx, tenantA.ID, func(ctx context.Context, tx TenantTx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO werk_core.documents (
				id, tenant_id, title, source_module, created_by_account_id, created_at, updated_at
			) VALUES ($1::uuid, $2::uuid, 'Bypass', 'core.documents', $3::uuid, $4, $4)
		`, freshDatabaseUUID(t), tenantA.ID.String(), accountA, createdAt)
		return err
	}); err == nil {
		t.Fatal("work runtime bypassed the future document service")
	}

	serviceDatabase, err := NewRuntime(ctx, serviceURL, RuntimeOptions{
		ExpectedRole:    ServiceRuntimeRole,
		ApplicationName: "werk-integration-document-service",
		MaxConnections:  1,
	})
	if err != nil {
		t.Fatalf("open service runtime: %v", err)
	}
	defer serviceDatabase.Close()
	var serviceRowsWithoutTenant int
	if err := serviceDatabase.pool.QueryRow(ctx, `SELECT count(*) FROM werk_core.documents`).Scan(&serviceRowsWithoutTenant); err != nil {
		t.Fatalf("query service documents without tenant: %v", err)
	}
	if serviceRowsWithoutTenant != 0 {
		t.Fatalf("service documents visible without tenant = %d, want 0", serviceRowsWithoutTenant)
	}
	for _, tenantID := range []tenancy.TenantID{tenantA.ID, tenantB.ID} {
		if err := serviceDatabase.WithinTenantRead(ctx, tenantID, func(ctx context.Context, tx TenantTx) error {
			var documentsCount, blobsCount int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM werk_core.documents`).Scan(&documentsCount); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM werk_core.storage_blobs`).Scan(&blobsCount); err != nil {
				return err
			}
			if documentsCount != 1 || blobsCount != 1 {
				return fmt.Errorf("service tenant counts documents=%d blobs=%d, want 1/1", documentsCount, blobsCount)
			}
			return nil
		}); err != nil {
			t.Fatalf("service tenant boundary for %s: %v", tenantID.String(), err)
		}
	}
	if err := serviceDatabase.WithinTenantWrite(ctx, tenantA.ID, func(ctx context.Context, tx TenantTx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO werk_core.storage_blobs (
				id, tenant_id, state, created_by_account_id, created_at, updated_at
			) VALUES ($1::uuid, $2::uuid, 'quarantined', $3::uuid, $4, $4)
		`, freshDatabaseUUID(t), tenantB.ID.String(), accountB, createdAt)
		return err
	}); err == nil {
		t.Fatal("service runtime inserted a blob for another tenant")
	}

	workerDatabase, err := NewWorker(ctx, workerURL, "werk-integration-document-worker")
	if err != nil {
		t.Fatalf("open worker runtime: %v", err)
	}
	defer workerDatabase.Close()
	if err := workerDatabase.WithinTenantWrite(ctx, tenantA.ID, func(ctx context.Context, tx TenantTx) error {
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM werk_core.storage_blobs`).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("tenant A storage blob count = %d, want 1", count)
		}
		return nil
	}); err != nil {
		t.Fatalf("worker tenant storage boundary: %v", err)
	}
	if err := workerDatabase.WithinTenantWrite(ctx, tenantA.ID, func(ctx context.Context, tx TenantTx) error {
		var count int
		return tx.QueryRow(ctx, `SELECT count(*) FROM werk_core.documents`).Scan(&count)
	}); err == nil {
		t.Fatal("storage worker can read document metadata")
	}
}

func freshDatabaseUUID(t *testing.T) string {
	t.Helper()
	id, err := tenancy.NewTenantID()
	if err != nil {
		t.Fatalf("generate database UUID: %v", err)
	}
	return id.String()
}

func databaseUUID(value [16]byte) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
}

func assertParallelLocationFailuresAreSerialized(
	t *testing.T,
	ctx context.Context,
	ownerPool *pgxpool.Pool,
	tenantID tenancy.TenantID,
	blobID, firstLocationID, secondLocationID string,
	changedAt time.Time,
) {
	t.Helper()
	firstConnection, err := ownerPool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire first parallel owner connection: %v", err)
	}
	defer firstConnection.Release()
	secondConnection, err := ownerPool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire second parallel owner connection: %v", err)
	}
	defer secondConnection.Release()
	for _, connection := range []*pgxpool.Conn{firstConnection, secondConnection} {
		if _, err := connection.Exec(ctx, `SET ROLE werk_owner`); err != nil {
			t.Fatalf("assume owner role for parallel location test: %v", err)
		}
	}

	firstTransaction, err := firstConnection.Begin(ctx)
	if err != nil {
		t.Fatalf("begin first location failure: %v", err)
	}
	if _, err := firstTransaction.Exec(ctx, `
		UPDATE werk_core.storage_blob_locations
		SET state = 'missing', version = 3, updated_at = $2
		WHERE id = $1::uuid
	`, firstLocationID, changedAt); err != nil {
		_ = firstTransaction.Rollback(ctx)
		t.Fatalf("stage first location failure: %v", err)
	}

	secondTransaction, err := secondConnection.Begin(ctx)
	if err != nil {
		_ = firstTransaction.Rollback(ctx)
		t.Fatalf("begin competing location failure: %v", err)
	}
	if _, err := secondTransaction.Exec(ctx, `SET LOCAL lock_timeout = '250ms'`); err != nil {
		_ = secondTransaction.Rollback(ctx)
		_ = firstTransaction.Rollback(ctx)
		t.Fatalf("set competing location lock timeout: %v", err)
	}
	if _, err := secondTransaction.Exec(ctx, `
		UPDATE werk_core.storage_blob_locations
		SET state = 'missing', version = 3, updated_at = $2
		WHERE id = $1::uuid
	`, secondLocationID, changedAt); err == nil {
		_ = secondTransaction.Rollback(ctx)
		_ = firstTransaction.Rollback(ctx)
		t.Fatal("parallel location failure bypassed blob serialization lock")
	}
	_ = secondTransaction.Rollback(ctx)
	if err := firstTransaction.Commit(ctx); err != nil {
		t.Fatalf("commit first serialized location failure: %v", err)
	}

	lastLocationTransaction, err := secondConnection.Begin(ctx)
	if err != nil {
		t.Fatalf("begin last location failure: %v", err)
	}
	if _, err := lastLocationTransaction.Exec(ctx, `
		UPDATE werk_core.storage_blob_locations
		SET state = 'missing', version = 3, updated_at = $2
		WHERE id = $1::uuid
	`, secondLocationID, changedAt.Add(time.Second)); err != nil {
		_ = lastLocationTransaction.Rollback(ctx)
		t.Fatalf("stage last location failure: %v", err)
	}
	if err := lastLocationTransaction.Commit(ctx); err == nil {
		t.Fatal("last available location disappeared while blob stayed available")
	}

	var availableLocations int
	if err := firstConnection.QueryRow(ctx, `
		SELECT count(*)
		FROM werk_core.storage_blob_locations
		WHERE tenant_id = $1::uuid AND blob_id = $2::uuid AND state = 'available'
	`, tenantID.String(), blobID).Scan(&availableLocations); err != nil {
		t.Fatalf("inspect serialized location result: %v", err)
	}
	if availableLocations != 1 {
		t.Fatalf("available locations after serialized failures = %d, want 1", availableLocations)
	}
}

func assertTenantDocumentVisibility(t *testing.T, ctx context.Context, database *WorkDB, tenantID tenancy.TenantID, expectedDocument string) {
	t.Helper()
	if err := database.WithinTenantRead(ctx, tenantID, func(ctx context.Context, tx TenantTx) error {
		var count int
		var documentID string
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM werk_core.documents`).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("visible document count = %d, want 1", count)
		}
		if err := tx.QueryRow(ctx, `SELECT id::text FROM werk_core.documents`).Scan(&documentID); err != nil {
			return err
		}
		if documentID != expectedDocument {
			return fmt.Errorf("visible document = %s, want %s", documentID, expectedDocument)
		}
		return nil
	}); err != nil {
		t.Fatalf("document visibility for tenant %s: %v", tenantID.String(), err)
	}
}

func assertDocumentStorageCatalog(t *testing.T, ctx context.Context, owner *pgxpool.Conn) {
	t.Helper()
	var secureTables int
	if err := owner.QueryRow(ctx, `
		SELECT count(*)
		FROM pg_catalog.pg_class AS relation
		JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid = relation.relnamespace
		WHERE namespace.nspname = 'werk_core'
		  AND relation.relname IN (
		    'storage_blobs', 'storage_blob_locations', 'documents',
		    'document_versions', 'document_classification_revisions'
		  )
		  AND relation.relrowsecurity
		  AND relation.relforcerowsecurity
		  AND pg_catalog.pg_get_userbyid(relation.relowner) = 'werk_owner'
	`).Scan(&secureTables); err != nil {
		t.Fatalf("inspect document/storage table security: %v", err)
	}
	if secureTables != 5 {
		t.Fatalf("secure document/storage table count = %d, want 5", secureTables)
	}

	var permissionContracts int
	if err := owner.QueryRow(ctx, `
		SELECT count(*)
		FROM werk_core.permissions AS permission
		JOIN werk_core.permission_resource_types AS target ON target.permission_id = permission.id
		JOIN werk_core.permission_processing_policies AS policy
		  ON policy.permission_id = target.permission_id AND policy.resource_kind = target.resource_kind
		JOIN werk_core.resource_data_profiles AS profile ON profile.resource_kind = target.resource_kind
		WHERE permission.permission_key LIKE 'core.documents.%'
		  AND permission.access_plane = 'work'
		  AND policy.processing_required
		  AND profile.processing_activity_required
	`).Scan(&permissionContracts); err != nil {
		t.Fatalf("inspect document permission contracts: %v", err)
	}
	if permissionContracts != 5 {
		t.Fatalf("complete document permission contracts = %d, want 5", permissionContracts)
	}

	var workDocuments, workStorage, workInsert, workerStorage, workerDocuments, runtimeDelete bool
	if err := owner.QueryRow(ctx, `
		SELECT
			pg_catalog.has_table_privilege('werk_work_runtime', 'werk_core.documents', 'SELECT'),
			pg_catalog.has_table_privilege('werk_work_runtime', 'werk_core.storage_blobs', 'SELECT'),
			pg_catalog.has_table_privilege('werk_work_runtime', 'werk_core.documents', 'INSERT'),
			pg_catalog.has_table_privilege('werk_worker_runtime', 'werk_core.storage_blobs', 'SELECT,INSERT,UPDATE'),
			pg_catalog.has_table_privilege('werk_worker_runtime', 'werk_core.documents', 'SELECT'),
			pg_catalog.has_table_privilege('werk_service_runtime', 'werk_core.document_versions', 'DELETE')
	`).Scan(&workDocuments, &workStorage, &workInsert, &workerStorage, &workerDocuments, &runtimeDelete); err != nil {
		t.Fatalf("inspect document/storage runtime grants: %v", err)
	}
	if !workDocuments || workStorage || workInsert || !workerStorage || workerDocuments || runtimeDelete {
		t.Fatalf("unexpected document/storage grants: workDocuments=%t workStorage=%t workInsert=%t workerStorage=%t workerDocuments=%t runtimeDelete=%t",
			workDocuments, workStorage, workInsert, workerStorage, workerDocuments, runtimeDelete)
	}
}
