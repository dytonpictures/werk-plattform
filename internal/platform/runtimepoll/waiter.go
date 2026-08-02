package runtimepoll

import (
	"context"
	"time"
)

// Waiter owns one reusable timer and a bounded idle backoff. Its stable jitter
// spreads slots and processes without global randomness or per-wait allocation.
type Waiter struct {
	timer   *time.Timer
	base    time.Duration
	maximum time.Duration
	current time.Duration
	jitter  int64
}

func NewWaiter(base, maximum time.Duration, identity string) *Waiter {
	if base <= 0 || maximum < base {
		panic("invalid runtime poll interval")
	}
	timer := time.NewTimer(time.Hour)
	if !timer.Stop() {
		<-timer.C
	}
	return &Waiter{
		timer: timer, base: base, maximum: maximum, current: base,
		jitter: stableJitter(identity),
	}
}

// Wait pauses after a claim attempt. Idle waits back off exponentially;
// dependency failures retain the short base retry. False means the context
// ended and the caller must stop.
func (waiter *Waiter) Wait(ctx context.Context, idle bool, wakeups <-chan struct{}) bool {
	delay := waiter.base
	if idle {
		delay = waiter.current
		waiter.current = min(waiter.current*2, waiter.maximum)
	} else {
		waiter.current = waiter.base
	}
	delay += time.Duration(int64(delay) * waiter.jitter / 1000)
	waiter.timer.Reset(delay)
	select {
	case <-ctx.Done():
		if !waiter.timer.Stop() {
			select {
			case <-waiter.timer.C:
			default:
			}
		}
		return false
	case <-wakeups:
		if !waiter.timer.Stop() {
			select {
			case <-waiter.timer.C:
			default:
			}
		}
		return true
	case <-waiter.timer.C:
		return true
	}
}

func (waiter *Waiter) Reset() { waiter.current = waiter.base }

func (waiter *Waiter) Close() { waiter.timer.Stop() }

func stableJitter(value string) int64 {
	const (
		offset = uint64(14695981039346656037)
		prime  = uint64(1099511628211)
	)
	hash := offset
	for index := 0; index < len(value); index++ {
		hash ^= uint64(value[index])
		hash *= prime
	}
	return int64(hash%201) - 100
}
