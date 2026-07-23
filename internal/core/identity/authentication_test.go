package identity

import (
	"context"
	"testing"
	"time"
)

type resolverStub struct {
	actor AuthenticatedActor
	err   error
}

func (a resolverStub) ResolveVerifiedIdentity(context.Context, VerifiedIdentity) (AuthenticatedActor, error) {
	return a.actor, a.err
}

type issuerStub struct{}

func (issuerStub) Issue(context.Context, AuthenticatedActor, time.Duration) (string, time.Time, error) {
	return "opaque", time.Now().Add(time.Hour), nil
}

func TestEstablishSessionResolvesCoreOwnedActorAndIssuesOpaqueSession(t *testing.T) {
	a := AuthenticatedActor{
		AccountID: AccountID{1}, AccountClass: AccountClassAdmin, Audience: AudienceAdmin,
		Kind: AuthenticationInteractive, Assurance: AssuranceMultiFactor,
	}
	proof := VerifiedIdentity{
		ProviderKey: "local", ProviderSubject: "subject-1", Method: AuthenticationMethodPassword,
		Assurance: AssuranceMultiFactor, AuthenticatedAt: time.Now().UTC(),
	}
	token, _, err := EstablishSession(context.Background(), proof, AudienceAdmin, resolverStub{actor: a}, issuerStub{}, time.Hour)
	if err != nil || token != "opaque" {
		t.Fatalf("establish session: token=%q err=%v", token, err)
	}
}

func TestEstablishSessionDoesNotRevealResolutionFailure(t *testing.T) {
	proof := VerifiedIdentity{
		ProviderKey: "local", ProviderSubject: "subject-1", Method: AuthenticationMethodPassword,
		Assurance: AssuranceSingleFactor, AuthenticatedAt: time.Now().UTC(),
	}
	_, _, err := EstablishSession(context.Background(), proof, AudienceWork, resolverStub{err: ErrAccountDisabled}, issuerStub{}, time.Hour)
	if err != ErrInvalidCredentials {
		t.Fatalf("got %v", err)
	}
}

func TestEstablishSessionRejectsProviderControlledAudienceMismatch(t *testing.T) {
	proof := VerifiedIdentity{
		ProviderKey: "local", ProviderSubject: "subject-1", Method: AuthenticationMethodPassword,
		Assurance: AssuranceMultiFactor, AuthenticatedAt: time.Now().UTC(),
	}
	actor := AuthenticatedActor{
		AccountID: AccountID{1}, AccountClass: AccountClassAdmin, Audience: AudienceAdmin,
		Kind: AuthenticationInteractive, Assurance: AssuranceMultiFactor,
	}
	_, _, err := EstablishSession(context.Background(), proof, AudienceWork, resolverStub{actor: actor}, issuerStub{}, time.Hour)
	if err != ErrInvalidCredentials {
		t.Fatalf("got %v", err)
	}
}

func TestEstablishSessionRejectsInvalidProof(t *testing.T) {
	_, _, err := EstablishSession(context.Background(), VerifiedIdentity{}, AudienceWork, resolverStub{}, issuerStub{}, time.Hour)
	if err != ErrInvalidLogin {
		t.Fatalf("got %v", err)
	}
}
