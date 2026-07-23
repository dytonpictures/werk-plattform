package identity

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/dytonpictures/werk/internal/core/tenancy"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrAccountDisabled    = errors.New("account disabled")
	ErrInvalidLogin       = errors.New("invalid login request")
)

type AuthenticationMethod string

const (
	AuthenticationMethodPassword     AuthenticationMethod = "password"
	AuthenticationMethodPasskey      AuthenticationMethod = "passkey"
	AuthenticationMethodAPIKey       AuthenticationMethod = "api-key"
	AuthenticationMethodOIDC         AuthenticationMethod = "oidc"
	AuthenticationMethodSAML         AuthenticationMethod = "saml"
	AuthenticationMethodLDAPPassword AuthenticationMethod = "ldap-password"
)

// LoginRequest contains only user-entered values. Tenant and access plane are
// selected by the server-side login surface, never trusted from the client.
type LoginRequest struct {
	LoginName string
	Password  string
}

// VerifiedIdentity is the maximum authority a credential/provider adapter may
// return. Account class, tenant, audience, roles and permissions are resolved
// exclusively by Core Identity after this proof has been validated.
type VerifiedIdentity struct {
	ProviderKey     string
	ProviderSubject string
	Method          AuthenticationMethod
	Assurance       AuthenticationAssurance
	AuthenticatedAt time.Time
}

func (proof VerifiedIdentity) Validate() error {
	if !stableIdentityKey(proof.ProviderKey) || strings.TrimSpace(proof.ProviderSubject) != proof.ProviderSubject ||
		proof.ProviderSubject == "" || len(proof.ProviderSubject) > 512 || !stableIdentityKey(string(proof.Method)) ||
		proof.AuthenticatedAt.IsZero() {
		return ErrInvalidCredentials
	}
	if proof.Assurance != AssuranceUnknown && proof.Assurance != AssuranceSingleFactor && proof.Assurance != AssuranceMultiFactor {
		return ErrInvalidCredentials
	}
	return nil
}

type VerifiedIdentityResolver interface {
	ResolveVerifiedIdentity(context.Context, VerifiedIdentity) (AuthenticatedActor, error)
}

type SessionIssuer interface {
	Issue(context.Context, AuthenticatedActor, time.Duration) (string, time.Time, error)
}

func ValidateLoginRequest(request LoginRequest) error {
	if strings.TrimSpace(request.LoginName) == "" || request.Password == "" || len(request.Password) > 1024 {
		return ErrInvalidLogin
	}
	return nil
}

// EstablishSession resolves a verified provider subject to a Core-owned actor
// and issues a session for the server-selected audience.
func EstablishSession(ctx context.Context, proof VerifiedIdentity, audience Audience, resolver VerifiedIdentityResolver, issuer SessionIssuer, ttl time.Duration) (string, time.Time, error) {
	if proof.Validate() != nil || audience == "" || resolver == nil || issuer == nil || ttl <= 0 {
		return "", time.Time{}, ErrInvalidLogin
	}
	actor, err := resolver.ResolveVerifiedIdentity(ctx, proof)
	if err != nil {
		return "", time.Time{}, ErrInvalidCredentials
	}
	if actor.Audience != audience || actor.Assurance != proof.Assurance || ValidateActorBoundary(actor) != nil {
		return "", time.Time{}, ErrInvalidCredentials
	}
	token, expires, err := issuer.Issue(ctx, actor, ttl)
	if err != nil || token == "" {
		return "", time.Time{}, ErrInvalidCredentials
	}
	return token, expires, nil
}

func stableIdentityKey(value string) bool {
	if value == "" {
		return false
	}
	for index, character := range value {
		if (character >= 'a' && character <= 'z') || (index > 0 && character >= '0' && character <= '9') ||
			(index > 0 && (character == '-' || character == '.')) {
			continue
		}
		return false
	}
	return true
}

// TenantForLogin is a helper for adapters resolving a work account's tenant.
func TenantForLogin(actor AuthenticatedActor) *tenancy.TenantID { return actor.TenantID }
