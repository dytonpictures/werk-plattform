package adminstore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/tenancy"
	"github.com/dytonpictures/werk/internal/platform/database"
)

var (
	ErrNotFound          = errors.New("admin resource not found")
	ErrVersionConflict   = errors.New("admin resource version conflict")
	ErrImmutable         = errors.New("admin resource is immutable")
	ErrInvalidAuditQuery = errors.New("invalid security audit query")
)

type Service struct {
	database *database.AdminDB
	now      func() time.Time
}

type CreateWorkUserInput struct {
	TenantID             string `json:"tenant_id"`
	OrganizationalUnitID string `json:"organizational_unit_id"`
	GivenName            string `json:"given_name"`
	FamilyName           string `json:"family_name"`
	LoginName            string `json:"login_name"`
	TemporaryPassword    string `json:"temporary_password"`
	MembershipType       string `json:"membership_type"`
}

type WorkUserView struct {
	AccountID          string `json:"account_id"`
	PartyID            string `json:"party_id"`
	TenantID           string `json:"tenant_id"`
	LoginName          string `json:"login_name"`
	MustChangePassword bool   `json:"must_change_password"`
}

type WorkUserDirectoryEntry struct {
	AccountID              string   `json:"account_id"`
	PartyID                string   `json:"party_id"`
	TenantID               string   `json:"tenant_id"`
	DisplayName            string   `json:"display_name"`
	LoginName              string   `json:"login_name"`
	Status                 string   `json:"status"`
	MustChangePassword     bool     `json:"must_change_password"`
	OrganizationalUnitID   string   `json:"organizational_unit_id"`
	OrganizationalUnitName string   `json:"organizational_unit_name"`
	MembershipType         string   `json:"membership_type"`
	Roles                  []string `json:"roles"`
}

func New(db *database.AdminDB) (*Service, error) {
	if db == nil {
		return nil, errors.New("admin database is required")
	}
	return &Service{database: db, now: func() time.Time { return time.Now().UTC() }}, nil
}

func (service *Service) CreateWorkUser(ctx context.Context, input CreateWorkUserInput, actor identity.AuthenticatedActor, requestID, correlationID string) (WorkUserView, error) {
	tenantID, err := tenancy.ParseTenantID(input.TenantID)
	if err != nil || strings.TrimSpace(input.GivenName) == "" || strings.TrimSpace(input.FamilyName) == "" || strings.TrimSpace(input.LoginName) == "" || len(input.TemporaryPassword) < 12 || strings.TrimSpace(input.MembershipType) == "" {
		return WorkUserView{}, errors.New("invalid work user")
	}
	unitID, err := tenancy.ParseUnitID(input.OrganizationalUnitID)
	if err != nil {
		return WorkUserView{}, errors.New("invalid organizational unit")
	}
	passwordHash, err := identity.HashPassword(input.TemporaryPassword)
	if err != nil {
		return WorkUserView{}, err
	}
	ids := make([]string, 7)
	for index := range ids {
		ids[index], err = randomUUID()
		if err != nil {
			return WorkUserView{}, err
		}
	}
	partyID, accountID, membershipID, roleID, assignmentID, auditID, eventID := ids[0], ids[1], ids[2], ids[3], ids[4], ids[5], ids[6]
	view := WorkUserView{AccountID: accountID, PartyID: partyID, TenantID: tenantID.String(), LoginName: strings.ToLower(strings.TrimSpace(input.LoginName)), MustChangePassword: true}
	err = service.database.WithinTenantWrite(ctx, tenantID, func(ctx context.Context, tx database.TenantTx) error {
		var valid bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM werk_core.tenants WHERE id=$1::uuid AND status='active') AND EXISTS(SELECT 1 FROM werk_core.organizational_units WHERE id=$2::uuid AND tenant_id=$1::uuid AND status='active')`, tenantID.String(), unitID.String()).Scan(&valid); err != nil || !valid {
			return errors.New("tenant or organizational unit unavailable")
		}
		if _, err := tx.Exec(ctx, `INSERT INTO werk_core.parties (id,tenant_id,party_type,display_name,status) VALUES ($1::uuid,$2::uuid,'person',$3,'active')`, partyID, tenantID.String(), strings.TrimSpace(input.GivenName)+" "+strings.TrimSpace(input.FamilyName)); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO werk_core.persons (party_id,tenant_id,given_name,family_name) VALUES ($1::uuid,$2::uuid,$3,$4)`, partyID, tenantID.String(), strings.TrimSpace(input.GivenName), strings.TrimSpace(input.FamilyName)); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO werk_core.memberships (id,tenant_id,party_id,organizational_unit_id,membership_type,valid_from) VALUES ($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5,$6)`, membershipID, tenantID.String(), partyID, unitID.String(), strings.TrimSpace(input.MembershipType), service.now()); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO werk_core.accounts (id,account_class,tenant_id,person_party_id,login_name,status,must_change_password) VALUES ($1::uuid,'work',$2::uuid,$3::uuid,$4,'active',true)`, accountID, tenantID.String(), partyID, view.LoginName); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO werk_core.account_credentials (account_id,credential_kind,secret_hash,assurance) VALUES ($1::uuid,'password',$2,'single-factor')`, accountID, passwordHash); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO werk_core.account_identity_bindings (account_id,provider_key,provider_subject) VALUES ($1::uuid,'local',$1::uuid::text)`, accountID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO werk_core.roles (id,tenant_id,role_key,display_name,access_plane,system_role) VALUES ($1::uuid,$2::uuid,'workspace-member','Workspace-Mitglied','work',true) ON CONFLICT (tenant_id,access_plane,role_key) DO UPDATE SET display_name=EXCLUDED.display_name RETURNING id::text`, roleID, tenantID.String()).Scan(&roleID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO werk_core.role_permissions (role_id,permission_id) SELECT $1::uuid,id FROM werk_core.permissions WHERE permission_key='core.workspace.access' ON CONFLICT DO NOTHING`, roleID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO werk_core.role_assignments (id,account_id,role_id,access_plane,scope_type,scope_tenant_id,granted_by_account_id,valid_from) VALUES ($1::uuid,$2::uuid,$3::uuid,'work','tenant',$4::uuid,$5::uuid,$6)`, assignmentID, accountID, roleID, tenantID.String(), formatUUID(actor.AccountID), service.now()); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO werk_core.security_audit_events (id,event_type,outcome,account_id,tenant_id,request_id,correlation_id,details) VALUES ($1::uuid,'identity.work-account.created.v1','succeeded',$2::uuid,$3::uuid,$4::uuid,$5::uuid,jsonb_build_object('created_account_id',$6::text,'organizational_unit_id',$7::text))`, auditID, formatUUID(actor.AccountID), tenantID.String(), requestID, correlationID, accountID, unitID.String()); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO werk_core.outbox_events (id,tenant_id,event_type,producer,subject_kind,subject_id,partition_key,occurred_at,correlation_id,payload) VALUES ($1::uuid,$2::uuid,'core.identity.work-account-created.v1','core.identity','core.identity.work-account',$3::uuid,$3,$4,$5::uuid,jsonb_build_object('account_id',$3::text,'party_id',$6::text))`, eventID, tenantID.String(), accountID, service.now(), correlationID, partyID)
		return err
	})
	if err != nil {
		return WorkUserView{}, err
	}
	return view, nil
}

func (service *Service) ListWorkUsers(ctx context.Context, tenantIDValue string, actor identity.AuthenticatedActor, requestID, correlationID string) ([]WorkUserDirectoryEntry, error) {
	tenantID, err := tenancy.ParseTenantID(tenantIDValue)
	if err != nil {
		return nil, errors.New("invalid tenant")
	}
	auditID, err := randomUUID()
	if err != nil {
		return nil, err
	}
	entries := make([]WorkUserDirectoryEntry, 0)
	err = service.database.WithinTenantWrite(ctx, tenantID, func(ctx context.Context, tx database.TenantTx) error {
		rows, err := tx.Query(ctx, `
			SELECT account.id::text, party.id::text, account.tenant_id::text,
			       party.display_name, account.login_name, account.status,
			       account.must_change_password,
			       COALESCE(membership.organizational_unit_id::text, ''),
			       COALESCE(unit.name, ''), COALESCE(membership.membership_type, ''),
			       ARRAY(
			         SELECT role.role_key
			         FROM werk_core.role_assignments AS assignment
			         JOIN werk_core.roles AS role ON role.id=assignment.role_id
			         WHERE assignment.account_id=account.id
			           AND assignment.access_plane='work' AND role.status='active'
			           AND assignment.valid_from <= $2
			           AND (assignment.valid_until IS NULL OR assignment.valid_until > $2)
			         ORDER BY role.role_key
			       )
			FROM werk_core.accounts AS account
			JOIN werk_core.parties AS party
			  ON party.id=account.person_party_id AND party.tenant_id=account.tenant_id
			LEFT JOIN LATERAL (
			  SELECT candidate.organizational_unit_id, candidate.membership_type
			  FROM werk_core.memberships AS candidate
			  WHERE candidate.party_id=party.id AND candidate.tenant_id=party.tenant_id
			    AND candidate.valid_from <= $2
			    AND (candidate.valid_until IS NULL OR candidate.valid_until > $2)
			  ORDER BY candidate.valid_from DESC, candidate.id
			  LIMIT 1
			) AS membership ON true
			LEFT JOIN werk_core.organizational_units AS unit
			  ON unit.id=membership.organizational_unit_id AND unit.tenant_id=account.tenant_id
			WHERE account.account_class='work' AND account.tenant_id=$1::uuid
			ORDER BY party.display_name, account.login_name
			LIMIT 200
		`, tenantID.String(), service.now())
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var entry WorkUserDirectoryEntry
			if err := rows.Scan(
				&entry.AccountID, &entry.PartyID, &entry.TenantID,
				&entry.DisplayName, &entry.LoginName, &entry.Status,
				&entry.MustChangePassword, &entry.OrganizationalUnitID,
				&entry.OrganizationalUnitName, &entry.MembershipType, &entry.Roles,
			); err != nil {
				return err
			}
			entries = append(entries, entry)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO werk_core.security_audit_events (
				id,event_type,outcome,account_id,tenant_id,request_id,correlation_id,details
			) VALUES (
				$1::uuid,'identity.work-account.listed.v1','succeeded',$2::uuid,$3::uuid,
				$4::uuid,$5::uuid,jsonb_build_object('result_count',$6::integer)
			)
		`, auditID, formatUUID(actor.AccountID), tenantID.String(), requestID, correlationID, len(entries))
		return err
	})
	return entries, err
}

func randomUUID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x70
	value[8] = (value[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(value[:])
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:], nil
}
func formatUUID(value identity.AccountID) string {
	encoded := hex.EncodeToString(value[:])
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:]
}
