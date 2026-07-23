package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/dytonpictures/werk/internal/core/events"
	"github.com/dytonpictures/werk/internal/core/tenancy"
	"github.com/dytonpictures/werk/internal/platform/database"
)

var ErrNoEventAvailable = errors.New("no outbox event available")

const (
	defaultLeaseDuration = 2 * time.Minute
	maximumErrorLength   = 2000
	AllEventTypes        = "*"
)

type Consumer interface {
	Key() string
	EventType() string
	Handle(context.Context, database.TenantTx, events.Event) error
}

type Registry struct {
	mu        sync.RWMutex
	consumers map[string][]Consumer
}

func NewRegistry() *Registry { return &Registry{consumers: make(map[string][]Consumer)} }

func (registry *Registry) Register(consumer Consumer) error {
	if consumer == nil || !events.ValidConsumerKey(consumer.Key()) {
		return events.ErrInvalidConsumer
	}
	if consumer.EventType() != AllEventTypes && !events.ValidEventType(consumer.EventType()) {
		return events.ErrInvalidConsumer
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	for _, registered := range registry.consumers[consumer.EventType()] {
		if registered.Key() == consumer.Key() {
			return events.ErrInvalidConsumer
		}
	}
	registry.consumers[consumer.EventType()] = append(registry.consumers[consumer.EventType()], consumer)
	return nil
}

func (registry *Registry) Consumers(eventType string) []Consumer {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	consumers := append([]Consumer(nil), registry.consumers[eventType]...)
	return append(consumers, registry.consumers[AllEventTypes]...)
}

// Enqueue writes the immutable fact into the caller's existing transaction.
// The caller must persist its business change through the same transaction.
func Enqueue(ctx context.Context, transaction database.TenantTx, event events.Event) error {
	if transaction == nil || event.Validate() != nil {
		return events.ErrInvalidEvent
	}
	var causation any
	if event.CausationID != nil {
		causation = uuidString(*event.CausationID)
	}
	tags, err := json.Marshal(events.NormalizeTags(event.Tags))
	if err != nil {
		return events.ErrInvalidEvent
	}
	_, err = transaction.Exec(ctx, `
		INSERT INTO werk_core.outbox_events (
			id, tenant_id, event_type, producer, subject_kind, subject_id,
			partition_key, occurred_at, correlation_id, causation_id, tags, payload
		) VALUES (
			$1::uuid, $2::uuid, $3, $4, $5, $6::uuid,
			$7, $8, $9::uuid, $10::uuid, $11::jsonb, $12::jsonb
		)
	`, uuidString(event.ID), event.TenantID.String(), event.Type, event.Producer,
		event.SubjectKind, uuidString(event.SubjectID), event.PartitionKey,
		event.OccurredAt, uuidString(event.CorrelationID), causation, string(tags), string(event.Payload))
	return err
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

func (store *Store) Claim(ctx context.Context, workerID string) (events.Event, int, int, error) {
	if strings.TrimSpace(workerID) == "" || len(workerID) > 120 {
		return events.Event{}, 0, 0, errors.New("worker ID is invalid")
	}
	var event events.Event
	var payload string
	var tags string
	var causation pgtype.UUID
	var attempts int
	var maxAttempts int
	err := store.database.WithinGlobalWrite(ctx, func(ctx context.Context, tx database.TenantTx) error {
		return tx.QueryRow(ctx, `
			WITH candidate AS (
				SELECT current.id
				FROM werk_core.outbox_events AS current
				WHERE (
					(current.status IN ('pending', 'retry') AND current.available_at <= $1)
					OR (current.status = 'processing' AND current.lease_expires_at <= $1)
				)
				AND NOT EXISTS (
					SELECT 1 FROM werk_core.outbox_events AS earlier
					WHERE earlier.tenant_id = current.tenant_id
					  AND earlier.partition_key = current.partition_key
					  AND (earlier.occurred_at, earlier.id) < (current.occurred_at, current.id)
					  AND earlier.status NOT IN ('completed', 'dead')
				)
				ORDER BY current.available_at, current.occurred_at, current.id
				FOR UPDATE SKIP LOCKED
				LIMIT 1
			)
			UPDATE werk_core.outbox_events AS event
			SET status = 'processing', attempts = attempts + 1,
				lease_owner = $2, lease_expires_at = $3, last_error = NULL
			FROM candidate
			WHERE event.id = candidate.id
			RETURNING event.id, event.tenant_id, event.event_type, event.producer,
				event.subject_kind, event.subject_id, event.partition_key,
				event.occurred_at, event.correlation_id, event.causation_id,
				event.tags::text, event.payload::text, event.attempts, event.max_attempts
		`, store.now(), workerID, store.now().Add(defaultLeaseDuration)).Scan(
			&event.ID, &event.TenantID, &event.Type, &event.Producer,
			&event.SubjectKind, &event.SubjectID, &event.PartitionKey,
			&event.OccurredAt, &event.CorrelationID, &causation,
			&tags, &payload, &attempts, &maxAttempts,
		)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return events.Event{}, 0, 0, ErrNoEventAvailable
		}
		return events.Event{}, 0, 0, err
	}
	if causation.Valid {
		value := causation.Bytes
		event.CausationID = &value
	}
	if err := json.Unmarshal([]byte(tags), &event.Tags); err != nil || events.ValidateTags(event.Tags) != nil {
		return events.Event{}, 0, 0, events.ErrInvalidEvent
	}
	event.Payload = []byte(payload)
	return event, attempts, maxAttempts, nil
}

func (store *Store) Process(ctx context.Context, event events.Event, consumer Consumer) error {
	return store.database.WithinTenantWrite(ctx, event.TenantID, func(ctx context.Context, tx database.TenantTx) error {
		var processed bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM werk_core.event_consumer_receipts
				WHERE event_id = $1::uuid AND consumer_key = $2
			)
		`, uuidString(event.ID), consumer.Key()).Scan(&processed); err != nil {
			return err
		}
		if processed {
			return nil
		}
		if err := consumer.Handle(ctx, tx, event); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO werk_core.event_consumer_receipts (tenant_id, event_id, consumer_key, processed_at)
			VALUES ($1::uuid, $2::uuid, $3, $4)
		`, event.TenantID.String(), uuidString(event.ID), consumer.Key(), store.now())
		return err
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
	message := "event processing failed"
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
			UPDATE werk_core.outbox_events
			SET status = $3::text, available_at = $4::timestamptz, lease_owner = NULL,
				lease_expires_at = NULL, last_error = NULLIF($5::text, ''),
				completed_at = CASE WHEN $3::text = 'completed' THEN $4::timestamptz ELSE NULL END
			WHERE id = $1::uuid AND status = 'processing' AND lease_owner = $2
		`, uuidString(eventID), workerID, status, availableAt, message)
		if err != nil {
			return err
		}
		if command.RowsAffected() != 1 {
			return errors.New("outbox lease was lost")
		}
		return nil
	})
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

type Runtime struct {
	store       *Store
	registry    *Registry
	logger      *slog.Logger
	workerID    string
	concurrency int
	pollDelay   time.Duration
}

func NewRuntime(store *Store, registry *Registry, logger *slog.Logger, workerID string, concurrency int) (*Runtime, error) {
	if store == nil || registry == nil || logger == nil || strings.TrimSpace(workerID) == "" || concurrency < 1 || concurrency > 128 {
		return nil, errors.New("invalid outbox runtime configuration")
	}
	return &Runtime{store: store, registry: registry, logger: logger, workerID: workerID, concurrency: concurrency, pollDelay: 500 * time.Millisecond}, nil
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
	workerID := fmt.Sprintf("%s-%d", runtime.workerID, slot)
	for ctx.Err() == nil {
		event, attempts, maxAttempts, err := runtime.store.Claim(ctx, workerID)
		if err != nil {
			if !errors.Is(err, ErrNoEventAvailable) && ctx.Err() == nil {
				runtime.logger.WarnContext(ctx, "outbox claim failed", "worker", workerID, "error", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(runtime.pollDelay):
				continue
			}
		}
		consumers := runtime.registry.Consumers(event.Type)
		if len(consumers) == 0 {
			err = errors.New("no consumer registered for " + event.Type)
		}
		for _, consumer := range consumers {
			if err = runtime.store.Process(ctx, event, consumer); err != nil {
				break
			}
		}
		if err != nil {
			if transitionErr := runtime.store.Fail(ctx, event.ID, workerID, attempts, maxAttempts, err); transitionErr != nil {
				runtime.logger.ErrorContext(ctx, "outbox failure transition failed", "error", transitionErr)
			}
			continue
		}
		if err := runtime.store.Complete(ctx, event.ID, workerID); err != nil {
			runtime.logger.ErrorContext(ctx, "outbox completion failed", "error", err)
		}
	}
}

func uuidString(value [16]byte) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
}

func TenantPartition(tenantID tenancy.TenantID, key string) string {
	return tenantID.String() + ":" + key
}
