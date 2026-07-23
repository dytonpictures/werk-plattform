package providerregistry

import (
	"errors"
	"testing"

	"github.com/dytonpictures/werk/internal/core/tenancy"
)

func TestServiceContractValidate(t *testing.T) {
	valid := ServiceContract{
		OwnerModule: "core.storage",
		ServiceKey:  "core.storage.service.object-store",
		Version:     1,
		Lifecycle:   LifecycleActive,
	}
	for _, lifecycle := range []Lifecycle{LifecycleActive, LifecycleDisabled, LifecycleRetired} {
		candidate := valid
		candidate.Lifecycle = lifecycle
		if err := candidate.Validate(); err != nil {
			t.Fatalf("valid lifecycle %q rejected: %v", lifecycle, err)
		}
	}

	tests := map[string]func(*ServiceContract){
		"invalid owner key":   func(value *ServiceContract) { value.OwnerModule = "Core Storage" },
		"invalid service key": func(value *ServiceContract) { value.ServiceKey = "Core.Storage" },
		"wrong namespace":     func(value *ServiceContract) { value.ServiceKey = "core.other.service.object-store" },
		"empty suffix":        func(value *ServiceContract) { value.ServiceKey = "core.storage.service." },
		"zero version":        func(value *ServiceContract) { value.Version = 0 },
		"invalid lifecycle":   func(value *ServiceContract) { value.Lifecycle = "unknown" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			if !errors.Is(candidate.Validate(), ErrInvalid) {
				t.Fatal("invalid service contract accepted")
			}
		})
	}
}

func TestCapabilityContractValidate(t *testing.T) {
	valid := validCapability()
	for _, lifecycle := range []Lifecycle{LifecycleActive, LifecycleDisabled, LifecycleRetired} {
		candidate := valid
		candidate.Lifecycle = lifecycle
		if err := candidate.Validate(); err != nil {
			t.Fatalf("valid lifecycle %q rejected: %v", lifecycle, err)
		}
	}

	tests := map[string]func(*CapabilityContract){
		"invalid service key": func(value *CapabilityContract) { value.ServiceKey = "Core.Storage" },
		"invalid capability":  func(value *CapabilityContract) { value.CapabilityKey = "Core.Read" },
		"wrong namespace":     func(value *CapabilityContract) { value.CapabilityKey = "core.other.capability.read" },
		"empty suffix":        func(value *CapabilityContract) { value.CapabilityKey = value.ServiceKey + ".capability." },
		"zero service version": func(value *CapabilityContract) {
			value.ServiceVersion = 0
		},
		"zero version":      func(value *CapabilityContract) { value.Version = 0 },
		"invalid boundary":  func(value *CapabilityContract) { value.OperationBoundary = "global" },
		"invalid lifecycle": func(value *CapabilityContract) { value.Lifecycle = "unknown" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			if !errors.Is(candidate.Validate(), ErrInvalid) {
				t.Fatal("invalid capability contract accepted")
			}
		})
	}
}

func TestProviderRegistrationValidate(t *testing.T) {
	installation := validProvider()
	if err := installation.Validate(); err != nil {
		t.Fatalf("installation provider rejected: %v", err)
	}

	tenantID := tenancy.TenantID{9}
	tenantProvider := installation
	tenantProvider.ConfigScope = ConfigScopeTenant
	tenantProvider.TenantID = &tenantID
	if err := tenantProvider.Validate(); err != nil {
		t.Fatalf("tenant provider rejected: %v", err)
	}

	disabled := installation
	disabled.Lifecycle = LifecycleDisabled
	if err := disabled.Validate(); err != nil {
		t.Fatalf("disabled provider must remain a valid persisted registration: %v", err)
	}

	tests := map[string]func(*ProviderRegistration){
		"zero id":             func(value *ProviderRegistration) { value.ID = ProviderID{} },
		"invalid service key": func(value *ProviderRegistration) { value.ServiceKey = "Core.Storage" },
		"invalid provider key": func(value *ProviderRegistration) {
			value.ProviderKey = "Core.Provider"
		},
		"wrong namespace": func(value *ProviderRegistration) {
			value.ProviderKey = "core.other.service.object-store.provider.local"
		},
		"empty suffix": func(value *ProviderRegistration) {
			value.ProviderKey = value.ServiceKey + ".provider."
		},
		"invalid adapter key":  func(value *ProviderRegistration) { value.AdapterKey = "local adapter" },
		"zero service version": func(value *ProviderRegistration) { value.ServiceVersion = 0 },
		"zero revision":        func(value *ProviderRegistration) { value.Revision = 0 },
		"zero registry contract version": func(value *ProviderRegistration) {
			value.RegistryContractVersion = 0
		},
		"unsupported registry contract version": func(value *ProviderRegistration) {
			value.RegistryContractVersion = ContractVersionV1 + 1
		},
		"invalid scope":     func(value *ProviderRegistration) { value.ConfigScope = "global" },
		"invalid lifecycle": func(value *ProviderRegistration) { value.Lifecycle = "unknown" },
		"installation with tenant": func(value *ProviderRegistration) {
			value.TenantID = &tenantID
		},
		"tenant without tenant": func(value *ProviderRegistration) {
			value.ConfigScope = ConfigScopeTenant
		},
		"tenant with zero tenant": func(value *ProviderRegistration) {
			zero := tenancy.TenantID{}
			value.ConfigScope = ConfigScopeTenant
			value.TenantID = &zero
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := installation
			mutate(&candidate)
			if !errors.Is(candidate.Validate(), ErrInvalid) {
				t.Fatal("invalid provider registration accepted")
			}
		})
	}
}

func TestProviderCapabilityBindingValidate(t *testing.T) {
	valid := validBinding()
	for _, lifecycle := range []Lifecycle{LifecycleActive, LifecycleDisabled, LifecycleRetired} {
		candidate := valid
		candidate.Lifecycle = lifecycle
		if err := candidate.Validate(); err != nil {
			t.Fatalf("valid lifecycle %q rejected: %v", lifecycle, err)
		}
	}

	tests := map[string]func(*ProviderCapabilityBinding){
		"zero provider id":       func(value *ProviderCapabilityBinding) { value.ProviderID = ProviderID{} },
		"invalid service key":    func(value *ProviderCapabilityBinding) { value.ServiceKey = "Core.Storage" },
		"invalid capability key": func(value *ProviderCapabilityBinding) { value.CapabilityKey = "Core.Read" },
		"wrong capability namespace": func(value *ProviderCapabilityBinding) {
			value.CapabilityKey = "core.other.service.object-store.capability.read"
		},
		"zero service version": func(value *ProviderCapabilityBinding) { value.ServiceVersion = 0 },
		"zero capability version": func(value *ProviderCapabilityBinding) {
			value.CapabilityVersion = 0
		},
		"zero revision":     func(value *ProviderCapabilityBinding) { value.Revision = 0 },
		"invalid lifecycle": func(value *ProviderCapabilityBinding) { value.Lifecycle = "unknown" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			if !errors.Is(candidate.Validate(), ErrInvalid) {
				t.Fatal("invalid provider capability binding accepted")
			}
		})
	}
}

func TestResolveRequestValidate(t *testing.T) {
	tenantID := tenancy.TenantID{9}
	installation := validRequest()
	tenantRequest := installation
	tenantRequest.Boundary = OperationBoundaryTenant
	tenantRequest.TenantID = &tenantID
	if err := installation.Validate(); err != nil {
		t.Fatalf("installation request rejected: %v", err)
	}
	if err := tenantRequest.Validate(); err != nil {
		t.Fatalf("tenant request rejected: %v", err)
	}

	tests := map[string]ResolveRequest{
		"zero provider id": withRequest(installation, func(value *ResolveRequest) { value.ProviderID = ProviderID{} }),
		"zero registry contract version": withRequest(installation, func(value *ResolveRequest) {
			value.RegistryContractVersion = 0
		}),
		"unsupported registry contract version": withRequest(installation, func(value *ResolveRequest) {
			value.RegistryContractVersion = ContractVersionV1 + 1
		}),
		"invalid service key":      withRequest(installation, func(value *ResolveRequest) { value.ServiceKey = "Core.Storage" }),
		"invalid capability key":   withRequest(installation, func(value *ResolveRequest) { value.CapabilityKey = "Core.Read" }),
		"zero service version":     withRequest(installation, func(value *ResolveRequest) { value.ServiceVersion = 0 }),
		"zero capability version":  withRequest(installation, func(value *ResolveRequest) { value.CapabilityVersion = 0 }),
		"invalid boundary":         withRequest(installation, func(value *ResolveRequest) { value.Boundary = "global" }),
		"installation with tenant": withRequest(installation, func(value *ResolveRequest) { value.TenantID = &tenantID }),
		"tenant without tenant": withRequest(installation, func(value *ResolveRequest) {
			value.Boundary = OperationBoundaryTenant
		}),
		"tenant with zero tenant": withRequest(installation, func(value *ResolveRequest) {
			zero := tenancy.TenantID{}
			value.Boundary = OperationBoundaryTenant
			value.TenantID = &zero
		}),
	}
	for name, candidate := range tests {
		t.Run(name, func(t *testing.T) {
			if !errors.Is(candidate.Validate(), ErrInvalid) {
				t.Fatal("invalid resolve request accepted")
			}
		})
	}
}

func TestResolveSupportsExplicitInstallationAndTenantScopes(t *testing.T) {
	service, capability, provider, binding, request := validResolution()

	resolved, err := Resolve(request, service, capability, provider, binding)
	if err != nil {
		t.Fatalf("resolve installation operation: %v", err)
	}
	if resolved.ProviderID != provider.ID || resolved.AdapterKey != provider.AdapterKey ||
		resolved.RegistryContractVersion != provider.RegistryContractVersion ||
		resolved.ProviderRevision != provider.Revision || resolved.BindingRevision != binding.Revision ||
		resolved.OperationBoundary != request.Boundary || !resolved.OperationTenantID.IsZero() {
		t.Fatal("resolution does not bind the requested provider and operation")
	}

	tenantID := tenancy.TenantID{9}
	tenantRequest := request
	tenantRequest.Boundary = OperationBoundaryTenant
	tenantRequest.TenantID = &tenantID
	tenantCapability := capability
	tenantCapability.OperationBoundary = OperationBoundaryTenant
	resolvedTenantOperation, err := Resolve(tenantRequest, service, tenantCapability, provider, binding)
	if err != nil {
		t.Fatalf("installation-scoped provider must support tenant operation: %v", err)
	}
	if resolvedTenantOperation.ProviderConfigScope != ConfigScopeInstallation ||
		!resolvedTenantOperation.ProviderTenantID.IsZero() ||
		resolvedTenantOperation.OperationTenantID != tenantID {
		t.Fatal("installation provider resolution lost the tenant operation boundary")
	}

	tenantProvider := provider
	tenantProvider.ConfigScope = ConfigScopeTenant
	tenantProvider.TenantID = &tenantID
	resolvedTenantProvider, err := Resolve(tenantRequest, service, tenantCapability, tenantProvider, binding)
	if err != nil {
		t.Fatalf("matching tenant-scoped provider rejected: %v", err)
	}
	if resolvedTenantProvider.ProviderConfigScope != ConfigScopeTenant ||
		resolvedTenantProvider.ProviderTenantID != tenantID ||
		resolvedTenantProvider.OperationTenantID != tenantID {
		t.Fatal("tenant provider resolution did not preserve both tenant coordinates")
	}
}

func TestResolutionValidateRejectsStructurallyInconsistentSnapshots(t *testing.T) {
	service, capability, provider, binding, request := validResolution()
	valid, err := Resolve(request, service, capability, provider, binding)
	if err != nil {
		t.Fatalf("resolve valid snapshot: %v", err)
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid resolution rejected: %v", err)
	}

	tenantID := tenancy.TenantID{9}
	tests := map[string]func(*Resolution){
		"zero provider revision": func(value *Resolution) { value.ProviderRevision = 0 },
		"zero binding revision":  func(value *Resolution) { value.BindingRevision = 0 },
		"wrong provider namespace": func(value *Resolution) {
			value.ProviderKey = "core.other.service.object-store.provider.local"
		},
		"wrong capability namespace": func(value *Resolution) {
			value.CapabilityKey = "core.other.service.object-store.capability.read"
		},
		"installation operation with tenant": func(value *Resolution) {
			value.OperationTenantID = tenantID
		},
		"tenant provider for installation operation": func(value *Resolution) {
			value.ProviderConfigScope = ConfigScopeTenant
			value.ProviderTenantID = tenantID
		},
		"tenant operation without tenant": func(value *Resolution) {
			value.OperationBoundary = OperationBoundaryTenant
		},
		"tenant provider mismatch": func(value *Resolution) {
			value.OperationBoundary = OperationBoundaryTenant
			value.OperationTenantID = tenantID
			value.ProviderConfigScope = ConfigScopeTenant
			value.ProviderTenantID = tenancy.TenantID{10}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			if !errors.Is(candidate.Validate(), ErrInvalid) {
				t.Fatal("invalid resolution snapshot accepted")
			}
		})
	}
}

func TestResolutionValidateDoesNotAuthorizeAConsistentSnapshot(t *testing.T) {
	service, capability, provider, binding, request := validResolution()
	resolved, err := Resolve(request, service, capability, provider, binding)
	if err != nil {
		t.Fatalf("resolve valid snapshot: %v", err)
	}

	constructed := resolved
	constructed.ProviderRevision++
	constructed.BindingRevision++
	if err := constructed.Validate(); err != nil {
		t.Fatalf("structurally consistent application data rejected: %v", err)
	}
	if constructed == resolved {
		t.Fatal("test did not construct a distinct non-authoritative snapshot")
	}
	// Validate deliberately cannot establish authority or freshness. A caller
	// must fully re-resolve the current four-part registry contract before use.
}

func TestResolveFailsClosedForScopeAndTenantMismatch(t *testing.T) {
	service, capability, provider, binding, request := validResolution()
	tenantID := tenancy.TenantID{9}
	otherTenantID := tenancy.TenantID{10}
	provider.ConfigScope = ConfigScopeTenant
	provider.TenantID = &tenantID

	assertUnresolved(t, request, service, capability, provider, binding)

	tenantRequest := request
	tenantRequest.Boundary = OperationBoundaryTenant
	tenantRequest.TenantID = &otherTenantID
	capability.OperationBoundary = OperationBoundaryTenant
	assertUnresolved(t, tenantRequest, service, capability, provider, binding)
}

func TestResolveRequiresEveryLifecycleActive(t *testing.T) {
	service, capability, provider, binding, request := validResolution()

	tests := map[string]func(*ServiceContract, *CapabilityContract, *ProviderRegistration, *ProviderCapabilityBinding){
		"service disabled": func(s *ServiceContract, _ *CapabilityContract, _ *ProviderRegistration, _ *ProviderCapabilityBinding) {
			s.Lifecycle = LifecycleDisabled
		},
		"capability retired": func(_ *ServiceContract, c *CapabilityContract, _ *ProviderRegistration, _ *ProviderCapabilityBinding) {
			c.Lifecycle = LifecycleRetired
		},
		"provider disabled": func(_ *ServiceContract, _ *CapabilityContract, p *ProviderRegistration, _ *ProviderCapabilityBinding) {
			p.Lifecycle = LifecycleDisabled
		},
		"binding retired": func(_ *ServiceContract, _ *CapabilityContract, _ *ProviderRegistration, b *ProviderCapabilityBinding) {
			b.Lifecycle = LifecycleRetired
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidateService := service
			candidateCapability := capability
			candidateProvider := provider
			candidateBinding := binding
			mutate(&candidateService, &candidateCapability, &candidateProvider, &candidateBinding)
			assertUnresolved(t, request, candidateService, candidateCapability, candidateProvider, candidateBinding)
		})
	}
}

func TestResolveRequiresExactVersionsAndReferences(t *testing.T) {
	service, capability, provider, binding, request := validResolution()

	tests := map[string]func(*ResolveRequest, *ServiceContract, *CapabilityContract, *ProviderRegistration, *ProviderCapabilityBinding){
		"request service version": func(r *ResolveRequest, _ *ServiceContract, _ *CapabilityContract, _ *ProviderRegistration, _ *ProviderCapabilityBinding) {
			r.ServiceVersion++
		},
		"request capability version": func(r *ResolveRequest, _ *ServiceContract, _ *CapabilityContract, _ *ProviderRegistration, _ *ProviderCapabilityBinding) {
			r.CapabilityVersion++
		},
		"operation boundary": func(r *ResolveRequest, _ *ServiceContract, c *CapabilityContract, _ *ProviderRegistration, _ *ProviderCapabilityBinding) {
			tenantID := tenancy.TenantID{9}
			r.Boundary = OperationBoundaryTenant
			r.TenantID = &tenantID
			c.OperationBoundary = OperationBoundaryInstallation
		},
		"capability service version": func(_ *ResolveRequest, _ *ServiceContract, c *CapabilityContract, _ *ProviderRegistration, _ *ProviderCapabilityBinding) {
			c.ServiceVersion++
		},
		"provider service version": func(_ *ResolveRequest, _ *ServiceContract, _ *CapabilityContract, p *ProviderRegistration, _ *ProviderCapabilityBinding) {
			p.ServiceVersion++
		},
		"registry contract version": func(r *ResolveRequest, _ *ServiceContract, _ *CapabilityContract, _ *ProviderRegistration, _ *ProviderCapabilityBinding) {
			r.RegistryContractVersion++
		},
		"binding service version": func(_ *ResolveRequest, _ *ServiceContract, _ *CapabilityContract, _ *ProviderRegistration, b *ProviderCapabilityBinding) {
			b.ServiceVersion++
		},
		"binding capability version": func(_ *ResolveRequest, _ *ServiceContract, _ *CapabilityContract, _ *ProviderRegistration, b *ProviderCapabilityBinding) {
			b.CapabilityVersion++
		},
		"request service key": func(r *ResolveRequest, _ *ServiceContract, _ *CapabilityContract, _ *ProviderRegistration, _ *ProviderCapabilityBinding) {
			r.ServiceKey = "core.other.service.object-store"
		},
		"request capability key": func(r *ResolveRequest, _ *ServiceContract, _ *CapabilityContract, _ *ProviderRegistration, _ *ProviderCapabilityBinding) {
			r.CapabilityKey = "core.storage.service.object-store.capability.write"
		},
		"provider id": func(_ *ResolveRequest, _ *ServiceContract, _ *CapabilityContract, p *ProviderRegistration, _ *ProviderCapabilityBinding) {
			p.ID = ProviderID{2}
		},
		"binding provider id": func(_ *ResolveRequest, _ *ServiceContract, _ *CapabilityContract, _ *ProviderRegistration, b *ProviderCapabilityBinding) {
			b.ProviderID = ProviderID{2}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidateRequest := request
			candidateService := service
			candidateCapability := capability
			candidateProvider := provider
			candidateBinding := binding
			mutate(&candidateRequest, &candidateService, &candidateCapability, &candidateProvider, &candidateBinding)
			assertUnresolved(t, candidateRequest, candidateService, candidateCapability, candidateProvider, candidateBinding)
		})
	}
}

func TestResolveFailsClosedForMalformedInputAndMissingProviderSelection(t *testing.T) {
	service, capability, provider, binding, request := validResolution()
	request.ProviderID = ProviderID{}

	resolved, err := Resolve(request, service, capability, provider, binding)
	if !errors.Is(err, ErrUnresolved) {
		t.Fatalf("expected ErrUnresolved, got %v", err)
	}
	if resolved != (Resolution{}) {
		t.Fatal("failed resolution leaked a partial provider")
	}
}

func validCapability() CapabilityContract {
	return CapabilityContract{
		ServiceKey:        "core.storage.service.object-store",
		ServiceVersion:    1,
		CapabilityKey:     "core.storage.service.object-store.capability.read",
		Version:           2,
		OperationBoundary: OperationBoundaryInstallation,
		Lifecycle:         LifecycleActive,
	}
}

func validProvider() ProviderRegistration {
	return ProviderRegistration{
		ID:                      ProviderID{1},
		ServiceKey:              "core.storage.service.object-store",
		ServiceVersion:          1,
		ProviderKey:             "core.storage.service.object-store.provider.local",
		AdapterKey:              "internal.storage.local",
		ConfigScope:             ConfigScopeInstallation,
		Lifecycle:               LifecycleActive,
		Revision:                3,
		RegistryContractVersion: ContractVersionV1,
	}
}

func validBinding() ProviderCapabilityBinding {
	return ProviderCapabilityBinding{
		ProviderID:        ProviderID{1},
		ServiceKey:        "core.storage.service.object-store",
		ServiceVersion:    1,
		CapabilityKey:     "core.storage.service.object-store.capability.read",
		CapabilityVersion: 2,
		Lifecycle:         LifecycleActive,
		Revision:          4,
	}
}

func validRequest() ResolveRequest {
	return ResolveRequest{
		ProviderID:              ProviderID{1},
		RegistryContractVersion: ContractVersionV1,
		ServiceKey:              "core.storage.service.object-store",
		ServiceVersion:          1,
		CapabilityKey:           "core.storage.service.object-store.capability.read",
		CapabilityVersion:       2,
		Boundary:                OperationBoundaryInstallation,
	}
}

func validResolution() (ServiceContract, CapabilityContract, ProviderRegistration, ProviderCapabilityBinding, ResolveRequest) {
	service := ServiceContract{
		OwnerModule: "core.storage",
		ServiceKey:  "core.storage.service.object-store",
		Version:     1,
		Lifecycle:   LifecycleActive,
	}
	return service, validCapability(), validProvider(), validBinding(), validRequest()
}

func withRequest(base ResolveRequest, mutate func(*ResolveRequest)) ResolveRequest {
	mutate(&base)
	return base
}

func assertUnresolved(
	t *testing.T,
	request ResolveRequest,
	service ServiceContract,
	capability CapabilityContract,
	provider ProviderRegistration,
	binding ProviderCapabilityBinding,
) {
	t.Helper()
	resolved, err := Resolve(request, service, capability, provider, binding)
	if !errors.Is(err, ErrUnresolved) {
		t.Fatalf("expected ErrUnresolved, got %v", err)
	}
	if resolved != (Resolution{}) {
		t.Fatal("failed resolution leaked a partial provider")
	}
}
