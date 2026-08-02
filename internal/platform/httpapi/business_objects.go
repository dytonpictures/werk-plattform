package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	coreauth "github.com/dytonpictures/werk/internal/core/authorization"
	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/resource"
)

// BusinessObjectRead is the trusted result of resolving a stored projection
// and its server-side read contract. Permission is deliberately separate from
// the public view and is never encoded into an HTTP response.
type BusinessObjectRead struct {
	Ref            resource.Ref
	OwnerModule    string
	Title          string
	Classification *string
	UpdatedAt      time.Time
	Version        uint64
	Permission     string
}

// BusinessObjectService resolves only within the actor's tenant. A false
// result intentionally does not distinguish a missing projection from one the
// store has made unavailable.
type BusinessObjectService interface {
	Resolve(context.Context, identity.AuthenticatedActor, resource.Ref) (BusinessObjectRead, bool, error)
}

type businessObjectReferenceResponse struct {
	TenantID string `json:"tenant_id"`
	Kind     string `json:"kind"`
	ID       string `json:"id"`
}

type businessObjectResponse struct {
	Ref            businessObjectReferenceResponse `json:"ref"`
	OwnerModule    string                          `json:"owner_module"`
	Title          string                          `json:"title"`
	Classification *string                         `json:"classification,omitempty"`
	UpdatedAt      time.Time                       `json:"updated_at"`
	Version        uint64                          `json:"version"`
}

func businessObjectRoutes(auth AuthService, service BusinessObjectService) http.Handler {
	router := chi.NewRouter()
	router.Get("/{resourceKind}/{resourceID}", func(writer http.ResponseWriter, request *http.Request) {
		identityService, ok := auth.(workIdentity)
		if !ok || service == nil {
			writeProblem(writer, request, http.StatusNotImplemented, "business-objects-unavailable", "Business objects unavailable", "The business object service is not configured.")
			return
		}

		actor, err := identityService.ResolveActor(request.Context(), cookieValue(request, "werk_session"), identity.AccessPlaneWork)
		if err != nil || actor.TenantID == nil || actor.TenantID.IsZero() {
			writeProblem(writer, request, http.StatusUnauthorized, "invalid-work-session", "Authentication required", "A valid work session is required.")
			return
		}

		ref := resource.TenantRef(
			*actor.TenantID,
			resource.Kind(chi.URLParam(request, "resourceKind")),
			chi.URLParam(request, "resourceID"),
		)
		if ref.Validate() != nil {
			writeProblem(writer, request, http.StatusBadRequest, "invalid-business-object-reference", "Invalid business object", "The business object reference is invalid.")
			return
		}

		result, found, err := service.Resolve(request.Context(), actor, ref)
		if errors.Is(err, identity.ErrAccessDenied) {
			writeBusinessObjectNotFound(writer, request)
			return
		}
		if err != nil {
			writeProblem(writer, request, http.StatusInternalServerError, "business-object-load-failed", "Business object unavailable", "The business object could not be loaded.")
			return
		}
		if !found {
			writeBusinessObjectNotFound(writer, request)
			return
		}
		if !validBusinessObjectRead(result, ref) {
			writeProblem(writer, request, http.StatusInternalServerError, "business-object-load-failed", "Business object unavailable", "The business object could not be loaded.")
			return
		}

		target := coreauth.TenantResource(*actor.TenantID, ref.Kind, ref.ID, coreauth.ScopeResource)
		if err := identityService.Authorize(request.Context(), actor, result.Permission, target); err != nil {
			writeBusinessObjectNotFound(writer, request)
			return
		}

		writer.Header().Set("Cache-Control", "no-store")
		writeVersionETag(writer, result.Version)
		writeJSON(writer, http.StatusOK, businessObjectResponse{
			Ref: businessObjectReferenceResponse{
				TenantID: actor.TenantID.String(),
				Kind:     string(result.Ref.Kind),
				ID:       result.Ref.ID,
			},
			OwnerModule:    result.OwnerModule,
			Title:          result.Title,
			Classification: result.Classification,
			UpdatedAt:      result.UpdatedAt,
			Version:        result.Version,
		})
	})
	return router
}

func validBusinessObjectRead(result BusinessObjectRead, requested resource.Ref) bool {
	if result.Ref.Validate() != nil || result.Ref.Boundary != requested.Boundary ||
		result.Ref.TenantID == nil || requested.TenantID == nil ||
		*result.Ref.TenantID != *requested.TenantID || result.Ref.Kind != requested.Kind ||
		result.Ref.ID != requested.ID || result.Version == 0 || result.UpdatedAt.IsZero() ||
		!resource.ValidKey(result.OwnerModule) || !strings.HasPrefix(string(result.Ref.Kind), result.OwnerModule+".") ||
		!resource.ValidKey(result.Permission) || strings.TrimSpace(result.Title) == "" {
		return false
	}
	return result.Classification == nil || strings.TrimSpace(*result.Classification) != ""
}

func writeBusinessObjectNotFound(writer http.ResponseWriter, request *http.Request) {
	writeProblem(writer, request, http.StatusNotFound, "business-object-not-found", "Business object not found", "The business object does not exist or is not visible.")
}
