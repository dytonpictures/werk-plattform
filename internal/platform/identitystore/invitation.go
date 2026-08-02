package identitystore

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/platform/database"
)

// AcceptInitialWorkAccountInvitation performs the one-time credential ceremony
// atomically in PostgreSQL. It intentionally returns no account data and does
// not issue a session; the activated account must authenticate normally.
func (service *Service) AcceptInitialWorkAccountInvitation(
	ctx context.Context,
	token string,
	newPassword string,
	requestID string,
	correlationID string,
) error {
	tokenHash, err := identity.HashInitialWorkAccountInvitationToken(token)
	if err != nil {
		return err
	}
	releaseHashSlot, acquired := invitationPasswordHashLimiter.acquire(ctx)
	if !acquired {
		return identity.ErrInitialWorkAccountInvitationBusy
	}
	defer releaseHashSlot()
	passwordHash, err := identity.HashPassword(newPassword)
	if err != nil {
		return err
	}
	credentialID, err := randomUUID()
	if err != nil {
		return fmt.Errorf("generate invitation credential ID: %w", err)
	}
	auditID, err := randomUUID()
	if err != nil {
		return fmt.Errorf("generate invitation audit ID: %w", err)
	}
	eventID, err := randomUUID()
	if err != nil {
		return fmt.Errorf("generate invitation event ID: %w", err)
	}

	err = service.database.WithinWrite(ctx, func(ctx context.Context, tx database.TenantTx) error {
		var accountID string
		var tenantID string
		return tx.QueryRow(ctx, `
			SELECT account_id::text, tenant_id::text
			FROM werk_security.accept_initial_work_account_invitation(
				$1, $2::uuid, $3, $4::uuid, $5::uuid, $6::uuid, $7::uuid
			)
		`, tokenHash[:], credentialID, passwordHash, auditID, eventID, requestID, correlationID).Scan(&accountID, &tenantID)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.ErrInitialWorkAccountInvitationInvalid
	}
	if err != nil {
		return fmt.Errorf("accept initial work account invitation: %w", err)
	}
	return nil
}
