package identitystore

import (
	"testing"

	"github.com/dytonpictures/werk/internal/core/identity"
)

func TestProviderKindAcceptsOnlyItsAuthenticationMethods(t *testing.T) {
	tests := []struct {
		provider string
		method   identity.AuthenticationMethod
		allowed  bool
	}{
		{provider: "local", method: identity.AuthenticationMethodPassword, allowed: true},
		{provider: "local", method: identity.AuthenticationMethodPasskey, allowed: true},
		{provider: "local", method: identity.AuthenticationMethodAPIKey, allowed: true},
		{provider: "oidc", method: identity.AuthenticationMethodOIDC, allowed: true},
		{provider: "saml", method: identity.AuthenticationMethodSAML, allowed: true},
		{provider: "ldap", method: identity.AuthenticationMethodLDAPPassword, allowed: true},
		{provider: "oidc", method: identity.AuthenticationMethodPassword, allowed: false},
		{provider: "unknown", method: identity.AuthenticationMethodOIDC, allowed: false},
	}
	for _, test := range tests {
		if got := providerAcceptsMethod(test.provider, test.method); got != test.allowed {
			t.Errorf("providerAcceptsMethod(%q, %q) = %v, want %v", test.provider, test.method, got, test.allowed)
		}
	}
}
