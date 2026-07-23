package documents

import (
	"time"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/tenancy"
)

// AccessReason explains why a work account can see a document. It is a domain
// fact for authorized projections, not a replacement for policy evaluation.
type AccessReason string

const (
	AccessReasonCreatedByMe          AccessReason = "created-by-me"
	AccessReasonSharedDirectlyWithMe AccessReason = "shared-directly-with-me"
)

type GrantID [16]byte

func (id GrantID) IsZero() bool { return id == GrantID{} }

type GrantStatus string

const (
	GrantStatusActive  GrantStatus = "active"
	GrantStatusRevoked GrantStatus = "revoked"
)

// DirectGrant records visibility granted to one work account. Account class
// and tenant membership are resolved by Core Identity before this contract is
// called; AccountID deliberately does not duplicate those identity facts.
type DirectGrant struct {
	ID         GrantID
	TenantID   tenancy.TenantID
	DocumentID DocumentID
	GrantedTo  identity.AccountID
	GrantedBy  identity.AccountID
	Status     GrantStatus
	GrantedAt  time.Time
	RevokedBy  identity.AccountID
	RevokedAt  *time.Time
	Version    uint64
}

// CreateDirectGrant creates an active, document-bound grant. Only the document
// creator may share an active document, and never with their own account.
func CreateDirectGrant(document Document, tenantID tenancy.TenantID, grantedTo, actor identity.AccountID, grantedAt time.Time) (DirectGrant, error) {
	if document.Validate() != nil || document.Status != StatusActive || tenantID.IsZero() || tenantID != document.TenantID ||
		grantedTo.IsZero() || actor.IsZero() || actor != document.CreatedBy || grantedTo == actor || grantedAt.IsZero() ||
		grantedAt.Before(document.CreatedAt) {
		return DirectGrant{}, ErrInvalid
	}
	id, err := newIdentifier[GrantID]()
	if err != nil {
		return DirectGrant{}, err
	}
	grant := DirectGrant{
		ID: id, TenantID: tenantID, DocumentID: document.ID, GrantedTo: grantedTo,
		GrantedBy: actor, Status: GrantStatusActive, GrantedAt: grantedAt.UTC(), Version: 1,
	}
	if err := grant.Validate(document); err != nil {
		return DirectGrant{}, err
	}
	return grant, nil
}

// RevokeDirectGrant performs the grant's only state transition. Revocation is
// intentionally allowed for an archived document so access can still be
// removed, but it remains restricted to the document creator.
func RevokeDirectGrant(document Document, grant DirectGrant, tenantID tenancy.TenantID, actor identity.AccountID, revokedAt time.Time) (DirectGrant, error) {
	if grant.Validate(document) != nil || tenantID.IsZero() || tenantID != document.TenantID || grant.TenantID != tenantID ||
		actor.IsZero() || actor != document.CreatedBy || grant.Status != GrantStatusActive || revokedAt.IsZero() ||
		revokedAt.Before(grant.GrantedAt) || grant.Version == ^uint64(0) {
		return DirectGrant{}, ErrInvalid
	}
	now := revokedAt.UTC()
	grant.Status = GrantStatusRevoked
	grant.RevokedBy = actor
	grant.RevokedAt = &now
	grant.Version++
	if err := grant.Validate(document); err != nil {
		return DirectGrant{}, err
	}
	return grant, nil
}

// Validate rejects grants detached from their authoritative tenant or
// document. A caller must supply the loaded document rather than trusting only
// the coordinates stored on the grant.
func (grant DirectGrant) Validate(document Document) error {
	if document.Validate() != nil || grant.ID.IsZero() || grant.TenantID.IsZero() || grant.TenantID != document.TenantID ||
		grant.DocumentID.IsZero() || grant.DocumentID != document.ID || grant.GrantedTo.IsZero() || grant.GrantedBy.IsZero() ||
		grant.GrantedBy != document.CreatedBy || grant.GrantedTo == document.CreatedBy || grant.GrantedAt.IsZero() ||
		grant.GrantedAt.Location() != time.UTC || grant.GrantedAt.Before(document.CreatedAt) || grant.Version == 0 {
		return ErrInvalid
	}
	switch grant.Status {
	case GrantStatusActive:
		if grant.Version != 1 || !grant.RevokedBy.IsZero() || grant.RevokedAt != nil {
			return ErrInvalid
		}
	case GrantStatusRevoked:
		if grant.Version != 2 || grant.RevokedBy != document.CreatedBy || grant.RevokedAt == nil || grant.RevokedAt.IsZero() ||
			grant.RevokedAt.Location() != time.UTC || grant.RevokedAt.Before(grant.GrantedAt) {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}
