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

type adminReauthentication interface {
	StartReauthentication(context.Context, string, string, identity.ReauthenticationBinding, string, string) (identity.ReauthenticationTicket, error)
	ConsumeReauthentication(context.Context, string, string, identity.ReauthenticationBinding, string, string) error
}

type adminTOTPReauthentication interface {
	StartTOTPReauthentication(context.Context, string, string, identity.ReauthenticationBinding, string, string) (identity.ReauthenticationTicket, error)
}

type AdminService interface {
	CreateWorkUser(context.Context, adminstore.CreateWorkUserInput, identity.AuthenticatedActor, string, string) (adminstore.WorkUserView, error)
	ReissueWorkUserInvitation(context.Context, string, string, adminstore.ReissueWorkUserInvitationInput, identity.AuthenticatedActor, string, string) (adminstore.WorkUserInvitationView, error)
	ListWorkUsers(context.Context, string, identity.AuthenticatedActor, string, string) ([]adminstore.WorkUserDirectoryEntry, error)
	GetWorkUserIdentity(context.Context, string, string, identity.AuthenticatedActor, string, string) (adminstore.WorkUserIdentityView, error)
	RevokeWorkUserSessions(context.Context, string, string, uint64, identity.AuthenticatedActor, string, string) (adminstore.WorkUserSessionRevocationView, error)
	UpdateWorkUserStatus(context.Context, string, string, uint64, adminstore.UpdateWorkUserStatusInput, identity.AuthenticatedActor, string, string) (adminstore.WorkUserStatusView, error)
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
	GetOperationsSummary(context.Context, identity.AuthenticatedActor, string, string) (adminstore.OperationsSummaryView, error)
	ListProviderRegistry(context.Context, identity.AuthenticatedActor, string, string) (adminstore.ProviderRegistryCatalog, error)
	ListIdentityProviders(context.Context, identity.AuthenticatedActor, string, string) (adminstore.IdentityProviderCatalog, error)
}

func adminRoutes(auth AuthService, service AdminService) http.Handler {
	router := chi.NewRouter()
	router.Post("/reauthentication", func(writer http.ResponseWriter, request *http.Request) {
		var input struct {
			CurrentPassword string                           `json:"current_password"`
			TOTPCode        string                           `json:"totp_code"`
			Method          string                           `json:"method"`
			Binding         identity.ReauthenticationBinding `json:"binding"`
		}
		if decodeJSON(writer, request, &input) != nil || !criticalReauthenticationBinding(input.Binding) {
			writeProblem(writer, request, http.StatusBadRequest, "invalid-reauthentication", "Invalid reauthentication", "The requested action binding is invalid.")
			return
		}
		target := coreauth.InstallationResource(resource.Kind(input.Binding.ResourceKind), input.Binding.ResourceID)
		if _, ok := authorizeAdminRequest(writer, request, auth, service, input.Binding.PermissionKey, target); !ok {
			return
		}
		reauth, ok := auth.(adminReauthentication)
		if !ok {
			writeProblem(writer, request, http.StatusNotImplemented, "reauthentication-unavailable", "Reauthentication unavailable", "Action-bound reauthentication is not configured.")
			return
		}
		var ticket identity.ReauthenticationTicket
		var err error
		if input.Method == "totp" {
			totp, supported := auth.(adminTOTPReauthentication)
			if !supported {
				writeProblem(writer, request, http.StatusNotImplemented, "reauthentication-unavailable", "Reauthentication unavailable", "TOTP reauthentication is not configured.")
				return
			}
			ticket, err = totp.StartTOTPReauthentication(request.Context(), cookieValue(request, "werk_session"), input.TOTPCode, input.Binding, requestIDFromContext(request.Context()), correlationIDFromContext(request.Context()))
		} else if input.Method == "" || input.Method == "password" {
			ticket, err = reauth.StartReauthentication(request.Context(), cookieValue(request, "werk_session"), input.CurrentPassword, input.Binding, requestIDFromContext(request.Context()), correlationIDFromContext(request.Context()))
		} else {
			writeProblem(writer, request, http.StatusBadRequest, "invalid-reauthentication", "Invalid reauthentication", "The selected authentication method is invalid.")
			return
		}
		if err != nil {
			writeProblem(writer, request, http.StatusUnauthorized, "reauthentication-failed", "Reauthentication failed", "The authentication proof was rejected or temporarily throttled.")
			return
		}
		writer.Header().Set("Cache-Control", "no-store")
		writeJSON(writer, http.StatusCreated, ticket)
	})
	router.Get("/operations/summary", func(writer http.ResponseWriter, request *http.Request) {
		actor, ok := authorizeAdminRequest(writer, request, auth, service, "core.platform.operations.read", coreauth.InstallationResource(resource.KindPlatformInstallation, resource.RootID))
		if !ok {
			return
		}
		summary, err := service.GetOperationsSummary(request.Context(), actor, requestIDFromContext(request.Context()), correlationIDFromContext(request.Context()))
		if err != nil {
			writeProblem(writer, request, http.StatusInternalServerError, "operations-summary-failed", "Operations summary failed", "The platform operations summary could not be loaded.")
			return
		}
		writer.Header().Set("Cache-Control", "no-store")
		writeJSON(writer, http.StatusOK, summary)
	})
	router.Get("/provider-registry", func(writer http.ResponseWriter, request *http.Request) {
		actor, ok := authorizeAdminRequest(writer, request, auth, service, "core.platform.provider-registry.read", coreauth.InstallationResource(resource.KindPlatformInstallation, resource.RootID))
		if !ok {
			return
		}
		catalog, err := service.ListProviderRegistry(request.Context(), actor, requestIDFromContext(request.Context()), correlationIDFromContext(request.Context()))
		if err != nil {
			writeProblem(writer, request, http.StatusInternalServerError, "provider-registry-list-failed", "Provider Registry listing failed", "The Provider Registry catalog could not be loaded.")
			return
		}
		writer.Header().Set("Cache-Control", "no-store")
		writeJSON(writer, http.StatusOK, catalog)
	})
	router.Get("/identity/providers", func(writer http.ResponseWriter, request *http.Request) {
		actor, ok := authorizeAdminRequest(writer, request, auth, service, "core.identity.provider.read", coreauth.InstallationResource(resource.KindPlatformInstallation, resource.RootID))
		if !ok {
			return
		}
		catalog, err := service.ListIdentityProviders(request.Context(), actor, requestIDFromContext(request.Context()), correlationIDFromContext(request.Context()))
		if err != nil {
			writeProblem(writer, request, http.StatusInternalServerError, "identity-provider-list-failed", "Identity provider listing failed", "The identity provider overview could not be loaded.")
			return
		}
		writer.Header().Set("Cache-Control", "no-store")
		writeJSON(writer, http.StatusOK, catalog)
	})
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
		target := coreauth.InstallationResource(resource.KindPlatformInstallation, resource.RootID)
		actor, ok := authorizeAdminRequest(writer, request, auth, service, "core.tenancy.tenant.create", target)
		if !ok {
			return
		}
		if !requireAdminReauthentication(writer, request, auth, "core.tenancy.tenant.create", target) {
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
		target := coreauth.InstallationResource(resource.KindTenant, tenantID.String())
		actor, ok := authorizeAdminRequest(writer, request, auth, service, "core.tenancy.tenant.update", target)
		if !ok {
			return
		}
		if !requireAdminReauthentication(writer, request, auth, "core.tenancy.tenant.update", target) {
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
		if errors.Is(err, adminstore.ErrOrganizationalUnitDepthConflict) {
			writeAdminUpdateProblem(writer, request, err, "organizational-unit")
			return
		}
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
		// This response may contain the only copy of an invitation token. Keep
		// the protection local to the route as well as in the global security
		// middleware so the contract remains safe when tested or embedded alone.
		writer.Header().Set("Cache-Control", "no-store")
		identityService, ok := auth.(adminIdentity)
		if !ok || service == nil {
			writeProblem(writer, request, http.StatusNotImplemented, "admin-unavailable", "Administration unavailable", "The administration service is not configured.")
			return
		}
		actor, err := identityService.ResolveActor(request.Context(), cookieValue(request, "werk_session"), identity.AccessPlaneAdmin)
		if err != nil {
			writeProblem(writer, request, http.StatusUnauthorized, "invalid-admin-session", "Authentication required", "A valid admin session is required.")
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
	router.Get("/tenants/{tenantID}/work-users/{accountID}/identity", func(writer http.ResponseWriter, request *http.Request) {
		tenantID, err := tenancy.ParseTenantID(chi.URLParam(request, "tenantID"))
		accountID := strings.ToLower(strings.TrimSpace(chi.URLParam(request, "accountID")))
		if err != nil || !validUUID(accountID) {
			writeProblem(writer, request, http.StatusBadRequest, "invalid-work-user", "Invalid work user", "The tenant or account identifier is invalid.")
			return
		}
		actor, ok := authorizeAdminRequest(writer, request, auth, service, "core.identity.work-account.read", coreauth.InstallationResource(resource.KindTenant, tenantID.String()))
		if !ok {
			return
		}
		view, err := service.GetWorkUserIdentity(request.Context(), tenantID.String(), accountID, actor, requestIDFromContext(request.Context()), correlationIDFromContext(request.Context()))
		if errors.Is(err, adminstore.ErrNotFound) {
			writeProblem(writer, request, http.StatusNotFound, "work-user-not-found", "Work user not found", "The work account was not found in this tenant.")
			return
		}
		if err != nil {
			writeProblem(writer, request, http.StatusInternalServerError, "work-user-identity-failed", "Identity details failed", "Authentication and session details could not be loaded.")
			return
		}
		writer.Header().Set("Cache-Control", "no-store")
		writeJSON(writer, http.StatusOK, view)
	})
	router.Post("/tenants/{tenantID}/work-users/{accountID}/sessions/revoke", func(writer http.ResponseWriter, request *http.Request) {
		tenantID, err := tenancy.ParseTenantID(chi.URLParam(request, "tenantID"))
		accountID := strings.ToLower(strings.TrimSpace(chi.URLParam(request, "accountID")))
		if err != nil || !validUUID(accountID) {
			writeProblem(writer, request, http.StatusBadRequest, "invalid-work-user", "Invalid work user", "The tenant or account identifier is invalid.")
			return
		}
		target := coreauth.InstallationResource(resource.KindWorkAccount, accountID)
		actor, ok := authorizeAdminRequest(writer, request, auth, service, "core.identity.work-account.update", target)
		if !ok {
			return
		}
		if !requireAdminReauthentication(writer, request, auth, "core.identity.work-account.update", target) {
			return
		}
		expectedVersion, ok := requireExpectedVersion(writer, request)
		if !ok {
			return
		}
		view, err := service.RevokeWorkUserSessions(request.Context(), tenantID.String(), accountID, expectedVersion, actor, requestIDFromContext(request.Context()), correlationIDFromContext(request.Context()))
		if err != nil {
			writeAdminUpdateProblem(writer, request, err, "work-user")
			return
		}
		writeVersionETag(writer, view.Version)
		writer.Header().Set("Cache-Control", "no-store")
		writeJSON(writer, http.StatusOK, view)
	})
	router.Put("/tenants/{tenantID}/work-users/{accountID}", func(writer http.ResponseWriter, request *http.Request) {
		tenantID, err := tenancy.ParseTenantID(chi.URLParam(request, "tenantID"))
		if err != nil {
			writeProblem(writer, request, http.StatusBadRequest, "invalid-tenant", "Invalid tenant", "The tenant identifier is invalid.")
			return
		}
		accountID := strings.ToLower(strings.TrimSpace(chi.URLParam(request, "accountID")))
		target := coreauth.InstallationResource(resource.KindWorkAccount, accountID)
		actor, ok := authorizeAdminRequest(writer, request, auth, service, "core.identity.work-account.update", target)
		if !ok {
			return
		}
		if !requireAdminReauthentication(writer, request, auth, "core.identity.work-account.update", target) {
			return
		}
		expectedVersion, ok := requireExpectedVersion(writer, request)
		if !ok {
			return
		}
		var input adminstore.UpdateWorkUserStatusInput
		if decodeJSON(writer, request, &input) != nil {
			writeProblem(writer, request, http.StatusBadRequest, "invalid-work-user", "Invalid work user", "The work account status data is invalid.")
			return
		}
		view, err := service.UpdateWorkUserStatus(
			request.Context(), tenantID.String(), accountID, expectedVersion, input, actor,
			requestIDFromContext(request.Context()), correlationIDFromContext(request.Context()),
		)
		if err != nil {
			writeAdminUpdateProblem(writer, request, err, "work-user")
			return
		}
		writeVersionETag(writer, view.Version)
		writer.Header().Set("Cache-Control", "no-store")
		writeJSON(writer, http.StatusOK, view)
	})
	router.Post("/tenants/{tenantID}/work-users/{accountID}/invitation", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Cache-Control", "no-store")
		tenantID, err := tenancy.ParseTenantID(chi.URLParam(request, "tenantID"))
		if err != nil {
			writeProblem(writer, request, http.StatusBadRequest, "invalid-tenant", "Invalid tenant", "The tenant identifier is invalid.")
			return
		}
		accountID := strings.ToLower(strings.TrimSpace(chi.URLParam(request, "accountID")))
		target := coreauth.InstallationResource(resource.KindWorkAccount, accountID)
		actor, ok := authorizeAdminRequest(writer, request, auth, service, "core.identity.work-account.update", target)
		if !ok {
			return
		}
		if !requireAdminReauthentication(writer, request, auth, "core.identity.work-account.update", target) {
			return
		}
		var input adminstore.ReissueWorkUserInvitationInput
		if decodeJSON(writer, request, &input) != nil {
			writeProblem(writer, request, http.StatusBadRequest, "invalid-work-user-invitation", "Invalid work user invitation", "The invitation draft data is invalid.")
			return
		}
		view, err := service.ReissueWorkUserInvitation(
			request.Context(), tenantID.String(), accountID, input, actor,
			requestIDFromContext(request.Context()), correlationIDFromContext(request.Context()),
		)
		switch {
		case errors.Is(err, adminstore.ErrInvalidWorkUserInvitationReissue):
			writeProblem(writer, request, http.StatusBadRequest, "work-user-invitation-reissue-failed", "Invitation reissue failed", "The initial work account invitation could not be reissued.")
			return
		case errors.Is(err, adminstore.ErrImmutable):
			writeProblem(writer, request, http.StatusConflict, "work-user-invitation-not-replaceable", "Invitation cannot be reissued", "The initial work account invitation cannot be replaced in its current state.")
			return
		case err != nil:
			writeProblem(writer, request, http.StatusInternalServerError, "work-user-invitation-reissue-processing-failed", "Invitation reissue failed", "The initial work account invitation could not be processed.")
			return
		}
		writeJSON(writer, http.StatusCreated, view)
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
	case errors.Is(err, adminstore.ErrOrganizationalUnitReferenced):
		writeProblem(writer, request, http.StatusConflict, "organizational-unit-referenced", "Organizational unit is referenced", "Active or scheduled organizational, access, app, or role links must be removed, revoked where supported, or allowed to expire before this organizational unit can be archived. Disabling a containing app, group, or role does not remove its durable edge.")
	case errors.Is(err, adminstore.ErrOrganizationalUnitInheritedAccessConflict):
		writeProblem(writer, request, http.StatusConflict, "organizational-unit-inherited-access-conflict", "Inherited access would change", "Reparenting this organizational unit would change the reach of an active or scheduled descendant-inclusive app entitlement or access-group membership.")
	case errors.Is(err, adminstore.ErrOrganizationalUnitDepthConflict):
		writeProblem(writer, request, http.StatusConflict, "organizational-unit-depth-limit-exceeded", "Organizational hierarchy is too deep", "The requested hierarchy change would exceed the supported organizational-unit depth.")
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
		writeProblem(writer, request, http.StatusUnauthorized, "invalid-admin-session", "Authentication required", "A valid admin session is required.")
		return identity.AuthenticatedActor{}, false
	}
	if err := identityService.Authorize(request.Context(), actor, permission, resource); err != nil {
		writeProblem(writer, request, http.StatusForbidden, "permission-denied", "Permission denied", "The admin account is not allowed to perform this operation.")
		return identity.AuthenticatedActor{}, false
	}
	return actor, true
}

func criticalReauthenticationBinding(binding identity.ReauthenticationBinding) bool {
	switch binding.PermissionKey {
	case "core.tenancy.tenant.create":
		return binding.ResourceKind == string(resource.KindPlatformInstallation) && binding.ResourceID == resource.RootID
	case "core.tenancy.tenant.update":
		return binding.ResourceKind == string(resource.KindTenant) && binding.ResourceID != ""
	case "core.identity.work-account.update":
		return binding.ResourceKind == string(resource.KindWorkAccount) && binding.ResourceID != ""
	default:
		return false
	}
}

func requireAdminReauthentication(writer http.ResponseWriter, request *http.Request, auth AuthService, permission string, target coreauth.Resource) bool {
	binding := identity.ReauthenticationBinding{PermissionKey: permission, ResourceKind: string(target.Reference.Kind), ResourceID: target.Reference.ID}
	reauth, ok := auth.(adminReauthentication)
	if !ok {
		writeProblem(writer, request, http.StatusNotImplemented, "reauthentication-unavailable", "Reauthentication unavailable", "Action-bound reauthentication is not configured.")
		return false
	}
	ticket := strings.TrimSpace(request.Header.Get("X-WERK-Reauth-Token"))
	if ticket == "" {
		writer.Header().Set("WERK-Reauth-Permission", binding.PermissionKey)
		writer.Header().Set("WERK-Reauth-Resource-Kind", binding.ResourceKind)
		writer.Header().Set("WERK-Reauth-Resource-ID", binding.ResourceID)
		writeProblem(writer, request, http.StatusPreconditionRequired, "reauthentication-required", "Reauthentication required", "Confirm an available administrator authentication method for this critical action.")
		return false
	}
	if err := reauth.ConsumeReauthentication(request.Context(), cookieValue(request, "werk_session"), ticket, binding, requestIDFromContext(request.Context()), correlationIDFromContext(request.Context())); err != nil {
		writeProblem(writer, request, http.StatusPreconditionRequired, "reauthentication-required", "Reauthentication required", "The action-bound confirmation is missing, expired, or already used.")
		return false
	}
	return true
}
