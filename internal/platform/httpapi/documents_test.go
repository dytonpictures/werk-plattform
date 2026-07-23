package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	coreauth "github.com/dytonpictures/werk/internal/core/authorization"
	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/tenancy"
	"github.com/dytonpictures/werk/internal/platform/config"
	"github.com/dytonpictures/werk/internal/platform/documentstore"
)

type documentServiceStub struct {
	page      documentstore.Page
	detail    documentstore.Detail
	listErr   error
	detailErr error
}

func (stub documentServiceStub) List(context.Context, identity.AuthenticatedActor, documentstore.ListQuery) (documentstore.Page, error) {
	return stub.page, stub.listErr
}

func (stub documentServiceStub) Detail(context.Context, identity.AuthenticatedActor, string) (documentstore.Detail, error) {
	return stub.detail, stub.detailErr
}

func TestDocumentRoutesRequireWorkPermissionsAndReturnMetadataOnly(t *testing.T) {
	tenantID, _ := tenancy.ParseTenantID("0196f000-0000-7000-8000-000000000a01")
	actor := identity.AuthenticatedActor{
		AccountID: identity.AccountID{1}, AccountClass: identity.AccountClassWork,
		Audience: identity.AudienceWork, Kind: identity.AuthenticationInteractive,
		Assurance: identity.AssuranceSingleFactor, TenantID: &tenantID,
	}
	documentID := "0196f000-0000-7000-8000-000000000a02"
	now := time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC)
	summary := documentstore.Summary{
		ID: documentID, Title: "Rahmenvertrag", Status: "active", SourceModule: "core.documents",
		CreatedAt: now, UpdatedAt: now, Version: 3, AccessReason: "created-by-me",
		LatestVersion:  documentstore.LatestVersionView{ID: "0196f000-0000-7000-8000-000000000a03", VersionNumber: 2, Source: "upload", PublishedAt: now},
		Classification: documentstore.ClassificationView{Revision: 1, Level: "confidential", RetentionClass: "business.standard"},
	}
	service := documentServiceStub{
		page: documentstore.Page{Items: []documentstore.Summary{summary}},
		detail: documentstore.Detail{Summary: summary, Versions: []documentstore.VersionView{{
			ID: summary.LatestVersion.ID, VersionNumber: 2, Source: "upload", PublishedAt: now,
		}}},
	}
	router := NewRouterWithServices(config.Config{}, readinessStub{}, testLogger(), workAuthStub{actor: actor}, nil, nil, WithDocumentService(service))

	listed := request(t, router, http.MethodGet, "/api/v1/documents?status=active&classification=confidential&access=created-by-me&limit=25", "")
	if listed.Code != http.StatusOK || listed.Header().Get("Cache-Control") != "no-store" ||
		!containsAll(listed.Body.String(), `"visibility_scope":"created-or-directly-shared-with-me"`, `"title":"Rahmenvertrag"`, `"access_reason":"created-by-me"`, `"level":"confidential"`, `"latest_version"`) {
		t.Fatalf("document list response = %d %s", listed.Code, listed.Body.String())
	}
	assertDocumentResponseHasNoInternalFields(t, listed.Body.String())

	detail := request(t, router, http.MethodGet, "/api/v1/documents/"+documentID, "")
	if detail.Code != http.StatusOK || detail.Header().Get("ETag") != `"3"` ||
		!containsAll(detail.Body.String(), `"versions":[`, `"version_number":2`) {
		t.Fatalf("document detail response = %d %s", detail.Code, detail.Body.String())
	}
	assertDocumentResponseHasNoInternalFields(t, detail.Body.String())

	listOnlyRouter := NewRouterWithServices(config.Config{}, readinessStub{}, testLogger(), workAuthStub{
		actor: actor,
		authorize: func(permission string, _ coreauth.Resource) error {
			if permission == "core.documents.document.read" {
				return coreauth.ErrDenied
			}
			return nil
		},
	}, nil, nil, WithDocumentService(service))
	if listOnly := request(t, listOnlyRouter, http.MethodGet, "/api/v1/documents", ""); listOnly.Code != http.StatusOK ||
		!containsAll(listOnly.Body.String(), `"title":"Rahmenvertrag"`) {
		t.Fatalf("list-only metadata response = %d %s", listOnly.Code, listOnly.Body.String())
	}
	if hidden := request(t, listOnlyRouter, http.MethodGet, "/api/v1/documents/"+documentID, ""); hidden.Code != http.StatusNotFound {
		t.Fatalf("list-only detail status = %d, want %d", hidden.Code, http.StatusNotFound)
	}

	invalid := request(t, router, http.MethodGet, "/api/v1/documents?limit=101", "")
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid query status = %d, want %d", invalid.Code, http.StatusBadRequest)
	}
	invalidAccess := request(t, router, http.MethodGet, "/api/v1/documents?access=tenant-wide", "")
	if invalidAccess.Code != http.StatusBadRequest {
		t.Fatalf("invalid access query status = %d, want %d", invalidAccess.Code, http.StatusBadRequest)
	}

	deniedRouter := NewRouterWithServices(config.Config{}, readinessStub{}, testLogger(), workAuthStub{actor: actor, authorizeErr: coreauth.ErrDenied}, nil, nil, WithDocumentService(service))
	if denied := request(t, deniedRouter, http.MethodGet, "/api/v1/documents", ""); denied.Code != http.StatusForbidden {
		t.Fatalf("denied list status = %d, want %d", denied.Code, http.StatusForbidden)
	}
	if hidden := request(t, deniedRouter, http.MethodGet, "/api/v1/documents/"+documentID, ""); hidden.Code != http.StatusNotFound {
		t.Fatalf("denied detail status = %d, want %d", hidden.Code, http.StatusNotFound)
	}
}

func assertDocumentResponseHasNoInternalFields(t *testing.T, body string) {
	t.Helper()
	for _, key := range []string{
		"blob_id", "created_by_account_id", "grantee_account_id",
		"granted_by_account_id", "revoked_by_account_id", "binding_id",
		"sha256", "size_bytes", "media_type", "provider_key", "opaque_key",
		"provider_checksum",
	} {
		if strings.Contains(body, `"`+key+`"`) {
			t.Errorf("document response exposes internal field %q: %s", key, body)
		}
	}
}

func TestDocumentRoutesHideMissingAndRejectNonWorkSessions(t *testing.T) {
	tenantID, _ := tenancy.ParseTenantID("0196f000-0000-7000-8000-000000000b01")
	workActor := identity.AuthenticatedActor{
		AccountID: identity.AccountID{1}, AccountClass: identity.AccountClassWork,
		Audience: identity.AudienceWork, Kind: identity.AuthenticationInteractive,
		Assurance: identity.AssuranceSingleFactor, TenantID: &tenantID,
	}
	missingService := documentServiceStub{detailErr: documentstore.ErrNotFound}
	missingRouter := NewRouterWithServices(config.Config{}, readinessStub{}, testLogger(), workAuthStub{actor: workActor}, nil, nil, WithDocumentService(missingService))
	missing := request(t, missingRouter, http.MethodGet, "/api/v1/documents/0196f000-0000-7000-8000-000000000b02", "")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing detail status = %d, want %d", missing.Code, http.StatusNotFound)
	}

	failedRouter := NewRouterWithServices(config.Config{}, readinessStub{}, testLogger(), workAuthStub{actor: workActor}, nil, nil, WithDocumentService(documentServiceStub{listErr: errors.New("database unavailable")}))
	if failed := request(t, failedRouter, http.MethodGet, "/api/v1/documents", ""); failed.Code != http.StatusInternalServerError {
		t.Fatalf("failed list status = %d, want %d", failed.Code, http.StatusInternalServerError)
	}

	adminActor := identity.AuthenticatedActor{
		AccountID: identity.AccountID{2}, AccountClass: identity.AccountClassAdmin,
		Audience: identity.AudienceAdmin, Kind: identity.AuthenticationInteractive,
		Assurance: identity.AssuranceMultiFactor,
	}
	adminRouter := NewRouterWithServices(config.Config{}, readinessStub{}, testLogger(), workAuthStub{actor: adminActor}, nil, nil, WithDocumentService(missingService))
	if admin := request(t, adminRouter, http.MethodGet, "/api/v1/documents", ""); admin.Code != http.StatusUnauthorized {
		t.Fatalf("admin list status = %d, want %d", admin.Code, http.StatusUnauthorized)
	}
}
