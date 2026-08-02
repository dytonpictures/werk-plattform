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
	"github.com/dytonpictures/werk/internal/platform/businessobjectstore"
	"github.com/dytonpictures/werk/internal/platform/database"
)

var (
	ErrNotFound                                  = errors.New("admin resource not found")
	ErrVersionConflict                           = errors.New("admin resource version conflict")
	ErrImmutable                                 = errors.New("admin resource is immutable")
	ErrOrganizationalUnitReferenced              = errors.New("organizational unit is referenced")
	ErrOrganizationalUnitInheritedAccessConflict = errors.New("organizational unit reparenting changes inherited access")
	ErrOrganizationalUnitDepthConflict           = errors.New("organizational unit hierarchy exceeds maximum depth")
	ErrInvalidAuditQuery                         = errors.New("invalid security audit query")
)

type Service struct {
	database          *database.AdminDB
	businessObjects   *businessobjectstore.Publisher
	now               func() time.Time
	runtime           RuntimeConfiguration
	runtimeConfigured bool
}

type CreateWorkUserInput struct {
	TenantID                 string `json:"tenant_id"`
	OrganizationalUnitID     string `json:"organizational_unit_id"`
	GivenName                string `json:"given_name"`
	FamilyName               string `json:"family_name"`
	LoginName                string `json:"login_name"`
	MembershipType           string `json:"membership_type"`
	OnboardingMethod         string `json:"onboarding_method"`
	InitialPassword          string `json:"initial_password"`
	RequirePasswordChange    *bool  `json:"require_password_change"`
	InvitationEmail          string `json:"invitation_email"`
	InvitationExpiresInHours int    `json:"invitation_expires_in_hours"`
}

type WorkUserView struct {
	AccountID          string                  `json:"account_id"`
	PartyID            string                  `json:"party_id"`
	TenantID           string                  `json:"tenant_id"`
	LoginName          string                  `json:"login_name"`
	MustChangePassword bool                    `json:"must_change_password"`
	OnboardingMethod   string                  `json:"onboarding_method"`
	Invitation         *WorkUserInvitationView `json:"invitation,omitempty"`
}

type WorkUserInvitationView struct {
	RecipientEmail string    `json:"recipient_email"`
	DeliveryMethod string    `json:"delivery_method"`
	ActivationPath string    `json:"activation_path"`
	ExpiresAt      time.Time `json:"expires_at"`
}

type WorkUserDirectoryEntry struct {
	AccountID              string     `json:"account_id"`
	PartyID                string     `json:"party_id"`
	TenantID               string     `json:"tenant_id"`
	DisplayName            string     `json:"display_name"`
	LoginName              string     `json:"login_name"`
	Status                 string     `json:"status"`
	Version                uint64     `json:"version"`
	MustChangePassword     bool       `json:"must_change_password"`
	OrganizationalUnitID   string     `json:"organizational_unit_id"`
	OrganizationalUnitName string     `json:"organizational_unit_name"`
	MembershipType         string     `json:"membership_type"`
	Roles                  []string   `json:"roles"`
	InvitationPending      bool       `json:"invitation_pending"`
	InvitationExpiresAt    *time.Time `json:"invitation_expires_at,omitempty"`
}

func New(db *database.AdminDB, options ...Option) (*Service, error) {
	if db == nil {
		return nil, errors.New("admin database is required")
	}
	service := &Service{
		database:        db,
		businessObjects: businessobjectstore.NewPublisher(),
		now:             func() time.Time { return time.Now().UTC() },
	}
	for _, option := range options {
		if option == nil {
			return nil, errors.New("admin service option is required")
		}
		if err := option(service); err != nil {
			return nil, err
		}
	}
	return service, nil
}

func (service *Service) CreateWorkUser(ctx context.Context, input CreateWorkUserInput, actor identity.AuthenticatedActor, requestID, correlationID string) (WorkUserView, error) {
	tenantID, err := tenancy.ParseTenantID(input.TenantID)
	if err != nil || strings.TrimSpace(input.GivenName) == "" || strings.TrimSpace(input.FamilyName) == "" || strings.TrimSpace(input.LoginName) == "" || strings.TrimSpace(input.MembershipType) == "" {
		return WorkUserView{}, errors.New("invalid work user")
	}
	unitID, err := tenancy.ParseUnitID(input.OrganizationalUnitID)
	if err != nil {
		return WorkUserView{}, errors.New("invalid organizational unit")
	}
	recipientEmail, mustChangePassword, err := validateWorkUserOnboarding(input)
	if err != nil {
		return WorkUserView{}, errors.New("invalid work user onboarding")
	}
	onboardingMethod := strings.TrimSpace(input.OnboardingMethod)
	accountStatus := "active"
	var passwordHash []byte
	var invitationToken string
	var invitationDigest [32]byte
	var invitationExpiresAt time.Time
	if onboardingMethod == OnboardingInitialPassword {
		passwordHash, err = identity.HashPassword(input.InitialPassword)
		if err != nil {
			return WorkUserView{}, err
		}
	} else {
		accountStatus = "disabled"
		invitationToken, invitationDigest, err = newInvitationToken()
		if err != nil {
			return WorkUserView{}, err
		}
	}
	ids := make([]string, 7)
	for index := range ids {
		ids[index], err = randomUUID()
		if err != nil {
			return WorkUserView{}, err
		}
	}
	partyID, accountID, membershipID, roleID, assignmentID, auditID, eventID := ids[0], ids[1], ids[2], ids[3], ids[4], ids[5], ids[6]
	var invitationID string
	if onboardingMethod == OnboardingInvitationLink {
		invitationID, err = randomUUID()
		if err != nil {
			return WorkUserView{}, err
		}
	}
	view := WorkUserView{
		AccountID: accountID, PartyID: partyID, TenantID: tenantID.String(),
		LoginName:          strings.ToLower(strings.TrimSpace(input.LoginName)),
		MustChangePassword: mustChangePassword, OnboardingMethod: onboardingMethod,
	}
	if onboardingMethod == OnboardingInvitationLink {
		view.Invitation = &WorkUserInvitationView{
			RecipientEmail: recipientEmail, DeliveryMethod: InvitationDeliveryManual,
			ActivationPath: "/activate#token=" + invitationToken,
		}
	}
	err = service.database.WithinTenantWrite(ctx, tenantID, func(ctx context.Context, tx database.TenantTx) error {
		var tenantActive bool
		if err := tx.QueryRow(ctx, `
			SELECT status='active'
			FROM werk_core.tenants
			WHERE id=$1::uuid
			FOR KEY SHARE
		`, tenantID.String()).Scan(&tenantActive); err != nil || !tenantActive {
			return errors.New("tenant or organizational unit unavailable")
		}
		// Topology mutations take the tenant row before organizational-unit
		// rows. Taking the matching key-share locks in the same order prevents
		// both a stale membership insert and a tenant/unit lock inversion.
		var unitActive bool
		if err := tx.QueryRow(ctx, `
			SELECT status='active'
			FROM werk_core.organizational_units
			WHERE id=$1::uuid AND tenant_id=$2::uuid
			FOR KEY SHARE
		`, unitID.String(), tenantID.String()).Scan(&unitActive); err != nil || !unitActive {
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
		if _, err := tx.Exec(ctx, `INSERT INTO werk_core.accounts (id,account_class,tenant_id,person_party_id,login_name,status,must_change_password) VALUES ($1::uuid,'work',$2::uuid,$3::uuid,$4,$5,$6)`, accountID, tenantID.String(), partyID, view.LoginName, accountStatus, mustChangePassword); err != nil {
			return err
		}
		if onboardingMethod == OnboardingInitialPassword {
			if _, err := tx.Exec(ctx, `INSERT INTO werk_core.account_credentials (account_id,credential_kind,secret_hash,assurance) VALUES ($1::uuid,'password',$2,'single-factor')`, accountID, passwordHash); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO werk_core.account_identity_bindings (account_id,provider_key,provider_subject) VALUES ($1::uuid,'local',$1::uuid::text)`, accountID); err != nil {
			return err
		}
		if onboardingMethod == OnboardingInvitationLink {
			if err := tx.QueryRow(ctx, `INSERT INTO werk_core.identity_account_invitations (id,tenant_id,account_id,purpose,token_hash,created_by_account_id,expires_at) VALUES ($1::uuid,$2::uuid,$3::uuid,'initial-activation',$4,$5::uuid,statement_timestamp() + ($6::integer * interval '1 hour')) RETURNING expires_at`, invitationID, tenantID.String(), accountID, invitationDigest[:], formatUUID(actor.AccountID), input.InvitationExpiresInHours).Scan(&invitationExpiresAt); err != nil {
				return err
			}
			view.Invitation.ExpiresAt = invitationExpiresAt
		}
		if _, err := tx.Exec(ctx, `INSERT INTO werk_core.roles (id,tenant_id,role_key,display_name,access_plane,system_role) VALUES ($1::uuid,$2::uuid,'workspace-member','Workspace-Mitglied','work',true) ON CONFLICT (tenant_id,access_plane,role_key) DO NOTHING`, roleID, tenantID.String()); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT id::text FROM werk_core.roles WHERE tenant_id=$1::uuid AND access_plane='work' AND role_key='workspace-member' AND system_role AND status='active'`, tenantID.String()).Scan(&roleID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO werk_core.role_permissions (role_id,permission_id) SELECT $1::uuid,id FROM werk_core.permissions WHERE permission_key='core.workspace.access' ON CONFLICT DO NOTHING`, roleID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO werk_core.role_assignments (id,account_id,role_id,access_plane,scope_type,scope_tenant_id,granted_by_account_id,valid_from) VALUES ($1::uuid,$2::uuid,$3::uuid,'work','tenant',$4::uuid,$5::uuid,$6)`, assignmentID, accountID, roleID, tenantID.String(), formatUUID(actor.AccountID), service.now()); err != nil {
			return err
		}
		var invitationAuditID any
		var invitationAuditExpiry any
		var passwordChangeAudit any
		if onboardingMethod == OnboardingInvitationLink {
			invitationAuditID = invitationID
			invitationAuditExpiry = invitationExpiresAt
		} else {
			passwordChangeAudit = mustChangePassword
		}
		if _, err := tx.Exec(ctx, `INSERT INTO werk_core.security_audit_events (id,event_type,outcome,account_id,tenant_id,request_id,correlation_id,details) VALUES ($1::uuid,'identity.work-account.created.v1','succeeded',$2::uuid,$3::uuid,$4::uuid,$5::uuid,jsonb_strip_nulls(jsonb_build_object('created_account_id',$6::text,'organizational_unit_id',$7::text,'onboarding_method',$8::text,'require_password_change',$9::boolean,'invitation_id',$10::text,'invitation_expires_at',$11::timestamptz)))`, auditID, formatUUID(actor.AccountID), tenantID.String(), requestID, correlationID, accountID, unitID.String(), onboardingMethod, passwordChangeAudit, invitationAuditID, invitationAuditExpiry); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO werk_core.outbox_events (id,tenant_id,event_type,producer,subject_kind,subject_id,partition_key,occurred_at,correlation_id,payload) VALUES ($1::uuid,$2::uuid,'core.identity.work-account-created.v1','core.identity','core.identity.work-account',$3::uuid,$3,$4,$5::uuid,jsonb_build_object('account_id',$3::text,'party_id',$6::text,'onboarding_method',$7::text,'must_change_password',$8::boolean))`, eventID, tenantID.String(), accountID, service.now(), correlationID, partyID, onboardingMethod, mustChangePassword)
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
			       account.version, account.must_change_password,
			       COALESCE(membership.organizational_unit_id::text, ''),
			       COALESCE(unit.name, ''), COALESCE(membership.membership_type, ''),
			       ARRAY(
			         SELECT role.role_key
			         FROM werk_core.role_assignments AS assignment
			         JOIN werk_core.roles AS role ON role.id=assignment.role_id
				         WHERE assignment.account_id=account.id
				           AND assignment.access_plane='work'
				           AND assignment.scope_type='tenant'
				           AND assignment.scope_tenant_id=account.tenant_id
				           AND assignment.scope_id IS NULL
				           AND role.status='active'
			           AND assignment.valid_from <= $2
			           AND (assignment.valid_until IS NULL OR assignment.valid_until > $2)
			         ORDER BY role.role_key
			       ),
			       invitation.expires_at IS NOT NULL,
			       invitation.expires_at
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
			LEFT JOIN LATERAL (
			  SELECT candidate.expires_at
			  FROM werk_core.identity_account_invitations AS candidate
			  WHERE candidate.account_id=account.id AND candidate.tenant_id=account.tenant_id
			    AND candidate.purpose='initial-activation'
			    AND candidate.consumed_at IS NULL AND candidate.revoked_at IS NULL
			  LIMIT 1
			) AS invitation ON true
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
				&entry.Version, &entry.MustChangePassword, &entry.OrganizationalUnitID,
				&entry.OrganizationalUnitName, &entry.MembershipType, &entry.Roles,
				&entry.InvitationPending, &entry.InvitationExpiresAt,
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
