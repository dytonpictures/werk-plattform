// Package resource defines the platform-wide identity of resources which can
// be addressed by authorization, audit, events and later search projections.
package resource

import (
	"errors"
	"regexp"
	"strings"

	"github.com/dytonpictures/werk/internal/core/tenancy"
)

var ErrInvalid = errors.New("invalid resource reference")

var (
	keyPattern = regexp.MustCompile(`^[a-z][a-z0-9.-]{1,159}$`)
	idPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,255}$`)
)

type Boundary string

const (
	BoundaryInstallation Boundary = "installation"
	BoundaryTenant       Boundary = "tenant"
)

type ModuleKind string

const (
	ModuleKindCore ModuleKind = "core"
	ModuleKindApp  ModuleKind = "app"
)

type RegistrationStatus string

const (
	RegistrationActive   RegistrationStatus = "active"
	RegistrationDisabled RegistrationStatus = "disabled"
	RegistrationRetired  RegistrationStatus = "retired"
)

type Kind string

const (
	KindPlatformInstallation Kind = "core.platform.installation"
	KindTenant               Kind = "core.tenancy.tenant"
	KindOrganizationalUnit   Kind = "core.tenancy.organizational-unit"
	KindWorkAccount          Kind = "core.identity.work-account"
	KindWorkRole             Kind = "core.authorization.work-role"
	KindSecurityLog          Kind = "core.audit.security-log"
	KindWorkspace            Kind = "core.workspace.workspace"
)

const RootID = "root"

// Ref is the canonical authorization target. A missing tenant never implies
// all tenants: it is valid only for an explicitly installation-bound kind.
type Ref struct {
	Boundary Boundary
	TenantID *tenancy.TenantID
	Kind     Kind
	ID       string
}

func InstallationRef(kind Kind, id string) Ref {
	return Ref{Boundary: BoundaryInstallation, Kind: kind, ID: id}
}

func TenantRef(tenantID tenancy.TenantID, kind Kind, id string) Ref {
	return Ref{Boundary: BoundaryTenant, TenantID: &tenantID, Kind: kind, ID: id}
}

func (ref Ref) Validate() error {
	if !ValidKey(string(ref.Kind)) || !idPattern.MatchString(ref.ID) {
		return ErrInvalid
	}
	switch ref.Boundary {
	case BoundaryInstallation:
		if ref.TenantID != nil {
			return ErrInvalid
		}
	case BoundaryTenant:
		if ref.TenantID == nil || ref.TenantID.IsZero() {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

type ModuleRegistration struct {
	Key     string
	Kind    ModuleKind
	Status  RegistrationStatus
	Version uint64
}

func (registration ModuleRegistration) Validate() error {
	if !ValidKey(registration.Key) || registration.Version == 0 || !validStatus(registration.Status) {
		return ErrInvalid
	}
	if registration.Kind == ModuleKindCore && !strings.HasPrefix(registration.Key, "core.") {
		return ErrInvalid
	}
	if registration.Kind == ModuleKindApp && !strings.HasPrefix(registration.Key, "app.") {
		return ErrInvalid
	}
	if registration.Kind != ModuleKindCore && registration.Kind != ModuleKindApp {
		return ErrInvalid
	}
	return nil
}

type TypeRegistration struct {
	Kind        Kind
	OwnerModule string
	Boundary    Boundary
	Status      RegistrationStatus
	Version     uint64
}

func (registration TypeRegistration) Validate() error {
	if !ValidKey(string(registration.Kind)) || !ValidKey(registration.OwnerModule) ||
		!strings.HasPrefix(string(registration.Kind), registration.OwnerModule+".") ||
		registration.Version == 0 || !validStatus(registration.Status) {
		return ErrInvalid
	}
	if registration.Boundary != BoundaryInstallation && registration.Boundary != BoundaryTenant {
		return ErrInvalid
	}
	return nil
}

func ValidKey(value string) bool {
	return keyPattern.MatchString(value)
}

func validStatus(status RegistrationStatus) bool {
	return status == RegistrationActive || status == RegistrationDisabled || status == RegistrationRetired
}
