package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	coreauth "github.com/dytonpictures/werk/internal/core/authorization"
	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/tenancy"
	"github.com/dytonpictures/werk/internal/platform/config"
	"github.com/dytonpictures/werk/internal/platform/workspacestore"
)

type workAuthStub struct {
	actor        identity.AuthenticatedActor
	resolveErr   error
	authorizeErr error
}

func (stub workAuthStub) Login(context.Context, string, string) (string, string, error) {
	return "", "", errors.New("not implemented")
}
func (stub workAuthStub) Session(context.Context, string) (any, error) {
	return nil, errors.New("not implemented")
}
func (stub workAuthStub) Logout(context.Context, string) error { return nil }
func (stub workAuthStub) ResolveActor(_ context.Context, _ string, plane identity.AccessPlane) (identity.AuthenticatedActor, error) {
	if stub.resolveErr != nil {
		return identity.AuthenticatedActor{}, stub.resolveErr
	}
	if err := identity.AuthorizeAccessPlane(stub.actor, plane); err != nil {
		return identity.AuthenticatedActor{}, err
	}
	return stub.actor, nil
}
func (stub workAuthStub) Authorize(context.Context, identity.AuthenticatedActor, string, coreauth.Resource) error {
	return stub.authorizeErr
}

type workspaceServiceStub struct {
	view workspacestore.Overview
}

func (stub workspaceServiceStub) Overview(context.Context, identity.AuthenticatedActor) (workspacestore.Overview, error) {
	return stub.view, nil
}

func TestWorkspaceRequiresWorkPlaneAndTenantPermission(t *testing.T) {
	tenantID, _ := tenancy.ParseTenantID("0196f000-0000-7000-8000-000000000701")
	workActor := identity.AuthenticatedActor{
		AccountID: identity.AccountID{1}, AccountClass: identity.AccountClassWork,
		Audience: identity.AudienceWork, Kind: identity.AuthenticationInteractive,
		Assurance: identity.AssuranceSingleFactor, TenantID: &tenantID,
	}
	view := workspacestore.Overview{Tenant: workspacestore.TenantView{ID: tenantID.String(), Name: "Tenant A", Status: "active"}, Permission: "core.workspace.access"}

	allowed := request(t, NewRouterWithServices(config.Config{}, readinessStub{}, testLogger(), workAuthStub{actor: workActor}, workspaceServiceStub{view: view}, nil), http.MethodGet, "/api/v1/workspace", "")
	if allowed.Code != http.StatusOK || !containsAll(allowed.Body.String(), `"name":"Tenant A"`, `"permission":"core.workspace.access"`) {
		t.Fatalf("allowed workspace response = %d %s", allowed.Code, allowed.Body.String())
	}

	adminActor := identity.AuthenticatedActor{
		AccountID: identity.AccountID{2}, AccountClass: identity.AccountClassAdmin,
		Audience: identity.AudienceAdmin, Kind: identity.AuthenticationInteractive,
		Assurance: identity.AssuranceMultiFactor,
	}
	adminDenied := request(t, NewRouterWithServices(config.Config{}, readinessStub{}, testLogger(), workAuthStub{actor: adminActor}, workspaceServiceStub{view: view}, nil), http.MethodGet, "/api/v1/workspace", "")
	if adminDenied.Code != http.StatusUnauthorized {
		t.Fatalf("admin workspace status = %d, want %d", adminDenied.Code, http.StatusUnauthorized)
	}

	permissionDenied := request(t, NewRouterWithServices(config.Config{}, readinessStub{}, testLogger(), workAuthStub{actor: workActor, authorizeErr: coreauth.ErrDenied}, workspaceServiceStub{view: view}, nil), http.MethodGet, "/api/v1/workspace", "")
	if permissionDenied.Code != http.StatusForbidden {
		t.Fatalf("unprivileged workspace status = %d, want %d", permissionDenied.Code, http.StatusForbidden)
	}
}

func containsAll(value string, fragments ...string) bool {
	for _, fragment := range fragments {
		if !strings.Contains(value, fragment) {
			return false
		}
	}
	return true
}
