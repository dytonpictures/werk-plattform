package identity

import (
	"context"
	"testing"
)

type bootstrapStore struct{ calls int }

func (s *bootstrapStore) BootstrapAdmin(context.Context, string) error {
	s.calls++
	if s.calls > 1 {
		return ErrBootstrapAlreadyUsed
	}
	return nil
}

func TestBootstrapRequiresExplicitStrongSecret(t *testing.T) {
	if err := BootstrapAdmin(context.Background(), "", &bootstrapStore{}); err != ErrBootstrapSecretMissing {
		t.Fatalf("got %v", err)
	}
	if err := BootstrapAdmin(context.Background(), "short", &bootstrapStore{}); err != ErrBootstrapSecretMissing {
		t.Fatalf("got %v", err)
	}
}

func TestBootstrapIsOneShotAtStoreBoundary(t *testing.T) {
	s := &bootstrapStore{}
	secret := "explicit-bootstrap-secret"
	if err := BootstrapAdmin(context.Background(), secret, s); err != nil {
		t.Fatal(err)
	}
	if err := BootstrapAdmin(context.Background(), secret, s); err != ErrBootstrapAlreadyUsed {
		t.Fatalf("got %v", err)
	}
}
