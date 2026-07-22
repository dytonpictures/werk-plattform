// Package compliance defines the small, jurisdiction-neutral contract used to
// classify resources and bind later processing-purpose decisions. It does not
// determine a legal basis; that remains an approved operator responsibility.
package compliance

import (
	"errors"

	"github.com/dytonpictures/werk/internal/core/resource"
)

var ErrDenied = errors.New("compliance processing denied")

type PersonalDataCategory string

const (
	PersonalDataNone            PersonalDataCategory = "none"
	PersonalDataPersonal        PersonalDataCategory = "personal"
	PersonalDataSpecialCategory PersonalDataCategory = "special-category"
	PersonalDataCriminalOffence PersonalDataCategory = "criminal-offence"
)

type ConfidentialityLevel string

const (
	ConfidentialityPublic       ConfidentialityLevel = "public"
	ConfidentialityInternal     ConfidentialityLevel = "internal"
	ConfidentialityConfidential ConfidentialityLevel = "confidential"
	ConfidentialityRestricted   ConfidentialityLevel = "restricted"
)

// ResourceDataProfile is mandatory metadata for an authorizable resource type.
// It describes data risk, not whether a concrete processing operation is lawful.
type ResourceDataProfile struct {
	ResourceKind               resource.Kind
	PersonalData               PersonalDataCategory
	Confidentiality            ConfidentialityLevel
	ProcessingActivityRequired bool
	Status                     resource.RegistrationStatus
	Version                    uint64
}

func (profile ResourceDataProfile) Validate() error {
	if !resource.ValidKey(string(profile.ResourceKind)) || profile.Version == 0 ||
		!validPersonalDataCategory(profile.PersonalData) || !validConfidentiality(profile.Confidentiality) ||
		!validStatus(profile.Status) {
		return ErrDenied
	}
	if profile.PersonalData != PersonalDataNone && !profile.ProcessingActivityRequired {
		return ErrDenied
	}
	return nil
}

// ProcessingContext is resolved by trusted server-side policy orchestration.
// Request data cannot establish its own purpose or legal-basis reference.
type ProcessingContext struct {
	ActivityKey   string
	PurposeKey    string
	LegalBasisRef string
}

// ProcessingPolicy binds one permission/resource pair to a server-controlled
// processing context. Every registered permission/resource pair needs a
// policy, including an explicit declaration that processing is not required.
type ProcessingPolicy struct {
	Permission   string
	ResourceKind resource.Kind
	Required     bool
	Context      ProcessingContext
	Status       resource.RegistrationStatus
	Version      uint64
}

func (policy ProcessingPolicy) Validate() error {
	if !resource.ValidKey(policy.Permission) || !resource.ValidKey(string(policy.ResourceKind)) ||
		policy.Version == 0 || !validStatus(policy.Status) {
		return ErrDenied
	}
	if policy.Required {
		if policy.Context == (ProcessingContext{}) {
			return ErrDenied
		}
		if !validProcessingContext(policy.Context) {
			return ErrDenied
		}
		return nil
	}
	if policy.Context != (ProcessingContext{}) {
		return ErrDenied
	}
	return nil
}

type DecisionEffect string

const (
	DecisionAllow DecisionEffect = "allow"
	DecisionDeny  DecisionEffect = "deny"
)

type DecisionReason string

const (
	ReasonProfileAccepted          DecisionReason = "profile-accepted"
	ReasonInvalidProfile           DecisionReason = "invalid-profile"
	ReasonProfileInactive          DecisionReason = "profile-inactive"
	ReasonInvalidProcessingPolicy  DecisionReason = "invalid-processing-policy"
	ReasonProcessingPolicyInactive DecisionReason = "processing-policy-inactive"
	ReasonProcessingContextMissing DecisionReason = "processing-context-missing"
)

type Decision struct {
	Effect DecisionEffect
	Reason DecisionReason
}

func (decision Decision) Allowed() bool {
	return decision.Effect == DecisionAllow
}

// Evaluate validates only the structural processing contract. It intentionally
// does not claim that the referenced legal basis has been approved; that
// registry and its governance are a later layer.
func Evaluate(profile ResourceDataProfile, policy ProcessingPolicy) Decision {
	if profile.Validate() != nil {
		return Decision{Effect: DecisionDeny, Reason: ReasonInvalidProfile}
	}
	if profile.Status != resource.RegistrationActive {
		return Decision{Effect: DecisionDeny, Reason: ReasonProfileInactive}
	}
	if policy.Validate() != nil || profile.ResourceKind != policy.ResourceKind {
		return Decision{Effect: DecisionDeny, Reason: ReasonInvalidProcessingPolicy}
	}
	if policy.Status != resource.RegistrationActive {
		return Decision{Effect: DecisionDeny, Reason: ReasonProcessingPolicyInactive}
	}
	if profile.ProcessingActivityRequired && !policy.Required {
		return Decision{Effect: DecisionDeny, Reason: ReasonProcessingContextMissing}
	}
	return Decision{Effect: DecisionAllow, Reason: ReasonProfileAccepted}
}

func Authorize(profile ResourceDataProfile, policy ProcessingPolicy) error {
	if !Evaluate(profile, policy).Allowed() {
		return ErrDenied
	}
	return nil
}

func validProcessingContext(processing ProcessingContext) bool {
	return resource.ValidKey(processing.ActivityKey) && resource.ValidKey(processing.PurposeKey) &&
		resource.ValidKey(processing.LegalBasisRef)
}

func validPersonalDataCategory(category PersonalDataCategory) bool {
	switch category {
	case PersonalDataNone, PersonalDataPersonal, PersonalDataSpecialCategory, PersonalDataCriminalOffence:
		return true
	default:
		return false
	}
}

func validConfidentiality(level ConfidentialityLevel) bool {
	switch level {
	case ConfidentialityPublic, ConfidentialityInternal, ConfidentialityConfidential, ConfidentialityRestricted:
		return true
	default:
		return false
	}
}

func validStatus(status resource.RegistrationStatus) bool {
	return status == resource.RegistrationActive || status == resource.RegistrationDisabled || status == resource.RegistrationRetired
}
