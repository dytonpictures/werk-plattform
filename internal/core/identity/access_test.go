package identity

import (
	"errors"
	"testing"

	"github.com/dytonpictures/werk/internal/core/tenancy"
)

func TestAuthorizeAccessPlaneAcceptsOnlyMatchingAccountBoundary(t *testing.T) {
	tenant := tenancy.TenantID{1}
	account := AccountID{1}

	tests := []struct {
		name   string
		actor  AuthenticatedActor
		plane  AccessPlane
		allows bool
	}{
		{
			name: "work",
			actor: AuthenticatedActor{
				AccountID: account, AccountClass: AccountClassWork, Audience: AudienceWork,
				Kind: AuthenticationInteractive, Assurance: AssuranceSingleFactor, TenantID: &tenant,
			},
			plane: AccessPlaneWork, allows: true,
		},
		{
			name: "admin",
			actor: AuthenticatedActor{
				AccountID: account, AccountClass: AccountClassAdmin, Audience: AudienceAdmin,
				Kind: AuthenticationInteractive, Assurance: AssuranceMultiFactor,
			},
			plane: AccessPlaneAdmin, allows: true,
		},
		{
			name: "service",
			actor: AuthenticatedActor{
				AccountID: account, AccountClass: AccountClassService, Audience: AudienceService,
				Kind: AuthenticationWorkload, Assurance: AssuranceUnknown,
			},
			plane: AccessPlaneService, allows: true,
		},
		{
			name: "agent uses technical plane",
			actor: AuthenticatedActor{
				AccountID: account, AccountClass: AccountClassAgent, Audience: AudienceService,
				Kind: AuthenticationWorkload, Assurance: AssuranceSingleFactor, TenantID: &tenant,
			},
			plane: AccessPlaneService, allows: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := AuthorizeAccessPlane(test.actor, test.plane)
			if test.allows && err != nil {
				t.Fatalf("authorize access plane: %v", err)
			}
			if !test.allows && !errors.Is(err, ErrAccessDenied) {
				t.Fatalf("error = %v, want ErrAccessDenied", err)
			}
		})
	}
}

func TestAuthorizeAccessPlaneRejectsMismatchesAndIncompleteContext(t *testing.T) {
	tenant := tenancy.TenantID{1}
	baseWork := AuthenticatedActor{
		AccountID: accountID(), AccountClass: AccountClassWork, Audience: AudienceWork,
		Kind: AuthenticationInteractive, Assurance: AssuranceSingleFactor, TenantID: &tenant,
	}
	tests := []struct {
		name  string
		actor AuthenticatedActor
		plane AccessPlane
	}{
		{name: "work cannot enter admin", actor: baseWork, plane: AccessPlaneAdmin},
		{name: "work cannot enter service", actor: baseWork, plane: AccessPlaneService},
		{name: "work needs tenant", actor: withoutTenant(baseWork), plane: AccessPlaneWork},
		{name: "work rejects zero tenant", actor: withTenant(baseWork, tenancy.TenantID{}), plane: AccessPlaneWork},
		{name: "admin needs multi factor", actor: AuthenticatedActor{
			AccountID: accountID(), AccountClass: AccountClassAdmin, Audience: AudienceAdmin,
			Kind: AuthenticationInteractive, Assurance: AssuranceSingleFactor,
		}, plane: AccessPlaneAdmin},
		{name: "admin rejects tenant context", actor: AuthenticatedActor{
			AccountID: accountID(), AccountClass: AccountClassAdmin, Audience: AudienceAdmin,
			Kind: AuthenticationInteractive, Assurance: AssuranceMultiFactor, TenantID: &tenant,
		}, plane: AccessPlaneAdmin},
		{name: "service rejects interactive authentication", actor: AuthenticatedActor{
			AccountID: accountID(), AccountClass: AccountClassService, Audience: AudienceService,
			Kind: AuthenticationInteractive,
		}, plane: AccessPlaneService},
		{name: "unknown audience", actor: AuthenticatedActor{
			AccountID: accountID(), AccountClass: AccountClassWork, Audience: "unknown",
			Kind: AuthenticationInteractive, TenantID: &tenant,
		}, plane: AccessPlaneWork},
		{name: "unknown plane", actor: baseWork, plane: "unknown"},
		{name: "agent cannot enter work", actor: AuthenticatedActor{
			AccountID: accountID(), AccountClass: AccountClassAgent, Audience: AudienceService,
			Kind: AuthenticationWorkload, Assurance: AssuranceSingleFactor, TenantID: &tenant,
		}, plane: AccessPlaneWork},
		{name: "agent requires tenant", actor: AuthenticatedActor{
			AccountID: accountID(), AccountClass: AccountClassAgent, Audience: AudienceService,
			Kind: AuthenticationWorkload, Assurance: AssuranceSingleFactor,
		}, plane: AccessPlaneService},
		{name: "zero account", actor: AuthenticatedActor{
			AccountClass: AccountClassWork, Audience: AudienceWork,
			Kind: AuthenticationInteractive, TenantID: &tenant,
		}, plane: AccessPlaneWork},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := AuthorizeAccessPlane(test.actor, test.plane); !errors.Is(err, ErrAccessDenied) {
				t.Fatalf("error = %v, want ErrAccessDenied", err)
			}
		})
	}
}

func accountID() AccountID {
	return AccountID{1}
}

func withoutTenant(actor AuthenticatedActor) AuthenticatedActor {
	actor.TenantID = nil
	return actor
}

func withTenant(actor AuthenticatedActor, tenant tenancy.TenantID) AuthenticatedActor {
	actor.TenantID = &tenant
	return actor
}
