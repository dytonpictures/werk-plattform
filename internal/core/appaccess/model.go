// Package appaccess defines the small platform contract for tenant app
// availability. It deliberately does not replace roles or an app's domain
// policy: an entitlement only opens the app gate for a resolved subject.
package appaccess

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/resource"
	"github.com/dytonpictures/werk/internal/core/tenancy"
)

var (
	ErrInvalid = errors.New("invalid app access contract")
	ErrDenied  = errors.New("app access denied")
)

type GroupID [16]byte
type GroupMembershipID [16]byte
type EntitlementID [16]byte

type GroupStatus string

const (
	GroupStatusActive   GroupStatus = "active"
	GroupStatusDisabled GroupStatus = "disabled"
)

type InstallationStatus string

const (
	InstallationStatusActive   InstallationStatus = "active"
	InstallationStatusDisabled InstallationStatus = "disabled"
	InstallationStatusRemoved  InstallationStatus = "removed"
)

type EntitlementStatus string

const (
	EntitlementStatusActive  EntitlementStatus = "active"
	EntitlementStatusRevoked EntitlementStatus = "revoked"
)

// SubjectRef addresses exactly one tenant-bound account, organizational unit,
// or access group. IncludeDescendants is meaningful only for an organizational
// unit and is resolved against server-derived organization coordinates.
type SubjectRef struct {
	TenantID             tenancy.TenantID
	AccountID            *identity.AccountID
	OrganizationalUnitID *tenancy.UnitID
	AccessGroupID        *GroupID
	IncludeDescendants   bool
}

func (subject SubjectRef) Validate() error {
	if subject.TenantID.IsZero() {
		return ErrInvalid
	}
	selected := 0
	if subject.AccountID != nil {
		selected++
		if *subject.AccountID == (identity.AccountID{}) {
			return ErrInvalid
		}
	}
	if subject.OrganizationalUnitID != nil {
		selected++
		if subject.OrganizationalUnitID.IsZero() {
			return ErrInvalid
		}
	}
	if subject.AccessGroupID != nil {
		selected++
		if *subject.AccessGroupID == (GroupID{}) {
			return ErrInvalid
		}
	}
	if selected != 1 || (subject.IncludeDescendants && subject.OrganizationalUnitID == nil) {
		return ErrInvalid
	}
	return nil
}

type AccessGroup struct {
	ID              GroupID
	TenantID        tenancy.TenantID
	Key             string
	DisplayName     string
	GoverningUnitID *tenancy.UnitID
	Status          GroupStatus
	ContractVersion uint64
}

func (group AccessGroup) Validate() error {
	if group.ID == (GroupID{}) || group.TenantID.IsZero() || !resource.ValidKey(group.Key) ||
		strings.TrimSpace(group.DisplayName) == "" || utf8.RuneCountInString(group.DisplayName) > 160 ||
		group.ContractVersion == 0 || (group.Status != GroupStatusActive && group.Status != GroupStatusDisabled) {
		return ErrInvalid
	}
	if group.GoverningUnitID != nil && group.GoverningUnitID.IsZero() {
		return ErrInvalid
	}
	return nil
}

// GroupMembership accepts accounts and organizational units but intentionally
// no nested access groups. This keeps the first graph cycle-free and usable.
type GroupMembership struct {
	ID              GroupMembershipID
	TenantID        tenancy.TenantID
	AccessGroupID   GroupID
	Subject         SubjectRef
	ValidFrom       time.Time
	ValidUntil      *time.Time
	Status          EntitlementStatus
	ContractVersion uint64
}

func (membership GroupMembership) Validate() error {
	if membership.ID == (GroupMembershipID{}) || membership.AccessGroupID == (GroupID{}) ||
		membership.TenantID.IsZero() || membership.Subject.Validate() != nil ||
		membership.Subject.TenantID != membership.TenantID || membership.Subject.AccessGroupID != nil ||
		membership.ValidFrom.IsZero() || membership.ContractVersion == 0 ||
		!validEntitlementStatus(membership.Status) {
		return ErrInvalid
	}
	if membership.ValidUntil != nil && !membership.ValidUntil.After(membership.ValidFrom) {
		return ErrInvalid
	}
	return nil
}

type Installation struct {
	TenantID        tenancy.TenantID
	AppModule       string
	Status          InstallationStatus
	ContractVersion uint64
}

func (installation Installation) Validate() error {
	if installation.TenantID.IsZero() || !validAppModule(installation.AppModule) ||
		installation.ContractVersion == 0 ||
		(installation.Status != InstallationStatusActive && installation.Status != InstallationStatusDisabled && installation.Status != InstallationStatusRemoved) {
		return ErrInvalid
	}
	return nil
}

type Entitlement struct {
	ID              EntitlementID
	TenantID        tenancy.TenantID
	AppModule       string
	Subject         SubjectRef
	ValidFrom       time.Time
	ValidUntil      *time.Time
	Status          EntitlementStatus
	ContractVersion uint64
}

func (entitlement Entitlement) Validate() error {
	if entitlement.ID == (EntitlementID{}) || entitlement.TenantID.IsZero() ||
		!validAppModule(entitlement.AppModule) || entitlement.Subject.Validate() != nil ||
		entitlement.Subject.TenantID != entitlement.TenantID || entitlement.ValidFrom.IsZero() ||
		entitlement.ContractVersion == 0 || !validEntitlementStatus(entitlement.Status) {
		return ErrInvalid
	}
	if entitlement.ValidUntil != nil && !entitlement.ValidUntil.After(entitlement.ValidFrom) {
		return ErrInvalid
	}
	return nil
}

// ActorCoordinates are resolved from active server-side memberships. Ancestor
// units contain only parents of the direct units; no client may add coordinates.
type ActorCoordinates struct {
	TenantID                    tenancy.TenantID
	AccountID                   identity.AccountID
	DirectOrganizationalUnitIDs []tenancy.UnitID
	AncestorUnitIDs             []tenancy.UnitID
	AccessGroupIDs              []GroupID
}

func (coordinates ActorCoordinates) Validate() error {
	if coordinates.TenantID.IsZero() || coordinates.AccountID == (identity.AccountID{}) {
		return ErrInvalid
	}
	for _, id := range coordinates.DirectOrganizationalUnitIDs {
		if id.IsZero() {
			return ErrInvalid
		}
	}
	for _, id := range coordinates.AncestorUnitIDs {
		if id.IsZero() {
			return ErrInvalid
		}
	}
	for _, id := range coordinates.AccessGroupIDs {
		if id == (GroupID{}) {
			return ErrInvalid
		}
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
	ReasonEntitlementMatched DecisionReason = "entitlement-matched"
	ReasonInvalidContract    DecisionReason = "invalid-contract"
	ReasonAppUnavailable     DecisionReason = "app-unavailable"
	ReasonNoEntitlement      DecisionReason = "no-entitlement"
)

type Decision struct {
	Effect DecisionEffect
	Reason DecisionReason
}

func (decision Decision) Allowed() bool {
	return decision.Effect == DecisionAllow
}

// Evaluate decides only whether an actor may enter an app. A positive result
// never grants an app role, permission, or access to a domain resource.
func Evaluate(installation Installation, coordinates ActorCoordinates, entitlements []Entitlement, now time.Time) Decision {
	if installation.Validate() != nil || coordinates.Validate() != nil || now.IsZero() ||
		installation.TenantID != coordinates.TenantID {
		return Decision{Effect: DecisionDeny, Reason: ReasonInvalidContract}
	}
	if installation.Status != InstallationStatusActive {
		return Decision{Effect: DecisionDeny, Reason: ReasonAppUnavailable}
	}
	for _, entitlement := range entitlements {
		if entitlement.Validate() != nil || entitlement.TenantID != coordinates.TenantID ||
			entitlement.AppModule != installation.AppModule {
			return Decision{Effect: DecisionDeny, Reason: ReasonInvalidContract}
		}
		if entitlement.Status != EntitlementStatusActive || now.Before(entitlement.ValidFrom) ||
			(entitlement.ValidUntil != nil && !now.Before(*entitlement.ValidUntil)) {
			continue
		}
		if subjectMatches(entitlement.Subject, coordinates) {
			return Decision{Effect: DecisionAllow, Reason: ReasonEntitlementMatched}
		}
	}
	return Decision{Effect: DecisionDeny, Reason: ReasonNoEntitlement}
}

func Authorize(installation Installation, coordinates ActorCoordinates, entitlements []Entitlement, now time.Time) error {
	if !Evaluate(installation, coordinates, entitlements, now).Allowed() {
		return ErrDenied
	}
	return nil
}

func subjectMatches(subject SubjectRef, coordinates ActorCoordinates) bool {
	if subject.AccountID != nil {
		return *subject.AccountID == coordinates.AccountID
	}
	if subject.AccessGroupID != nil {
		return containsGroup(coordinates.AccessGroupIDs, *subject.AccessGroupID)
	}
	if subject.OrganizationalUnitID == nil {
		return false
	}
	if containsUnit(coordinates.DirectOrganizationalUnitIDs, *subject.OrganizationalUnitID) {
		return true
	}
	return subject.IncludeDescendants && containsUnit(coordinates.AncestorUnitIDs, *subject.OrganizationalUnitID)
}

func containsUnit(values []tenancy.UnitID, target tenancy.UnitID) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsGroup(values []GroupID, target GroupID) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func validAppModule(value string) bool {
	return resource.ValidKey(value) && strings.HasPrefix(value, "app.")
}

func validEntitlementStatus(status EntitlementStatus) bool {
	return status == EntitlementStatusActive || status == EntitlementStatusRevoked
}
