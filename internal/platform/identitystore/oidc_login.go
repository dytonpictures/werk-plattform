package identitystore

import (
	"context"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/platform/identityprotocol/oidc"
)

// OIDCLoginCoordinator composes the protocol adapter with Core-owned provider
// resolution and account/session issuance. Neither side can infer authority
// from unverified provider claims.
type OIDCLoginCoordinator struct {
	service *Service
	adapter *oidc.Adapter
}

func NewOIDCLoginCoordinator(service *Service, adapter *oidc.Adapter) (*OIDCLoginCoordinator, error) {
	if service == nil || adapter == nil {
		return nil, oidc.ErrLoginUnavailable
	}
	return &OIDCLoginCoordinator{service: service, adapter: adapter}, nil
}

func (coordinator *OIDCLoginCoordinator) Begin(ctx context.Context, providerKey string) (string, error) {
	provider, err := coordinator.service.ResolveOIDCLoginProvider(ctx, providerKey)
	if err != nil {
		return "", oidc.ErrLoginUnavailable
	}
	request, err := coordinator.adapter.Begin(ctx, provider.Resolution, provider.Configuration)
	if err != nil {
		return "", oidc.ErrLoginUnavailable
	}
	return request.URL, nil
}

func (coordinator *OIDCLoginCoordinator) Complete(ctx context.Context, providerKey string, callback oidc.Callback, requestID, correlationID string) (identity.LoginResult, error) {
	provider, err := coordinator.service.ResolveOIDCLoginProvider(ctx, providerKey)
	if err != nil {
		return identity.LoginResult{}, oidc.ErrAuthenticationFailed
	}
	proof, err := coordinator.adapter.VerifyCallback(ctx, provider.Resolution, provider.Configuration, callback)
	if err != nil {
		return identity.LoginResult{}, oidc.ErrAuthenticationFailed
	}
	result, err := coordinator.service.CompleteFederatedLogin(ctx, proof, provider.Configuration.Audience, requestID, correlationID)
	if err != nil {
		return identity.LoginResult{}, oidc.ErrAuthenticationFailed
	}
	return result, nil
}
