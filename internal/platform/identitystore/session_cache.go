package identitystore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"time"
)

const invalidSessionCacheTTL = 15 * time.Second

var invalidSessionCacheMarker = []byte("invalid:v1")

func invalidSessionCacheKey(tokenHash []byte) string {
	if len(tokenHash) != sha256.Size {
		return ""
	}
	return "werk:v1:identity:invalid-session:" + base64.RawURLEncoding.EncodeToString(tokenHash)
}

func (service *Service) sessionKnownInvalid(ctx context.Context, tokenHash []byte) bool {
	key := invalidSessionCacheKey(tokenHash)
	if service.cache == nil || key == "" {
		return false
	}
	value, found, err := service.cache.Get(ctx, key)
	return err == nil && found && bytes.Equal(value, invalidSessionCacheMarker)
}

func (service *Service) markSessionInvalid(ctx context.Context, tokenHash []byte) {
	key := invalidSessionCacheKey(tokenHash)
	if service.cache != nil && key != "" {
		_ = service.cache.Set(ctx, key, invalidSessionCacheMarker, invalidSessionCacheTTL)
	}
}
