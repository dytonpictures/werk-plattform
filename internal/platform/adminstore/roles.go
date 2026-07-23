package adminstore

import (
	"context"
	"encoding/hex"
	"errors"
	"regexp"
	"sort"
	"strings"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/tenancy"
	"github.com/dytonpictures/werk/internal/platform/database"
)

var workRoleKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9.-]+$`)

type WorkPermissionView struct {
	ID            string `json:"id"`
	PermissionKey string `json:"permission_key"`
	DisplayName   string `json:"display_name"`
	OwningModule  string `json:"owning_module"`
	RiskLevel     string `json:"risk_level"`
}

type WorkRoleView struct {
	ID              string               `json:"id"`
	TenantID        string               `json:"tenant_id"`
	RoleKey         string               `json:"role_key"`
	DisplayName     string               `json:"display_name"`
	SystemRole      bool                 `json:"system_role"`
	Status          string               `json:"status"`
	Version         int64                `json:"version"`
	AssignmentCount int                  `json:"assignment_count"`
	Permissions     []WorkPermissionView `json:"permissions"`
}

type WorkRoleCatalog struct {
	Roles       []WorkRoleView       `json:"roles"`
	Permissions []WorkPermissionView `json:"permissions"`
}

type CreateWorkRoleInput struct {
	TenantID       string   `json:"tenant_id"`
	RoleKey        string   `json:"role_key"`
	DisplayName    string   `json:"display_name"`
	PermissionKeys []string `json:"permission_keys"`
}

type UpdateWorkRoleInput struct {
	TenantID       string   `json:"tenant_id"`
	DisplayName    string   `json:"display_name"`
	Status         string   `json:"status"`
	PermissionKeys []string `json:"permission_keys"`
}

type ReplaceWorkUserRolesInput struct {
	TenantID string   `json:"tenant_id"`
	RoleIDs  []string `json:"role_ids"`
}

func (service *Service) ListWorkRoles(ctx context.Context, tenantIDValue string, actor identity.AuthenticatedActor, requestID, correlationID string) (WorkRoleCatalog, error) {
	tenantID, err := tenancy.ParseTenantID(tenantIDValue)
	if err != nil {
		return WorkRoleCatalog{}, errors.New("invalid tenant")
	}
	auditID, err := randomUUID()
	if err != nil {
		return WorkRoleCatalog{}, err
	}
	catalog := WorkRoleCatalog{Roles: make([]WorkRoleView, 0), Permissions: make([]WorkPermissionView, 0)}
	err = service.database.WithinTenantWrite(ctx, tenantID, func(ctx context.Context, tx database.TenantTx) error {
		permissionRows, err := tx.Query(ctx, `
			SELECT id::text, permission_key, display_name, owning_module, risk_level
			FROM werk_core.permissions
			WHERE access_plane='work' AND status='active'
			ORDER BY owning_module, permission_key
		`)
		if err != nil {
			return err
		}
		for permissionRows.Next() {
			var permission WorkPermissionView
			if err := permissionRows.Scan(&permission.ID, &permission.PermissionKey, &permission.DisplayName, &permission.OwningModule, &permission.RiskLevel); err != nil {
				permissionRows.Close()
				return err
			}
			catalog.Permissions = append(catalog.Permissions, permission)
		}
		if err := permissionRows.Err(); err != nil {
			permissionRows.Close()
			return err
		}
		permissionRows.Close()

		roleRows, err := tx.Query(ctx, `
			SELECT role.id::text, role.tenant_id::text, role.role_key, role.display_name,
			       role.system_role, role.status, role.version,
			       count(DISTINCT assignment.id) FILTER (
			         WHERE assignment.valid_from <= $2
			           AND (assignment.valid_until IS NULL OR assignment.valid_until > $2)
			       )::integer
			FROM werk_core.roles AS role
			LEFT JOIN werk_core.role_assignments AS assignment ON assignment.role_id=role.id
			WHERE role.tenant_id=$1::uuid AND role.access_plane='work'
			GROUP BY role.id
			ORDER BY role.system_role DESC, role.display_name, role.role_key
		`, tenantID.String(), service.now())
		if err != nil {
			return err
		}
		roleIndexes := make(map[string]int)
		for roleRows.Next() {
			var role WorkRoleView
			if err := roleRows.Scan(&role.ID, &role.TenantID, &role.RoleKey, &role.DisplayName, &role.SystemRole, &role.Status, &role.Version, &role.AssignmentCount); err != nil {
				roleRows.Close()
				return err
			}
			role.Permissions = make([]WorkPermissionView, 0)
			roleIndexes[role.ID] = len(catalog.Roles)
			catalog.Roles = append(catalog.Roles, role)
		}
		if err := roleRows.Err(); err != nil {
			roleRows.Close()
			return err
		}
		roleRows.Close()

		bindingRows, err := tx.Query(ctx, `
			SELECT role_permission.role_id::text, permission.id::text, permission.permission_key,
			       permission.display_name, permission.owning_module, permission.risk_level
			FROM werk_core.role_permissions AS role_permission
			JOIN werk_core.roles AS role ON role.id=role_permission.role_id
			JOIN werk_core.permissions AS permission ON permission.id=role_permission.permission_id
			WHERE role.tenant_id=$1::uuid AND role.access_plane='work' AND permission.access_plane='work'
			ORDER BY permission.owning_module, permission.permission_key
		`, tenantID.String())
		if err != nil {
			return err
		}
		for bindingRows.Next() {
			var roleID string
			var permission WorkPermissionView
			if err := bindingRows.Scan(&roleID, &permission.ID, &permission.PermissionKey, &permission.DisplayName, &permission.OwningModule, &permission.RiskLevel); err != nil {
				bindingRows.Close()
				return err
			}
			if index, ok := roleIndexes[roleID]; ok {
				catalog.Roles[index].Permissions = append(catalog.Roles[index].Permissions, permission)
			}
		}
		if err := bindingRows.Err(); err != nil {
			bindingRows.Close()
			return err
		}
		bindingRows.Close()

		_, err = tx.Exec(ctx, `
			INSERT INTO werk_core.security_audit_events (
				id,event_type,outcome,account_id,tenant_id,request_id,correlation_id,details
			) VALUES (
				$1::uuid,'authorization.work-role.catalog-listed.v1','succeeded',$2::uuid,$3::uuid,
				$4::uuid,$5::uuid,jsonb_build_object('role_count',$6::integer,'permission_count',$7::integer)
			)
		`, auditID, formatUUID(actor.AccountID), tenantID.String(), requestID, correlationID, len(catalog.Roles), len(catalog.Permissions))
		return err
	})
	return catalog, err
}

func (service *Service) CreateWorkRole(ctx context.Context, input CreateWorkRoleInput, actor identity.AuthenticatedActor, requestID, correlationID string) (WorkRoleView, error) {
	tenantID, err := tenancy.ParseTenantID(input.TenantID)
	roleKey := strings.TrimSpace(input.RoleKey)
	displayName := strings.TrimSpace(input.DisplayName)
	permissionKeys := uniqueStrings(input.PermissionKeys)
	if err != nil || !workRoleKeyPattern.MatchString(roleKey) || len(roleKey) > 120 || displayName == "" || len(displayName) > 160 || len(permissionKeys) == 0 || len(permissionKeys) > 100 {
		return WorkRoleView{}, errors.New("invalid work role")
	}
	ids := make([]string, 3)
	for index := range ids {
		ids[index], err = randomUUID()
		if err != nil {
			return WorkRoleView{}, err
		}
	}
	roleID, auditID, eventID := ids[0], ids[1], ids[2]
	view := WorkRoleView{ID: roleID, TenantID: tenantID.String(), RoleKey: roleKey, DisplayName: displayName, Status: "active", Version: 1, Permissions: make([]WorkPermissionView, 0)}
	err = service.database.WithinTenantWrite(ctx, tenantID, func(ctx context.Context, tx database.TenantTx) error {
		var tenantAvailable bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM werk_core.tenants WHERE id=$1::uuid AND status='active')`, tenantID.String()).Scan(&tenantAvailable); err != nil || !tenantAvailable {
			return errors.New("tenant unavailable")
		}
		rows, err := tx.Query(ctx, `
			SELECT id::text, permission_key, display_name, owning_module, risk_level
			FROM werk_core.permissions
			WHERE access_plane='work' AND status='active' AND permission_key=ANY($1::text[])
			ORDER BY owning_module, permission_key
		`, permissionKeys)
		if err != nil {
			return err
		}
		for rows.Next() {
			var permission WorkPermissionView
			if err := rows.Scan(&permission.ID, &permission.PermissionKey, &permission.DisplayName, &permission.OwningModule, &permission.RiskLevel); err != nil {
				rows.Close()
				return err
			}
			view.Permissions = append(view.Permissions, permission)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		if len(view.Permissions) != len(permissionKeys) {
			return errors.New("work permission unavailable")
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO werk_core.roles (id,tenant_id,role_key,display_name,access_plane,system_role)
			VALUES ($1::uuid,$2::uuid,$3,$4,'work',false)
		`, roleID, tenantID.String(), roleKey, displayName); err != nil {
			return err
		}
		for _, permission := range view.Permissions {
			if _, err := tx.Exec(ctx, `INSERT INTO werk_core.role_permissions (role_id,permission_id) VALUES ($1::uuid,$2::uuid)`, roleID, permission.ID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO werk_core.security_audit_events (
				id,event_type,outcome,account_id,tenant_id,request_id,correlation_id,details
			) VALUES (
				$1::uuid,'authorization.work-role.created.v1','succeeded',$2::uuid,$3::uuid,$4::uuid,$5::uuid,
				jsonb_build_object('role_id',$6::text,'role_key',$7::text,'permission_keys',to_jsonb($8::text[]))
			)
		`, auditID, formatUUID(actor.AccountID), tenantID.String(), requestID, correlationID, roleID, roleKey, permissionKeys); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO werk_core.outbox_events (
				id,tenant_id,event_type,producer,subject_kind,subject_id,partition_key,occurred_at,correlation_id,payload
			) VALUES (
				$1::uuid,$2::uuid,'core.authorization.work-role-created.v1','core.authorization',
				'core.authorization.work-role',$3::uuid,$3,$4,$5::uuid,
				jsonb_build_object('role_id',$3::text,'role_key',$6::text,'permission_keys',to_jsonb($7::text[]))
			)
		`, eventID, tenantID.String(), roleID, service.now(), correlationID, roleKey, permissionKeys)
		return err
	})
	if err != nil {
		return WorkRoleView{}, err
	}
	return view, nil
}

func (service *Service) UpdateWorkRole(ctx context.Context, roleID string, expectedVersion uint64, input UpdateWorkRoleInput, actor identity.AuthenticatedActor, requestID, correlationID string) (WorkRoleView, error) {
	tenantID, err := tenancy.ParseTenantID(input.TenantID)
	displayName := strings.TrimSpace(input.DisplayName)
	status := strings.TrimSpace(input.Status)
	permissionKeys := uniqueStrings(input.PermissionKeys)
	if err != nil || !validUUIDString(roleID) || expectedVersion == 0 || displayName == "" || len(displayName) > 160 ||
		(status != "active" && status != "retired") || len(permissionKeys) == 0 || len(permissionKeys) > 100 {
		return WorkRoleView{}, errors.New("invalid work role update")
	}
	ids := make([]string, 2)
	for index := range ids {
		ids[index], err = randomUUID()
		if err != nil {
			return WorkRoleView{}, err
		}
	}
	view := WorkRoleView{
		ID: roleID, TenantID: tenantID.String(), DisplayName: displayName,
		Status: status, Permissions: make([]WorkPermissionView, 0),
	}
	err = service.database.WithinTenantWrite(ctx, tenantID, func(ctx context.Context, tx database.TenantTx) error {
		var previousDisplayName, previousStatus string
		var currentVersion uint64
		if err := tx.QueryRow(ctx, `
			SELECT role_key, system_role, display_name, status, version
			FROM werk_core.roles
			WHERE id=$1::uuid AND tenant_id=$2::uuid AND access_plane='work'
			FOR UPDATE
		`, roleID, tenantID.String()).Scan(&view.RoleKey, &view.SystemRole, &previousDisplayName, &previousStatus, &currentVersion); err != nil {
			return ErrNotFound
		}
		if view.SystemRole {
			return ErrImmutable
		}
		if currentVersion != expectedVersion {
			return ErrVersionConflict
		}
		var previousPermissionKeys []string
		if err := tx.QueryRow(ctx, `
			SELECT COALESCE(array_agg(permission.permission_key ORDER BY permission.permission_key), ARRAY[]::text[])
			FROM werk_core.role_permissions AS mapping
			JOIN werk_core.permissions AS permission ON permission.id=mapping.permission_id
			WHERE mapping.role_id=$1::uuid
		`, roleID).Scan(&previousPermissionKeys); err != nil {
			return err
		}
		permissionRows, err := tx.Query(ctx, `
			SELECT id::text, permission_key, display_name, owning_module, risk_level
			FROM werk_core.permissions
			WHERE access_plane='work' AND status='active' AND permission_key=ANY($1::text[])
			ORDER BY owning_module, permission_key
		`, permissionKeys)
		if err != nil {
			return err
		}
		for permissionRows.Next() {
			var permission WorkPermissionView
			if err := permissionRows.Scan(&permission.ID, &permission.PermissionKey, &permission.DisplayName, &permission.OwningModule, &permission.RiskLevel); err != nil {
				permissionRows.Close()
				return err
			}
			view.Permissions = append(view.Permissions, permission)
		}
		if err := permissionRows.Err(); err != nil {
			permissionRows.Close()
			return err
		}
		permissionRows.Close()
		if len(view.Permissions) != len(permissionKeys) {
			return errors.New("work permission unavailable")
		}
		if err := tx.QueryRow(ctx, `
			UPDATE werk_core.roles
			SET display_name=$3,status=$4,updated_at=$5,version=version+1
			WHERE id=$1::uuid AND tenant_id=$2::uuid AND access_plane='work'
			  AND NOT system_role AND version=$6
			RETURNING version
		`, roleID, tenantID.String(), displayName, status, service.now(), expectedVersion).Scan(&view.Version); err != nil {
			return ErrVersionConflict
		}
		if _, err := tx.Exec(ctx, `DELETE FROM werk_core.role_permissions WHERE role_id=$1::uuid`, roleID); err != nil {
			return err
		}
		for _, permission := range view.Permissions {
			if _, err := tx.Exec(ctx, `
				INSERT INTO werk_core.role_permissions (role_id,permission_id)
				VALUES ($1::uuid,$2::uuid)
			`, roleID, permission.ID); err != nil {
				return err
			}
		}
		if err := tx.QueryRow(ctx, `
			SELECT count(*)::integer
			FROM werk_core.role_assignments
			WHERE role_id=$1::uuid AND valid_from <= $2
			  AND (valid_until IS NULL OR valid_until > $2)
		`, roleID, service.now()).Scan(&view.AssignmentCount); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO werk_core.security_audit_events (
				id,event_type,outcome,account_id,tenant_id,request_id,correlation_id,details
			) VALUES (
				$1::uuid,'authorization.work-role.updated.v1','succeeded',$2::uuid,$3::uuid,$4::uuid,$5::uuid,
				jsonb_build_object(
					'role_id',$6::text,'role_key',$7::text,
					'previous',jsonb_build_object('display_name',$8::text,'status',$9::text,'version',$10::bigint,'permission_keys',to_jsonb($11::text[])),
					'current',jsonb_build_object('display_name',$12::text,'status',$13::text,'version',$14::bigint,'permission_keys',to_jsonb($15::text[]))
				)
			)
		`, ids[0], formatUUID(actor.AccountID), tenantID.String(), requestID, correlationID,
			roleID, view.RoleKey, previousDisplayName, previousStatus, expectedVersion, previousPermissionKeys,
			view.DisplayName, view.Status, view.Version, permissionKeys); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO werk_core.outbox_events (
				id,tenant_id,event_type,producer,subject_kind,subject_id,partition_key,occurred_at,correlation_id,payload
			) VALUES (
				$1::uuid,$2::uuid,'core.authorization.work-role-updated.v1','core.authorization',
				'core.authorization.work-role',$3::uuid,$3,$4,$5::uuid,
				jsonb_build_object('role_id',$3::text,'role_key',$6::text,'status',$7::text,'version',$8::bigint,'permission_keys',to_jsonb($9::text[]))
			)
		`, ids[1], tenantID.String(), roleID, service.now(), correlationID, view.RoleKey, view.Status, view.Version, permissionKeys)
		return err
	})
	if err != nil {
		return WorkRoleView{}, err
	}
	return view, nil
}

func (service *Service) ReplaceWorkUserRoles(ctx context.Context, accountID string, input ReplaceWorkUserRolesInput, actor identity.AuthenticatedActor, requestID, correlationID string) ([]string, error) {
	tenantID, err := tenancy.ParseTenantID(input.TenantID)
	roleIDs := uniqueStrings(input.RoleIDs)
	if err != nil || !validUUIDString(accountID) || len(roleIDs) > 100 {
		return nil, errors.New("invalid work role assignment")
	}
	for _, roleID := range roleIDs {
		if !validUUIDString(roleID) {
			return nil, errors.New("invalid work role assignment")
		}
	}
	ids := make([]string, 2+len(roleIDs))
	for index := range ids {
		ids[index], err = randomUUID()
		if err != nil {
			return nil, err
		}
	}
	auditID, eventID := ids[0], ids[1]
	roleKeys := make([]string, 0, len(roleIDs))
	err = service.database.WithinTenantWrite(ctx, tenantID, func(ctx context.Context, tx database.TenantTx) error {
		var accountAvailable bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM werk_core.accounts
				WHERE id=$1::uuid AND account_class='work' AND tenant_id=$2::uuid AND status='active'
			)
		`, accountID, tenantID.String()).Scan(&accountAvailable); err != nil || !accountAvailable {
			return errors.New("work account unavailable")
		}
		rows, err := tx.Query(ctx, `
			SELECT id::text, role_key
			FROM werk_core.roles
			WHERE id=ANY($1::uuid[]) AND tenant_id=$2::uuid AND access_plane='work' AND status='active'
			ORDER BY role_key
		`, roleIDs, tenantID.String())
		if err != nil {
			return err
		}
		validRoleIDs := make([]string, 0, len(roleIDs))
		for rows.Next() {
			var roleID, roleKey string
			if err := rows.Scan(&roleID, &roleKey); err != nil {
				rows.Close()
				return err
			}
			validRoleIDs = append(validRoleIDs, roleID)
			roleKeys = append(roleKeys, roleKey)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		if len(validRoleIDs) != len(roleIDs) {
			return errors.New("work role unavailable")
		}
		now := service.now()
		if _, err := tx.Exec(ctx, `
			UPDATE werk_core.role_assignments
			SET valid_until=CASE WHEN valid_from < $3 THEN $3 ELSE valid_from + interval '1 microsecond' END
			WHERE account_id=$1::uuid AND access_plane='work' AND scope_tenant_id=$2::uuid
			  AND valid_from <= $3 AND (valid_until IS NULL OR valid_until > $3)
			  AND NOT (role_id=ANY($4::uuid[]))
		`, accountID, tenantID.String(), now, validRoleIDs); err != nil {
			return err
		}
		for index, roleID := range validRoleIDs {
			if _, err := tx.Exec(ctx, `
				INSERT INTO werk_core.role_assignments (
					id,account_id,role_id,access_plane,scope_type,scope_tenant_id,granted_by_account_id,valid_from
				)
				SELECT $1::uuid,$2::uuid,$3::uuid,'work','tenant',$4::uuid,$5::uuid,$6
				WHERE NOT EXISTS (
					SELECT 1 FROM werk_core.role_assignments
					WHERE account_id=$2::uuid AND role_id=$3::uuid AND access_plane='work'
					  AND scope_type='tenant' AND scope_tenant_id=$4::uuid
					  AND valid_from <= $6 AND (valid_until IS NULL OR valid_until > $6)
				)
			`, ids[index+2], accountID, roleID, tenantID.String(), formatUUID(actor.AccountID), now); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO werk_core.security_audit_events (
				id,event_type,outcome,account_id,tenant_id,request_id,correlation_id,details
			) VALUES (
				$1::uuid,'authorization.work-role.assignments-replaced.v1','succeeded',$2::uuid,$3::uuid,$4::uuid,$5::uuid,
				jsonb_build_object('target_account_id',$6::text,'role_ids',to_jsonb($7::text[]),'role_keys',to_jsonb($8::text[]))
			)
		`, auditID, formatUUID(actor.AccountID), tenantID.String(), requestID, correlationID, accountID, validRoleIDs, roleKeys); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO werk_core.outbox_events (
				id,tenant_id,event_type,producer,subject_kind,subject_id,partition_key,occurred_at,correlation_id,payload
			) VALUES (
				$1::uuid,$2::uuid,'core.authorization.work-role-assignments-replaced.v1','core.authorization',
				'core.identity.work-account',$3::uuid,$3,$4,$5::uuid,
				jsonb_build_object('account_id',$3::text,'role_ids',to_jsonb($6::text[]),'role_keys',to_jsonb($7::text[]))
			)
		`, eventID, tenantID.String(), accountID, now, correlationID, validRoleIDs, roleKeys)
		return err
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(roleKeys)
	return roleKeys, nil
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func validUUIDString(value string) bool {
	compact := strings.ReplaceAll(strings.TrimSpace(value), "-", "")
	decoded, err := hex.DecodeString(compact)
	return err == nil && len(decoded) == 16
}
