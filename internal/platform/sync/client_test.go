package platformsync

import (
	"context"
	"errors"
	"testing"
	"time"
)

type authoritySnapshotSourceStub struct {
	snapshot AuthoritySnapshot
	err      error
	calls    int
}

func (source *authoritySnapshotSourceStub) AuthoritySnapshot(context.Context) (AuthoritySnapshot, error) {
	source.calls++
	return source.snapshot, source.err
}

func TestClientEvaluatesWithTrustedSnapshot(t *testing.T) {
	at := time.Date(2026, 7, 22, 17, 0, 0, 0, time.UTC)
	source := &authoritySnapshotSourceStub{snapshot: validAuthority(at, CoordinationSharedDatabase)}
	client, err := newClient(source, func() time.Time { return at })
	if err != nil {
		t.Fatal(err)
	}
	decision, err := client.Evaluate(context.Background(), validPolicyRequest(at))
	if err != nil || !decision.Allowed() || source.calls != 1 {
		t.Fatalf("decision = %#v, error = %v, calls = %d", decision, err, source.calls)
	}
}

func TestClientFailsClosedWhenSnapshotIsUnavailable(t *testing.T) {
	at := time.Date(2026, 7, 22, 17, 0, 0, 0, time.UTC)
	sourceError := errors.New("source offline")
	source := &authoritySnapshotSourceStub{err: sourceError}
	client, err := newClient(source, func() time.Time { return at })
	if err != nil {
		t.Fatal(err)
	}
	decision, err := client.Evaluate(context.Background(), validPolicyRequest(at))
	if decision.Allowed() || decision.Reason != ReasonAuthorityUnavailable ||
		!errors.Is(err, ErrAuthoritySnapshotUnavailable) || !errors.Is(err, sourceError) {
		t.Fatalf("decision = %#v, error = %v", decision, err)
	}
}

func TestClientDoesNotResolveSnapshotForCancelledRequest(t *testing.T) {
	at := time.Date(2026, 7, 22, 17, 0, 0, 0, time.UTC)
	source := &authoritySnapshotSourceStub{snapshot: validAuthority(at, CoordinationSharedDatabase)}
	client, err := newClient(source, func() time.Time { return at })
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	decision, err := client.Evaluate(ctx, validPolicyRequest(at))
	if decision.Allowed() || decision.Reason != ReasonAuthorityUnavailable ||
		!errors.Is(err, context.Canceled) || source.calls != 0 {
		t.Fatalf("decision = %#v, error = %v, calls = %d", decision, err, source.calls)
	}
}

func TestNewClientRequiresTrustedSource(t *testing.T) {
	if _, err := NewClient(nil); !errors.Is(err, ErrInvalidClient) {
		t.Fatalf("error = %v", err)
	}
}
