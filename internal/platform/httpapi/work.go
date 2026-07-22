package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	coreauth "github.com/dytonpictures/werk/internal/core/authorization"
	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/resource"
	"github.com/dytonpictures/werk/internal/platform/workspacestore"
)

type workIdentity interface {
	ResolveActor(context.Context, string, identity.AccessPlane) (identity.AuthenticatedActor, error)
	Authorize(context.Context, identity.AuthenticatedActor, string, coreauth.Resource) error
}

type WorkspaceService interface {
	Overview(context.Context, identity.AuthenticatedActor) (workspacestore.Overview, error)
}

func workRoutes(auth AuthService, service WorkspaceService) http.Handler {
	router := chi.NewRouter()
	router.Get("/workspace", func(writer http.ResponseWriter, request *http.Request) {
		identityService, ok := auth.(workIdentity)
		if !ok || service == nil {
			writeProblem(writer, request, http.StatusNotImplemented, "workspace-unavailable", "Workspace unavailable", "The workspace service is not configured.")
			return
		}
		actor, err := identityService.ResolveActor(request.Context(), cookieValue(request, "werk_session"), identity.AccessPlaneWork)
		if err != nil || actor.TenantID == nil {
			writeProblem(writer, request, http.StatusUnauthorized, "invalid-work-session", "Authentication required", "A valid work session is required.")
			return
		}
		target := coreauth.TenantResource(*actor.TenantID, resource.KindWorkspace, actor.TenantID.String(), coreauth.ScopeTenant)
		if err := identityService.Authorize(request.Context(), actor, "core.workspace.access", target); err != nil {
			writeProblem(writer, request, http.StatusForbidden, "permission-denied", "Permission denied", "The work account is not allowed to access this workspace.")
			return
		}
		view, err := service.Overview(request.Context(), actor)
		if err != nil {
			if errors.Is(err, identity.ErrAccessDenied) {
				writeProblem(writer, request, http.StatusForbidden, "workspace-context-denied", "Workspace unavailable", "The tenant-bound workspace context could not be resolved.")
				return
			}
			writeProblem(writer, request, http.StatusInternalServerError, "workspace-load-failed", "Workspace unavailable", "The workspace could not be loaded.")
			return
		}
		writeJSON(writer, http.StatusOK, view)
	})
	return router
}
