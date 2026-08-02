package adminstore

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/tenancy"
	"github.com/dytonpictures/werk/internal/platform/database"
)

const (
	workUserStatusActive   = "active"
	workUserStatusDisabled = "disabled"
)

type UpdateWorkUserStatusInput struct {
	Status string `json:"status"`
}

type WorkUserStatusView struct {
	AccountID string `json:"account_id"`
	TenantID  string `json:"tenant_id"`
	Status    string `json:"status"`
	Version   uint64 `json:"version"`
}

type WorkUserSessionRevocationView struct {
	AccountID string `json:"account_id"`
	TenantID  string `json:"tenant_id"`
	Version   uint64 `json:"version"`
}

// RevokeWorkUserSessions advances the authoritative session generation. This
// invalidates every current session without making a cache the source of truth.
func (service *Service) RevokeWorkUserSessions(ctx context.Context, tenantIDValue, accountIDValue string, expectedVersion uint64, actor identity.AuthenticatedActor, requestID, correlationID string) (WorkUserSessionRevocationView, error) {
	tenantID, err := tenancy.ParseTenantID(strings.TrimSpace(tenantIDValue))
	accountID := strings.ToLower(strings.TrimSpace(accountIDValue))
	if err != nil || !validCanonicalUUID(accountID) || expectedVersion == 0 {
		return WorkUserSessionRevocationView{}, errors.New("invalid work user session revocation")
	}
	view := WorkUserSessionRevocationView{AccountID: accountID, TenantID: tenantID.String()}
	err = service.database.WithinTenantWrite(ctx, tenantID, func(ctx context.Context, tx database.TenantTx) error {
		auditID, err := randomUUID()
		if err != nil {
			return err
		}
		eventID, err := randomUUID()
		if err != nil {
			return err
		}
		changedAt := service.now().UTC()
		if err := tx.QueryRow(ctx, `
			UPDATE werk_core.accounts
			SET session_generation=session_generation+1, updated_at=$3, version=version+1
			WHERE id=$1::uuid AND tenant_id=$2::uuid AND account_class='work' AND version=$4
			RETURNING version
		`, accountID, tenantID.String(), changedAt, expectedVersion).Scan(&view.Version); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				var exists bool
				if lookupErr := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM werk_core.accounts WHERE id=$1::uuid AND tenant_id=$2::uuid AND account_class='work')`, accountID, tenantID.String()).Scan(&exists); lookupErr != nil {
					return lookupErr
				}
				if !exists {
					return ErrNotFound
				}
				return ErrVersionConflict
			}
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO werk_core.security_audit_events (id,event_type,outcome,account_id,tenant_id,request_id,correlation_id,details)
			VALUES ($1::uuid,'identity.work-account-sessions-revoked.v1','succeeded',$2::uuid,$3::uuid,$4::uuid,$5::uuid,
			jsonb_build_object('target_account_id',$6::text,'previous_version',$7::bigint,'current_version',$8::bigint))
		`, auditID, formatUUID(actor.AccountID), tenantID.String(), requestID, correlationID, accountID, expectedVersion, view.Version); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO werk_core.outbox_events (id,tenant_id,event_type,producer,subject_kind,subject_id,partition_key,occurred_at,correlation_id,payload)
			VALUES ($1::uuid,$2::uuid,'core.identity.work-account-sessions-revoked.v1','core.identity','core.identity.work-account',$3::uuid,$3,$4,$5::uuid,
			jsonb_build_object('account_id',$3::text,'version',$6::bigint))
		`, eventID, tenantID.String(), accountID, changedAt, correlationID, view.Version)
		return err
	})
	return view, err
}

// UpdateWorkUserStatus changes only the lifecycle state of an existing,
// tenant-bound work account. Its tenant, person, login and memberships remain
// immutable through this command. A real state transition advances the
// session generation so every previously issued session fails closed.
func (service *Service) UpdateWorkUserStatus(
	ctx context.Context,
	tenantIDValue string,
	accountIDValue string,
	expectedVersion uint64,
	input UpdateWorkUserStatusInput,
	actor identity.AuthenticatedActor,
	requestID string,
	correlationID string,
) (WorkUserStatusView, error) {
	tenantID, accountID, status, err := normalizeWorkUserStatusUpdate(
		tenantIDValue, accountIDValue, expectedVersion, input,
	)
	if err != nil {
		return WorkUserStatusView{}, err
	}

	view := WorkUserStatusView{
		AccountID: accountID,
		TenantID:  tenantID.String(),
		Status:    status,
	}
	err = service.database.WithinTenantWrite(ctx, tenantID, func(ctx context.Context, tx database.TenantTx) error {
		var previousStatus string
		var invitationPending bool
		if err := tx.QueryRow(ctx, `
			SELECT account.status, account.version, EXISTS (
				SELECT 1
				FROM werk_core.identity_account_invitations AS invitation
				WHERE invitation.account_id=account.id
				  AND invitation.tenant_id=account.tenant_id
				  AND invitation.purpose='initial-activation'
				  AND invitation.consumed_at IS NULL
				  AND invitation.revoked_at IS NULL
			)
			FROM werk_core.accounts AS account
			WHERE account.id=$1::uuid AND account.tenant_id=$2::uuid
			  AND account.account_class='work'
			FOR UPDATE OF account
		`, accountID, tenantID.String()).Scan(&previousStatus, &view.Version, &invitationPending); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if view.Version != expectedVersion {
			return ErrVersionConflict
		}
		if !validWorkUserStatusTransition(previousStatus, status) {
			return ErrImmutable
		}
		if invitationPending {
			return ErrImmutable
		}
		if previousStatus == status {
			return nil
		}

		auditID, err := randomUUID()
		if err != nil {
			return err
		}
		eventID, err := randomUUID()
		if err != nil {
			return err
		}
		changedAt := service.now().UTC()
		if err := tx.QueryRow(ctx, `
			UPDATE werk_core.accounts
			SET status=$3, session_generation=session_generation+1,
			    updated_at=$4, version=version+1
			WHERE id=$1::uuid AND tenant_id=$2::uuid AND account_class='work'
			  AND version=$5
			RETURNING version
		`, accountID, tenantID.String(), status, changedAt, expectedVersion).Scan(&view.Version); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrVersionConflict
			}
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO werk_core.security_audit_events (
				id,event_type,outcome,account_id,tenant_id,request_id,correlation_id,details
			) VALUES (
				$1::uuid,'identity.work-account-status-updated.v1','succeeded',
				$2::uuid,$3::uuid,$4::uuid,$5::uuid,
				jsonb_build_object(
					'target_account_id',$6::text,
					'previous_status',$7::text,
					'current_status',$8::text,
					'previous_version',$9::bigint,
					'current_version',$10::bigint
				)
			)
		`, auditID, formatUUID(actor.AccountID), tenantID.String(), requestID, correlationID,
			accountID, previousStatus, status, expectedVersion, view.Version); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO werk_core.outbox_events (
				id,tenant_id,event_type,producer,subject_kind,subject_id,
				partition_key,occurred_at,correlation_id,payload
			) VALUES (
				$1::uuid,$2::uuid,'core.identity.work-account-status-updated.v1','core.identity',
				'core.identity.work-account',$3::uuid,$3,$4,$5::uuid,
				jsonb_build_object('account_id',$3::text,'status',$6::text,'version',$7::bigint)
			)
		`, eventID, tenantID.String(), accountID, changedAt, correlationID, status, view.Version)
		return err
	})
	if err != nil {
		return WorkUserStatusView{}, err
	}
	return view, nil
}

func normalizeWorkUserStatusUpdate(
	tenantIDValue string,
	accountIDValue string,
	expectedVersion uint64,
	input UpdateWorkUserStatusInput,
) (tenancy.TenantID, string, string, error) {
	tenantID, err := tenancy.ParseTenantID(strings.TrimSpace(tenantIDValue))
	accountID := strings.ToLower(strings.TrimSpace(accountIDValue))
	status := strings.TrimSpace(input.Status)
	if err != nil || !validCanonicalUUID(accountID) || expectedVersion == 0 || !validWorkUserStatus(status) {
		return tenancy.TenantID{}, "", "", errors.New("invalid work user status update")
	}
	return tenantID, accountID, status, nil
}

func validWorkUserStatus(status string) bool {
	return status == workUserStatusActive || status == workUserStatusDisabled
}

func validWorkUserStatusTransition(previousStatus, status string) bool {
	return validWorkUserStatus(previousStatus) && validWorkUserStatus(status)
}
