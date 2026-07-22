package adminstore

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/tenancy"
	"github.com/dytonpictures/werk/internal/platform/database"
)

const (
	developmentTenantID      = "0196f000-0000-7000-8000-000000000401"
	developmentUnitID        = "0196f000-0000-7000-8000-000000000402"
	developmentPartyID       = "0196f000-0000-7000-8000-000000000403"
	developmentMembershipID  = "0196f000-0000-7000-8000-000000000404"
	developmentAccountID     = "0196f000-0000-7000-8000-000000000405"
	developmentRoleID        = "0196f000-0000-7000-8000-000000000406"
	developmentAssignmentID  = "0196f000-0000-7000-8000-000000000407"
	developmentAuditID       = "0196f000-0000-7000-8000-000000000408"
	developmentEventID       = "0196f000-0000-7000-8000-000000000409"
	developmentRequestID     = "0196f000-0000-7000-8000-00000000040a"
	developmentCorrelationID = "0196f000-0000-7000-8000-00000000040b"
	developmentLoginName     = "dev-worker@werk.local"
)

// EnsureDevelopmentWorkAccount creates a deterministic, idempotent local
// workspace only when the API is explicitly running in the development profile.
// It never resets an existing credential.
func (service *Service) EnsureDevelopmentWorkAccount(ctx context.Context, password string) error {
	if len(password) < 16 {
		return errors.New("development work password must contain at least 16 characters")
	}
	passwordHash, err := identity.HashPassword(password)
	if err != nil {
		return err
	}
	tenantID, err := tenancy.ParseTenantID(developmentTenantID)
	if err != nil {
		return err
	}
	return service.database.WithinTenantWrite(ctx, tenantID, func(ctx context.Context, tx database.TenantTx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_catalog.pg_advisory_xact_lock(24578472680768854)`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO werk_core.tenants (id,name,status,default_locale,default_timezone)
			VALUES ($1::uuid,'WERK Development','active','de-DE','Europe/Berlin')
			ON CONFLICT (id) DO NOTHING
		`, developmentTenantID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO werk_core.organizational_units (id,tenant_id,unit_type,name,status)
			VALUES ($1::uuid,$2::uuid,'team','Development Team','active')
			ON CONFLICT (id) DO NOTHING
		`, developmentUnitID, developmentTenantID); err != nil {
			return err
		}
		var accountClass, accountTenant, status string
		err := tx.QueryRow(ctx, `
			SELECT account_class, tenant_id::text, status
			FROM werk_core.accounts WHERE login_name=$1
		`, developmentLoginName).Scan(&accountClass, &accountTenant, &status)
		if err == nil {
			if accountClass != "work" || accountTenant != developmentTenantID || status != "active" {
				return errors.New("development work login is occupied by an incompatible account")
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO werk_core.parties (id,tenant_id,party_type,display_name,status) VALUES ($1::uuid,$2::uuid,'person','Dev Worker','active')`, developmentPartyID, developmentTenantID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO werk_core.persons (party_id,tenant_id,given_name,family_name) VALUES ($1::uuid,$2::uuid,'Dev','Worker')`, developmentPartyID, developmentTenantID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO werk_core.memberships (id,tenant_id,party_id,organizational_unit_id,membership_type,valid_from) VALUES ($1::uuid,$2::uuid,$3::uuid,$4::uuid,'team.member',$5)`, developmentMembershipID, developmentTenantID, developmentPartyID, developmentUnitID, service.now()); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO werk_core.accounts (id,account_class,tenant_id,person_party_id,login_name,status,must_change_password) VALUES ($1::uuid,'work',$2::uuid,$3::uuid,$4,'active',true)`, developmentAccountID, developmentTenantID, developmentPartyID, developmentLoginName); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO werk_core.account_credentials (account_id,credential_kind,secret_hash,assurance) VALUES ($1::uuid,'password',$2,'single-factor')`, developmentAccountID, passwordHash); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO werk_core.account_identity_bindings (account_id,provider_key,provider_subject) VALUES ($1::uuid,'local',$1::uuid::text)`, developmentAccountID); err != nil {
			return err
		}
		roleID := developmentRoleID
		if err := tx.QueryRow(ctx, `INSERT INTO werk_core.roles (id,tenant_id,role_key,display_name,access_plane,system_role) VALUES ($1::uuid,$2::uuid,'workspace-member','Workspace-Mitglied','work',true) ON CONFLICT (tenant_id,access_plane,role_key) DO UPDATE SET display_name=EXCLUDED.display_name RETURNING id::text`, roleID, developmentTenantID).Scan(&roleID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO werk_core.role_permissions (role_id,permission_id) SELECT $1::uuid,id FROM werk_core.permissions WHERE permission_key='core.workspace.access' ON CONFLICT DO NOTHING`, roleID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO werk_core.role_assignments (id,account_id,role_id,access_plane,scope_type,scope_tenant_id,valid_from) VALUES ($1::uuid,$2::uuid,$3::uuid,'work','tenant',$4::uuid,$5)`, developmentAssignmentID, developmentAccountID, roleID, developmentTenantID, service.now()); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO werk_core.security_audit_events (id,event_type,outcome,account_id,tenant_id,request_id,correlation_id,details) VALUES ($1::uuid,'identity.work-account.created.v1','succeeded',NULL,$2::uuid,$3::uuid,$4::uuid,jsonb_build_object('created_account_id',$5::text,'development_bootstrap',true))`, developmentAuditID, developmentTenantID, developmentRequestID, developmentCorrelationID, developmentAccountID); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO werk_core.outbox_events (id,tenant_id,event_type,producer,subject_kind,subject_id,partition_key,occurred_at,correlation_id,payload) VALUES ($1::uuid,$2::uuid,'core.identity.work-account-created.v1','core.identity','core.identity.work-account',$3::uuid,$3,$4,$5::uuid,jsonb_build_object('account_id',$3::text,'party_id',$6::text,'development_bootstrap',true))`, developmentEventID, developmentTenantID, developmentAccountID, service.now(), developmentCorrelationID, developmentPartyID)
		return err
	})
}
