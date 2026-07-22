package authorization

import (
	"errors"
	"strings"
	"time"

	"github.com/dytonpictures/werk/internal/core/compliance"
	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/resource"
	"github.com/dytonpictures/werk/internal/core/tenancy"
)

var ErrDenied = errors.New("authorization denied")

type ScopeType string

const (
	ScopeInstallation       ScopeType = "installation"
	ScopeTenant             ScopeType = "tenant"
	ScopeOrganizationalUnit ScopeType = "organizational-unit"
	ScopeResource           ScopeType = "resource"
)

type Resource struct {
	Reference resource.Ref
	Scope     ScopeType
}

func InstallationResource(kind resource.Kind, id string) Resource {
	return Resource{Reference: resource.InstallationRef(kind, id), Scope: ScopeInstallation}
}

func TenantResource(tenantID tenancy.TenantID, kind resource.Kind, id string, scope ScopeType) Resource {
	return Resource{Reference: resource.TenantRef(tenantID, kind, id), Scope: scope}
}

type Grant struct {
	AccessPlane identity.AccessPlane
	Permission  string
	Scope       ScopeType
	TenantID    *tenancy.TenantID
	ScopeID     string
	ValidFrom   time.Time
	ValidUntil  *time.Time
}

// PolicyRequest is the complete platform policy input. Actor context, data
// profile and processing policy are resolved by trusted server components and
// cannot be widened by request payloads.
type PolicyRequest struct {
	Actor            identity.AuthenticatedActor
	Permission       string
	Target           Resource
	Grants           []Grant
	DataProfile      compliance.ResourceDataProfile
	ProcessingPolicy compliance.ProcessingPolicy
	EvaluatedAt      time.Time
}

// PlatformContext is derived exclusively from the authenticated server-side
// actor. Request payloads may not select or widen it.
type PlatformContext struct {
	ActorID      identity.AccountID
	AccountClass identity.AccountClass
	AccessPlane  identity.AccessPlane
	TenantID     *tenancy.TenantID
}

func ResolvePlatformContext(actor identity.AuthenticatedActor) (PlatformContext, error) {
	plane := planeForActor(actor)
	if plane == "" || identity.AuthorizeAccessPlane(actor, plane) != nil {
		return PlatformContext{}, ErrDenied
	}
	switch actor.AccountClass {
	case identity.AccountClassAdmin:
		if actor.TenantID != nil {
			return PlatformContext{}, ErrDenied
		}
	case identity.AccountClassWork, identity.AccountClassAgent:
		if actor.TenantID == nil || actor.TenantID.IsZero() {
			return PlatformContext{}, ErrDenied
		}
	case identity.AccountClassService:
		if actor.TenantID != nil && actor.TenantID.IsZero() {
			return PlatformContext{}, ErrDenied
		}
	}
	return PlatformContext{
		ActorID: actor.AccountID, AccountClass: actor.AccountClass,
		AccessPlane: plane, TenantID: actor.TenantID,
	}, nil
}

type DecisionEffect string

const (
	DecisionAllow DecisionEffect = "allow"
	DecisionDeny  DecisionEffect = "deny"
)

type DecisionReason string

const (
	ReasonGrantMatched      DecisionReason = "grant-matched"
	ReasonInvalidRequest    DecisionReason = "invalid-request"
	ReasonActorBoundary     DecisionReason = "actor-boundary"
	ReasonProcessingDenied  DecisionReason = "processing-denied"
	ReasonNoApplicableGrant DecisionReason = "no-applicable-grant"
)

type Decision struct {
	Effect DecisionEffect
	Reason DecisionReason
}

func (decision Decision) Allowed() bool {
	return decision.Effect == DecisionAllow
}

func Evaluate(request PolicyRequest) Decision {
	permission := strings.TrimSpace(request.Permission)
	if permission == "" || request.EvaluatedAt.IsZero() || !resource.ValidKey(permission) || validateTarget(request.Target) != nil {
		return Decision{Effect: DecisionDeny, Reason: ReasonInvalidRequest}
	}
	platformContext, err := ResolvePlatformContext(request.Actor)
	if err != nil || !platformContextMayAddress(platformContext, request.Target.Reference) {
		return Decision{Effect: DecisionDeny, Reason: ReasonActorBoundary}
	}
	if request.DataProfile.ResourceKind != request.Target.Reference.Kind ||
		request.ProcessingPolicy.Permission != permission ||
		request.ProcessingPolicy.ResourceKind != request.Target.Reference.Kind ||
		compliance.Authorize(request.DataProfile, request.ProcessingPolicy) != nil {
		return Decision{Effect: DecisionDeny, Reason: ReasonProcessingDenied}
	}
	for _, grant := range request.Grants {
		if grant.AccessPlane != platformContext.AccessPlane || grant.Permission != permission || request.EvaluatedAt.Before(grant.ValidFrom) ||
			(grant.ValidUntil != nil && !request.EvaluatedAt.Before(*grant.ValidUntil)) {
			continue
		}
		if scopeMatches(grant, request.Target) {
			return Decision{Effect: DecisionAllow, Reason: ReasonGrantMatched}
		}
	}
	return Decision{Effect: DecisionDeny, Reason: ReasonNoApplicableGrant}
}

func Authorize(request PolicyRequest) error {
	if !Evaluate(request).Allowed() {
		return ErrDenied
	}
	return nil
}

func validateTarget(target Resource) error {
	if err := target.Reference.Validate(); err != nil {
		return err
	}
	switch target.Scope {
	case ScopeInstallation:
		if target.Reference.Boundary != resource.BoundaryInstallation {
			return resource.ErrInvalid
		}
	case ScopeTenant, ScopeOrganizationalUnit, ScopeResource:
		if target.Reference.Boundary != resource.BoundaryTenant {
			return resource.ErrInvalid
		}
	default:
		return resource.ErrInvalid
	}
	return nil
}

func platformContextMayAddress(platformContext PlatformContext, reference resource.Ref) bool {
	if platformContext.AccountClass == identity.AccountClassAdmin ||
		(platformContext.AccountClass == identity.AccountClassService && platformContext.TenantID == nil) {
		return reference.Boundary == resource.BoundaryInstallation && platformContext.TenantID == nil
	}
	return reference.Boundary == resource.BoundaryTenant && sameTenant(platformContext.TenantID, reference.TenantID)
}

func scopeMatches(grant Grant, target Resource) bool {
	switch grant.Scope {
	case ScopeInstallation:
		return target.Scope == ScopeInstallation && target.Reference.Boundary == resource.BoundaryInstallation &&
			(grant.AccessPlane == identity.AccessPlaneAdmin || grant.AccessPlane == identity.AccessPlaneService) &&
			grant.TenantID == nil && grant.ScopeID == ""
	case ScopeTenant:
		return target.Reference.Boundary == resource.BoundaryTenant && sameTenant(grant.TenantID, target.Reference.TenantID)
	case ScopeOrganizationalUnit, ScopeResource:
		return target.Reference.Boundary == resource.BoundaryTenant && sameTenant(grant.TenantID, target.Reference.TenantID) &&
			grant.Scope == target.Scope && grant.ScopeID != "" && grant.ScopeID == target.Reference.ID
	default:
		return false
	}
}

func sameTenant(left, right *tenancy.TenantID) bool {
	return left != nil && right != nil && !left.IsZero() && *left == *right
}

func planeForActor(actor identity.AuthenticatedActor) identity.AccessPlane {
	switch actor.AccountClass {
	case identity.AccountClassWork:
		return identity.AccessPlaneWork
	case identity.AccountClassAdmin:
		return identity.AccessPlaneAdmin
	case identity.AccountClassService:
		return identity.AccessPlaneService
	case identity.AccountClassAgent:
		return identity.AccessPlaneService
	default:
		return ""
	}
}
