package documentstore

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/tenancy"
	"github.com/dytonpictures/werk/internal/platform/database"
)

func TestDocumentReadSliceTenantAndCreatorIsolationIntegration(t *testing.T) {
	workURL := os.Getenv("WERK_TEST_WORK_DATABASE_URL")
	migratorURL := os.Getenv("WERK_TEST_MIGRATOR_DATABASE_URL")
	if workURL == "" || migratorURL == "" {
		t.Skip("integration database URLs are not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ownerPool, err := pgxpool.New(ctx, migratorURL)
	if err != nil {
		t.Fatal(err)
	}
	defer ownerPool.Close()
	owner, err := ownerPool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Release()
	if _, err := owner.Exec(ctx, `SET ROLE werk_owner`); err != nil {
		t.Fatal(err)
	}

	const (
		tenantA        = "0196f000-0000-7000-8000-000000000c01"
		tenantB        = "0196f000-0000-7000-8000-000000000c02"
		partyA         = "0196f000-0000-7000-8000-000000000c03"
		partyOther     = "0196f000-0000-7000-8000-000000000c04"
		partyB         = "0196f000-0000-7000-8000-000000000c05"
		accountA       = "0196f000-0000-7000-8000-000000000c06"
		accountOther   = "0196f000-0000-7000-8000-000000000c07"
		accountB       = "0196f000-0000-7000-8000-000000000c08"
		blobA1         = "0196f000-0000-7000-8000-000000000c09"
		blobA2         = "0196f000-0000-7000-8000-000000000c0a"
		blobOther      = "0196f000-0000-7000-8000-000000000c0b"
		blobB          = "0196f000-0000-7000-8000-000000000c0c"
		locationA1     = "0196f000-0000-7000-8000-000000000c0d"
		locationA2     = "0196f000-0000-7000-8000-000000000c0e"
		locationOther  = "0196f000-0000-7000-8000-000000000c0f"
		locationB      = "0196f000-0000-7000-8000-000000000c10"
		documentA1     = "0196f000-0000-7000-8000-000000000c11"
		documentA2     = "0196f000-0000-7000-8000-000000000c12"
		documentOther  = "0196f000-0000-7000-8000-000000000c13"
		documentB      = "0196f000-0000-7000-8000-000000000c14"
		versionA1      = "0196f000-0000-7000-8000-000000000c15"
		versionA2      = "0196f000-0000-7000-8000-000000000c16"
		versionOther   = "0196f000-0000-7000-8000-000000000c17"
		versionB       = "0196f000-0000-7000-8000-000000000c18"
		classA1        = "0196f000-0000-7000-8000-000000000c19"
		classA2        = "0196f000-0000-7000-8000-000000000c1a"
		classOther     = "0196f000-0000-7000-8000-000000000c1b"
		classB         = "0196f000-0000-7000-8000-000000000c1c"
		documentHidden = "0196f000-0000-7000-8000-000000000c1d"
		versionHidden  = "0196f000-0000-7000-8000-000000000c1e"
		classHidden    = "0196f000-0000-7000-8000-000000000c1f"
		bindingShared  = "0196f000-0000-7000-8000-000000000c20"
		bindingRevoked = "0196f000-0000-7000-8000-000000000c21"
		bindingForeign = "0196f000-0000-7000-8000-000000000c22"
	)
	createdAt := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	if _, err := owner.Exec(ctx, `
		INSERT INTO werk_core.tenants (id,name,status,default_locale,default_timezone)
		VALUES ($1::uuid,'Document read A','active','de-DE','Europe/Berlin'),
		       ($2::uuid,'Document read B','active','de-DE','Europe/Berlin')
	`, tenantA, tenantB); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `
		INSERT INTO werk_core.parties (id,tenant_id,party_type,display_name,status)
		VALUES ($1::uuid,$2::uuid,'person','Document A','active'),
		       ($3::uuid,$2::uuid,'person','Document Other','active'),
		       ($4::uuid,$5::uuid,'person','Document B','active')
	`, partyA, tenantA, partyOther, partyB, tenantB); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `
		INSERT INTO werk_core.persons (party_id,tenant_id,given_name,family_name)
		VALUES ($1::uuid,$2::uuid,'Document','A'),
		       ($3::uuid,$2::uuid,'Document','Other'),
		       ($4::uuid,$5::uuid,'Document','B')
	`, partyA, tenantA, partyOther, partyB, tenantB); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `
		INSERT INTO werk_core.accounts (id,account_class,tenant_id,person_party_id,login_name,status)
		VALUES ($1::uuid,'work',$2::uuid,$3::uuid,'document-a@werk.test','active'),
		       ($4::uuid,'work',$2::uuid,$5::uuid,'document-other@werk.test','active'),
		       ($6::uuid,'work',$7::uuid,$8::uuid,'document-b@werk.test','active')
	`, accountA, tenantA, partyA, accountOther, partyOther, accountB, tenantB, partyB); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `
		INSERT INTO werk_core.storage_blobs (id,tenant_id,state,created_by_account_id,created_at,updated_at)
		VALUES ($1::uuid,$2::uuid,'quarantined',$3::uuid,$9,$9),
		       ($4::uuid,$2::uuid,'quarantined',$3::uuid,$9,$9),
		       ($5::uuid,$2::uuid,'quarantined',$6::uuid,$9,$9),
		       ($7::uuid,$8::uuid,'quarantined',$10::uuid,$9,$9)
	`, blobA1, tenantA, accountA, blobA2, blobOther, accountOther, blobB, tenantB, createdAt, accountB); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `
		INSERT INTO werk_core.storage_blob_locations (id,tenant_id,blob_id,provider_key,opaque_key,state,created_at,updated_at)
		VALUES ($1::uuid,$2::uuid,$3::uuid,'internal.test',$1::uuid,'quarantined',$11,$11),
		       ($4::uuid,$2::uuid,$5::uuid,'internal.test',$4::uuid,'quarantined',$11,$11),
		       ($6::uuid,$2::uuid,$7::uuid,'internal.test',$6::uuid,'quarantined',$11,$11),
		       ($8::uuid,$9::uuid,$10::uuid,'internal.test',$8::uuid,'quarantined',$11,$11)
	`, locationA1, tenantA, blobA1, locationA2, blobA2, locationOther, blobOther, locationB, tenantB, blobB, createdAt); err != nil {
		t.Fatal(err)
	}
	verifiedAt := createdAt.Add(time.Second)
	if _, err := owner.Exec(ctx, `
		UPDATE werk_core.storage_blob_locations
		SET state='available', provider_checksum='verified', activated_at=$1, updated_at=$1, version=2
		WHERE id IN ($2::uuid,$3::uuid,$4::uuid,$5::uuid)
	`, verifiedAt, locationA1, locationA2, locationOther, locationB); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `
		UPDATE werk_core.storage_blobs
		SET state='available', size_bytes=64, sha256=decode(repeat('12',32),'hex'),
		    media_type='application/pdf', verified_at=$1, updated_at=$1, version=2
		WHERE id IN ($2::uuid,$3::uuid,$4::uuid,$5::uuid)
	`, verifiedAt.Add(time.Second), blobA1, blobA2, blobOther, blobB); err != nil {
		t.Fatal(err)
	}

	tx, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO werk_core.documents (id,tenant_id,title,status,source_module,created_by_account_id,created_at,updated_at)
		VALUES ($1::uuid,$2::uuid,'Projektvertrag','active','core.documents',$3::uuid,$10,$10),
		       ($4::uuid,$2::uuid,'Interne Richtlinie','active','core.documents',$3::uuid,$11,$11),
		       ($5::uuid,$2::uuid,'Anderes Konto','active','core.documents',$6::uuid,$12,$12),
		       ($7::uuid,$8::uuid,'Fremder Tenant','active','core.documents',$9::uuid,$13,$13)
	`, documentA1, tenantA, accountA, documentA2, documentOther, accountOther,
		documentB, tenantB, accountB, createdAt.Add(6*time.Second), createdAt.Add(5*time.Second),
		createdAt.Add(4*time.Second), createdAt.Add(3*time.Second)); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO werk_core.document_versions (id,tenant_id,document_id,version_number,blob_id,source,created_by_account_id,published_at)
		VALUES ($1::uuid,$2::uuid,$3::uuid,1,$4::uuid,'upload',$5::uuid,$13),
		       ($6::uuid,$2::uuid,$7::uuid,1,$8::uuid,'import',$5::uuid,$14),
		       ($9::uuid,$2::uuid,$10::uuid,1,$11::uuid,'upload',$12::uuid,$15),
		       ($16::uuid,$17::uuid,$18::uuid,1,$19::uuid,'upload',$20::uuid,$21)
	`, versionA1, tenantA, documentA1, blobA1, accountA,
		versionA2, documentA2, blobA2,
		versionOther, documentOther, blobOther, accountOther,
		createdAt.Add(6*time.Second), createdAt.Add(5*time.Second), createdAt.Add(4*time.Second),
		versionB, tenantB, documentB, blobB, accountB, createdAt.Add(3*time.Second)); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO werk_core.documents (id,tenant_id,title,status,source_module,created_by_account_id,created_at,updated_at)
		VALUES ($1::uuid,$2::uuid,'Nicht mehr geteilt','active','core.documents',$3::uuid,$4,$4)
	`, documentHidden, tenantA, accountOther, createdAt.Add(2*time.Second)); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO werk_core.document_versions
		(id,tenant_id,document_id,version_number,blob_id,source,created_by_account_id,published_at)
		VALUES ($1::uuid,$2::uuid,$3::uuid,1,$4::uuid,'upload',$5::uuid,$6)
	`, versionHidden, tenantA, documentHidden, blobOther, accountOther, createdAt.Add(2*time.Second)); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO werk_core.document_classification_revisions
		(id,tenant_id,document_id,revision,classification,retention_class,legal_hold,recorded_by_account_id,recorded_at)
		VALUES ($1::uuid,$2::uuid,$3::uuid,1,'confidential','business.standard',false,$4::uuid,$13),
		       ($5::uuid,$2::uuid,$6::uuid,1,'internal','business.standard',false,$4::uuid,$14),
		       ($7::uuid,$2::uuid,$8::uuid,1,'restricted','business.standard',true,$9::uuid,$15),
		       ($10::uuid,$11::uuid,$12::uuid,1,'restricted','business.standard',false,$16::uuid,$17)
	`, classA1, tenantA, documentA1, accountA, classA2, documentA2,
		classOther, documentOther, accountOther, classB, tenantB, documentB,
		createdAt.Add(6*time.Second), createdAt.Add(5*time.Second), createdAt.Add(4*time.Second), accountB, createdAt.Add(3*time.Second)); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO werk_core.document_classification_revisions
		(id,tenant_id,document_id,revision,classification,retention_class,legal_hold,recorded_by_account_id,recorded_at)
		VALUES ($1::uuid,$2::uuid,$3::uuid,1,'internal','business.standard',false,$4::uuid,$5)
	`, classHidden, tenantA, documentHidden, accountOther, createdAt.Add(2*time.Second)); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO werk_core.document_account_visibility_bindings
		(id,tenant_id,document_id,grantee_account_id,granted_by_account_id,granted_at)
		VALUES ($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,$6),
		       ($7::uuid,$2::uuid,$8::uuid,$4::uuid,$5::uuid,$6)
	`, bindingShared, tenantA, documentOther, accountA, accountOther, createdAt.Add(7*time.Second),
		bindingRevoked, documentHidden); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE werk_core.document_account_visibility_bindings
		SET revoked_by_account_id=$2::uuid, revoked_at=$3, version=2
		WHERE id=$1::uuid
	`, bindingRevoked, accountOther, createdAt.Add(8*time.Second)); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `
		INSERT INTO werk_core.document_account_visibility_bindings
		(id,tenant_id,document_id,grantee_account_id,granted_by_account_id,granted_at)
		VALUES ($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,$6)
	`, bindingForeign, tenantA, documentOther, accountB, accountOther, createdAt.Add(7*time.Second)); err == nil {
		t.Fatal("cross-tenant document visibility binding was accepted")
	}

	workDatabase, err := database.NewWork(ctx, workURL, "werk-document-read-integration")
	if err != nil {
		t.Fatal(err)
	}
	defer workDatabase.Close()
	service, err := New(workDatabase)
	if err != nil {
		t.Fatal(err)
	}
	parsedTenantA, _ := tenancy.ParseTenantID(tenantA)
	actorA := identity.AuthenticatedActor{
		AccountID: integrationAccountID(accountA), AccountClass: identity.AccountClassWork,
		Audience: identity.AudienceWork, Kind: identity.AuthenticationInteractive,
		Assurance: identity.AssuranceSingleFactor, TenantID: &parsedTenantA,
	}
	page, err := service.List(ctx, actorA, ListQuery{})
	if err != nil || len(page.Items) != 3 || page.Items[0].ID != documentA1 || page.Items[1].ID != documentA2 ||
		page.Items[2].ID != documentOther || page.Items[0].AccessReason != "created-by-me" ||
		page.Items[2].AccessReason != "shared-directly-with-me" {
		t.Fatalf("actor A page = %#v, err = %v", page, err)
	}
	firstPage, err := service.List(ctx, actorA, ListQuery{Limit: 1})
	if err != nil || len(firstPage.Items) != 1 || firstPage.NextCursor == nil {
		t.Fatalf("first page = %#v, err = %v", firstPage, err)
	}
	secondPage, err := service.List(ctx, actorA, ListQuery{Limit: 1, Cursor: firstPage.NextCursor})
	if err != nil || len(secondPage.Items) != 1 || secondPage.Items[0].ID != documentA2 {
		t.Fatalf("second page = %#v, err = %v", secondPage, err)
	}
	filtered, err := service.List(ctx, actorA, ListQuery{Search: "richt", Classification: "internal"})
	if err != nil || len(filtered.Items) != 1 || filtered.Items[0].ID != documentA2 {
		t.Fatalf("filtered page = %#v, err = %v", filtered, err)
	}
	shared, err := service.List(ctx, actorA, ListQuery{AccessReason: "shared-directly-with-me"})
	if err != nil || len(shared.Items) != 1 || shared.Items[0].ID != documentOther {
		t.Fatalf("shared page = %#v, err = %v", shared, err)
	}
	created, err := service.List(ctx, actorA, ListQuery{AccessReason: "created-by-me"})
	if err != nil || len(created.Items) != 2 || created.Items[0].ID != documentA1 || created.Items[1].ID != documentA2 {
		t.Fatalf("created page = %#v, err = %v", created, err)
	}
	detail, err := service.Detail(ctx, actorA, documentA1)
	if err != nil || detail.ID != documentA1 || len(detail.Versions) != 1 || detail.Classification.Level != "confidential" {
		t.Fatalf("document detail = %#v, err = %v", detail, err)
	}
	sharedDetail, err := service.Detail(ctx, actorA, documentOther)
	if err != nil || sharedDetail.ID != documentOther || sharedDetail.AccessReason != "shared-directly-with-me" {
		t.Fatalf("shared detail = %#v, err = %v", sharedDetail, err)
	}
	if _, err := service.Detail(ctx, actorA, documentHidden); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoked direct visibility error = %v, want not found", err)
	}
	if _, err := service.Detail(ctx, actorA, documentB); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign tenant error = %v, want not found", err)
	}
	adminActor := identity.AuthenticatedActor{
		AccountID: identity.AccountID{9}, AccountClass: identity.AccountClassAdmin,
		Audience: identity.AudienceAdmin, Kind: identity.AuthenticationInteractive,
		Assurance: identity.AssuranceMultiFactor,
	}
	if _, err := service.List(ctx, adminActor, ListQuery{}); !errors.Is(err, identity.ErrAccessDenied) {
		t.Fatalf("admin list error = %v, want access denied", err)
	}
}

func integrationAccountID(value string) identity.AccountID {
	raw, _ := hex.DecodeString(strings.ReplaceAll(value, "-", ""))
	var id identity.AccountID
	copy(id[:], raw)
	return id
}
