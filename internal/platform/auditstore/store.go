// Package auditstore persists Core business audit entries in the caller's
// existing tenant transaction. It does not open or commit transactions itself.
package auditstore

import (
	"context"
	"errors"
	"fmt"

	coreaudit "github.com/dytonpictures/werk/internal/core/audit"
	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/platform/database"
)

var ErrInvalidExecutor = errors.New("invalid business audit executor")

// Executor is created from the already authenticated workload actor. Its
// fields are intentionally private so Append cannot accept an executor ID from
// command data or from the freely assembled audit entry.
type Executor struct {
	ref coreaudit.ActorRef
}

func NewExecutor(actor identity.AuthenticatedActor) (Executor, error) {
	if identity.AuthorizeAccessPlane(actor, identity.AccessPlaneService) != nil ||
		actor.TenantID == nil || actor.TenantID.IsZero() {
		return Executor{}, ErrInvalidExecutor
	}
	return Executor{ref: coreaudit.ActorRef{
		AccountID: actor.AccountID, AccountClass: actor.AccountClass, TenantID: *actor.TenantID,
	}}, nil
}

// Append writes an immutable audit record into the same transaction as the
// protected business mutation. account_id remains populated as the compatible
// single-actor projection while the structured fields keep both real actors.
func Append(ctx context.Context, transaction database.TenantTx, executor Executor, entry coreaudit.Entry) error {
	if transaction == nil || executor.ref.AccountID.IsZero() || executor.ref.TenantID.IsZero() {
		return coreaudit.ErrInvalidEntry
	}
	entry.ExecutedBy = executor.ref
	if entry.Validate() != nil {
		return coreaudit.ErrInvalidEntry
	}

	var activityKey any
	var purposeKey any
	var legalBasisRef any
	if entry.Policy.ProcessingRequired {
		activityKey = entry.Policy.Processing.ActivityKey
		purposeKey = entry.Policy.Processing.PurposeKey
		legalBasisRef = entry.Policy.Processing.LegalBasisRef
	}

	_, err := transaction.Exec(ctx, `
		INSERT INTO werk_core.security_audit_events (
			id, tenant_id, occurred_at, event_type, action_key, outcome,
			account_id, initiated_by_account_id, executed_by_account_id,
			subject_boundary, subject_tenant_id, subject_kind, subject_id,
			permission_key, policy_contract_version, processing_required,
			processing_activity_key, processing_purpose_key, legal_basis_ref,
			request_id, correlation_id
		) VALUES (
			$1::uuid, $2::uuid, $3, $4, $5, $6,
			$7::uuid, $7::uuid, $8::uuid,
			$9, $2::uuid, $10, $11,
			$12, $13, $14, $15, $16, $17,
			$18::uuid, $19::uuid
		)
	`, uuidString(entry.ID), entry.TenantID.String(), entry.OccurredAt, entry.EventType,
		entry.Action, string(entry.Outcome), uuidString(entry.InitiatedBy.AccountID),
		uuidString(entry.ExecutedBy.AccountID), string(entry.Subject.Boundary),
		string(entry.Subject.Kind), entry.Subject.ID, entry.Policy.Permission,
		entry.Policy.ContractVersion, entry.Policy.ProcessingRequired,
		activityKey, purposeKey, legalBasisRef, uuidString(entry.RequestID),
		uuidString(entry.CorrelationID))
	return err
}

func uuidString(value [16]byte) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
}
