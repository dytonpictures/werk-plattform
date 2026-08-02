// Package cache defines replaceable, non-authoritative cache infrastructure.
package cache

import (
	"context"
	"time"
)

// Port stores opaque technical hints for a bounded lifetime. Callers must
// remain correct when entries are absent, evicted, stale, or the port fails.
type Port interface {
	Get(context.Context, string) ([]byte, bool, error)
	Set(context.Context, string, []byte, time.Duration) error
	Delete(context.Context, string) error
}

// CounterPort provides one atomic, expiring technical counter. It is used for
// distributed rate windows only; callers must retain a safe local fallback and
// must never treat the counter as business or security truth.
type CounterPort interface {
	Increment(context.Context, string, time.Duration) (uint64, error)
}
