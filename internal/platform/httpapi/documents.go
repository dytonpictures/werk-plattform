package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	coreauth "github.com/dytonpictures/werk/internal/core/authorization"
	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/resource"
	"github.com/dytonpictures/werk/internal/platform/documentstore"
)

type DocumentService interface {
	List(context.Context, identity.AuthenticatedActor, documentstore.ListQuery) (documentstore.Page, error)
	Detail(context.Context, identity.AuthenticatedActor, string) (documentstore.Detail, error)
}

func documentRoutes(auth AuthService, service DocumentService) http.Handler {
	router := chi.NewRouter()
	router.Get("/", func(writer http.ResponseWriter, request *http.Request) {
		identityService, actor, ok := resolveDocumentActor(writer, request, auth, service)
		if !ok {
			return
		}
		collection := coreauth.TenantResource(*actor.TenantID, resource.KindDocumentCollection, resource.RootID, coreauth.ScopeTenant)
		if err := identityService.Authorize(request.Context(), actor, "core.documents.document.list", collection); err != nil {
			writeProblem(writer, request, http.StatusForbidden, "document-list-denied", "Permission denied", "The work account is not allowed to list documents.")
			return
		}
		query, err := documentListQueryFromRequest(request)
		if err != nil {
			writeProblem(writer, request, http.StatusBadRequest, "invalid-document-query", "Invalid document query", "The document filters or cursor are invalid.")
			return
		}
		page, err := service.List(request.Context(), actor, query)
		if errors.Is(err, documentstore.ErrInvalidQuery) {
			writeProblem(writer, request, http.StatusBadRequest, "invalid-document-query", "Invalid document query", "The document filters or cursor are invalid.")
			return
		}
		if errors.Is(err, identity.ErrAccessDenied) {
			writeProblem(writer, request, http.StatusForbidden, "document-list-denied", "Permission denied", "The work account is not allowed to list documents.")
			return
		}
		if err != nil {
			writeProblem(writer, request, http.StatusInternalServerError, "document-list-failed", "Documents unavailable", "The documents could not be loaded.")
			return
		}
		response := map[string]any{
			"visibility_scope": documentstore.VisibilityScope,
			"items":            page.Items,
		}
		if page.NextCursor != nil {
			cursor, err := encodeDocumentCursor(*page.NextCursor)
			if err != nil {
				writeProblem(writer, request, http.StatusInternalServerError, "document-list-failed", "Documents unavailable", "The documents could not be loaded.")
				return
			}
			response["next_cursor"] = cursor
		}
		writer.Header().Set("Cache-Control", "no-store")
		writeJSON(writer, http.StatusOK, response)
	})
	router.Get("/{documentID}", func(writer http.ResponseWriter, request *http.Request) {
		identityService, actor, ok := resolveDocumentActor(writer, request, auth, service)
		if !ok {
			return
		}
		documentID := strings.TrimSpace(chi.URLParam(request, "documentID"))
		if !documentstore.ValidDocumentID(documentID) {
			writeProblem(writer, request, http.StatusBadRequest, "invalid-document-id", "Invalid document", "The document identifier is invalid.")
			return
		}
		target := coreauth.TenantResource(*actor.TenantID, resource.KindDocument, documentID, coreauth.ScopeResource)
		if err := identityService.Authorize(request.Context(), actor, "core.documents.document.read", target); err != nil {
			writeProblem(writer, request, http.StatusNotFound, "document-not-found", "Document not found", "The document does not exist or is not visible.")
			return
		}
		detail, err := service.Detail(request.Context(), actor, documentID)
		if errors.Is(err, documentstore.ErrNotFound) || errors.Is(err, identity.ErrAccessDenied) {
			writeProblem(writer, request, http.StatusNotFound, "document-not-found", "Document not found", "The document does not exist or is not visible.")
			return
		}
		if err != nil {
			writeProblem(writer, request, http.StatusInternalServerError, "document-load-failed", "Document unavailable", "The document could not be loaded.")
			return
		}
		writer.Header().Set("Cache-Control", "no-store")
		writeVersionETag(writer, detail.Version)
		writeJSON(writer, http.StatusOK, map[string]any{"document": detail.Summary, "versions": detail.Versions})
	})
	return router
}

func resolveDocumentActor(writer http.ResponseWriter, request *http.Request, auth AuthService, service DocumentService) (workIdentity, identity.AuthenticatedActor, bool) {
	identityService, ok := auth.(workIdentity)
	if !ok || service == nil {
		writeProblem(writer, request, http.StatusNotImplemented, "documents-unavailable", "Documents unavailable", "The document service is not configured.")
		return nil, identity.AuthenticatedActor{}, false
	}
	actor, err := identityService.ResolveActor(request.Context(), cookieValue(request, "werk_session"), identity.AccessPlaneWork)
	if err != nil || actor.TenantID == nil {
		writeProblem(writer, request, http.StatusUnauthorized, "invalid-work-session", "Authentication required", "A valid work session is required.")
		return nil, identity.AuthenticatedActor{}, false
	}
	return identityService, actor, true
}

func documentListQueryFromRequest(request *http.Request) (documentstore.ListQuery, error) {
	values := request.URL.Query()
	query := documentstore.ListQuery{
		Search:         values.Get("q"),
		Status:         values.Get("status"),
		Classification: values.Get("classification"),
		AccessReason:   values.Get("access"),
	}
	if rawLimit := strings.TrimSpace(values.Get("limit")); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil {
			return documentstore.ListQuery{}, documentstore.ErrInvalidQuery
		}
		query.Limit = limit
	}
	if rawCursor := strings.TrimSpace(values.Get("cursor")); rawCursor != "" {
		cursor, err := decodeDocumentCursor(rawCursor)
		if err != nil {
			return documentstore.ListQuery{}, documentstore.ErrInvalidQuery
		}
		query.Cursor = &cursor
	}
	return documentstore.NormalizeListQuery(query)
}

func encodeDocumentCursor(cursor documentstore.Cursor) (string, error) {
	value, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func decodeDocumentCursor(value string) (documentstore.Cursor, error) {
	if len(value) > 512 {
		return documentstore.Cursor{}, documentstore.ErrInvalidQuery
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return documentstore.Cursor{}, documentstore.ErrInvalidQuery
	}
	var cursor documentstore.Cursor
	if err := json.Unmarshal(raw, &cursor); err != nil {
		return documentstore.Cursor{}, documentstore.ErrInvalidQuery
	}
	query, err := documentstore.NormalizeListQuery(documentstore.ListQuery{Cursor: &cursor})
	if err != nil {
		return documentstore.Cursor{}, err
	}
	return *query.Cursor, nil
}
