// Package oidc implements the narrowly scoped OpenID Connect relying-party
// adapter for Core Identity. It verifies a provider proof but deliberately
// does not resolve or create an account, choose a provider, issue a session,
// or derive WERK authority from claims.
package oidc

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	oidclib "github.com/coreos/go-oidc/v3/oidc"
	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/providerregistry"
	"github.com/dytonpictures/werk/internal/platform/providertransport"
	"golang.org/x/oauth2"
)

var (
	// ErrLoginUnavailable is intentionally general. Configuration, discovery,
	// transport, persistence, and secret-provider details must not cross the
	// external login boundary.
	ErrLoginUnavailable = errors.New("oidc login unavailable")
	// ErrAuthenticationFailed is returned for every rejected callback. It must
	// not reveal whether state, provider proof, or an account binding existed.
	ErrAuthenticationFailed = errors.New("oidc authentication failed")
)

const (
	defaultCeremonyLifetime = 5 * time.Minute
	maximumValueLength      = 2048
	maximumCodeLength       = 8192
	maximumIDTokenLength    = 64 << 10
)

// ClientAuthentication selects an exact token-endpoint authentication method.
// It contains no secret material.
type ClientAuthentication string

const (
	ClientAuthenticationNone        ClientAuthentication = "none"
	ClientAuthenticationSecretBasic ClientAuthentication = "client-secret-basic"
)

// Configuration is one immutable, Identity-owned OIDC client revision. A
// client secret is intentionally absent and can only be used through
// ClientSecretPort during the token exchange.
type Configuration struct {
	Revision             uint64
	IdentityProviderKey  string
	IssuerURL            string
	ClientID             string
	RedirectURI          string
	Audience             identity.Audience
	ClientAuthentication ClientAuthentication
}

// AuthorizationRequest contains the only browser-facing result of Begin.
// State remains inside the URL and is persisted only as a digest.
type AuthorizationRequest struct {
	URL string
}

// Callback contains only the parameters needed from the redirect endpoint.
// The caller must reject duplicate HTTP parameters before constructing it.
type Callback struct {
	State string
	Code  string
	Error string
}

// Ceremony contains the sensitive, short-lived server-side state needed to
// complete one authorization request. Stores must encrypt it as appropriate
// for their persistence boundary and must never expose it to a browser.
type Ceremony struct {
	StateDigest           [sha256.Size]byte
	Nonce                 string
	PKCEVerifier          string
	ProviderID            providerregistry.ProviderID
	ProviderKey           string
	ProviderRevision      uint64
	BindingRevision       uint64
	ConfigurationRevision uint64
	IdentityProviderKey   string
	IssuerURL             string
	ClientID              string
	RedirectURI           string
	Audience              identity.Audience
	ClientAuthentication  ClientAuthentication
	StartedAt             time.Time
	ExpiresAt             time.Time
}

// ConsumeRequest identifies one exact, provider- and redirect-bound ceremony.
// The store must atomically return and delete at most one record. A failed or
// concurrent second Consume must never return the ceremony again.
type ConsumeRequest struct {
	StateDigest [sha256.Size]byte
	ProviderID  providerregistry.ProviderID
	RedirectURI string
}

// CeremonyPort owns persistence and one-time state consumption for Core
// Identity. Save must fail on a digest collision; Consume must be atomic.
type CeremonyPort interface {
	Save(context.Context, Ceremony) error
	Consume(context.Context, ConsumeRequest) (Ceremony, error)
}

// ClientSecretRequest selects secret material without carrying it.
type ClientSecretRequest struct {
	ProviderID            providerregistry.ProviderID
	ConfigurationRevision uint64
}

// ClientSecretPort lends a secret only for the duration of one callback. The
// implementation retains ownership and should clear mutable material after
// use. The secret is never stored in Configuration or returned by the adapter.
type ClientSecretPort interface {
	UseClientSecret(context.Context, ClientSecretRequest, func([]byte) error) error
}

// Adapter performs only OIDC protocol verification for an explicitly resolved
// provider. It has no account store or automatic provider selection facility.
type Adapter struct {
	transport  *providertransport.Client
	ceremonies CeremonyPort
	secrets    ClientSecretPort
	httpClient func() *http.Client
	now        func() time.Time
}

// NewAdapter constructs a fail-closed OIDC adapter. secrets may be nil only
// for configurations using ClientAuthenticationNone.
func NewAdapter(
	transport *providertransport.Client,
	ceremonies CeremonyPort,
	secrets ClientSecretPort,
) (*Adapter, error) {
	if transport == nil || transport.HTTPClient() == nil || ceremonies == nil {
		return nil, ErrLoginUnavailable
	}
	return &Adapter{
		transport: transport, ceremonies: ceremonies, secrets: secrets,
		httpClient: transport.HTTPClient, now: time.Now,
	}, nil
}

// Begin discovers the exact issuer and starts one Authorization Code + PKCE
// S256 ceremony for the caller-supplied registry resolution.
func (adapter *Adapter) Begin(
	ctx context.Context,
	resolution providerregistry.Resolution,
	configuration Configuration,
) (AuthorizationRequest, error) {
	if adapter == nil || validateResolution(resolution) != nil || adapter.validateConfiguration(configuration) != nil {
		return AuthorizationRequest{}, ErrLoginUnavailable
	}
	providerContext, err := adapter.bindHTTPContext(ctx)
	if err != nil {
		return AuthorizationRequest{}, ErrLoginUnavailable
	}
	provider, metadata, err := adapter.discover(providerContext, configuration)
	if err != nil || !metadata.supports(configuration.ClientAuthentication) {
		return AuthorizationRequest{}, ErrLoginUnavailable
	}

	state, err := randomValue()
	if err != nil {
		return AuthorizationRequest{}, ErrLoginUnavailable
	}
	nonce, err := randomValue()
	if err != nil {
		return AuthorizationRequest{}, ErrLoginUnavailable
	}
	verifier, err := randomValue()
	if err != nil {
		return AuthorizationRequest{}, ErrLoginUnavailable
	}

	now := adapter.now().UTC()
	ceremony := Ceremony{
		StateDigest: stateDigest(state), Nonce: nonce, PKCEVerifier: verifier,
		ProviderID: resolution.ProviderID, ProviderKey: resolution.ProviderKey,
		ProviderRevision: resolution.ProviderRevision, BindingRevision: resolution.BindingRevision,
		ConfigurationRevision: configuration.Revision,
		IdentityProviderKey:   configuration.IdentityProviderKey, IssuerURL: configuration.IssuerURL,
		ClientID: configuration.ClientID, RedirectURI: configuration.RedirectURI, Audience: configuration.Audience,
		ClientAuthentication: configuration.ClientAuthentication,
		StartedAt:            now, ExpiresAt: now.Add(defaultCeremonyLifetime),
	}
	oauthConfiguration := oauthConfiguration(provider, configuration, "")
	authorizationURL := oauthConfiguration.AuthCodeURL(
		state,
		oidclib.Nonce(nonce),
		oauth2.S256ChallengeOption(verifier),
	)
	if len(authorizationURL) > maximumValueLength*4 {
		return AuthorizationRequest{}, ErrLoginUnavailable
	}
	if err := adapter.ceremonies.Save(ctx, ceremony); err != nil {
		return AuthorizationRequest{}, ErrLoginUnavailable
	}
	return AuthorizationRequest{URL: authorizationURL}, nil
}

// VerifyCallback atomically consumes state, exchanges the authorization code,
// and returns only a provider-neutral VerifiedIdentity. All account, tenant,
// audience, role, and session decisions remain with Core Identity.
func (adapter *Adapter) VerifyCallback(
	ctx context.Context,
	resolution providerregistry.Resolution,
	configuration Configuration,
	callback Callback,
) (identity.VerifiedIdentity, error) {
	if adapter == nil || validateResolution(resolution) != nil || adapter.validateConfiguration(configuration) != nil ||
		invalidCallbackState(callback.State) {
		return identity.VerifiedIdentity{}, ErrAuthenticationFailed
	}

	digest := stateDigest(callback.State)
	ceremony, err := adapter.ceremonies.Consume(ctx, ConsumeRequest{
		StateDigest: digest, ProviderID: resolution.ProviderID,
		RedirectURI: configuration.RedirectURI,
	})
	if err != nil || ceremony.StateDigest != digest ||
		!ceremonyMatches(ceremony, resolution, configuration, adapter.now().UTC()) {
		return identity.VerifiedIdentity{}, ErrAuthenticationFailed
	}
	// State is now burned even when the OP returned an error or the code is bad.
	if callback.Error != "" || invalidAuthorizationCode(callback.Code) {
		return identity.VerifiedIdentity{}, ErrAuthenticationFailed
	}

	providerContext, err := adapter.bindHTTPContext(ctx)
	if err != nil {
		return identity.VerifiedIdentity{}, ErrAuthenticationFailed
	}
	provider, metadata, err := adapter.discover(providerContext, configuration)
	if err != nil || !metadata.supports(configuration.ClientAuthentication) {
		return identity.VerifiedIdentity{}, ErrAuthenticationFailed
	}

	token, err := adapter.exchange(providerContext, provider, resolution, configuration, callback.Code, ceremony.PKCEVerifier)
	if err != nil || token == nil || token.TokenType == "" || !strings.EqualFold(token.TokenType, "Bearer") ||
		token.AccessToken == "" {
		return identity.VerifiedIdentity{}, ErrAuthenticationFailed
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" || len(rawIDToken) > maximumIDTokenLength || !validIDTokenType(rawIDToken) {
		return identity.VerifiedIdentity{}, ErrAuthenticationFailed
	}

	now := adapter.now().UTC()
	verifiedToken, err := provider.VerifierContext(providerContext, &oidclib.Config{
		ClientID: configuration.ClientID,
		Now:      func() time.Time { return now },
	}).Verify(providerContext, rawIDToken)
	if err != nil || verifiedToken.Issuer != configuration.IssuerURL ||
		len(verifiedToken.Audience) != 1 || verifiedToken.Audience[0] != configuration.ClientID ||
		verifiedToken.Subject == "" || len(verifiedToken.Subject) > 512 ||
		strings.TrimSpace(verifiedToken.Subject) != verifiedToken.Subject ||
		verifiedToken.IssuedAt.IsZero() || verifiedToken.IssuedAt.After(now.Add(time.Minute)) ||
		!constantTimeEqual(verifiedToken.Nonce, ceremony.Nonce) {
		return identity.VerifiedIdentity{}, ErrAuthenticationFailed
	}

	providerSubject, err := identity.OIDCProviderSubject(configuration.IssuerURL, verifiedToken.Subject)
	if err != nil {
		return identity.VerifiedIdentity{}, ErrAuthenticationFailed
	}
	proof := identity.VerifiedIdentity{
		ProviderKey: configuration.IdentityProviderKey, ProviderSubject: providerSubject,
		Method: identity.AuthenticationMethodOIDC, Assurance: identity.AssuranceSingleFactor,
		AuthenticatedAt: now,
	}
	if proof.Validate() != nil {
		return identity.VerifiedIdentity{}, ErrAuthenticationFailed
	}
	return proof, nil
}

func (adapter *Adapter) exchange(
	ctx context.Context,
	provider *oidclib.Provider,
	resolution providerregistry.Resolution,
	configuration Configuration,
	code string,
	verifier string,
) (*oauth2.Token, error) {
	var token *oauth2.Token
	usedSecret := false
	exchange := func(secret []byte) error {
		if usedSecret {
			return ErrAuthenticationFailed
		}
		usedSecret = true
		if configuration.ClientAuthentication == ClientAuthenticationSecretBasic &&
			(len(secret) == 0 || len(secret) > maximumValueLength) {
			return ErrAuthenticationFailed
		}
		oauthConfiguration := oauthConfiguration(provider, configuration, string(secret))
		defer func() { oauthConfiguration.ClientSecret = "" }()
		var err error
		token, err = oauthConfiguration.Exchange(ctx, code, oauth2.VerifierOption(verifier))
		return err
	}

	if configuration.ClientAuthentication == ClientAuthenticationNone {
		if err := exchange(nil); err != nil {
			return nil, err
		}
		return token, nil
	}
	if adapter.secrets == nil {
		return nil, ErrAuthenticationFailed
	}
	err := adapter.secrets.UseClientSecret(ctx, ClientSecretRequest{
		ProviderID: resolution.ProviderID, ConfigurationRevision: configuration.Revision,
	}, exchange)
	if err != nil {
		return nil, err
	}
	if !usedSecret {
		return nil, ErrAuthenticationFailed
	}
	return token, nil
}

type discoveryMetadata struct {
	JWKSURI                       string   `json:"jwks_uri"`
	ResponseTypesSupported        []string `json:"response_types_supported"`
	CodeChallengeMethodsSupported []string `json:"code_challenge_methods_supported"`
	TokenAuthMethodsSupported     []string `json:"token_endpoint_auth_methods_supported"`
}

func (metadata discoveryMetadata) supports(authentication ClientAuthentication) bool {
	if metadata.JWKSURI == "" || !slices.Contains(metadata.ResponseTypesSupported, "code") ||
		!slices.Contains(metadata.CodeChallengeMethodsSupported, "S256") {
		return false
	}
	switch authentication {
	case ClientAuthenticationNone:
		return slices.Contains(metadata.TokenAuthMethodsSupported, "none")
	case ClientAuthenticationSecretBasic:
		return slices.Contains(metadata.TokenAuthMethodsSupported, "client_secret_basic")
	default:
		return false
	}
}

func (adapter *Adapter) discover(ctx context.Context, configuration Configuration) (*oidclib.Provider, discoveryMetadata, error) {
	provider, err := oidclib.NewProvider(ctx, configuration.IssuerURL)
	if err != nil {
		return nil, discoveryMetadata{}, err
	}
	var metadata discoveryMetadata
	if err := provider.Claims(&metadata); err != nil {
		return nil, discoveryMetadata{}, err
	}
	endpoint := provider.Endpoint()
	for _, rawURL := range []string{endpoint.AuthURL, endpoint.TokenURL, metadata.JWKSURI} {
		parsed, err := adapter.transport.ValidateURL(rawURL)
		if err != nil || invalidExactValue(rawURL, maximumValueLength) ||
			parsed.Scheme != "https" || parsed.String() != rawURL {
			return nil, discoveryMetadata{}, ErrLoginUnavailable
		}
	}
	return provider, metadata, nil
}

func oauthConfiguration(provider *oidclib.Provider, configuration Configuration, secret string) oauth2.Config {
	endpoint := provider.Endpoint()
	switch configuration.ClientAuthentication {
	case ClientAuthenticationNone:
		endpoint.AuthStyle = oauth2.AuthStyleInParams
	case ClientAuthenticationSecretBasic:
		endpoint.AuthStyle = oauth2.AuthStyleInHeader
	}
	return oauth2.Config{
		ClientID: configuration.ClientID, ClientSecret: secret, Endpoint: endpoint,
		RedirectURL: configuration.RedirectURI, Scopes: []string{oidclib.ScopeOpenID},
	}
}

func (adapter *Adapter) bindHTTPContext(ctx context.Context) (context.Context, error) {
	if ctx == nil || adapter.transport == nil || adapter.httpClient == nil {
		return nil, ErrLoginUnavailable
	}
	httpClient := adapter.httpClient()
	if httpClient == nil {
		return nil, ErrLoginUnavailable
	}
	return oidclib.ClientContext(ctx, httpClient), nil
}

func (adapter *Adapter) validateConfiguration(configuration Configuration) error {
	if adapter == nil || adapter.transport == nil || configuration.Revision == 0 ||
		!validStableKey(configuration.IdentityProviderKey) ||
		invalidExactValue(configuration.ClientID, maximumValueLength) ||
		(configuration.Audience != identity.AudienceWork && configuration.Audience != identity.AudienceAdmin) ||
		(configuration.ClientAuthentication != ClientAuthenticationNone &&
			configuration.ClientAuthentication != ClientAuthenticationSecretBasic) {
		return ErrLoginUnavailable
	}
	if configuration.ClientAuthentication == ClientAuthenticationSecretBasic && adapter.secrets == nil {
		return ErrLoginUnavailable
	}
	issuer, err := adapter.transport.ValidateURL(configuration.IssuerURL)
	if err != nil || invalidExactValue(configuration.IssuerURL, maximumValueLength) ||
		issuer.Scheme != "https" || issuer.String() != configuration.IssuerURL ||
		issuer.RawQuery != "" {
		return ErrLoginUnavailable
	}
	redirect, err := url.Parse(configuration.RedirectURI)
	if err != nil || invalidExactValue(configuration.RedirectURI, maximumValueLength) ||
		redirect.Scheme != "https" || redirect.Host == "" || redirect.User != nil || redirect.Fragment != "" ||
		redirect.String() != configuration.RedirectURI {
		return ErrLoginUnavailable
	}
	return nil
}

func validateResolution(resolution providerregistry.Resolution) error {
	if resolution.Validate() != nil ||
		resolution.RegistryContractVersion != providerregistry.ContractVersionV1 ||
		resolution.ServiceKey != providerregistry.IdentityLoginFederationServiceKey ||
		resolution.ServiceVersion != providerregistry.IdentityServiceContractVersionV1 ||
		resolution.CapabilityKey != providerregistry.IdentityOIDCLoginCapability ||
		resolution.CapabilityVersion != providerregistry.IdentityCapabilityContractVersionV1 ||
		resolution.AdapterKey != providerregistry.IdentityOIDCLoginAdapterV1 ||
		resolution.ProviderConfigScope != providerregistry.ConfigScopeInstallation ||
		resolution.OperationBoundary != providerregistry.OperationBoundaryInstallation {
		return ErrLoginUnavailable
	}
	return nil
}

func ceremonyMatches(
	ceremony Ceremony,
	resolution providerregistry.Resolution,
	configuration Configuration,
	now time.Time,
) bool {
	return ceremony.ProviderID == resolution.ProviderID && ceremony.ProviderKey == resolution.ProviderKey &&
		ceremony.ProviderRevision == resolution.ProviderRevision &&
		ceremony.BindingRevision == resolution.BindingRevision &&
		ceremony.ConfigurationRevision == configuration.Revision &&
		ceremony.IdentityProviderKey == configuration.IdentityProviderKey &&
		ceremony.IssuerURL == configuration.IssuerURL && ceremony.ClientID == configuration.ClientID &&
		ceremony.RedirectURI == configuration.RedirectURI && ceremony.Audience == configuration.Audience &&
		ceremony.ClientAuthentication == configuration.ClientAuthentication &&
		ceremony.Nonce != "" && ceremony.PKCEVerifier != "" &&
		!ceremony.StartedAt.IsZero() && !ceremony.ExpiresAt.IsZero() &&
		ceremony.ExpiresAt.After(ceremony.StartedAt) && now.Before(ceremony.ExpiresAt) &&
		!now.Before(ceremony.StartedAt.Add(-time.Minute))
}

func randomValue() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func stateDigest(state string) [sha256.Size]byte { return sha256.Sum256([]byte(state)) }

func invalidCallbackState(value string) bool {
	if len(value) != 43 || invalidExactValue(value, 43) {
		return true
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err != nil || len(decoded) != 32
}

func invalidAuthorizationCode(value string) bool {
	return invalidExactValue(value, maximumCodeLength)
}

func invalidExactValue(value string, maximum int) bool {
	if value == "" || len(value) > maximum || strings.TrimSpace(value) != value {
		return true
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return true
		}
	}
	return false
}

func validStableKey(value string) bool {
	if value == "" {
		return false
	}
	for index, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(index > 0 && character >= '0' && character <= '9') ||
			(index > 0 && (character == '-' || character == '.')) {
			continue
		}
		return false
	}
	return true
}

func constantTimeEqual(left, right string) bool {
	return len(left) == len(right) && subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

// OIDC allows the JOSE typ header to be absent. When present, it must identify
// an ID Token JWT and must not be an access-token or other JWT profile. The
// signature and all security claims are still verified by go-oidc afterwards.
func validIDTokenType(raw string) bool {
	separator := strings.IndexByte(raw, '.')
	if separator < 1 || strings.Count(raw, ".") != 2 {
		return false
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(raw[:separator])
	if err != nil || len(headerBytes) == 0 || len(headerBytes) > 4096 {
		return false
	}
	var header map[string]json.RawMessage
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return false
	}
	rawType, present := header["typ"]
	if !present {
		return true
	}
	var tokenType string
	return json.Unmarshal(rawType, &tokenType) == nil && tokenType == "JWT"
}
