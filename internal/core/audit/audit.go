// Package audit defines the provider-independent contract for protected
// business activity. Identity security events keep their narrower identity
// contract; business modules depend on this package instead of identity audit.
package audit

import (
	"errors"
	"strings"
	"time"

	"github.com/dytonpictures/werk/internal/core/compliance"
	"github.com/dytonpictures/werk/internal/core/events"
	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/resource"
	"github.com/dytonpictures/werk/internal/core/tenancy"
)

var ErrInvalidEntry = errors.New("invalid business audit entry")

type Outcome string

const (
	OutcomeSucceeded Outcome = "succeeded"
	OutcomeDenied    Outcome = "denied"
	OutcomeFailed    Outcome = "failed"
)

// ActorRef preserves who initiated an operation separately from the technical
// principal that executed it. Account classes are immutable identity data and
// are verified again by the persistent store.
type ActorRef struct {
	AccountID    identity.AccountID
	AccountClass identity.AccountClass
	TenantID     tenancy.TenantID
}

// PolicySnapshot records the server-resolved policy and processing context
// used for the decision. Client input must never establish these fields.
type PolicySnapshot struct {
	Permission         string
	ContractVersion    uint64
	ProcessingRequired bool
	Processing         compliance.ProcessingContext
}

// Entry is the tenant-bound, append-only business audit contract. RequestID is
// required in this first slice because only request-driven service operations
// may produce entries. Background jobs need an explicit operation-ID extension
// before they become producers.
type Entry struct {
	ID            [16]byte
	TenantID      tenancy.TenantID
	OccurredAt    time.Time
	EventType     string
	Action        string
	Outcome       Outcome
	InitiatedBy   ActorRef
	ExecutedBy    ActorRef
	Subject       resource.Ref
	Policy        PolicySnapshot
	RequestID     [16]byte
	CorrelationID [16]byte
}

func (entry Entry) Validate() error {
	if entry.ID == [16]byte{} || entry.TenantID.IsZero() || entry.OccurredAt.IsZero() ||
		entry.RequestID == [16]byte{} || entry.CorrelationID == [16]byte{} {
		return ErrInvalidEntry
	}
	if !events.ValidEventType(entry.EventType) || !strings.HasPrefix(entry.EventType, "core.") ||
		!resource.ValidKey(entry.Action) || !sameModuleNamespace(entry.EventType, entry.Action) ||
		!validOutcome(entry.Outcome) {
		return ErrInvalidEntry
	}
	if entry.Subject.Validate() != nil || entry.Subject.Boundary != resource.BoundaryTenant ||
		entry.Subject.TenantID == nil || *entry.Subject.TenantID != entry.TenantID {
		return ErrInvalidEntry
	}
	if entry.InitiatedBy.validate() != nil || entry.ExecutedBy.validate() != nil ||
		entry.InitiatedBy.TenantID != entry.TenantID || entry.ExecutedBy.TenantID != entry.TenantID {
		return ErrInvalidEntry
	}
	if entry.ExecutedBy.AccountClass != identity.AccountClassService &&
		entry.ExecutedBy.AccountClass != identity.AccountClassAgent {
		return ErrInvalidEntry
	}
	if entry.Policy.Validate(entry.Subject.Kind) != nil {
		return ErrInvalidEntry
	}
	return nil
}

func sameModuleNamespace(eventType, action string) bool {
	eventParts := strings.Split(eventType, ".")
	actionParts := strings.Split(action, ".")
	return len(eventParts) >= 3 && len(actionParts) >= 3 &&
		eventParts[0] == actionParts[0] && eventParts[1] == actionParts[1]
}

func (actor ActorRef) validate() error {
	if actor.AccountID.IsZero() || actor.TenantID.IsZero() {
		return ErrInvalidEntry
	}
	switch actor.AccountClass {
	case identity.AccountClassWork, identity.AccountClassService, identity.AccountClassAgent:
		return nil
	default:
		return ErrInvalidEntry
	}
}

func (snapshot PolicySnapshot) Validate(subjectKind resource.Kind) error {
	policy := compliance.ProcessingPolicy{
		Permission:   snapshot.Permission,
		ResourceKind: subjectKind,
		Required:     snapshot.ProcessingRequired,
		Context:      snapshot.Processing,
		Status:       resource.RegistrationActive,
		Version:      snapshot.ContractVersion,
	}
	if policy.Validate() != nil {
		return ErrInvalidEntry
	}
	return nil
}

func validOutcome(outcome Outcome) bool {
	return outcome == OutcomeSucceeded || outcome == OutcomeDenied || outcome == OutcomeFailed
}
