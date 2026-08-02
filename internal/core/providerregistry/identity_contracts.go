package providerregistry

// Identity provider registry contracts are fixed coordinates, not a list of
// automatically selected providers. A caller still supplies one exact
// ProviderID and the registry resolves it fail-closed.
const (
	IdentityDirectoryServiceKey         = "core.identity.service.directory"
	IdentityLoginFederationServiceKey   = "core.identity.service.login-federation"
	IdentityServiceContractVersionV1    = uint64(1)
	IdentityCapabilityContractVersionV1 = uint64(1)

	IdentityDirectoryUserReadCapability            = "core.identity.service.directory.capability.user.read"
	IdentityDirectoryGroupReadCapability           = "core.identity.service.directory.capability.group.read"
	IdentityDirectoryGroupMembershipReadCapability = "core.identity.service.directory.capability.group-membership.read"
	IdentityDirectoryChangeSetReadCapability       = "core.identity.service.directory.capability.change-set.read"

	IdentityOIDCLoginCapability = "core.identity.service.login-federation.capability.oidc.login"
	IdentitySAMLLoginCapability = "core.identity.service.login-federation.capability.saml.login"

	IdentityLDAPDirectoryAdapterV1 = "core.identity.adapter.ldap-directory.v1"
	IdentityOIDCLoginAdapterV1     = "core.identity.adapter.oidc-login.v1"
	IdentitySAMLLoginAdapterV1     = "core.identity.adapter.saml-login.v1"
)

// IdentityServiceContractsV1 returns fresh values so callers cannot mutate a
// package-level catalog and thereby change the meaning of a contract.
func IdentityServiceContractsV1() []ServiceContract {
	return []ServiceContract{
		{
			OwnerModule: "core.identity", ServiceKey: IdentityDirectoryServiceKey,
			Version: IdentityServiceContractVersionV1, Lifecycle: LifecycleActive,
		},
		{
			OwnerModule: "core.identity", ServiceKey: IdentityLoginFederationServiceKey,
			Version: IdentityServiceContractVersionV1, Lifecycle: LifecycleActive,
		},
	}
}

// IdentityCapabilityContractsV1 describes only what the versioned services
// can technically perform. It grants no RBAC permission and registers no
// concrete provider instance.
func IdentityCapabilityContractsV1() []CapabilityContract {
	return []CapabilityContract{
		identityDirectoryCapability(IdentityDirectoryUserReadCapability),
		identityDirectoryCapability(IdentityDirectoryGroupReadCapability),
		identityDirectoryCapability(IdentityDirectoryGroupMembershipReadCapability),
		identityDirectoryCapability(IdentityDirectoryChangeSetReadCapability),
		identityLoginCapability(IdentityOIDCLoginCapability),
		identityLoginCapability(IdentitySAMLLoginCapability),
	}
}

func identityDirectoryCapability(key string) CapabilityContract {
	return CapabilityContract{
		ServiceKey: IdentityDirectoryServiceKey, ServiceVersion: IdentityServiceContractVersionV1,
		CapabilityKey: key, Version: IdentityCapabilityContractVersionV1,
		OperationBoundary: OperationBoundaryTenant, Lifecycle: LifecycleActive,
	}
}

func identityLoginCapability(key string) CapabilityContract {
	return CapabilityContract{
		ServiceKey: IdentityLoginFederationServiceKey, ServiceVersion: IdentityServiceContractVersionV1,
		CapabilityKey: key, Version: IdentityCapabilityContractVersionV1,
		OperationBoundary: OperationBoundaryInstallation, Lifecycle: LifecycleActive,
	}
}
