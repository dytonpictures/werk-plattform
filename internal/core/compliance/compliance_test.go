package compliance

import (
	"testing"

	"github.com/dytonpictures/werk/internal/core/resource"
)

func TestPersonalResourceRequiresProcessingContext(t *testing.T) {
	profile := ResourceDataProfile{
		ResourceKind: resource.KindWorkAccount, PersonalData: PersonalDataPersonal,
		Confidentiality: ConfidentialityRestricted, ProcessingActivityRequired: true,
		Status: resource.RegistrationActive, Version: 1,
	}
	policy := ProcessingPolicy{
		Permission: "core.identity.work-account.read", ResourceKind: resource.KindWorkAccount,
		Status: resource.RegistrationActive, Version: 1,
	}
	decision := Evaluate(profile, policy)
	if decision.Allowed() || decision.Reason != ReasonProcessingContextMissing {
		t.Fatalf("decision = %#v, want missing processing context", decision)
	}
	policy.Required = true
	policy.Context = ProcessingContext{
		ActivityKey: "core.identity.work-account-administration", PurposeKey: "core.identity.account-administration",
		LegalBasisRef: "operator.processing-register.identity-access",
	}
	if err := Authorize(profile, policy); err != nil {
		t.Fatalf("valid processing policy denied: %v", err)
	}
}

func TestPersonalProfileCannotDisableProcessingRequirement(t *testing.T) {
	profile := ResourceDataProfile{
		ResourceKind: resource.KindWorkspace, PersonalData: PersonalDataSpecialCategory,
		Confidentiality: ConfidentialityRestricted, ProcessingActivityRequired: false,
		Status: resource.RegistrationActive, Version: 1,
	}
	if err := profile.Validate(); err == nil {
		t.Fatal("personal profile accepted without processing activity requirement")
	}
}

func TestNonPersonalProfileAllowsEmptyProcessingContext(t *testing.T) {
	profile := ResourceDataProfile{
		ResourceKind: resource.KindPlatformInstallation, PersonalData: PersonalDataNone,
		Confidentiality: ConfidentialityInternal, ProcessingActivityRequired: false,
		Status: resource.RegistrationActive, Version: 1,
	}
	policy := ProcessingPolicy{
		Permission: "core.platform.health.read", ResourceKind: resource.KindPlatformInstallation,
		Status: resource.RegistrationActive, Version: 1,
	}
	if err := Authorize(profile, policy); err != nil {
		t.Fatalf("non-personal profile denied: %v", err)
	}
}

func TestInactiveProfileIsDenied(t *testing.T) {
	profile := ResourceDataProfile{
		ResourceKind: resource.KindPlatformInstallation, PersonalData: PersonalDataNone,
		Confidentiality: ConfidentialityInternal, ProcessingActivityRequired: false,
		Status: resource.RegistrationDisabled, Version: 1,
	}
	policy := ProcessingPolicy{
		Permission: "core.platform.health.read", ResourceKind: resource.KindPlatformInstallation,
		Status: resource.RegistrationActive, Version: 1,
	}
	decision := Evaluate(profile, policy)
	if decision.Allowed() || decision.Reason != ReasonProfileInactive {
		t.Fatalf("decision = %#v, want inactive profile", decision)
	}
}

func TestProcessingPolicyMustMatchResourceAndBeActive(t *testing.T) {
	profile := ResourceDataProfile{
		ResourceKind: resource.KindWorkspace, PersonalData: PersonalDataPersonal,
		Confidentiality: ConfidentialityConfidential, ProcessingActivityRequired: true,
		Status: resource.RegistrationActive, Version: 1,
	}
	policy := ProcessingPolicy{
		Permission: "core.workspace.access", ResourceKind: resource.KindWorkAccount, Required: true,
		Context: ProcessingContext{
			ActivityKey: "core.workspace.context-access", PurposeKey: "core.workspace.work-delivery",
			LegalBasisRef: "operator.processing-register.workspace",
		},
		Status: resource.RegistrationActive, Version: 1,
	}
	decision := Evaluate(profile, policy)
	if decision.Allowed() || decision.Reason != ReasonInvalidProcessingPolicy {
		t.Fatalf("decision = %#v, want mismatched processing policy", decision)
	}
	policy.ResourceKind = resource.KindWorkspace
	policy.Status = resource.RegistrationDisabled
	decision = Evaluate(profile, policy)
	if decision.Allowed() || decision.Reason != ReasonProcessingPolicyInactive {
		t.Fatalf("decision = %#v, want inactive processing policy", decision)
	}
}
