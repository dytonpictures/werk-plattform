// Package businessobject defines the minimal, owner-published projection used
// to address tenant-bound business objects across module boundaries. A view is
// navigation metadata only; the owning module remains the source of truth for
// the referenced object and all of its domain data.
package businessobject

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dytonpictures/werk/internal/core/compliance"
	"github.com/dytonpictures/werk/internal/core/resource"
	"github.com/dytonpictures/werk/internal/core/tenancy"
)

var ErrInvalid = errors.New("invalid business object view")

const maximumTitleLength = 240

// Contract contains the server-resolved registrations which govern a view.
// Callers must not construct it from request data. An active module, tenant-
// bound resource type and data profile are all required; absence fails closed.
type Contract struct {
	Module       resource.ModuleRegistration
	ResourceType resource.TypeRegistration
	DataProfile  compliance.ResourceDataProfile
}

// Validate ensures that all registrations describe one active, tenant-bound
// owner contract. It deliberately accepts no default module or boundary.
func (contract Contract) Validate() error {
	if contract.Module.Validate() != nil || contract.ResourceType.Validate() != nil ||
		contract.DataProfile.Validate() != nil {
		return ErrInvalid
	}
	if contract.Module.Status != resource.RegistrationActive ||
		contract.ResourceType.Status != resource.RegistrationActive ||
		contract.DataProfile.Status != resource.RegistrationActive {
		return ErrInvalid
	}
	if contract.ResourceType.OwnerModule != contract.Module.Key ||
		contract.ResourceType.Boundary != resource.BoundaryTenant ||
		contract.DataProfile.ResourceKind != contract.ResourceType.Kind {
		return ErrInvalid
	}
	return nil
}

// View is a minimized projection of an object owned by another Core or app
// module. OwnerModule is derived from Contract and never accepted independently.
// SourceVersion is the owner's monotonic object/projection version, not a second
// version of the domain object maintained by this package.
type View struct {
	Ref            resource.Ref
	OwnerModule    string
	Title          string
	Classification *compliance.ConfidentialityLevel
	SourceVersion  uint64
	UpdatedAt      time.Time
}

// NewView builds a projection in an explicit, trusted tenant context. The
// tenant is supplied separately from Ref so a caller cannot select a different
// tenant merely by changing the reference in its payload.
func NewView(contract Contract, tenantID tenancy.TenantID, ref resource.Ref, title string,
	classification *compliance.ConfidentialityLevel, sourceVersion uint64, updatedAt time.Time) (View, error) {
	view := View{
		Ref:            cloneRef(ref),
		OwnerModule:    contract.ResourceType.OwnerModule,
		Title:          strings.TrimSpace(title),
		Classification: cloneClassification(classification),
		SourceVersion:  sourceVersion,
		UpdatedAt:      updatedAt.UTC(),
	}
	if err := view.Validate(contract, tenantID); err != nil {
		return View{}, err
	}
	return view, nil
}

// Validate checks both the value shape and its server-resolved owner, tenant
// and data-profile contract. Validation without those contracts would turn the
// projection into an unauthoritative, potentially cross-tenant record.
func (view View) Validate(contract Contract, tenantID tenancy.TenantID) error {
	if contract.Validate() != nil || tenantID.IsZero() || view.Ref.Validate() != nil ||
		view.Ref.Boundary != resource.BoundaryTenant || view.Ref.TenantID == nil ||
		*view.Ref.TenantID != tenantID || view.Ref.Kind != contract.ResourceType.Kind ||
		view.OwnerModule != contract.ResourceType.OwnerModule || view.SourceVersion == 0 ||
		view.UpdatedAt.IsZero() || view.UpdatedAt.Location() != time.UTC {
		return ErrInvalid
	}
	if !utf8.ValidString(view.Title) || strings.IndexByte(view.Title, 0) >= 0 ||
		view.Title != strings.TrimSpace(view.Title) || view.Title == "" ||
		utf8.RuneCountInString(view.Title) > maximumTitleLength {
		return ErrInvalid
	}
	if !classificationAllowed(contract.DataProfile.Confidentiality, view.Classification) {
		return ErrInvalid
	}
	return nil
}

// Revise returns the next projection snapshot while preserving its canonical
// reference and owner. Source versions must advance monotonically; skipped
// owner versions are allowed because not every domain change affects the view.
func (view View) Revise(contract Contract, tenantID tenancy.TenantID, title string,
	classification *compliance.ConfidentialityLevel, sourceVersion uint64, updatedAt time.Time) (View, error) {
	if view.Validate(contract, tenantID) != nil || sourceVersion <= view.SourceVersion ||
		updatedAt.IsZero() || updatedAt.Before(view.UpdatedAt) {
		return View{}, ErrInvalid
	}
	revised := view
	revised.Ref = cloneRef(view.Ref)
	revised.Title = strings.TrimSpace(title)
	revised.Classification = cloneClassification(classification)
	revised.SourceVersion = sourceVersion
	revised.UpdatedAt = updatedAt.UTC()
	if err := revised.Validate(contract, tenantID); err != nil {
		return View{}, err
	}
	return revised, nil
}

// EffectiveClassification returns the object override or, when none was
// published, the mandatory resource-type classification. An override may only
// make the effective level stricter.
func (view View) EffectiveClassification(contract Contract, tenantID tenancy.TenantID) (compliance.ConfidentialityLevel, error) {
	if err := view.Validate(contract, tenantID); err != nil {
		return "", err
	}
	if view.Classification == nil {
		return contract.DataProfile.Confidentiality, nil
	}
	return *view.Classification, nil
}

func classificationAllowed(baseline compliance.ConfidentialityLevel, override *compliance.ConfidentialityLevel) bool {
	baselineRank, valid := confidentialityRank(baseline)
	if !valid {
		return false
	}
	if override == nil {
		return true
	}
	overrideRank, valid := confidentialityRank(*override)
	return valid && overrideRank >= baselineRank
}

func confidentialityRank(level compliance.ConfidentialityLevel) (int, bool) {
	switch level {
	case compliance.ConfidentialityPublic:
		return 0, true
	case compliance.ConfidentialityInternal:
		return 1, true
	case compliance.ConfidentialityConfidential:
		return 2, true
	case compliance.ConfidentialityRestricted:
		return 3, true
	default:
		return 0, false
	}
}

func cloneRef(ref resource.Ref) resource.Ref {
	if ref.TenantID == nil {
		return ref
	}
	tenantID := *ref.TenantID
	ref.TenantID = &tenantID
	return ref
}

func cloneClassification(value *compliance.ConfidentialityLevel) *compliance.ConfidentialityLevel {
	if value == nil {
		return nil
	}
	classification := *value
	return &classification
}
