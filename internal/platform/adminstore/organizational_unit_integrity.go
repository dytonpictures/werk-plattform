package adminstore

import (
	"context"
	"errors"

	"github.com/dytonpictures/werk/internal/core/tenancy"
	"github.com/dytonpictures/werk/internal/platform/database"
)

// The topology lock is deliberately ordered by UUID. It covers the current
// subtree and the old and proposed parent paths, serializing hierarchy changes
// without imposing a tenant-wide table lock. The caller repeats it until two
// consecutive snapshots agree, because a row may have acquired a new parent or
// child while the first locking statement waited for a concurrent transaction.
const organizationalUnitMutationPathLockSQL = `
	WITH RECURSIVE
	affected_path(id, parent_id) AS (
		SELECT id, parent_id
		FROM werk_core.organizational_units
		WHERE tenant_id=$1::uuid
		  AND (id=$2::uuid OR id=$3::uuid)
		UNION
		SELECT parent.id, parent.parent_id
		FROM werk_core.organizational_units AS parent
		JOIN affected_path AS child ON child.parent_id=parent.id
		WHERE parent.tenant_id=$1::uuid
	),
	affected_subtree(id, parent_id) AS (
		SELECT id, parent_id
		FROM werk_core.organizational_units
		WHERE tenant_id=$1::uuid AND id=$2::uuid
		UNION
		SELECT child.id, child.parent_id
		FROM werk_core.organizational_units AS child
		JOIN affected_subtree AS parent ON child.parent_id=parent.id
		WHERE child.tenant_id=$1::uuid
	),
	affected_units(id) AS (
		SELECT id FROM affected_path
		UNION
		SELECT id FROM affected_subtree
	)
	SELECT unit.id::text
	FROM werk_core.organizational_units AS unit
	JOIN affected_units AS affected ON affected.id=unit.id
	WHERE unit.tenant_id=$1::uuid
	ORDER BY unit.id
	FOR UPDATE OF unit
`

const organizationalUnitTenantTopologyLockSQL = `
	SELECT id::text
	FROM werk_core.tenants
	WHERE id=$1::uuid
	ORDER BY id
	FOR UPDATE
`

const organizationalUnitCreateDepthSQL = `
	WITH RECURSIVE ancestors(id, parent_id, depth, path, cycle) AS (
		SELECT parent.id, parent.parent_id, 1, ARRAY[parent.id], false
		FROM werk_core.organizational_units AS parent
		WHERE parent.tenant_id=$1::uuid AND parent.id=$2::uuid
		UNION ALL
		SELECT parent.id, parent.parent_id, child.depth+1,
		       child.path || parent.id, parent.id=ANY(child.path)
		FROM werk_core.organizational_units AS parent
		JOIN ancestors AS child ON child.parent_id=parent.id
		WHERE parent.tenant_id=$1::uuid
		  AND NOT child.cycle
		  AND child.depth <= $3::integer
	)
	SELECT COALESCE(MAX(depth), 0)::integer + 1,
	       COALESCE(BOOL_OR(cycle), false)
	FROM ancestors
`

const organizationalUnitReparentDepthSQL = `
	WITH RECURSIVE
	new_ancestors(id, parent_id, depth, path, cycle) AS (
		SELECT parent.id, parent.parent_id, 1, ARRAY[parent.id], false
		FROM werk_core.organizational_units AS parent
		WHERE parent.tenant_id=$1::uuid AND parent.id=$3::uuid
		UNION ALL
		SELECT parent.id, parent.parent_id, child.depth+1,
		       child.path || parent.id, parent.id=ANY(child.path)
		FROM werk_core.organizational_units AS parent
		JOIN new_ancestors AS child ON child.parent_id=parent.id
		WHERE parent.tenant_id=$1::uuid
		  AND NOT child.cycle
		  AND child.depth <= $4::integer
	),
	subtree(id, depth, path, cycle) AS (
		SELECT subject.id, 1, ARRAY[subject.id], false
		FROM werk_core.organizational_units AS subject
		WHERE subject.tenant_id=$1::uuid AND subject.id=$2::uuid
		UNION ALL
		SELECT child.id, parent.depth+1,
		       parent.path || child.id, child.id=ANY(parent.path)
		FROM werk_core.organizational_units AS child
		JOIN subtree AS parent ON child.parent_id=parent.id
		WHERE child.tenant_id=$1::uuid
		  AND NOT parent.cycle
		  AND parent.depth <= $4::integer
	)
	SELECT COALESCE((SELECT MAX(depth) FROM new_ancestors), 0)::integer
	         + COALESCE((SELECT MAX(depth) FROM subtree), 0)::integer,
	       COALESCE((SELECT BOOL_OR(cycle) FROM new_ancestors), false)
	         OR COALESCE((SELECT BOOL_OR(cycle) FROM subtree), false)
`

const organizationalUnitChildLockSQL = `
	SELECT id::text
	FROM werk_core.organizational_units
	WHERE tenant_id=$1::uuid AND parent_id=$2::uuid
	ORDER BY id
	FOR UPDATE
`

const organizationalUnitGoverningGroupLockSQL = `
	SELECT id::text
	FROM werk_core.access_groups
	WHERE tenant_id=$1::uuid AND governing_unit_id=$2::uuid
	ORDER BY id
	FOR UPDATE
`

const organizationalUnitGroupMembershipLockSQL = `
	SELECT id::text
	FROM werk_core.access_group_memberships
	WHERE tenant_id=$1::uuid AND organizational_unit_id=$2::uuid
	ORDER BY id
	FOR UPDATE
`

const organizationalUnitAppEntitlementLockSQL = `
	SELECT id::text
	FROM werk_core.app_entitlements
	WHERE tenant_id=$1::uuid AND organizational_unit_id=$2::uuid
	ORDER BY id
	FOR UPDATE
`

// valid_from is intentionally absent. A scheduled edge is still a durable
// reference and must be removed or revoked explicitly before the unit can be
// archived. An edge whose validity has ended is no longer a reference.
const organizationalUnitArchiveReferenceSQL = `
	SELECT EXISTS(
		SELECT 1
		FROM werk_core.organizational_units AS child
		WHERE child.tenant_id=$1::uuid
		  AND child.parent_id=$2::uuid
		  AND child.status='active'
	) OR EXISTS(
		SELECT 1
		FROM werk_core.memberships AS membership
		WHERE membership.tenant_id=$1::uuid
		  AND membership.organizational_unit_id=$2::uuid
		  AND (membership.valid_until IS NULL OR membership.valid_until > $3)
	) OR EXISTS(
		SELECT 1
		FROM werk_core.access_groups AS access_group
		WHERE access_group.tenant_id=$1::uuid
		  AND access_group.governing_unit_id=$2::uuid
		  AND access_group.status='active'
	) OR EXISTS(
		SELECT 1
		FROM werk_core.access_group_memberships AS group_membership
		WHERE group_membership.tenant_id=$1::uuid
		  AND group_membership.organizational_unit_id=$2::uuid
		  AND group_membership.status='active'
		  AND (group_membership.valid_until IS NULL OR group_membership.valid_until > $3)
	) OR EXISTS(
		SELECT 1
		FROM werk_core.app_entitlements AS entitlement
		WHERE entitlement.tenant_id=$1::uuid
		  AND entitlement.organizational_unit_id=$2::uuid
		  AND entitlement.status='active'
		  AND (entitlement.valid_until IS NULL OR entitlement.valid_until > $3)
	) OR werk_security.lock_organizational_unit_role_assignment_references(
		$1::uuid,
		$2::uuid,
		$3::timestamptz
	)
`

const organizationalUnitChangedAncestorsSQL = `
	WITH RECURSIVE
	old_ancestors(id, parent_id) AS (
		SELECT parent.id, parent.parent_id
		FROM werk_core.organizational_units AS subject
		JOIN werk_core.organizational_units AS parent
		  ON parent.tenant_id=subject.tenant_id AND parent.id=subject.parent_id
		WHERE subject.tenant_id=$1::uuid AND subject.id=$2::uuid
		UNION
		SELECT parent.id, parent.parent_id
		FROM werk_core.organizational_units AS parent
		JOIN old_ancestors AS child ON child.parent_id=parent.id
		WHERE parent.tenant_id=$1::uuid
	),
	new_ancestors(id, parent_id) AS (
		SELECT candidate.id, candidate.parent_id
		FROM werk_core.organizational_units AS candidate
		WHERE candidate.tenant_id=$1::uuid AND candidate.id=$3::uuid
		UNION
		SELECT parent.id, parent.parent_id
		FROM werk_core.organizational_units AS parent
		JOIN new_ancestors AS child ON child.parent_id=parent.id
		WHERE parent.tenant_id=$1::uuid
	),
	changed_ancestors(id) AS (
		SELECT id FROM (
			SELECT id FROM old_ancestors
			EXCEPT
			SELECT id FROM new_ancestors
		) AS old_only
		UNION
		SELECT id FROM (
			SELECT id FROM new_ancestors
			EXCEPT
			SELECT id FROM old_ancestors
		) AS new_only
	)
`

const organizationalUnitInheritedAppEntitlementLockSQL = organizationalUnitChangedAncestorsSQL + `
	SELECT entitlement.id::text
	FROM werk_core.app_entitlements AS entitlement
	JOIN changed_ancestors AS changed
	  ON changed.id=entitlement.organizational_unit_id
	WHERE entitlement.tenant_id=$1::uuid
	ORDER BY entitlement.id
	FOR UPDATE OF entitlement
`

const organizationalUnitInheritedGroupMembershipLockSQL = organizationalUnitChangedAncestorsSQL + `
	SELECT membership.id::text
	FROM werk_core.access_group_memberships AS membership
	JOIN changed_ancestors AS changed
	  ON changed.id=membership.organizational_unit_id
	WHERE membership.tenant_id=$1::uuid
	ORDER BY membership.id
	FOR UPDATE OF membership
`

// As for archival, valid_from is intentionally absent so that a scheduled
// inherited edge cannot silently change its future reach through reparenting.
const organizationalUnitInheritedAccessConflictSQL = organizationalUnitChangedAncestorsSQL + `
	SELECT EXISTS(
		SELECT 1
		FROM werk_core.app_entitlements AS entitlement
		JOIN changed_ancestors AS changed
		  ON changed.id=entitlement.organizational_unit_id
		WHERE entitlement.tenant_id=$1::uuid
		  AND entitlement.include_descendants
		  AND entitlement.status='active'
		  AND (entitlement.valid_until IS NULL OR entitlement.valid_until > $4)
	) OR EXISTS(
		SELECT 1
		FROM werk_core.access_group_memberships AS membership
		JOIN changed_ancestors AS changed
		  ON changed.id=membership.organizational_unit_id
		WHERE membership.tenant_id=$1::uuid
		  AND membership.include_descendants
		  AND membership.status='active'
		  AND (membership.valid_until IS NULL OR membership.valid_until > $4)
	)
`

func lockOrganizationalUnitMutationPaths(
	ctx context.Context,
	tx database.TenantTx,
	tenantID string,
	unitID string,
	newParentID any,
) error {
	// Serialize topology mutations per tenant before taking UUID-ordered row
	// locks. Besides eliminating lock-order inversion between two hierarchy
	// changes, the tenant-row lock also coordinates inserts that acquire a
	// foreign-key key-share lock on the same tenant.
	if err := lockUUIDRows(ctx, tx, organizationalUnitTenantTopologyLockSQL, tenantID); err != nil {
		return err
	}

	var previous []string
	for attempt := 0; attempt < 4; attempt++ {
		current, err := lockUUIDRowIDs(ctx, tx, organizationalUnitMutationPathLockSQL, tenantID, unitID, newParentID)
		if err != nil {
			return err
		}
		if attempt > 0 && equalSortedUUIDRows(previous, current) {
			return nil
		}
		previous = current
	}
	return errors.New("organizational unit mutation path did not stabilize")
}

func ensureOrganizationalUnitCreateDepth(
	ctx context.Context,
	tx database.TenantTx,
	tenantID string,
	parentID string,
) error {
	var resultingDepth int
	var cycle bool
	if err := tx.QueryRow(
		ctx,
		organizationalUnitCreateDepthSQL,
		tenantID,
		parentID,
		tenancy.MaximumOrganizationalUnitDepth,
	).Scan(&resultingDepth, &cycle); err != nil {
		return err
	}
	if cycle || resultingDepth > tenancy.MaximumOrganizationalUnitDepth {
		return ErrOrganizationalUnitDepthConflict
	}
	return nil
}

func ensureOrganizationalUnitReparentDepth(
	ctx context.Context,
	tx database.TenantTx,
	tenantID string,
	unitID string,
	newParentID any,
) error {
	var resultingDepth int
	var cycle bool
	if err := tx.QueryRow(
		ctx,
		organizationalUnitReparentDepthSQL,
		tenantID,
		unitID,
		newParentID,
		tenancy.MaximumOrganizationalUnitDepth,
	).Scan(&resultingDepth, &cycle); err != nil {
		return err
	}
	if cycle || resultingDepth > tenancy.MaximumOrganizationalUnitDepth {
		return ErrOrganizationalUnitDepthConflict
	}
	return nil
}

func lockOrganizationalUnitDirectLinks(
	ctx context.Context,
	tx database.TenantTx,
	tenantID string,
	unitID string,
) error {
	// Membership validity has no mutation contract and the admin runtime
	// intentionally has no UPDATE grant on memberships. Its current insert
	// writer instead holds the referenced organizational-unit row FOR KEY
	// SHARE, which coordinates with the path's FOR UPDATE lock here.
	queries := []string{
		organizationalUnitChildLockSQL,
		organizationalUnitGoverningGroupLockSQL,
		organizationalUnitGroupMembershipLockSQL,
		organizationalUnitAppEntitlementLockSQL,
	}
	for _, query := range queries {
		if err := lockUUIDRows(ctx, tx, query, tenantID, unitID); err != nil {
			return err
		}
	}
	return nil
}

func lockOrganizationalUnitInheritedLinks(
	ctx context.Context,
	tx database.TenantTx,
	tenantID string,
	unitID string,
	newParentID any,
) error {
	for _, query := range []string{
		organizationalUnitInheritedAppEntitlementLockSQL,
		organizationalUnitInheritedGroupMembershipLockSQL,
	} {
		if err := lockUUIDRows(ctx, tx, query, tenantID, unitID, newParentID); err != nil {
			return err
		}
	}
	return nil
}

func lockUUIDRows(ctx context.Context, tx database.TenantTx, query string, arguments ...any) error {
	_, err := lockUUIDRowIDs(ctx, tx, query, arguments...)
	return err
}

func lockUUIDRowIDs(ctx context.Context, tx database.TenantTx, query string, arguments ...any) ([]string, error) {
	rows, err := tx.Query(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

func equalSortedUUIDRows(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
