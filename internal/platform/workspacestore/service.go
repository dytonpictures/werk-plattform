package workspacestore

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/platform/database"
)

type Service struct {
	database *database.WorkDB
	now      func() time.Time
}

type TenantView struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type OrganizationalUnitView struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	UnitType string `json:"unit_type"`
}

type Overview struct {
	Tenant             TenantView              `json:"tenant"`
	OrganizationalUnit *OrganizationalUnitView `json:"organizational_unit,omitempty"`
	OrganizationalPath []OrganizationalUnitView `json:"organizational_path"`
	MembershipType     string                  `json:"membership_type,omitempty"`
	Permission         string                  `json:"permission"`
}

func New(db *database.WorkDB) (*Service, error) {
	if db == nil {
		return nil, errors.New("work database is required")
	}
	return &Service{database: db, now: func() time.Time { return time.Now().UTC() }}, nil
}

func (service *Service) Overview(ctx context.Context, actor identity.AuthenticatedActor) (Overview, error) {
	if err := identity.AuthorizeAccessPlane(actor, identity.AccessPlaneWork); err != nil || actor.TenantID == nil {
		return Overview{}, identity.ErrAccessDenied
	}
	view := Overview{
		OrganizationalPath: make([]OrganizationalUnitView, 0),
		Permission:         "core.workspace.access",
	}
	err := service.database.WithinTenantRead(ctx, *actor.TenantID, func(ctx context.Context, tx database.TenantTx) error {
		var unitID, unitName, unitType, membershipType *string
		err := tx.QueryRow(ctx, `
			SELECT tenant.id::text, tenant.name, tenant.status,
			       unit.id::text, unit.name, unit.unit_type, membership.membership_type
			FROM werk_core.accounts AS account
			JOIN werk_core.tenants AS tenant ON tenant.id=account.tenant_id
			JOIN werk_core.parties AS party
			  ON party.id=account.person_party_id AND party.tenant_id=account.tenant_id
			LEFT JOIN LATERAL (
			  SELECT candidate.organizational_unit_id, candidate.membership_type
			  FROM werk_core.memberships AS candidate
			  WHERE candidate.tenant_id=account.tenant_id AND candidate.party_id=party.id
			    AND candidate.valid_from <= $3
			    AND (candidate.valid_until IS NULL OR candidate.valid_until > $3)
			  ORDER BY candidate.valid_from DESC, candidate.id
			  LIMIT 1
			) AS membership ON true
			LEFT JOIN werk_core.organizational_units AS unit
			  ON unit.id=membership.organizational_unit_id AND unit.tenant_id=account.tenant_id
			WHERE account.id=$1::uuid AND account.tenant_id=$2::uuid
			  AND account.account_class='work' AND account.status='active'
			  AND tenant.status='active' AND party.status='active'
		`, formatUUID(actor.AccountID), actor.TenantID.String(), service.now()).Scan(
			&view.Tenant.ID, &view.Tenant.Name, &view.Tenant.Status,
			&unitID, &unitName, &unitType, &membershipType,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return identity.ErrAccessDenied
		}
		if err != nil {
			return err
		}
		if unitID != nil && unitName != nil && unitType != nil {
			view.OrganizationalUnit = &OrganizationalUnitView{ID: *unitID, Name: *unitName, UnitType: *unitType}
			rows, err := tx.Query(ctx, `
				WITH RECURSIVE lineage AS (
				  SELECT unit.id, unit.parent_id, unit.name, unit.unit_type, 0 AS depth
				  FROM werk_core.organizational_units AS unit
				  WHERE unit.tenant_id=$1::uuid AND unit.id=$2::uuid AND unit.status='active'
				  UNION ALL
				  SELECT parent.id, parent.parent_id, parent.name, parent.unit_type, child.depth + 1
				  FROM werk_core.organizational_units AS parent
				  JOIN lineage AS child ON child.parent_id=parent.id
				  WHERE parent.tenant_id=$1::uuid AND parent.status='active'
				)
				SELECT id::text, name, unit_type
				FROM lineage
				ORDER BY depth DESC
			`, actor.TenantID.String(), *unitID)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var item OrganizationalUnitView
				if err := rows.Scan(&item.ID, &item.Name, &item.UnitType); err != nil {
					return err
				}
				view.OrganizationalPath = append(view.OrganizationalPath, item)
			}
			if err := rows.Err(); err != nil {
				return err
			}
		}
		if membershipType != nil {
			view.MembershipType = *membershipType
		}
		return nil
	})
	if err != nil {
		return Overview{}, err
	}
	return view, nil
}

func formatUUID(value identity.AccountID) string {
	const hex = "0123456789abcdef"
	buffer := make([]byte, 36)
	positions := []int{0, 2, 4, 6, 9, 11, 14, 16, 19, 21, 24, 26, 28, 30, 32, 34}
	for index, current := range value {
		position := positions[index]
		buffer[position] = hex[current>>4]
		buffer[position+1] = hex[current&0x0f]
	}
	buffer[8], buffer[13], buffer[18], buffer[23] = '-', '-', '-', '-'
	return string(buffer)
}
