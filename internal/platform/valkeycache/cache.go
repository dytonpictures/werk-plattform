// Package valkeycache implements the neutral cache port with Valkey.
package valkeycache

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	valkey "github.com/valkey-io/valkey-go"
)

type Cache struct {
	mu               sync.Mutex
	option           valkey.ClientOption
	newClient        func(valkey.ClientOption) (valkey.Client, error)
	client           valkey.Client
	closed           bool
	unavailableUntil atomic.Int64
}

const failureCooldown = 5 * time.Second

var errTemporarilyUnavailable = errors.New("cache temporarily unavailable")

func New(rawURL string) (*Cache, error) {
	option, err := valkey.ParseURL(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse cache URL: %w", err)
	}
	option.Dialer.Timeout = 300 * time.Millisecond
	option.ConnWriteTimeout = 500 * time.Millisecond
	option.DisableRetry = true
	option.ClientName = "werk-api-cache"
	return &Cache{option: option, newClient: valkey.NewClient}, nil
}

func (cache *Cache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	client, err := cache.clientForUse()
	if err != nil {
		return nil, false, err
	}
	value, err := client.Do(ctx, client.B().Get().Key(key).Build()).ToString()
	if valkey.IsValkeyNil(err) {
		return nil, false, nil
	}
	if err != nil {
		cache.markUnavailable()
		return nil, false, err
	}
	return []byte(value), true, nil
}

func (cache *Cache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if ttl < time.Second {
		return errors.New("cache TTL must be at least one second")
	}
	client, err := cache.clientForUse()
	if err != nil {
		return err
	}
	err = client.Do(ctx, client.B().Set().Key(key).Value(string(value)).ExSeconds(int64(ttl/time.Second)).Build()).Error()
	if err != nil {
		cache.markUnavailable()
	}
	return err
}

func (cache *Cache) Delete(ctx context.Context, key string) error {
	client, err := cache.clientForUse()
	if err != nil {
		return err
	}
	err = client.Do(ctx, client.B().Del().Key(key).Build()).Error()
	if err != nil {
		cache.markUnavailable()
	}
	return err
}

func (cache *Cache) Increment(ctx context.Context, key string, ttl time.Duration) (uint64, error) {
	if key == "" || ttl < time.Second {
		return 0, errors.New("cache counter requires a key and TTL of at least one second")
	}
	client, err := cache.clientForUse()
	if err != nil {
		return 0, err
	}
	// Set the expiry only for the first increment so every process observes one
	// fixed window without extending it on subsequent attempts.
	const script = `local value=redis.call('INCR',KEYS[1]); if value==1 then redis.call('PEXPIRE',KEYS[1],ARGV[1]); end; return value`
	value, err := client.Do(ctx, client.B().Eval().Script(script).Numkeys(1).Key(key).Arg(fmt.Sprintf("%d", ttl.Milliseconds())).Build()).AsInt64()
	if err != nil || value < 1 {
		cache.markUnavailable()
		if err == nil {
			err = errors.New("cache counter returned an invalid value")
		}
		return 0, err
	}
	return uint64(value), nil
}

func (cache *Cache) clientForUse() (valkey.Client, error) {
	if cache.coolingDown() {
		return nil, errTemporarilyUnavailable
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.closed {
		return nil, errors.New("cache is closed")
	}
	if cache.client != nil {
		return cache.client, nil
	}
	client, err := cache.newClient(cache.option)
	if err != nil {
		cache.markUnavailable()
		return nil, fmt.Errorf("connect Valkey cache client: %w", err)
	}
	cache.client = client
	return client, nil
}

func (cache *Cache) coolingDown() bool {
	return time.Now().UnixNano() < cache.unavailableUntil.Load()
}

func (cache *Cache) markUnavailable() {
	cache.unavailableUntil.Store(time.Now().Add(failureCooldown).UnixNano())
}

func (cache *Cache) Close() {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.closed = true
	if cache.client != nil {
		cache.client.Close()
		cache.client = nil
	}
}
