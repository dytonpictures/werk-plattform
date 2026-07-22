// Package identity contains provider-independent identity and access contracts.
package identity

import (
	"errors"

	"github.com/dytonpictures/werk/internal/core/tenancy"
)

var ErrAccessDenied = errors.New("access denied")

type AccountID [16]byte

func (id AccountID) IsZero() bool {
	return id == AccountID{}
}

type AccountClass string

const (
	AccountClassWork    AccountClass = "work"
	AccountClassAdmin   AccountClass = "admin"
	AccountClassService AccountClass = "service"
	AccountClassAgent   AccountClass = "agent"
)

type AccessPlane string

const (
	AccessPlaneWork    AccessPlane = "work"
	AccessPlaneAdmin   AccessPlane = "admin"
	AccessPlaneService AccessPlane = "service"
)

type Audience string

const (
	AudienceWork    Audience = "work"
	AudienceAdmin   Audience = "admin"
	AudienceService Audience = "service"
)

type AuthenticationKind string

const (
	AuthenticationInteractive AuthenticationKind = "interactive"
	AuthenticationWorkload    AuthenticationKind = "workload"
)

type AuthenticationAssurance string

const (
	AssuranceUnknown      AuthenticationAssurance = "unknown"
	AssuranceSingleFactor AuthenticationAssurance = "single-factor"
	AssuranceMultiFactor  AuthenticationAssurance = "multi-factor"
	// Authentication methods are recorded separately. Assurance describes the
	// verified result of a ceremony, not whether it used a password or passkey.
)

// AuthenticatedActor is created only after a provider or service-token adapter
// has verified the credential. Clients cannot construct an authorized actor by
// sending these values as request data.
type AuthenticatedActor struct {
	AccountID    AccountID
	AccountClass AccountClass
	Audience     Audience
	Kind         AuthenticationKind
	Assurance    AuthenticationAssurance
	TenantID     *tenancy.TenantID
}

// AuthorizeAccessPlane applies the immutable account-class boundary. It is
// intentionally independent from a concrete provider, token, session, or HTTP
// request so every future adapter must use the same fail-closed decision.
func AuthorizeAccessPlane(actor AuthenticatedActor, expected AccessPlane) error {
	if ValidateActorBoundary(actor) != nil {
		return ErrAccessDenied
	}

	switch expected {
	case AccessPlaneWork:
		if actor.AccountClass != AccountClassWork {
			return ErrAccessDenied
		}
	case AccessPlaneAdmin:
		if actor.AccountClass != AccountClassAdmin || actor.Assurance != AssuranceMultiFactor {
			return ErrAccessDenied
		}
	case AccessPlaneService:
		if actor.AccountClass != AccountClassService && actor.AccountClass != AccountClassAgent {
			return ErrAccessDenied
		}
	default:
		return ErrAccessDenied
	}

	return nil
}

// ValidateActorBoundary checks the immutable identity dimensions without
// granting access to an API plane. It is suitable for session self-service and
// as the common precondition for authorization.
func ValidateActorBoundary(actor AuthenticatedActor) error {
	if actor.AccountID.IsZero() {
		return ErrAccessDenied
	}
	switch actor.AccountClass {
	case AccountClassWork:
		if actor.Audience != AudienceWork || actor.Kind != AuthenticationInteractive || !hasTenant(actor.TenantID) {
			return ErrAccessDenied
		}
	case AccountClassAdmin:
		if actor.Audience != AudienceAdmin || actor.Kind != AuthenticationInteractive || actor.TenantID != nil {
			return ErrAccessDenied
		}
	case AccountClassService:
		if actor.Audience != AudienceService || actor.Kind != AuthenticationWorkload {
			return ErrAccessDenied
		}
	case AccountClassAgent:
		if actor.Audience != AudienceService || actor.Kind != AuthenticationWorkload || !hasTenant(actor.TenantID) {
			return ErrAccessDenied
		}
	default:
		return ErrAccessDenied
	}
	if actor.Assurance != AssuranceUnknown && actor.Assurance != AssuranceSingleFactor && actor.Assurance != AssuranceMultiFactor {
		return ErrAccessDenied
	}
	return nil
}

func hasTenant(tenantID *tenancy.TenantID) bool {
	return tenantID != nil && !tenantID.IsZero()
}
