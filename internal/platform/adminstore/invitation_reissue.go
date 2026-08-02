package adminstore

import (
	"context"
	"errors"
	"net/mail"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/tenancy"
	"github.com/dytonpictures/werk/internal/platform/database"
)

var ErrInvalidWorkUserInvitationReissue = errors.New("invalid work user invitation reissue")

type ReissueWorkUserInvitationInput struct {
	RecipientEmail string `json:"recipient_email"`
	ExpiresInHours int    `json:"expires_in_hours"`
}

// ReissueWorkUserInvitation replaces an existing initial activation ceremony.
// The raw token and the draft recipient remain response-only; PostgreSQL sees
// only the token digest and the non-secret invitation coordinates.
func (service *Service) ReissueWorkUserInvitation(
	ctx context.Context,
	tenantIDValue string,
	accountIDValue string,
	input ReissueWorkUserInvitationInput,
	actor identity.AuthenticatedActor,
	requestID string,
	correlationID string,
) (WorkUserInvitationView, error) {
	tenantID, accountID, recipient, expiresInHours, err := normalizeWorkUserInvitationReissue(
		tenantIDValue, accountIDValue, input,
	)
	if err != nil {
		return WorkUserInvitationView{}, err
	}

	token, tokenDigest, err := newInvitationToken()
	if err != nil {
		return WorkUserInvitationView{}, err
	}
	invitationID, err := randomUUID()
	if err != nil {
		return WorkUserInvitationView{}, err
	}
	auditID, err := randomUUID()
	if err != nil {
		return WorkUserInvitationView{}, err
	}
	eventID, err := randomUUID()
	if err != nil {
		return WorkUserInvitationView{}, err
	}

	view := WorkUserInvitationView{
		RecipientEmail: recipient,
		DeliveryMethod: InvitationDeliveryManual,
		ActivationPath: "/activate#token=" + token,
	}
	err = service.database.WithinTenantWrite(ctx, tenantID, func(ctx context.Context, tx database.TenantTx) error {
		if err := tx.QueryRow(ctx, `
			SELECT werk_security.replace_initial_work_account_invitation(
				$1::uuid,$2::uuid,$3::uuid,$4,$5::uuid,$6::integer
			)
		`, tenantID.String(), accountID, invitationID, tokenDigest[:], formatUUID(actor.AccountID), expiresInHours).Scan(&view.ExpiresAt); err != nil {
			return classifyWorkUserInvitationReissueError(err)
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO werk_core.security_audit_events (
				id,event_type,outcome,account_id,tenant_id,request_id,correlation_id,details
			) VALUES (
				$1::uuid,'identity.work-account-invitation.reissued.v1','succeeded',
				$2::uuid,$3::uuid,$4::uuid,$5::uuid,
				jsonb_build_object(
					'target_account_id',$6::text,
					'invitation_id',$7::text,
					'invitation_expires_at',$8::timestamptz
				)
			)
		`, auditID, formatUUID(actor.AccountID), tenantID.String(), requestID, correlationID,
			accountID, invitationID, view.ExpiresAt); err != nil {
			return err
		}

		_, err := tx.Exec(ctx, `
			INSERT INTO werk_core.outbox_events (
				id,tenant_id,event_type,producer,subject_kind,subject_id,
				partition_key,occurred_at,correlation_id,payload
			) VALUES (
				$1::uuid,$2::uuid,'core.identity.work-account-invitation-reissued.v1',
				'core.identity','core.identity.work-account',$3::uuid,$3,$4,$5::uuid,
				jsonb_build_object(
					'account_id',$3::text,
					'invitation_id',$6::text,
					'expires_at',$7::timestamptz
				)
			)
		`, eventID, tenantID.String(), accountID, service.now().UTC(), correlationID,
			invitationID, view.ExpiresAt)
		return err
	})
	if err != nil {
		return WorkUserInvitationView{}, err
	}
	return view, nil
}

func normalizeWorkUserInvitationReissue(
	tenantIDValue string,
	accountIDValue string,
	input ReissueWorkUserInvitationInput,
) (tenancy.TenantID, string, string, int, error) {
	tenantID, err := tenancy.ParseTenantID(strings.TrimSpace(tenantIDValue))
	accountID := strings.ToLower(strings.TrimSpace(accountIDValue))
	recipient := strings.TrimSpace(input.RecipientEmail)
	if err != nil || !validCanonicalUUID(accountID) || input.ExpiresInHours < 1 || input.ExpiresInHours > maximumInvitationHours {
		return tenancy.TenantID{}, "", "", 0, ErrInvalidWorkUserInvitationReissue
	}
	parsed, err := mail.ParseAddress(recipient)
	if err != nil || parsed.Name != "" || parsed.Address != recipient || len(recipient) > 320 {
		return tenancy.TenantID{}, "", "", 0, ErrInvalidWorkUserInvitationReissue
	}
	return tenantID, accountID, recipient, input.ExpiresInHours, nil
}

func classifyWorkUserInvitationReissueError(err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && (postgresError.Code == "23514" || postgresError.Code == "40001") {
		return ErrImmutable
	}
	return err
}
