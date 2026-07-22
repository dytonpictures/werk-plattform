package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	coreauth "github.com/dytonpictures/werk/internal/core/authorization"
	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/resource"
	"github.com/dytonpictures/werk/internal/core/tenancy"
	"github.com/dytonpictures/werk/internal/platform/adminstore"
)

type adminIdentity interface {
	ResolveActor(context.Context, string, identity.AccessPlane) (identity.AuthenticatedActor, error)
	Authorize(context.Context, identity.AuthenticatedActor, string, coreauth.Resource) error
}

type AdminService interface {
	CreateWorkUser(context.Context, adminstore.CreateWorkUserInput, identity.AuthenticatedActor, string, string) (adminstore.WorkUserView, error)
	ListWorkUsers(context.Context, string, identity.AuthenticatedActor, string, string) ([]adminstore.WorkUserDirectoryEntry, error)
	ListWorkRoles(context.Context, string, identity.AuthenticatedActor, string, string) (adminstore.WorkRoleCatalog, error)
	CreateWorkRole(context.Context, adminstore.CreateWorkRoleInput, identity.AuthenticatedActor, string, string) (adminstore.WorkRoleView, error)
	UpdateWorkRole(context.Context, string, uint64, adminstore.UpdateWorkRoleInput, identity.AuthenticatedActor, string, string) (adminstore.WorkRoleView, error)
	ReplaceWorkUserRoles(context.Context, string, adminstore.ReplaceWorkUserRolesInput, identity.AuthenticatedActor, string, string) ([]string, error)
	ListTenants(context.Context) ([]adminstore.TenantView, error)
	CreateTenant(context.Context, adminstore.CreateTenantInput, identity.AuthenticatedActor, string, string) (adminstore.TenantView, error)
	UpdateTenant(context.Context, string, uint64, adminstore.UpdateTenantInput, identity.AuthenticatedActor, string, string) (adminstore.TenantView, error)
	ListOrganizationalUnits(context.Context, string) ([]adminstore.OrganizationalUnitView, error)
	CreateOrganizationalUnit(context.Context, string, adminstore.CreateOrganizationalUnitInput, identity.AuthenticatedActor, string, string) (adminstore.OrganizationalUnitView, error)
	UpdateOrganizationalUnit(context.Context, string, string, uint64, adminstore.UpdateOrganizationalUnitInput, identity.AuthenticatedActor, string, string) (adminstore.OrganizationalUnitView, error)
	ListSecurityAuditEvents(context.Context, adminstore.SecurityAuditQuery, identity.AuthenticatedActor, string, string) (adminstore.SecurityAuditPage, error)
}

func adminRoutes(auth AuthService, service AdminService) http.Handler {
	router := chi.NewRouter()
	router.Get("/security-audit", func(writer http.ResponseWriter, request *http.Request) {
		query, err := securityAuditQueryFromRequest(request)
		if err != nil {
			writeProblem(writer, request, http.StatusBadRequest, "invalid-security-audit-query", "Invalid audit query", "The audit filters or cursor are invalid.")
			return
		}
		actor, ok := authorizeAdminRequest(writer, request, auth, service, "core.audit.security-event.read", coreauth.InstallationResource(resource.KindSecurityLog, resource.RootID))
		if !ok {
			return
		}
		page, err := service.ListSecurityAuditEvents(request.Context(), query, actor, requestIDFromContext(request.Context()), correlationIDFromContext(request.Context()))
		if errors.Is(err, adminstore.ErrInvalidAuditQuery) {
			writeProblem(writer, request, http.StatusBadRequest, "invalid-security-audit-query", "Invalid audit query", "The audit filters or cursor are invalid.")
			return
		}
		if err != nil {
			writeProblem(writer, request, http.StatusInternalServerError, "security-audit-list-failed", "Audit listing failed", "The security audit events could not be loaded.")
			return
		}
		response := map[string]any{"items": page.Items}
		if page.NextCursor != nil {
			cursor, err := encodeSecurityAuditCursor(*page.NextCursor)
			if err != nil {
				writeProblem(writer, request, http.StatusInternalServerError, "security-audit-list-failed", "Audit listing failed", "The security audit events could not be loaded.")
				return
			}
			response["next_cursor"] = cursor
		}
		writer.Header().Set("Cache-Control", "no-store")
		writeJSON(writer, http.StatusOK, response)
	})
	router.Get("/tenants", func(writer http.ResponseWriter, request *http.Request) {
		actor, ok := authorizeAdminRequest(writer, request, auth, service, "core.tenancy.tenant.read", coreauth.InstallationResource(resource.KindPlatformInstallation, resource.RootID))
		if !ok {
			return
		}
		_ = actor
		views, err := service.ListTenants(request.Context())
		if err != nil {
			writeProblem(writer, request, http.StatusInternalServerError, "tenant-list-failed", "Tenant listing failed", "The tenants could not be loaded.")
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"items": views})
	})
	router.Post("/tenants", func(writer http.ResponseWriter, request *http.Request) {
		actor, ok := authorizeAdminRequest(writer, request, auth, service, "core.tenancy.tenant.create", coreauth.InstallationResource(resource.KindPlatformInstallation, resource.RootID))
		if !ok {
			return
		}
		var input adminstore.CreateTenantInput
		if decodeJSON(writer, request, &input) != nil {
			writeProblem(writer, request, http.StatusBadRequest, "invalid-tenant", "Invalid tenant", "The tenant data is invalid.")
			return
		}
		view, err := service.CreateTenant(request.Context(), input, actor, requestIDFromContext(request.Context()), correlationIDFromContext(request.Context()))
		if err != nil {
			writeProblem(writer, request, http.StatusBadRequest, "tenant-create-failed", "Tenant creation failed", "The tenant could not be created.")
			return
		}
		writeVersionETag(writer, view.Version)
		writeJSON(writer, http.StatusCreated, view)
	})
	router.Put("/tenants/{tenantID}", func(writer http.ResponseWriter, request *http.Request) {
		tenantID, err := tenancy.ParseTenantID(chi.URLParam(request, "tenantID"))
		if err != nil {
			writeProblem(writer, request, http.StatusBadRequest, "invalid-tenant", "Invalid tenant", "The tenant identifier is invalid.")
			return
		}
		actor, ok := authorizeAdminRequest(writer, request, auth, service, "core.tenancy.tenant.update", coreauth.InstallationResource(resource.KindTenant, tenantID.String()))
		if !ok {
			return
		}
		expectedVersion, ok := requireExpectedVersion(writer, request)
		if !ok {
			return
		}
		var input adminstore.UpdateTenantInput
		if decodeJSON(writer, request, &input) != nil {
			writeProblem(writer, request, http.StatusBadRequest, "invalid-tenant", "Invalid tenant", "The tenant data is invalid.")
			return
		}
		view, err := service.UpdateTenant(request.Context(), tenantID.String(), expectedVersion, input, actor, requestIDFromContext(request.Context()), correlationIDFromContext(request.Context()))
		if err != nil {
			writeAdminUpdateProblem(writer, request, err, "tenant")
			return
		}
		writeVersionETag(writer, view.Version)
		writeJSON(writer, http.StatusOK, view)
	})
	router.Get("/tenants/{tenantID}/organizational-units", func(writer http.ResponseWriter, request *http.Request) {
		tenantID, err := tenancy.ParseTenantID(chi.URLParam(request, "tenantID"))
		if err != nil {
			writeProblem(writer, request, http.StatusBadRequest, "invalid-tenant", "Invalid tenant", "The tenant identifier is invalid.")
			return
		}
		if _, ok := authorizeAdminRequest(writer, request, auth, service, "core.tenancy.organizational-unit.read", coreauth.InstallationResource(resource.KindTenant, tenantID.String())); !ok {
			return
		}
		views, err := service.ListOrganizationalUnits(request.Context(), tenantID.String())
		if err != nil {
			writeProblem(writer, request, http.StatusInternalServerError, "organizational-unit-list-failed", "Organizational unit listing failed", "The organizational units could not be loaded.")
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"items": views})
	})
	router.Post("/tenants/{tenantID}/organizational-units", func(writer http.ResponseWriter, request *http.Request) {
		tenantID, err := tenancy.ParseTenantID(chi.URLParam(request, "tenantID"))
		if err != nil {
			writeProblem(writer, request, http.StatusBadRequest, "invalid-tenant", "Invalid tenant", "The tenant identifier is invalid.")
			return
		}
		actor, ok := authorizeAdminRequest(writer, request, auth, service, "core.tenancy.organizational-unit.create", coreauth.InstallationResource(resource.KindTenant, tenantID.String()))
		if !ok {
			return
		}
		var input adminstore.CreateOrganizationalUnitInput
		if decodeJSON(writer, request, &input) != nil {
			writeProblem(writer, request, http.StatusBadRequest, "invalid-organizational-unit", "Invalid organizational unit", "The organizational unit data is invalid.")
			return
		}
		view, err := service.CreateOrganizationalUnit(request.Context(), tenantID.String(), input, actor, requestIDFromContext(request.Context()), correlationIDFromContext(request.Context()))
		if err != nil {
			writeProblem(writer, request, http.StatusBadRequest, "organizational-unit-create-failed", "Organizational unit creation failed", "The organizational unit could not be created.")
			return
		}
		writeVersionETag(writer, view.Version)
		writeJSON(writer, http.StatusCreated, view)
	})
	router.Put("/tenants/{tenantID}/organizational-units/{unitID}", func(writer http.ResponseWriter, request *http.Request) {
		tenantID, err := tenancy.ParseTenantID(chi.URLParam(request, "tenantID"))
		if err != nil {
			writeProblem(writer, request, http.StatusBadRequest, "invalid-tenant", "Invalid tenant", "The tenant identifier is invalid.")
			return
		}
		unitID, err := tenancy.ParseUnitID(chi.URLParam(request, "unitID"))
		if err != nil {
			writeProblem(writer, request, http.StatusBadRequest, "invalid-organizational-unit", "Invalid organizational unit", "The organizational unit identifier is invalid.")
			return
		}
		actor, ok := authorizeAdminRequest(writer, request, auth, service, "core.tenancy.organizational-unit.update", coreauth.InstallationResource(resource.KindOrganizationalUnit, unitID.String()))
		if !ok {
			return
		}
		expectedVersion, ok := requireExpectedVersion(writer, request)
		if !ok {
			return
		}
		var input adminstore.UpdateOrganizationalUnitInput
		if decodeJSON(writer, request, &input) != nil {
			writeProblem(writer, request, http.StatusBadRequest, "invalid-organizational-unit", "Invalid organizational unit", "The organizational unit data is invalid.")
			return
		}
		view, err := service.UpdateOrganizationalUnit(request.Context(), tenantID.String(), unitID.String(), expectedVersion, input, actor, requestIDFromContext(request.Context()), correlationIDFromContext(request.Context()))
		if err != nil {
			writeAdminUpdateProblem(writer, request, err, "organizational-unit")
			return
		}
		writeVersionETag(writer, view.Version)
		writeJSON(writer, http.StatusOK, view)
	})
	router.Post("/work-users", func(writer http.ResponseWriter, request *http.Request) {
		identityService, ok := auth.(adminIdentity)
		if !ok || service == nil {
			writeProblem(writer, request, http.StatusNotImplemented, "admin-unavailable", "Administration unavailable", "The administration service is not configured.")
			return
		}
		actor, err := identityService.ResolveActor(request.Context(), cookieValue(request, "werk_session"), identity.AccessPlaneAdmin)
		if err != nil {
			writeProblem(writer, request, http.StatusUnauthorized, "invalid-admin-session", "Authentication required", "A valid multi-factor admin session is required.")
			return
		}
		if err := identityService.Authorize(request.Context(), actor, "core.identity.work-account.create", coreauth.InstallationResource(resource.KindPlatformInstallation, resource.RootID)); err != nil {
			writeProblem(writer, request, http.StatusForbidden, "permission-denied", "Permission denied", "The admin account is not allowed to create work accounts.")
			return
		}
		var input adminstore.CreateWorkUserInput
		if decodeJSON(writer, request, &input) != nil {
			writeProblem(writer, request, http.StatusBadRequest, "invalid-work-user", "Invalid work user", "The work user data is invalid.")
			return
		}
		view, err := service.CreateWorkUser(request.Context(), input, actor, requestIDFromContext(request.Context()), correlationIDFromContext(request.Context()))
		if err != nil {
			writeProblem(writer, request, http.StatusBadRequest, "work-user-create-failed", "Work user creation failed", "The work account could not be created.")
			return
		}
		writeJSON(writer, http.StatusCreated, view)
	})
	router.Get("/work-users", func(writer http.ResponseWriter, request *http.Request) {
		tenantID, err := tenancy.ParseTenantID(request.URL.Query().Get("tenant_id"))
		if err != nil {
			writeProblem(writer, request, http.StatusBadRequest, "invalid-tenant", "Invalid tenant", "A valid tenant identifier is required.")
			return
		}
		actor, ok := authorizeAdminRequest(writer, request, auth, service, "core.identity.work-account.read", coreauth.InstallationResource(resource.KindTenant, tenantID.String()))
		if !ok {
			return
		}
		entries, err := service.ListWorkUsers(request.Context(), tenantID.String(), actor, requestIDFromContext(request.Context()), correlationIDFromContext(request.Context()))
		if err != nil {
			writeProblem(writer, request, http.StatusInternalServerError, "work-user-list-failed", "Work user listing failed", "The work accounts could not be loaded.")
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"items": entries})
	})
	router.Get("/work-roles", func(writer http.ResponseWriter, request *http.Request) {
		tenantID, err := tenancy.ParseTenantID(request.URL.Query().Get("tenant_id"))
		if err != nil {
			writeProblem(writer, request, http.StatusBadRequest, "invalid-tenant", "Invalid tenant", "A valid tenant identifier is required.")
			return
		}
		actor, ok := authorizeAdminRequest(writer, request, auth, service, "core.authorization.work-role.read", coreauth.InstallationResource(resource.KindTenant, tenantID.String()))
		if !ok {
			return
		}
		catalog, err := service.ListWorkRoles(request.Context(), tenantID.String(), actor, requestIDFromContext(request.Context()), correlationIDFromContext(request.Context()))
		if err != nil {
			writeProblem(writer, request, http.StatusInternalServerError, "work-role-list-failed", "Work role listing failed", "The work roles could not be loaded.")
			return
		}
		writeJSON(writer, http.StatusOK, catalog)
	})
	router.Post("/work-roles", func(writer http.ResponseWriter, request *http.Request) {
		var input adminstore.CreateWorkRoleInput
		if decodeJSON(writer, request, &input) != nil {
			writeProblem(writer, request, http.StatusBadRequest, "invalid-work-role", "Invalid work role", "The work role data is invalid.")
			return
		}
		tenantID, err := tenancy.ParseTenantID(input.TenantID)
		if err != nil {
			writeProblem(writer, request, http.StatusBadRequest, "invalid-tenant", "Invalid tenant", "A valid tenant identifier is required.")
			return
		}
		actor, ok := authorizeAdminRequest(writer, request, auth, service, "core.authorization.work-role.create", coreauth.InstallationResource(resource.KindTenant, tenantID.String()))
		if !ok {
			return
		}
		view, err := service.CreateWorkRole(request.Context(), input, actor, requestIDFromContext(request.Context()), correlationIDFromContext(request.Context()))
		if err != nil {
			writeProblem(writer, request, http.StatusBadRequest, "work-role-create-failed", "Work role creation failed", "The tenant-bound work role could not be created.")
			return
		}
		writeVersionETag(writer, uint64(view.Version))
		writeJSON(writer, http.StatusCreated, view)
	})
	router.Put("/work-roles/{roleID}", func(writer http.ResponseWriter, request *http.Request) {
		var input adminstore.UpdateWorkRoleInput
		if decodeJSON(writer, request, &input) != nil {
			writeProblem(writer, request, http.StatusBadRequest, "invalid-work-role", "Invalid work role", "The work role data is invalid.")
			return
		}
		_, err := tenancy.ParseTenantID(input.TenantID)
		if err != nil {
			writeProblem(writer, request, http.StatusBadRequest, "invalid-tenant", "Invalid tenant", "A valid tenant identifier is required.")
			return
		}
		actor, ok := authorizeAdminRequest(writer, request, auth, service, "core.authorization.work-role.update", coreauth.InstallationResource(resource.KindWorkRole, chi.URLParam(request, "roleID")))
		if !ok {
			return
		}
		expectedVersion, ok := requireExpectedVersion(writer, request)
		if !ok {
			return
		}
		view, err := service.UpdateWorkRole(request.Context(), chi.URLParam(request, "roleID"), expectedVersion, input, actor, requestIDFromContext(request.Context()), correlationIDFromContext(request.Context()))
		if err != nil {
			writeAdminUpdateProblem(writer, request, err, "work-role")
			return
		}
		writeVersionETag(writer, uint64(view.Version))
		writeJSON(writer, http.StatusOK, view)
	})
	router.Put("/work-users/{accountID}/roles", func(writer http.ResponseWriter, request *http.Request) {
		var input adminstore.ReplaceWorkUserRolesInput
		if decodeJSON(writer, request, &input) != nil {
			writeProblem(writer, request, http.StatusBadRequest, "invalid-work-role-assignment", "Invalid work role assignment", "The work role assignment data is invalid.")
			return
		}
		_, err := tenancy.ParseTenantID(input.TenantID)
		if err != nil {
			writeProblem(writer, request, http.StatusBadRequest, "invalid-tenant", "Invalid tenant", "A valid tenant identifier is required.")
			return
		}
		actor, ok := authorizeAdminRequest(writer, request, auth, service, "core.authorization.work-role.assign", coreauth.InstallationResource(resource.KindWorkAccount, chi.URLParam(request, "accountID")))
		if !ok {
			return
		}
		accountID := chi.URLParam(request, "accountID")
		roleKeys, err := service.ReplaceWorkUserRoles(request.Context(), accountID, input, actor, requestIDFromContext(request.Context()), correlationIDFromContext(request.Context()))
		if err != nil {
			writeProblem(writer, request, http.StatusBadRequest, "work-role-assignment-failed", "Work role assignment failed", "The tenant-bound work roles could not be assigned.")
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"account_id": accountID, "role_keys": roleKeys})
	})
	return router
}

func securityAuditQueryFromRequest(request *http.Request) (adminstore.SecurityAuditQuery, error) {
	values := request.URL.Query()
	for _, name := range []string{"tenant_id", "event_type", "outcome", "limit", "cursor"} {
		if len(values[name]) > 1 {
			return adminstore.SecurityAuditQuery{}, fmt.Errorf("security audit query parameter %q must not be repeated", name)
		}
	}
	query := adminstore.SecurityAuditQuery{
		TenantID:  values.Get("tenant_id"),
		EventType: values.Get("event_type"),
		Outcome:   values.Get("outcome"),
	}
	if rawLimit := strings.TrimSpace(values.Get("limit")); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil {
			return adminstore.SecurityAuditQuery{}, err
		}
		query.Limit = limit
	}
	if rawCursor := strings.TrimSpace(values.Get("cursor")); rawCursor != "" {
		cursor, err := decodeSecurityAuditCursor(rawCursor)
		if err != nil {
			return adminstore.SecurityAuditQuery{}, err
		}
		query.Cursor = &cursor
	}
	return query, nil
}

func encodeSecurityAuditCursor(cursor adminstore.SecurityAuditCursor) (string, error) {
	payload, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeSecurityAuditCursor(value string) (adminstore.SecurityAuditCursor, error) {
	if len(value) > 512 {
		return adminstore.SecurityAuditCursor{}, errors.New("security audit cursor is too long")
	}
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return adminstore.SecurityAuditCursor{}, err
	}
	var cursor adminstore.SecurityAuditCursor
	if err := json.Unmarshal(payload, &cursor); err != nil {
		return adminstore.SecurityAuditCursor{}, err
	}
	return cursor, nil
}

func requireExpectedVersion(writer http.ResponseWriter, request *http.Request) (uint64, bool) {
	values := request.Header.Values("If-Match")
	if len(values) == 0 || strings.TrimSpace(values[0]) == "" {
		writeProblem(writer, request, http.StatusPreconditionRequired, "version-required", "Version required", "The current resource version must be supplied in If-Match.")
		return 0, false
	}
	if len(values) != 1 {
		writeProblem(writer, request, http.StatusBadRequest, "invalid-version", "Invalid version", "If-Match must contain exactly one strong numeric entity tag.")
		return 0, false
	}
	value := strings.TrimSpace(values[0])
	if len(value) < 3 || value[0] != '"' || value[len(value)-1] != '"' || strings.Contains(value, ",") || strings.HasPrefix(value, "W/") {
		writeProblem(writer, request, http.StatusBadRequest, "invalid-version", "Invalid version", "If-Match must contain exactly one strong numeric entity tag.")
		return 0, false
	}
	version, err := strconv.ParseUint(value[1:len(value)-1], 10, 63)
	if err != nil || version == 0 {
		writeProblem(writer, request, http.StatusBadRequest, "invalid-version", "Invalid version", "If-Match must contain exactly one strong numeric entity tag.")
		return 0, false
	}
	return version, true
}

func writeVersionETag(writer http.ResponseWriter, version uint64) {
	writer.Header().Set("ETag", fmt.Sprintf("\"%d\"", version))
}

func writeAdminUpdateProblem(writer http.ResponseWriter, request *http.Request, err error, resource string) {
	switch {
	case errors.Is(err, adminstore.ErrNotFound):
		writeProblem(writer, request, http.StatusNotFound, resource+"-not-found", "Resource not found", "The requested administrative resource does not exist in this tenant.")
	case errors.Is(err, adminstore.ErrVersionConflict):
		writeProblem(writer, request, http.StatusPreconditionFailed, "version-conflict", "Version conflict", "The resource changed after it was loaded. Reload it and retry the update.")
	case errors.Is(err, adminstore.ErrImmutable):
		writeProblem(writer, request, http.StatusConflict, "immutable-resource", "Resource is immutable", "The protected system resource cannot be changed through this contract.")
	default:
		writeProblem(writer, request, http.StatusBadRequest, resource+"-update-failed", "Update failed", "The administrative resource could not be updated.")
	}
}

func authorizeAdminRequest(writer http.ResponseWriter, request *http.Request, auth AuthService, service AdminService, permission string, resource coreauth.Resource) (identity.AuthenticatedActor, bool) {
	identityService, ok := auth.(adminIdentity)
	if !ok || service == nil {
		writeProblem(writer, request, http.StatusNotImplemented, "admin-unavailable", "Administration unavailable", "The administration service is not configured.")
		return identity.AuthenticatedActor{}, false
	}
	actor, err := identityService.ResolveActor(request.Context(), cookieValue(request, "werk_session"), identity.AccessPlaneAdmin)
	if err != nil {
		writeProblem(writer, request, http.StatusUnauthorized, "invalid-admin-session", "Authentication required", "A valid multi-factor admin session is required.")
		return identity.AuthenticatedActor{}, false
	}
	if err := identityService.Authorize(request.Context(), actor, permission, resource); err != nil {
		writeProblem(writer, request, http.StatusForbidden, "permission-denied", "Permission denied", "The admin account is not allowed to perform this operation.")
		return identity.AuthenticatedActor{}, false
	}
	return actor, true
}
