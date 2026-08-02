package identitystore

import (
	"context"
	"time"
)

const (
	maximumConcurrentInvitationPasswordHashes = 2
	invitationPasswordHashQueueTimeout        = 2 * time.Second
)

// boundedWorkLimiter prevents a public ceremony from starting an unbounded
// number of memory-hard password hashes in one API process. It is deliberately
// process-wide; constructing additional identity Service values must not open
// additional Argon2 capacity.
type boundedWorkLimiter struct {
	slots chan struct{}
	wait  time.Duration
}

var invitationPasswordHashLimiter = newBoundedWorkLimiter(
	maximumConcurrentInvitationPasswordHashes,
	invitationPasswordHashQueueTimeout,
)

func newBoundedWorkLimiter(parallelism int, wait time.Duration) *boundedWorkLimiter {
	return &boundedWorkLimiter{slots: make(chan struct{}, parallelism), wait: wait}
}

func (limiter *boundedWorkLimiter) acquire(ctx context.Context) (func(), bool) {
	timer := time.NewTimer(limiter.wait)
	defer timer.Stop()
	select {
	case limiter.slots <- struct{}{}:
		return func() { <-limiter.slots }, true
	case <-ctx.Done():
		return nil, false
	case <-timer.C:
		return nil, false
	}
}
