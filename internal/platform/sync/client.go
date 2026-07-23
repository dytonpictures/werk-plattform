package platformsync

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrInvalidClient                = errors.New("invalid platform sync client")
	ErrAuthoritySnapshotUnavailable = errors.New("authority snapshot unavailable")
)

// AuthoritySnapshotSource resolves security state from trusted local platform
// infrastructure. A network implementation is intentionally not provided: it
// would first require mutual authentication, replay protection, and a separate
// witness transport contract.
type AuthoritySnapshotSource interface {
	AuthoritySnapshot(context.Context) (AuthoritySnapshot, error)
}

// Client combines a trusted authority snapshot with the pure fail-closed policy
// evaluator. It is not a credential and does not acquire leases or promote an
// instance.
type Client struct {
	source AuthoritySnapshotSource
	now    func() time.Time
}

func NewClient(source AuthoritySnapshotSource) (*Client, error) {
	return newClient(source, time.Now)
}

func newClient(source AuthoritySnapshotSource, now func() time.Time) (*Client, error) {
	if source == nil || now == nil {
		return nil, ErrInvalidClient
	}
	return &Client{source: source, now: now}, nil
}

// Evaluate always returns a deny decision when the trusted snapshot cannot be
// resolved. The accompanying error exists for audit and operational handling;
// callers must never interpret it as permission to bypass the decision.
func (client *Client) Evaluate(ctx context.Context, request PolicyRequest) (Decision, error) {
	if client == nil || client.source == nil || client.now == nil {
		return unavailableDecision(), ErrInvalidClient
	}
	if err := ctx.Err(); err != nil {
		return unavailableDecision(), err
	}
	authority, err := client.source.AuthoritySnapshot(ctx)
	if err != nil {
		return unavailableDecision(), fmt.Errorf("%w: %w", ErrAuthoritySnapshotUnavailable, err)
	}
	if err := ctx.Err(); err != nil {
		return unavailableDecision(), err
	}
	return Evaluate(request, authority, client.now().UTC()), nil
}

func unavailableDecision() Decision {
	return Decision{Effect: DecisionDeny, Reason: ReasonAuthorityUnavailable}
}
