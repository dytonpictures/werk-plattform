package adminstore

import (
	"context"
	"errors"
	"strings"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/tenancy"
	"github.com/dytonpictures/werk/internal/platform/database"
)

type CreateTenantInput struct {
	Name            string `json:"name"`
	DefaultLocale   string `json:"default_locale"`
	DefaultTimezone string `json:"default_timezone"`
}

type UpdateTenantInput struct {
	Name            string `json:"name"`
	Status          string `json:"status"`
	DefaultLocale   string `json:"default_locale"`
	DefaultTimezone string `json:"default_timezone"`
}

type TenantView struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Status          string `json:"status"`
	DefaultLocale   string `json:"default_locale"`
	DefaultTimezone string `json:"default_timezone"`
	Version         uint64 `json:"version"`
}

type CreateOrganizationalUnitInput struct {
	ParentID string `json:"parent_id,omitempty"`
	UnitType string `json:"unit_type"`
	Name     string `json:"name"`
}

type UpdateOrganizationalUnitInput struct {
	ParentID string `json:"parent_id,omitempty"`
	UnitType string `json:"unit_type"`
	Name     string `json:"name"`
	Status   string `json:"status"`
}

type OrganizationalUnitView struct {
	ID       string  `json:"id"`
	TenantID string  `json:"tenant_id"`
	ParentID *string `json:"parent_id,omitempty"`
	UnitType string  `json:"unit_type"`
	Name     string  `json:"name"`
	Status   string  `json:"status"`
	Version  uint64  `json:"version"`
}

func (service *Service) ListTenants(ctx context.Context) ([]TenantView, error) {
	views := make([]TenantView, 0)
	err := service.database.WithinInstallationRead(ctx, func(ctx context.Context, tx database.TenantTx) error {
		rows, err := tx.Query(ctx, `
			SELECT id::text, name, status, default_locale, default_timezone, version
			FROM werk_core.tenants
			ORDER BY name, id
			LIMIT 200
		`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var view TenantView
			if err := rows.Scan(&view.ID, &view.Name, &view.Status, &view.DefaultLocale, &view.DefaultTimezone, &view.Version); err != nil {
				return err
			}
			views = append(views, view)
		}
		return rows.Err()
	})
	return views, err
}

func (service *Service) CreateTenant(ctx context.Context, input CreateTenantInput, actor identity.AuthenticatedActor, requestID, correlationID string) (TenantView, error) {
	tenant, err := tenancy.NewTenant(input.Name, input.DefaultLocale, input.DefaultTimezone)
	if err != nil {
		return TenantView{}, err
	}
	auditID, err := randomUUID()
	if err != nil {
		return TenantView{}, err
	}
	eventID, err := randomUUID()
	if err != nil {
		return TenantView{}, err
	}
	view := TenantView{ID: tenant.ID.String(), Name: tenant.Name, Status: string(tenant.Status), DefaultLocale: tenant.DefaultLocale, DefaultTimezone: tenant.DefaultTimezone, Version: tenant.Version}
	err = service.database.WithinTenantWrite(ctx, tenant.ID, func(ctx context.Context, tx database.TenantTx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO werk_core.tenants (
				id, name, status, default_locale, default_timezone, created_at, updated_at, version
			) VALUES ($1::uuid, $2, $3, $4, $5, $6, $6, 1)
		`, view.ID, view.Name, view.Status, view.DefaultLocale, view.DefaultTimezone, service.now()); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO werk_core.security_audit_events (
				id, event_type, outcome, account_id, tenant_id, request_id, correlation_id, details
			) VALUES (
				$1::uuid, 'core.tenancy.tenant-created.v1', 'succeeded', $2::uuid,
				$3::uuid, $4::uuid, $5::uuid, jsonb_build_object('tenant_id', $3::text)
			)
		`, auditID, formatUUID(actor.AccountID), view.ID, requestID, correlationID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO werk_core.outbox_events (
				id, tenant_id, event_type, producer, subject_kind, subject_id,
				partition_key, occurred_at, correlation_id, payload
			) VALUES (
				$1::uuid, $2::uuid, 'core.tenancy.tenant-created.v1', 'core.tenancy',
				'core.tenancy.tenant', $2::uuid, $2, $3, $4::uuid,
				jsonb_build_object('tenant_id', $2::text, 'name', $5::text)
			)
		`, eventID, view.ID, service.now(), correlationID, view.Name)
		return err
	})
	if err != nil {
		return TenantView{}, err
	}
	return view, nil
}

func (service *Service) UpdateTenant(ctx context.Context, tenantIDValue string, expectedVersion uint64, input UpdateTenantInput, actor identity.AuthenticatedActor, requestID, correlationID string) (TenantView, error) {
	tenantID, err := tenancy.ParseTenantID(tenantIDValue)
	if err != nil || expectedVersion == 0 {
		return TenantView{}, errors.New("invalid tenant update")
	}
	candidate := tenancy.Tenant{
		ID:              tenantID,
		Name:            strings.TrimSpace(input.Name),
		Status:          tenancy.TenantStatus(input.Status),
		DefaultLocale:   strings.TrimSpace(input.DefaultLocale),
		DefaultTimezone: strings.TrimSpace(input.DefaultTimezone),
		Version:         expectedVersion,
	}
	if err := candidate.Validate(); err != nil {
		return TenantView{}, err
	}
	auditID, err := randomUUID()
	if err != nil {
		return TenantView{}, err
	}
	eventID, err := randomUUID()
	if err != nil {
		return TenantView{}, err
	}
	view := TenantView{
		ID:              tenantID.String(),
		Name:            candidate.Name,
		Status:          string(candidate.Status),
		DefaultLocale:   candidate.DefaultLocale,
		DefaultTimezone: candidate.DefaultTimezone,
	}
	err = service.database.WithinTenantWrite(ctx, tenantID, func(ctx context.Context, tx database.TenantTx) error {
		var previous TenantView
		if err := tx.QueryRow(ctx, `
			SELECT id::text, name, status, default_locale, default_timezone, version
			FROM werk_core.tenants
			WHERE id=$1::uuid
			FOR UPDATE
		`, tenantID.String()).Scan(&previous.ID, &previous.Name, &previous.Status, &previous.DefaultLocale, &previous.DefaultTimezone, &previous.Version); err != nil {
			return ErrNotFound
		}
		if previous.Version != expectedVersion {
			return ErrVersionConflict
		}
		if err := tx.QueryRow(ctx, `
			UPDATE werk_core.tenants
			SET name=$2, status=$3, default_locale=$4, default_timezone=$5,
			    updated_at=$6, version=version+1
			WHERE id=$1::uuid AND version=$7
			RETURNING version
		`, tenantID.String(), view.Name, view.Status, view.DefaultLocale, view.DefaultTimezone, service.now(), expectedVersion).Scan(&view.Version); err != nil {
			return ErrVersionConflict
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO werk_core.security_audit_events (
				id,event_type,outcome,account_id,tenant_id,request_id,correlation_id,details
			) VALUES (
				$1::uuid,'core.tenancy.tenant-updated.v1','succeeded',$2::uuid,$3::uuid,$4::uuid,$5::uuid,
				jsonb_build_object(
					'tenant_id',$3::text,
					'previous',jsonb_build_object('name',$8::text,'status',$9::text,'default_locale',$10::text,'default_timezone',$11::text,'version',$6::bigint),
					'current',jsonb_build_object('name',$12::text,'status',$13::text,'default_locale',$14::text,'default_timezone',$15::text,'version',$7::bigint)
				)
			)
		`, auditID, formatUUID(actor.AccountID), tenantID.String(), requestID, correlationID,
			previous.Version, view.Version, previous.Name, previous.Status, previous.DefaultLocale, previous.DefaultTimezone,
			view.Name, view.Status, view.DefaultLocale, view.DefaultTimezone); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO werk_core.outbox_events (
				id,tenant_id,event_type,producer,subject_kind,subject_id,partition_key,occurred_at,correlation_id,payload
			) VALUES (
				$1::uuid,$2::uuid,'core.tenancy.tenant-updated.v1','core.tenancy',
				'core.tenancy.tenant',$2::uuid,$2,$3,$4::uuid,
				jsonb_build_object('tenant_id',$2::text,'name',$5::text,'status',$6::text,'version',$7::bigint)
			)
		`, eventID, tenantID.String(), service.now(), correlationID, view.Name, view.Status, view.Version)
		return err
	})
	if err != nil {
		return TenantView{}, err
	}
	return view, nil
}

func (service *Service) ListOrganizationalUnits(ctx context.Context, tenantIDValue string) ([]OrganizationalUnitView, error) {
	tenantID, err := tenancy.ParseTenantID(tenantIDValue)
	if err != nil {
		return nil, errors.New("invalid tenant")
	}
	views := make([]OrganizationalUnitView, 0)
	err = service.database.WithinTenantRead(ctx, tenantID, func(ctx context.Context, tx database.TenantTx) error {
		rows, err := tx.Query(ctx, `
			SELECT id::text, tenant_id::text, COALESCE(parent_id::text, ''), unit_type, name, status, version
			FROM werk_core.organizational_units
			WHERE tenant_id = $1::uuid
			ORDER BY name, id
			LIMIT 500
		`, tenantID.String())
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var view OrganizationalUnitView
			var parentID string
			if err := rows.Scan(&view.ID, &view.TenantID, &parentID, &view.UnitType, &view.Name, &view.Status, &view.Version); err != nil {
				return err
			}
			if parentID != "" {
				view.ParentID = &parentID
			}
			views = append(views, view)
		}
		return rows.Err()
	})
	return views, err
}

func (service *Service) CreateOrganizationalUnit(ctx context.Context, tenantIDValue string, input CreateOrganizationalUnitInput, actor identity.AuthenticatedActor, requestID, correlationID string) (OrganizationalUnitView, error) {
	tenantID, err := tenancy.ParseTenantID(tenantIDValue)
	if err != nil {
		return OrganizationalUnitView{}, errors.New("invalid tenant")
	}
	var parentID *tenancy.UnitID
	if strings.TrimSpace(input.ParentID) != "" {
		parsed, err := tenancy.ParseUnitID(strings.TrimSpace(input.ParentID))
		if err != nil {
			return OrganizationalUnitView{}, errors.New("invalid parent organizational unit")
		}
		parentID = &parsed
	}
	unit, err := tenancy.NewOrganizationalUnit(tenantID, parentID, tenancy.UnitType(input.UnitType), input.Name)
	if err != nil {
		return OrganizationalUnitView{}, err
	}
	auditID, err := randomUUID()
	if err != nil {
		return OrganizationalUnitView{}, err
	}
	eventID, err := randomUUID()
	if err != nil {
		return OrganizationalUnitView{}, err
	}
	view := OrganizationalUnitView{ID: unit.ID.String(), TenantID: tenantID.String(), UnitType: string(unit.Type), Name: unit.Name, Status: string(unit.Status), Version: unit.Version}
	if parentID != nil {
		value := parentID.String()
		view.ParentID = &value
	}
	err = service.database.WithinTenantWrite(ctx, tenantID, func(ctx context.Context, tx database.TenantTx) error {
		var tenantActive bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM werk_core.tenants WHERE id=$1::uuid AND status='active')`, tenantID.String()).Scan(&tenantActive); err != nil || !tenantActive {
			return errors.New("tenant unavailable")
		}
		var parent any
		if view.ParentID != nil {
			var parentActive bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM werk_core.organizational_units WHERE id=$1::uuid AND tenant_id=$2::uuid AND status='active')`, *view.ParentID, tenantID.String()).Scan(&parentActive); err != nil || !parentActive {
				return errors.New("parent organizational unit unavailable")
			}
			parent = *view.ParentID
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO werk_core.organizational_units (
				id, tenant_id, parent_id, unit_type, name, status, created_at, updated_at, version
			) VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, 'active', $6, $6, 1)
		`, view.ID, tenantID.String(), parent, view.UnitType, view.Name, service.now()); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO werk_core.security_audit_events (
				id, event_type, outcome, account_id, tenant_id, request_id, correlation_id, details
			) VALUES (
				$1::uuid, 'core.tenancy.organizational-unit-created.v1', 'succeeded',
				$2::uuid, $3::uuid, $4::uuid, $5::uuid,
				jsonb_build_object('organizational_unit_id', $6::text, 'parent_id', $7::text)
			)
		`, auditID, formatUUID(actor.AccountID), tenantID.String(), requestID, correlationID, view.ID, parent); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO werk_core.outbox_events (
				id, tenant_id, event_type, producer, subject_kind, subject_id,
				partition_key, occurred_at, correlation_id, payload
			) VALUES (
				$1::uuid, $2::uuid, 'core.tenancy.organizational-unit-created.v1', 'core.tenancy',
				'core.tenancy.organizational-unit', $3::uuid, $2, $4, $5::uuid,
				jsonb_build_object('organizational_unit_id', $3::text, 'unit_type', $6::text)
			)
		`, eventID, tenantID.String(), view.ID, service.now(), correlationID, view.UnitType)
		return err
	})
	if err != nil {
		return OrganizationalUnitView{}, err
	}
	return view, nil
}

func (service *Service) UpdateOrganizationalUnit(ctx context.Context, tenantIDValue, unitIDValue string, expectedVersion uint64, input UpdateOrganizationalUnitInput, actor identity.AuthenticatedActor, requestID, correlationID string) (OrganizationalUnitView, error) {
	tenantID, err := tenancy.ParseTenantID(tenantIDValue)
	if err != nil || expectedVersion == 0 {
		return OrganizationalUnitView{}, errors.New("invalid organizational unit update")
	}
	unitID, err := tenancy.ParseUnitID(unitIDValue)
	if err != nil {
		return OrganizationalUnitView{}, errors.New("invalid organizational unit")
	}
	var parentID *tenancy.UnitID
	if strings.TrimSpace(input.ParentID) != "" {
		parsed, err := tenancy.ParseUnitID(strings.TrimSpace(input.ParentID))
		if err != nil {
			return OrganizationalUnitView{}, errors.New("invalid parent organizational unit")
		}
		parentID = &parsed
	}
	candidate := tenancy.OrganizationalUnit{
		ID:       unitID,
		TenantID: tenantID,
		ParentID: parentID,
		Type:     tenancy.UnitType(strings.TrimSpace(input.UnitType)),
		Name:     strings.TrimSpace(input.Name),
		Status:   tenancy.UnitStatus(input.Status),
		Version:  expectedVersion,
	}
	if err := candidate.Validate(); err != nil {
		return OrganizationalUnitView{}, err
	}
	ids := make([]string, 2)
	for index := range ids {
		ids[index], err = randomUUID()
		if err != nil {
			return OrganizationalUnitView{}, err
		}
	}
	view := OrganizationalUnitView{
		ID: unitID.String(), TenantID: tenantID.String(), UnitType: string(candidate.Type),
		Name: candidate.Name, Status: string(candidate.Status),
	}
	if parentID != nil {
		value := parentID.String()
		view.ParentID = &value
	}
	err = service.database.WithinTenantWrite(ctx, tenantID, func(ctx context.Context, tx database.TenantTx) error {
		var previous OrganizationalUnitView
		var previousParent string
		if err := tx.QueryRow(ctx, `
			SELECT id::text, tenant_id::text, COALESCE(parent_id::text,''), unit_type, name, status, version
			FROM werk_core.organizational_units
			WHERE id=$1::uuid AND tenant_id=$2::uuid
			FOR UPDATE
		`, unitID.String(), tenantID.String()).Scan(
			&previous.ID, &previous.TenantID, &previousParent, &previous.UnitType,
			&previous.Name, &previous.Status, &previous.Version,
		); err != nil {
			return ErrNotFound
		}
		if previous.Version != expectedVersion {
			return ErrVersionConflict
		}
		if previousParent != "" {
			previous.ParentID = &previousParent
		}
		var parent any
		if view.ParentID != nil {
			var parentAvailable, createsCycle bool
			if err := tx.QueryRow(ctx, `
				SELECT EXISTS(
					SELECT 1 FROM werk_core.organizational_units
					WHERE id=$1::uuid AND tenant_id=$2::uuid AND status='active'
				), EXISTS(
					WITH RECURSIVE ancestors AS (
						SELECT id,parent_id FROM werk_core.organizational_units
						WHERE id=$1::uuid AND tenant_id=$2::uuid
						UNION
						SELECT candidate.id,candidate.parent_id
						FROM werk_core.organizational_units AS candidate
						JOIN ancestors ON candidate.id=ancestors.parent_id
						WHERE candidate.tenant_id=$2::uuid
					)
					SELECT 1 FROM ancestors WHERE id=$3::uuid
				)
			`, *view.ParentID, tenantID.String(), unitID.String()).Scan(&parentAvailable, &createsCycle); err != nil {
				return err
			}
			if !parentAvailable {
				return errors.New("parent organizational unit unavailable")
			}
			if createsCycle {
				return errors.New("organizational unit hierarchy cycle")
			}
			parent = *view.ParentID
		}
		if candidate.Status == tenancy.UnitStatusArchived {
			var inUse bool
			if err := tx.QueryRow(ctx, `
				SELECT EXISTS(
					SELECT 1 FROM werk_core.organizational_units
					WHERE tenant_id=$1::uuid AND parent_id=$2::uuid AND status='active'
				) OR EXISTS(
					SELECT 1 FROM werk_core.memberships
					WHERE tenant_id=$1::uuid AND organizational_unit_id=$2::uuid
					  AND valid_from <= $3 AND (valid_until IS NULL OR valid_until > $3)
				)
			`, tenantID.String(), unitID.String(), service.now()).Scan(&inUse); err != nil {
				return err
			}
			if inUse {
				return errors.New("organizational unit is still in use")
			}
		}
		if err := tx.QueryRow(ctx, `
			UPDATE werk_core.organizational_units
			SET parent_id=$3::uuid, unit_type=$4, name=$5, status=$6,
			    updated_at=$7, version=version+1
			WHERE id=$1::uuid AND tenant_id=$2::uuid AND version=$8
			RETURNING version
		`, unitID.String(), tenantID.String(), parent, view.UnitType, view.Name, view.Status, service.now(), expectedVersion).Scan(&view.Version); err != nil {
			return ErrVersionConflict
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO werk_core.security_audit_events (
				id,event_type,outcome,account_id,tenant_id,request_id,correlation_id,details
			) VALUES (
				$1::uuid,'core.tenancy.organizational-unit-updated.v1','succeeded',$2::uuid,$3::uuid,$4::uuid,$5::uuid,
				jsonb_build_object(
					'organizational_unit_id',$6::text,
					'previous',jsonb_build_object('parent_id',NULLIF($7::text,''),'unit_type',$8::text,'name',$9::text,'status',$10::text,'version',$11::bigint),
					'current',jsonb_build_object('parent_id',$12::text,'unit_type',$13::text,'name',$14::text,'status',$15::text,'version',$16::bigint)
				)
			)
		`, ids[0], formatUUID(actor.AccountID), tenantID.String(), requestID, correlationID, unitID.String(),
			previousParent, previous.UnitType, previous.Name, previous.Status, previous.Version,
			parent, view.UnitType, view.Name, view.Status, view.Version); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO werk_core.outbox_events (
				id,tenant_id,event_type,producer,subject_kind,subject_id,partition_key,occurred_at,correlation_id,payload
			) VALUES (
				$1::uuid,$2::uuid,'core.tenancy.organizational-unit-updated.v1','core.tenancy',
				'core.tenancy.organizational-unit',$3::uuid,$2,$4,$5::uuid,
				jsonb_build_object('organizational_unit_id',$3::text,'name',$6::text,'status',$7::text,'version',$8::bigint)
			)
		`, ids[1], tenantID.String(), unitID.String(), service.now(), correlationID, view.Name, view.Status, view.Version)
		return err
	})
	if err != nil {
		return OrganizationalUnitView{}, err
	}
	return view, nil
}
