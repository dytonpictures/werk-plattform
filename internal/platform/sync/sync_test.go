package platformsync

import (
	"testing"
	"time"

	"github.com/dytonpictures/werk/internal/core/authorization"
	"github.com/dytonpictures/werk/internal/core/compliance"
	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/resource"
	"github.com/dytonpictures/werk/internal/core/tenancy"
)

func validPolicyRequest(at time.Time) PolicyRequest {
	tenantID := tenancy.TenantID{1}
	return PolicyRequest{
		ID: [16]byte{1}, CorrelationID: [16]byte{2},
		ExpectedInstanceID: "instance.primary", ExpectedRealmID: "realm.main",
		ExpectedAuthorityDomain:     DomainIdentityControl,
		ExpectedAuthorityGeneration: 3, ExpectedPolicyRevision: 7,
		RequestedAt: at.Add(-time.Second), ExpiresAt: at.Add(time.Minute),
		Authorization: authorization.PolicyRequest{
			Actor: identity.AuthenticatedActor{
				AccountID: identity.AccountID{3}, AccountClass: identity.AccountClassWork,
				Audience: identity.AudienceWork, Kind: identity.AuthenticationInteractive,
				Assurance: identity.AssuranceSingleFactor, TenantID: &tenantID,
			},
			Permission: "core.workspace.access",
			Target:     authorization.TenantResource(tenantID, resource.KindWorkspace, resource.RootID, authorization.ScopeTenant),
			Grants: []authorization.Grant{{
				AccessPlane: identity.AccessPlaneWork, Permission: "core.workspace.access",
				Scope: authorization.ScopeTenant, TenantID: &tenantID, ValidFrom: at.Add(-time.Hour),
			}},
			DataProfile: compliance.ResourceDataProfile{
				ResourceKind: resource.KindWorkspace, PersonalData: compliance.PersonalDataPersonal,
				Confidentiality:            compliance.ConfidentialityConfidential,
				ProcessingActivityRequired: true, Status: resource.RegistrationActive, Version: 1,
			},
			ProcessingPolicy: compliance.ProcessingPolicy{
				Permission: "core.workspace.access", ResourceKind: resource.KindWorkspace, Required: true,
				Context: compliance.ProcessingContext{
					ActivityKey:   "core.workspace.context-access",
					PurposeKey:    "core.workspace.work-delivery",
					LegalBasisRef: "operator.processing-register.workspace",
				},
				Status: resource.RegistrationActive, Version: 1,
			},
		},
	}
}

func validAuthority(at time.Time, coordination AuthorityCoordination) AuthoritySnapshot {
	return AuthoritySnapshot{
		InstanceID: "instance.primary", RealmID: "realm.main",
		Domain: DomainIdentityControl, Coordination: coordination,
		AuthorityGeneration: 3, PolicyRevision: 7, ObservedAt: at.Add(-time.Second),
	}
}

func TestSharedDatabaseEvaluatesCanonicalPolicy(t *testing.T) {
	at := time.Date(2026, 7, 22, 15, 0, 0, 0, time.UTC)
	decision := Evaluate(validPolicyRequest(at), validAuthority(at, CoordinationSharedDatabase), at)
	if !decision.Allowed() || decision.Reason != ReasonAuthorized || !decision.Authorization.Allowed() {
		t.Fatalf("decision = %#v", decision)
	}
}

func TestStaleGenerationAndPolicyRevisionFailClosed(t *testing.T) {
	at := time.Date(2026, 7, 22, 15, 0, 0, 0, time.UTC)
	request := validPolicyRequest(at)
	authority := validAuthority(at, CoordinationSharedDatabase)
	authority.AuthorityGeneration++
	if decision := Evaluate(request, authority, at); decision.Reason != ReasonAuthorityGenerationOld {
		t.Fatalf("generation decision = %#v", decision)
	}
	authority = validAuthority(at, CoordinationSharedDatabase)
	authority.PolicyRevision++
	if decision := Evaluate(request, authority, at); decision.Reason != ReasonPolicyRevisionOld {
		t.Fatalf("policy revision decision = %#v", decision)
	}
}

func TestPlatformWitnessRequiresLeaseAndFencing(t *testing.T) {
	at := time.Date(2026, 7, 22, 15, 0, 0, 0, time.UTC)
	request := validPolicyRequest(at)
	authority := validAuthority(at, CoordinationPlatformWitness)
	if decision := Evaluate(request, authority, at); decision.Reason != ReasonAuthorityUnavailable {
		t.Fatalf("missing lease decision = %#v", decision)
	}
	leaseExpiry := at.Add(time.Minute)
	authority.LeaseHeld = true
	authority.LeaseExpiresAt = &leaseExpiry
	authority.FencingTokenVerified = true
	if decision := Evaluate(request, authority, at); !decision.Allowed() {
		t.Fatalf("valid lease decision = %#v", decision)
	}
	authority.Fenced = true
	if decision := Evaluate(request, authority, at); decision.Reason != ReasonAuthorityFenced {
		t.Fatalf("fenced decision = %#v", decision)
	}
}

func TestExpiredEnvelopeAndCoreDenialRemainDenied(t *testing.T) {
	at := time.Date(2026, 7, 22, 15, 0, 0, 0, time.UTC)
	request := validPolicyRequest(at)
	request.ExpiresAt = at
	if decision := Evaluate(request, validAuthority(at, CoordinationLocal), at); decision.Reason != ReasonRequestExpired {
		t.Fatalf("expired decision = %#v", decision)
	}
	request = validPolicyRequest(at)
	request.Authorization.Grants = nil
	if decision := Evaluate(request, validAuthority(at, CoordinationLocal), at); decision.Reason != ReasonAuthorizationDenied {
		t.Fatalf("authorization decision = %#v", decision)
	}
}

func TestHealthLikeStateCannotCreateAuthority(t *testing.T) {
	at := time.Date(2026, 7, 22, 15, 0, 0, 0, time.UTC)
	authority := validAuthority(at, CoordinationPlatformWitness)
	authority.LeaseHeld = true
	leaseExpiry := at.Add(time.Minute)
	authority.LeaseExpiresAt = &leaseExpiry
	if decision := Evaluate(validPolicyRequest(at), authority, at); decision.Reason != ReasonAuthorityUnavailable {
		t.Fatalf("lease without verified fencing decision = %#v", decision)
	}
}

func TestAuthorityDomainMismatchFailsClosed(t *testing.T) {
	at := time.Date(2026, 7, 22, 15, 0, 0, 0, time.UTC)
	request := validPolicyRequest(at)
	authority := validAuthority(at, CoordinationLocal)
	authority.Domain = "unregistered-control"
	if decision := Evaluate(request, authority, at); decision.Reason != ReasonInvalidRequest {
		t.Fatalf("unregistered domain decision = %#v", decision)
	}

	authority = validAuthority(at, CoordinationLocal)
	request.ExpectedAuthorityDomain = "unregistered-control"
	if decision := Evaluate(request, authority, at); decision.Reason != ReasonInvalidRequest {
		t.Fatalf("unregistered request domain decision = %#v", decision)
	}
}
