package auditexport

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/dytonpictures/werk/internal/platform/database"
	"github.com/dytonpictures/werk/internal/platform/kafkastream"
)

var ErrNoAuditAvailable = errors.New("no security audit event available")

const (
	leaseDuration      = 2 * time.Minute
	maximumErrorLength = 2000
)

type Publisher interface {
	PublishAudit(context.Context, kafkastream.AuditRecord) error
}

type Store struct {
	database *database.WorkerDB
	now      func() time.Time
}

func NewStore(workerDatabase *database.WorkerDB) (*Store, error) {
	if workerDatabase == nil {
		return nil, errors.New("worker database is required")
	}
	return &Store{database: workerDatabase, now: func() time.Time { return time.Now().UTC() }}, nil
}

func (store *Store) Claim(ctx context.Context, workerID string) (kafkastream.AuditRecord, int, int, error) {
	if strings.TrimSpace(workerID) == "" || len(workerID) > 120 {
		return kafkastream.AuditRecord{}, 0, 0, errors.New("worker ID is invalid")
	}
	var record kafkastream.AuditRecord
	var accountID pgtype.UUID
	var tenantID pgtype.UUID
	var attempts int
	var maxAttempts int
	err := store.database.WithinGlobalWrite(ctx, func(ctx context.Context, tx database.TenantTx) error {
		return tx.QueryRow(ctx, `
			WITH candidate AS (
				SELECT queue.audit_event_id
				FROM werk_core.security_audit_export_queue AS queue
				JOIN werk_core.security_audit_events AS audit ON audit.id = queue.audit_event_id
				WHERE (
					(queue.status IN ('pending', 'retry') AND queue.available_at <= $1)
					OR (queue.status = 'processing' AND queue.lease_expires_at <= $1)
				)
				AND NOT EXISTS (
					SELECT 1
					FROM werk_core.security_audit_export_queue AS earlier_queue
					JOIN werk_core.security_audit_events AS earlier_audit
					  ON earlier_audit.id = earlier_queue.audit_event_id
					WHERE earlier_audit.tenant_id IS NOT DISTINCT FROM audit.tenant_id
					  AND (earlier_audit.occurred_at, earlier_audit.id) < (audit.occurred_at, audit.id)
					  AND earlier_queue.status NOT IN ('completed', 'dead')
				)
				ORDER BY queue.available_at, audit.occurred_at, audit.id
				FOR UPDATE OF queue SKIP LOCKED
				LIMIT 1
			)
			UPDATE werk_core.security_audit_export_queue AS queue
			SET status = 'processing', attempts = attempts + 1,
				lease_owner = $2, lease_expires_at = $3, last_error = NULL
			FROM candidate, werk_core.security_audit_events AS audit
			WHERE queue.audit_event_id = candidate.audit_event_id
			  AND audit.id = queue.audit_event_id
			RETURNING audit.id, audit.occurred_at, audit.event_type, audit.outcome,
				audit.account_id, audit.tenant_id, audit.request_id, audit.correlation_id,
				queue.attempts, queue.max_attempts
		`, store.now(), workerID, store.now().Add(leaseDuration)).Scan(
			&record.ID, &record.OccurredAt, &record.EventType, &record.Outcome,
			&accountID, &tenantID, &record.RequestID, &record.CorrelationID,
			&attempts, &maxAttempts,
		)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return kafkastream.AuditRecord{}, 0, 0, ErrNoAuditAvailable
		}
		return kafkastream.AuditRecord{}, 0, 0, err
	}
	if accountID.Valid {
		value := accountID.Bytes
		record.AccountID = &value
	}
	if tenantID.Valid {
		value := tenantID.Bytes
		record.TenantID = &value
	}
	return record, attempts, maxAttempts, nil
}

func (store *Store) Complete(ctx context.Context, eventID [16]byte, workerID string) error {
	return store.transition(ctx, eventID, workerID, "completed", store.now(), "")
}

func (store *Store) Fail(ctx context.Context, eventID [16]byte, workerID string, attempts, maxAttempts int, cause error) error {
	status := "retry"
	availableAt := store.now().Add(retryDelay(attempts))
	if attempts >= maxAttempts {
		status = "dead"
		availableAt = store.now()
	}
	message := "audit export failed"
	if cause != nil {
		message = cause.Error()
	}
	if len(message) > maximumErrorLength {
		message = message[:maximumErrorLength]
	}
	return store.transition(ctx, eventID, workerID, status, availableAt, message)
}

func (store *Store) transition(ctx context.Context, eventID [16]byte, workerID, status string, availableAt time.Time, message string) error {
	return store.database.WithinGlobalWrite(ctx, func(ctx context.Context, tx database.TenantTx) error {
		command, err := tx.Exec(ctx, `
			UPDATE werk_core.security_audit_export_queue
			SET status = $3::text, available_at = $4::timestamptz, lease_owner = NULL,
				lease_expires_at = NULL, last_error = NULLIF($5::text, ''),
				completed_at = CASE WHEN $3::text = 'completed' THEN $4::timestamptz ELSE NULL END
			WHERE audit_event_id = $1::uuid AND status = 'processing' AND lease_owner = $2
		`, uuidString(eventID), workerID, status, availableAt, message)
		if err != nil {
			return err
		}
		if command.RowsAffected() != 1 {
			return errors.New("audit export lease was lost")
		}
		return nil
	})
}

type Runtime struct {
	store       *Store
	publisher   Publisher
	logger      *slog.Logger
	workerID    string
	concurrency int
	pollDelay   time.Duration
}

func NewRuntime(store *Store, publisher Publisher, logger *slog.Logger, workerID string, concurrency int) (*Runtime, error) {
	if store == nil || publisher == nil || logger == nil || strings.TrimSpace(workerID) == "" || concurrency < 1 || concurrency > 16 {
		return nil, errors.New("invalid audit export runtime configuration")
	}
	return &Runtime{
		store: store, publisher: publisher, logger: logger,
		workerID: workerID, concurrency: concurrency, pollDelay: 500 * time.Millisecond,
	}, nil
}

func (runtime *Runtime) Run(ctx context.Context) {
	var workers sync.WaitGroup
	for index := 0; index < runtime.concurrency; index++ {
		workers.Add(1)
		go func(slot int) {
			defer workers.Done()
			runtime.runSlot(ctx, slot)
		}(index)
	}
	workers.Wait()
}

func (runtime *Runtime) runSlot(ctx context.Context, slot int) {
	workerID := fmt.Sprintf("%s-audit-%d", runtime.workerID, slot)
	for ctx.Err() == nil {
		record, attempts, maxAttempts, err := runtime.store.Claim(ctx, workerID)
		if err != nil {
			if !errors.Is(err, ErrNoAuditAvailable) && ctx.Err() == nil {
				runtime.logger.WarnContext(ctx, "security audit export claim failed", "worker", workerID, "error", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(runtime.pollDelay):
				continue
			}
		}
		if err = runtime.publisher.PublishAudit(ctx, record); err != nil {
			if transitionErr := runtime.store.Fail(ctx, record.ID, workerID, attempts, maxAttempts, err); transitionErr != nil {
				runtime.logger.ErrorContext(ctx, "security audit export failure transition failed", "error", transitionErr)
			}
			continue
		}
		if err := runtime.store.Complete(ctx, record.ID, workerID); err != nil {
			runtime.logger.ErrorContext(ctx, "security audit export completion failed", "error", err)
		}
	}
}

func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := time.Second << min(attempt-1, 9)
	if delay > 5*time.Minute {
		return 5 * time.Minute
	}
	return delay
}

func uuidString(value [16]byte) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
}
