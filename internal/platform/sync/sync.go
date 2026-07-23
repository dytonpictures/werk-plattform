// Package platformsync guards policy evaluation against stale platform and
// authority state. It reports liveness without deriving authority from it and
// does not implement database replication, consensus, witness lease
// acquisition, or document synchronization.
package platformsync

import (
	"regexp"
	"time"

	"github.com/dytonpictures/werk/internal/core/authorization"
)

var stableAuthorityIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,127}$`)

type AuthorityCoordination string

const (
	CoordinationLocal           AuthorityCoordination = "local"
	CoordinationSharedDatabase  AuthorityCoordination = "shared-database"
	CoordinationPlatformWitness AuthorityCoordination = "platform-witness"
)

type AuthorityDomain string

const DomainIdentityControl AuthorityDomain = "identity-control"

// AuthoritySnapshot is resolved by trusted local platform infrastructure. For
// shared-database deployments, PolicyRevision must come from the same
// authoritative PostgreSQL transaction as the policy inputs. Request payloads
// cannot construct or widen this snapshot.
type AuthoritySnapshot struct {
	InstanceID           string
	RealmID              string
	Domain               AuthorityDomain
	Coordination         AuthorityCoordination
	AuthorityGeneration  uint64
	PolicyRevision       uint64
	ObservedAt           time.Time
	Fenced               bool
	LeaseHeld            bool
	LeaseExpiresAt       *time.Time
	FencingTokenVerified bool
}

// PolicyRequest is an internal freshness envelope around the canonical Core
// authorization request. It is not a bearer token and makes no availability or
// consensus guarantee. Any transport carrying it must authenticate the caller
// independently.
type PolicyRequest struct {
	ID                          [16]byte
	CorrelationID               [16]byte
	ExpectedInstanceID          string
	ExpectedRealmID             string
	ExpectedAuthorityDomain     AuthorityDomain
	ExpectedAuthorityGeneration uint64
	ExpectedPolicyRevision      uint64
	RequestedAt                 time.Time
	ExpiresAt                   time.Time
	Authorization               authorization.PolicyRequest
}

type DecisionEffect string

const (
	DecisionAllow DecisionEffect = "allow"
	DecisionDeny  DecisionEffect = "deny"
)

type DecisionReason string

const (
	ReasonAuthorized              DecisionReason = "authorized"
	ReasonInvalidRequest          DecisionReason = "invalid-request"
	ReasonRequestExpired          DecisionReason = "request-expired"
	ReasonInstanceMismatch        DecisionReason = "instance-mismatch"
	ReasonRealmMismatch           DecisionReason = "realm-mismatch"
	ReasonAuthorityDomainMismatch DecisionReason = "authority-domain-mismatch"
	ReasonAuthorityFenced         DecisionReason = "authority-fenced"
	ReasonAuthorityGenerationOld  DecisionReason = "authority-generation-stale"
	ReasonPolicyRevisionOld       DecisionReason = "policy-revision-stale"
	ReasonAuthorityUnavailable    DecisionReason = "authority-unavailable"
	ReasonAuthorizationDenied     DecisionReason = "authorization-denied"
)

type Decision struct {
	Effect              DecisionEffect
	Reason              DecisionReason
	Authorization       authorization.Decision
	AuthorityDomain     AuthorityDomain
	AuthorityGeneration uint64
	PolicyRevision      uint64
}

func (decision Decision) Allowed() bool { return decision.Effect == DecisionAllow }

// Evaluate applies the instance/authority guard before the canonical
// authorization evaluator. evaluatedAt must be supplied by trusted server
// time; the time embedded in the nested request is deliberately overwritten.
func Evaluate(request PolicyRequest, authority AuthoritySnapshot, evaluatedAt time.Time) Decision {
	denied := func(reason DecisionReason) Decision {
		return Decision{
			Effect: DecisionDeny, Reason: reason,
			AuthorityDomain:     authority.Domain,
			AuthorityGeneration: authority.AuthorityGeneration,
			PolicyRevision:      authority.PolicyRevision,
		}
	}

	if !validRequest(request, evaluatedAt) || !validAuthoritySnapshot(authority, evaluatedAt) {
		return denied(ReasonInvalidRequest)
	}
	if !evaluatedAt.Before(request.ExpiresAt) {
		return denied(ReasonRequestExpired)
	}
	if authority.Fenced {
		return denied(ReasonAuthorityFenced)
	}
	if request.ExpectedInstanceID != authority.InstanceID {
		return denied(ReasonInstanceMismatch)
	}
	if request.ExpectedRealmID != authority.RealmID {
		return denied(ReasonRealmMismatch)
	}
	if request.ExpectedAuthorityDomain != authority.Domain {
		return denied(ReasonAuthorityDomainMismatch)
	}
	if request.ExpectedAuthorityGeneration != authority.AuthorityGeneration {
		return denied(ReasonAuthorityGenerationOld)
	}
	if request.ExpectedPolicyRevision != authority.PolicyRevision {
		return denied(ReasonPolicyRevisionOld)
	}
	if authority.Coordination == CoordinationPlatformWitness &&
		(!authority.LeaseHeld || !authority.FencingTokenVerified ||
			authority.LeaseExpiresAt == nil || !evaluatedAt.Before(*authority.LeaseExpiresAt)) {
		return denied(ReasonAuthorityUnavailable)
	}

	authorizationRequest := request.Authorization
	authorizationRequest.EvaluatedAt = evaluatedAt.UTC()
	authorizationDecision := authorization.Evaluate(authorizationRequest)
	if !authorizationDecision.Allowed() {
		decision := denied(ReasonAuthorizationDenied)
		decision.Authorization = authorizationDecision
		return decision
	}
	return Decision{
		Effect: DecisionAllow, Reason: ReasonAuthorized, Authorization: authorizationDecision,
		AuthorityDomain:     authority.Domain,
		AuthorityGeneration: authority.AuthorityGeneration,
		PolicyRevision:      authority.PolicyRevision,
	}
}

func validRequest(request PolicyRequest, evaluatedAt time.Time) bool {
	return request.ID != [16]byte{} && request.CorrelationID != [16]byte{} &&
		stableAuthorityIDPattern.MatchString(request.ExpectedInstanceID) &&
		stableAuthorityIDPattern.MatchString(request.ExpectedRealmID) &&
		validAuthorityDomain(request.ExpectedAuthorityDomain) &&
		request.ExpectedAuthorityGeneration > 0 && request.ExpectedPolicyRevision > 0 &&
		!request.RequestedAt.IsZero() && !request.ExpiresAt.IsZero() && !evaluatedAt.IsZero() &&
		request.ExpiresAt.After(request.RequestedAt) && !evaluatedAt.Before(request.RequestedAt) &&
		!request.Authorization.Actor.AccountID.IsZero()
}

func validAuthoritySnapshot(authority AuthoritySnapshot, evaluatedAt time.Time) bool {
	if !stableAuthorityIDPattern.MatchString(authority.InstanceID) ||
		!stableAuthorityIDPattern.MatchString(authority.RealmID) ||
		!validAuthorityDomain(authority.Domain) ||
		authority.AuthorityGeneration == 0 || authority.PolicyRevision == 0 ||
		authority.ObservedAt.IsZero() || authority.ObservedAt.After(evaluatedAt) {
		return false
	}
	if !validAuthorityCoordination(authority.Coordination) {
		return false
	}
	switch authority.Coordination {
	case CoordinationLocal, CoordinationSharedDatabase:
		return !authority.LeaseHeld && authority.LeaseExpiresAt == nil && !authority.FencingTokenVerified
	case CoordinationPlatformWitness:
		return true
	}
	return false
}

func validAuthorityDomain(domain AuthorityDomain) bool {
	return domain == DomainIdentityControl
}
