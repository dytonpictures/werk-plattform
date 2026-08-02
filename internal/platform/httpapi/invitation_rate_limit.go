package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"

	corecache "github.com/dytonpictures/werk/internal/core/cache"
)

const (
	invitationActivationAttemptsPerWindow = 12
	invitationActivationWindow            = time.Minute
	invitationActivationMaximumSources    = 4096
)

type sourceWindow struct {
	startedAt time.Time
	attempts  int
}

// sourceWindowLimiter is a bounded, process-local first line of defence for
// the public activation ceremony. It intentionally uses the direct peer
// address: an untrusted Forwarded/X-Forwarded-For header must never create
// attacker-selected buckets. Deployments behind a proxy therefore get the
// safer, stricter shared proxy bucket until an authenticated edge policy is
// configured outside this handler.
type sourceWindowLimiter struct {
	mu             sync.Mutex
	limit          int
	window         time.Duration
	maximumSources int
	sources        map[string]sourceWindow
	overflow       sourceWindow
	nextSweepAt    time.Time
	counter        corecache.CounterPort
	counterPrefix  string
}

func (limiter *sourceWindowLimiter) withCounter(counter corecache.CounterPort, prefix string) *sourceWindowLimiter {
	limiter.counter = counter
	limiter.counterPrefix = strings.TrimSpace(prefix)
	return limiter
}

func newSourceWindowLimiter(limit int, window time.Duration, maximumSources int) *sourceWindowLimiter {
	return &sourceWindowLimiter{
		limit: limit, window: window, maximumSources: maximumSources,
		sources: make(map[string]sourceWindow),
	}
}

func (limiter *sourceWindowLimiter) allow(remoteAddress string, now time.Time) bool {
	return limiter.allowContext(context.Background(), remoteAddress, now)
}

func (limiter *sourceWindowLimiter) allowContext(ctx context.Context, remoteAddress string, now time.Time) bool {
	key := directPeerKey(remoteAddress)
	if limiter.counter != nil && limiter.counterPrefix != "" {
		digest := sha256.Sum256([]byte(key))
		counterKey := limiter.counterPrefix + ":" + base64.RawURLEncoding.EncodeToString(digest[:])
		if value, err := limiter.counter.Increment(ctx, counterKey, limiter.window); err == nil {
			return value <= uint64(limiter.limit)
		}
	}
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	if entry, exists := limiter.sources[key]; exists {
		allowed, updated := allowSourceWindow(entry, now, limiter.window, limiter.limit)
		limiter.sources[key] = updated
		return allowed
	}
	if len(limiter.sources) >= limiter.maximumSources &&
		(limiter.nextSweepAt.IsZero() || !now.Before(limiter.nextSweepAt)) {
		for existingKey, entry := range limiter.sources {
			if !now.Before(entry.startedAt.Add(limiter.window)) {
				delete(limiter.sources, existingKey)
			}
		}
		limiter.nextSweepAt = now.Add(limiter.window)
	}
	if len(limiter.sources) >= limiter.maximumSources {
		allowed, updated := allowSourceWindow(limiter.overflow, now, limiter.window, limiter.limit)
		limiter.overflow = updated
		return allowed
	}
	allowed, entry := allowSourceWindow(sourceWindow{}, now, limiter.window, limiter.limit)
	limiter.sources[key] = entry
	return allowed
}

func allowSourceWindow(entry sourceWindow, now time.Time, window time.Duration, limit int) (bool, sourceWindow) {
	if entry.startedAt.IsZero() || !now.Before(entry.startedAt.Add(window)) {
		entry = sourceWindow{startedAt: now}
	}
	if entry.attempts >= limit {
		return false, entry
	}
	entry.attempts++
	return true, entry
}

func directPeerKey(remoteAddress string) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddress))
	if err != nil {
		host = strings.TrimSpace(remoteAddress)
	}
	address, err := netip.ParseAddr(host)
	if err != nil {
		return "unknown-peer"
	}
	return address.Unmap().String()
}
