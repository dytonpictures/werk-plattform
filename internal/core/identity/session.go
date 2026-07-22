package identity

import (
	"errors"
	"time"

	"github.com/dytonpictures/werk/internal/core/tenancy"
)

var (
	ErrSessionInvalid = errors.New("invalid session")
	ErrSessionExpired = errors.New("session expired")
)

type SessionID [16]byte

func (id SessionID) IsZero() bool { return id == SessionID{} }

// SessionRecord is the server-side result of looking up a hashed session token.
// The token itself is never retained in this structure or returned to clients.
type SessionRecord struct {
	ID        SessionID
	Account   AuthenticatedActor
	Audience  Audience
	TenantID  *tenancy.TenantID
	ExpiresAt time.Time
	RevokedAt *time.Time
}

// ValidateSessionRecord checks lifecycle and the persisted identity boundary
// without authorizing an API plane. This permits tightly scoped password-change
// and MFA-enrollment flows to inspect a valid single-factor admin session.
func ValidateSessionRecord(record SessionRecord, now time.Time) (AuthenticatedActor, error) {
	if record.ID.IsZero() || record.Account.AccountID.IsZero() || record.ExpiresAt.IsZero() {
		return AuthenticatedActor{}, ErrSessionInvalid
	}
	if record.RevokedAt != nil || !now.Before(record.ExpiresAt) {
		return AuthenticatedActor{}, ErrSessionExpired
	}
	if record.Account.Audience != record.Audience || !sameTenant(record.Account.TenantID, record.TenantID) {
		return AuthenticatedActor{}, ErrSessionInvalid
	}
	if ValidateActorBoundary(record.Account) != nil {
		return AuthenticatedActor{}, ErrSessionInvalid
	}
	return record.Account, nil
}

// ResolveSession applies lifecycle and audience checks after a token lookup.
// It deliberately accepts the expected access plane from the route, not from
// request data, so a valid session cannot be replayed against another API.
func ResolveSession(record SessionRecord, expected AccessPlane, now time.Time) (AuthenticatedActor, error) {
	actor, err := ValidateSessionRecord(record, now)
	if err != nil {
		return AuthenticatedActor{}, err
	}
	if err := AuthorizeAccessPlane(actor, expected); err != nil {
		return AuthenticatedActor{}, ErrAccessDenied
	}
	return actor, nil
}

func sameTenant(left, right *tenancy.TenantID) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
