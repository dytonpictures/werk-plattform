package auditexport

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/dytonpictures/werk/internal/platform/database"
	"github.com/dytonpictures/werk/internal/platform/kafkastream"
	"github.com/dytonpictures/werk/internal/platform/runtimepoll"
)

var ErrNoAuditAvailable = errors.New("no security audit event available")

const (
	leaseDuration      = 2 * time.Minute
	maximumErrorLength = 2000
	maximumIdlePoll    = 2 * time.Second
	claimBatchSize     = 4
)

type Claim struct {
	Record      kafkastream.AuditRecord
	Attempts    int
	MaxAttempts int
}

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
	claims, err := store.ClaimBatch(ctx, workerID, 1)
	if err != nil {
		return kafkastream.AuditRecord{}, 0, 0, err
	}
	claim := claims[0]
	return claim.Record, claim.Attempts, claim.MaxAttempts, nil
}

func (store *Store) ClaimBatch(ctx context.Context, workerID string, limit int) ([]Claim, error) {
	if strings.TrimSpace(workerID) == "" || len(workerID) > 120 {
		return nil, errors.New("worker ID is invalid")
	}
	if limit < 1 || limit > claimBatchSize {
		return nil, errors.New("audit claim batch size is invalid")
	}
	claimedAt := store.now()
	claims := make([]Claim, 0, limit)
	err := store.database.WithinGlobalWrite(ctx, func(ctx context.Context, tx database.TenantTx) error {
		rows, err := tx.Query(ctx, `
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
				LIMIT $4
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
		`, claimedAt, workerID, claimedAt.Add(leaseDuration), limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var claim Claim
			var accountID pgtype.UUID
			var tenantID pgtype.UUID
			if err := rows.Scan(
				&claim.Record.ID, &claim.Record.OccurredAt, &claim.Record.EventType, &claim.Record.Outcome,
				&accountID, &tenantID, &claim.Record.RequestID, &claim.Record.CorrelationID,
				&claim.Attempts, &claim.MaxAttempts,
			); err != nil {
				return err
			}
			if accountID.Valid {
				value := accountID.Bytes
				claim.Record.AccountID = &value
			}
			if tenantID.Valid {
				value := tenantID.Bytes
				claim.Record.TenantID = &value
			}
			claims = append(claims, claim)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	if len(claims) == 0 {
		return nil, ErrNoAuditAvailable
	}
	return claims, nil
}

func (store *Store) Release(ctx context.Context, claims []Claim, workerID string) error {
	if len(claims) == 0 {
		return nil
	}
	ids := make([]string, len(claims))
	for index, claim := range claims {
		ids[index] = uuidString(claim.Record.ID)
	}
	return store.database.WithinGlobalWrite(ctx, func(ctx context.Context, tx database.TenantTx) error {
		command, err := tx.Exec(ctx, `
			UPDATE werk_core.security_audit_export_queue
			SET status = CASE WHEN attempts <= 1 THEN 'pending' ELSE 'retry' END,
				attempts = GREATEST(attempts - 1, 0), available_at = $3,
				lease_owner = NULL, lease_expires_at = NULL, last_error = NULL
			WHERE audit_event_id = ANY($1::uuid[]) AND status = 'processing' AND lease_owner = $2
		`, ids, workerID, store.now())
		if err != nil {
			return err
		}
		if command.RowsAffected() != int64(len(ids)) {
			return errors.New("audit batch release lost a lease")
		}
		return nil
	})
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
	wakeups     <-chan struct{}
}

func NewRuntime(store *Store, publisher Publisher, logger *slog.Logger, workerID string, concurrency int, wakeups ...<-chan struct{}) (*Runtime, error) {
	if store == nil || publisher == nil || logger == nil || strings.TrimSpace(workerID) == "" || concurrency < 1 || concurrency > 16 {
		return nil, errors.New("invalid audit export runtime configuration")
	}
	runtime := &Runtime{
		store: store, publisher: publisher, logger: logger,
		workerID: workerID, concurrency: concurrency, pollDelay: 500 * time.Millisecond,
	}
	if len(wakeups) > 0 {
		runtime.wakeups = wakeups[0]
	}
	return runtime, nil
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
	waiter := runtimepoll.NewWaiter(runtime.pollDelay, maximumIdlePoll, workerID)
	defer waiter.Close()
	for ctx.Err() == nil {
		claims, err := runtime.store.ClaimBatch(ctx, workerID, claimBatchSize)
		if err != nil {
			idle := errors.Is(err, ErrNoAuditAvailable)
			if !idle && ctx.Err() == nil {
				runtime.logger.WarnContext(ctx, "security audit export claim failed", "worker", workerID, "error", err)
			}
			if !waiter.Wait(ctx, idle, runtime.wakeups) {
				return
			}
			continue
		}
		waiter.Reset()
		for index, claim := range claims {
			if ctx.Err() != nil {
				runtime.releaseClaims(claims[index:], workerID)
				return
			}
			record := claim.Record
			if err = runtime.publisher.PublishAudit(ctx, record); err != nil {
				if ctx.Err() != nil {
					runtime.releaseClaims(claims[index:], workerID)
					return
				}
				if transitionErr := runtime.store.Fail(ctx, record.ID, workerID, claim.Attempts, claim.MaxAttempts, err); transitionErr != nil {
					runtime.logger.ErrorContext(ctx, "security audit export failure transition failed", "error", transitionErr)
				}
				continue
			}
			if err := runtime.store.Complete(ctx, record.ID, workerID); err != nil {
				runtime.logger.ErrorContext(ctx, "security audit export completion failed", "error", err)
			}
		}
	}
}

func (runtime *Runtime) releaseClaims(claims []Claim, workerID string) {
	releaseContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := runtime.store.Release(releaseContext, claims, workerID); err != nil {
		runtime.logger.Error("audit shutdown claim release failed", "error", err)
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
