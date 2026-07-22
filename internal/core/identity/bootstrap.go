package identity

import (
	"context"
	"errors"
	"strings"
)

var (
	ErrBootstrapSecretMissing = errors.New("bootstrap secret missing")
	ErrBootstrapAlreadyUsed   = errors.New("bootstrap already used")
)

// BootstrapAdminStore atomically creates the first admin and reports whether
// bootstrap was already consumed. Implementations must enforce this in the
// database (unique singleton/transaction), not only in process memory.
type BootstrapAdminStore interface {
	BootstrapAdmin(context.Context, string) error
}

// BootstrapAdmin consumes an explicitly supplied secret. There is no default
// credential and the secret is never returned or persisted by this contract.
func BootstrapAdmin(ctx context.Context, secret string, store BootstrapAdminStore) error {
	if strings.TrimSpace(secret) == "" || len(secret) < 16 || store == nil {
		return ErrBootstrapSecretMissing
	}
	if err := store.BootstrapAdmin(ctx, secret); err != nil {
		return err
	}
	return nil
}
