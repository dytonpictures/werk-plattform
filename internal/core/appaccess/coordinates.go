package appaccess

import (
	"bytes"
	"slices"
	"time"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/tenancy"
)

// OrganizationalMembership is one time-bound, tenant-local organizational
// membership projected for exactly one authenticated work account. AccountID
// repeats the trusted join anchor so a mislabeled tenant-wide projection fails
// closed instead of granting another party's coordinates.
type OrganizationalMembership struct {
	TenantID             tenancy.TenantID
	AccountID            identity.AccountID
	OrganizationalUnitID tenancy.UnitID
	ValidFrom            time.Time
	ValidUntil           *time.Time
}

func (membership OrganizationalMembership) validate() error {
	if membership.TenantID.IsZero() || membership.AccountID == (identity.AccountID{}) ||
		membership.OrganizationalUnitID.IsZero() || membership.ValidFrom.IsZero() {
		return ErrInvalid
	}
	// The persisted organization contract permits an empty [from, until)
	// interval. It never becomes effective, but remains structurally valid.
	if membership.ValidUntil != nil && membership.ValidUntil.Before(membership.ValidFrom) {
		return ErrInvalid
	}
	return nil
}

// OrganizationalUnitCoordinate is the minimal organization record needed to
// derive direct and ancestor coordinates. Governing/delegation metadata is not
// an app-access coordinate.
type OrganizationalUnitCoordinate struct {
	ID       tenancy.UnitID
	TenantID tenancy.TenantID
	ParentID *tenancy.UnitID
	Status   tenancy.UnitStatus
}

func (unit OrganizationalUnitCoordinate) validate() error {
	if unit.ID.IsZero() || unit.TenantID.IsZero() || !unit.Status.Valid() {
		return ErrInvalid
	}
	if unit.ParentID != nil && (unit.ParentID.IsZero() || *unit.ParentID == unit.ID) {
		return ErrInvalid
	}
	return nil
}

// OrganizationSnapshot is an organization-only projection produced by a
// trusted server adapter for exactly one authenticated work account. TenantID
// and AccountID anchor the projection to that actor. It intentionally has no
// field through which a caller can inject access-group coordinates.
type OrganizationSnapshot struct {
	TenantID    tenancy.TenantID
	AccountID   identity.AccountID
	Memberships []OrganizationalMembership
	Units       []OrganizationalUnitCoordinate
}

// ResolveActorCoordinates derives the complete app-gate coordinates for one
// authenticated work actor. Every supplied record must be structurally valid,
// tenant-local and internally connected. Invalid or incomplete active
// organization paths fail closed; inactive and out-of-window edges are
// ignored. Returned coordinates are deduplicated and deterministically sorted.
func ResolveActorCoordinates(
	actor identity.AuthenticatedActor,
	organization OrganizationSnapshot,
	groups []AccessGroup,
	groupMemberships []GroupMembership,
	evaluatedAt time.Time,
) (ActorCoordinates, error) {
	if identity.AuthorizeAccessPlane(actor, identity.AccessPlaneWork) != nil || evaluatedAt.IsZero() ||
		actor.TenantID == nil || organization.TenantID != *actor.TenantID ||
		organization.AccountID != actor.AccountID {
		return ActorCoordinates{}, ErrInvalid
	}

	directUnitIDs, ancestorUnitIDs, err := resolveOrganizationalCoordinates(organization, evaluatedAt)
	if err != nil {
		return ActorCoordinates{}, err
	}

	coordinates := ActorCoordinates{
		tenantID:                    organization.TenantID,
		accountID:                   organization.AccountID,
		resolvedAt:                  evaluatedAt,
		directOrganizationalUnitIDs: directUnitIDs,
		ancestorUnitIDs:             ancestorUnitIDs,
	}
	groupIDs, err := resolveAccessGroupCoordinates(groups, groupMemberships, coordinates, evaluatedAt)
	if err != nil {
		return ActorCoordinates{}, err
	}
	coordinates.accessGroupIDs = groupIDs
	if coordinates.Validate() != nil {
		return ActorCoordinates{}, ErrInvalid
	}
	return coordinates, nil
}

func resolveOrganizationalCoordinates(
	organization OrganizationSnapshot,
	evaluatedAt time.Time,
) ([]tenancy.UnitID, []tenancy.UnitID, error) {
	if organization.TenantID.IsZero() || organization.AccountID.IsZero() {
		return nil, nil, ErrInvalid
	}

	unitsByID := make(map[tenancy.UnitID]OrganizationalUnitCoordinate, len(organization.Units))
	for _, unit := range organization.Units {
		if unit.validate() != nil || unit.TenantID != organization.TenantID {
			return nil, nil, ErrInvalid
		}
		if _, duplicate := unitsByID[unit.ID]; duplicate {
			return nil, nil, ErrInvalid
		}
		unitsByID[unit.ID] = unit
	}

	directSet := make(map[tenancy.UnitID]struct{}, len(organization.Memberships))
	for _, membership := range organization.Memberships {
		if membership.validate() != nil || membership.TenantID != organization.TenantID ||
			membership.AccountID != organization.AccountID {
			return nil, nil, ErrInvalid
		}
		unit, exists := unitsByID[membership.OrganizationalUnitID]
		if !exists {
			return nil, nil, ErrInvalid
		}
		if effectiveAt(membership.ValidFrom, membership.ValidUntil, evaluatedAt) && unit.Status == tenancy.UnitStatusActive {
			directSet[unit.ID] = struct{}{}
		}
	}

	ancestorSet := make(map[tenancy.UnitID]struct{})
	for directID := range directSet {
		seen := make(map[tenancy.UnitID]struct{}, tenancy.MaximumOrganizationalUnitDepth)
		currentID := directID
		for depth := 0; ; depth++ {
			if depth >= tenancy.MaximumOrganizationalUnitDepth {
				return nil, nil, ErrInvalid
			}
			if _, cycle := seen[currentID]; cycle {
				return nil, nil, ErrInvalid
			}
			seen[currentID] = struct{}{}

			unit, exists := unitsByID[currentID]
			if !exists || unit.Status != tenancy.UnitStatusActive {
				return nil, nil, ErrInvalid
			}
			if currentID != directID {
				ancestorSet[currentID] = struct{}{}
			}
			if unit.ParentID == nil {
				break
			}
			currentID = *unit.ParentID
		}
	}

	return sortedUnitIDs(directSet), sortedUnitIDs(ancestorSet), nil
}

func resolveAccessGroupCoordinates(
	groups []AccessGroup,
	memberships []GroupMembership,
	coordinates ActorCoordinates,
	evaluatedAt time.Time,
) ([]GroupID, error) {
	groupsByID := make(map[GroupID]AccessGroup, len(groups))
	for _, group := range groups {
		if group.Validate() != nil || group.TenantID != coordinates.tenantID {
			return nil, ErrInvalid
		}
		if _, duplicate := groupsByID[group.ID]; duplicate {
			return nil, ErrInvalid
		}
		groupsByID[group.ID] = group
	}

	membershipIDs := make(map[GroupMembershipID]struct{}, len(memberships))
	resolved := make(map[GroupID]struct{})
	for _, membership := range memberships {
		if membership.Validate() != nil || membership.TenantID != coordinates.tenantID {
			return nil, ErrInvalid
		}
		if _, duplicate := membershipIDs[membership.ID]; duplicate {
			return nil, ErrInvalid
		}
		membershipIDs[membership.ID] = struct{}{}

		group, exists := groupsByID[membership.AccessGroupID]
		if !exists {
			return nil, ErrInvalid
		}
		if group.Status != GroupStatusActive || membership.Status != EntitlementStatusActive ||
			!effectiveAt(membership.ValidFrom, membership.ValidUntil, evaluatedAt) {
			continue
		}
		if subjectMatches(membership.Subject, coordinates) {
			resolved[group.ID] = struct{}{}
		}
	}

	return sortedGroupIDs(resolved), nil
}

func effectiveAt(validFrom time.Time, validUntil *time.Time, evaluatedAt time.Time) bool {
	return !evaluatedAt.Before(validFrom) && (validUntil == nil || evaluatedAt.Before(*validUntil))
}

func sortedUnitIDs(values map[tenancy.UnitID]struct{}) []tenancy.UnitID {
	result := make([]tenancy.UnitID, 0, len(values))
	for id := range values {
		result = append(result, id)
	}
	slices.SortFunc(result, func(left, right tenancy.UnitID) int {
		return bytes.Compare(left[:], right[:])
	})
	return result
}

func sortedGroupIDs(values map[GroupID]struct{}) []GroupID {
	result := make([]GroupID, 0, len(values))
	for id := range values {
		result = append(result, id)
	}
	slices.SortFunc(result, func(left, right GroupID) int {
		return bytes.Compare(left[:], right[:])
	})
	return result
}
