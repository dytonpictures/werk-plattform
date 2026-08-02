package adminstore

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/operations"
	"github.com/dytonpictures/werk/internal/platform/database"
)

type OperationsInstallationView struct {
	Environment          string `json:"environment"`
	BuildVersion         string `json:"build_version"`
	APIVersion           string `json:"api_version"`
	APIUptimeSeconds     int64  `json:"api_uptime_seconds"`
	TransportSecurity    string `json:"transport_security"`
	DomainEventTransport string `json:"domain_event_transport"`
}

type OperationsComponentView struct {
	Key            string     `json:"key"`
	State          string     `json:"state"`
	Required       bool       `json:"required"`
	BuildVersion   string     `json:"build_version,omitempty"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	LastObservedAt *time.Time `json:"last_observed_at,omitempty"`
}

type OperationsQueueView struct {
	Key                 string     `json:"key"`
	State               string     `json:"state"`
	Pending             uint64     `json:"pending"`
	Processing          uint64     `json:"processing"`
	Retry               uint64     `json:"retry"`
	Dead                uint64     `json:"dead"`
	OldestOutstandingAt *time.Time `json:"oldest_outstanding_at,omitempty"`
}

type OperationsMigrationView struct {
	AppliedCount    uint64     `json:"applied_count"`
	LatestName      string     `json:"latest_name,omitempty"`
	LatestAppliedAt *time.Time `json:"latest_applied_at,omitempty"`
}

type OperationsExecutionView struct {
	ExecutorConnected bool   `json:"executor_connected"`
	RestartAvailable  bool   `json:"restart_available"`
	UpdateAvailable   bool   `json:"update_available"`
	Reason            string `json:"reason"`
}

type OperationsSummaryView struct {
	ObservedAt   time.Time                  `json:"observed_at"`
	Overall      string                     `json:"overall"`
	Installation OperationsInstallationView `json:"installation"`
	Components   []OperationsComponentView  `json:"components"`
	Queues       []OperationsQueueView      `json:"queues"`
	Migrations   OperationsMigrationView    `json:"migrations"`
	Execution    OperationsExecutionView    `json:"execution"`
}

type operationsSummaryRow struct {
	ObservedAt                time.Time
	WorkerBuildVersion        pgtype.Text
	WorkerStartedAt           pgtype.Timestamptz
	WorkerLastSeenAt          pgtype.Timestamptz
	WorkerExpiresAt           pgtype.Timestamptz
	OutboxPending             int64
	OutboxProcessing          int64
	OutboxRetry               int64
	OutboxDead                int64
	OutboxOldestOutstandingAt pgtype.Timestamptz
	AuditPending              int64
	AuditProcessing           int64
	AuditRetry                int64
	AuditDead                 int64
	AuditOldestOutstandingAt  pgtype.Timestamptz
	MigrationCount            int64
	LatestMigrationName       pgtype.Text
	LatestMigrationAppliedAt  pgtype.Timestamptz
	KafkaState                string
	KafkaObservedAt           pgtype.Timestamptz
}

func (service *Service) GetOperationsSummary(
	ctx context.Context,
	actor identity.AuthenticatedActor,
	requestID string,
	correlationID string,
) (OperationsSummaryView, error) {
	if service == nil || service.database == nil || !service.runtimeConfigured {
		return OperationsSummaryView{}, errors.New("operations summary is not configured")
	}
	auditID, err := randomUUID()
	if err != nil {
		return OperationsSummaryView{}, err
	}

	var row operationsSummaryRow
	var summary OperationsSummaryView
	err = service.database.WithinInstallationAuditRead(ctx, func(ctx context.Context, tx database.TenantTx) error {
		if err := tx.QueryRow(ctx, `
			SELECT observed_at,
			       worker_build_version, worker_started_at, worker_last_seen_at, worker_expires_at,
			       outbox_pending, outbox_processing, outbox_retry, outbox_dead,
			       outbox_oldest_outstanding_at,
			       audit_pending, audit_processing, audit_retry, audit_dead,
			       audit_oldest_outstanding_at,
			       migration_count, latest_migration_name, latest_migration_applied_at,
			       kafka_state, kafka_observed_at
			FROM werk_core.platform_operations_summary
		`).Scan(
			&row.ObservedAt,
			&row.WorkerBuildVersion, &row.WorkerStartedAt, &row.WorkerLastSeenAt, &row.WorkerExpiresAt,
			&row.OutboxPending, &row.OutboxProcessing, &row.OutboxRetry, &row.OutboxDead,
			&row.OutboxOldestOutstandingAt,
			&row.AuditPending, &row.AuditProcessing, &row.AuditRetry, &row.AuditDead,
			&row.AuditOldestOutstandingAt,
			&row.MigrationCount, &row.LatestMigrationName, &row.LatestMigrationAppliedAt,
			&row.KafkaState, &row.KafkaObservedAt,
		); err != nil {
			return err
		}

		var buildErr error
		summary, buildErr = buildOperationsSummary(row, service.runtime)
		if buildErr != nil {
			return buildErr
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO werk_core.security_audit_events (
				id, occurred_at, event_type, outcome, account_id, tenant_id,
				request_id, correlation_id, details
			) VALUES (
				$1::uuid, $2, 'core.platform.operations-summary-read.v1', 'succeeded',
				$3::uuid, NULL, $4::uuid, $5::uuid,
				jsonb_build_object('overall_state', $6::text)
			)
		`, auditID, service.now(), formatUUID(actor.AccountID), requestID, correlationID, summary.Overall)
		return err
	})
	if err != nil {
		return OperationsSummaryView{}, err
	}
	return summary, nil
}

func buildOperationsSummary(row operationsSummaryRow, runtime RuntimeConfiguration) (OperationsSummaryView, error) {
	if !validRuntimeConfiguration(runtime) || row.ObservedAt.IsZero() ||
		row.OutboxPending < 0 || row.OutboxProcessing < 0 || row.OutboxRetry < 0 || row.OutboxDead < 0 ||
		row.AuditPending < 0 || row.AuditProcessing < 0 || row.AuditRetry < 0 || row.AuditDead < 0 || row.MigrationCount < 0 {
		return OperationsSummaryView{}, operations.ErrInvalidSummary
	}
	observedAt := row.ObservedAt.UTC()
	workerActive := row.WorkerLastSeenAt.Valid && row.WorkerExpiresAt.Valid &&
		row.WorkerExpiresAt.Time.After(observedAt)
	workerState := operations.StateUnknown
	if workerActive {
		workerState = operations.StateReady
	}
	kafkaState := kafkaComponentState(runtime.KafkaEnabled, workerActive, row.KafkaState)

	domainState := deliveryQueueState(runtime.KafkaEnabled, kafkaState, row.OutboxRetry, row.OutboxDead)
	auditState := deliveryQueueState(runtime.KafkaEnabled, kafkaState, row.AuditRetry, row.AuditDead)
	overall := operations.StateReady
	if row.OutboxDead > 0 || row.AuditDead > 0 {
		overall = operations.StateAttention
	} else if row.OutboxRetry > 0 || row.AuditRetry > 0 ||
		(runtime.KafkaEnabled && (workerState != operations.StateReady || kafkaState != operations.StateReady)) {
		overall = operations.StateDegraded
	}

	uptime := observedAt.Sub(runtime.APIStartedAt.UTC())
	if uptime < 0 {
		uptime = 0
	}
	coreSummary := operations.Summary{
		ObservedAt: observedAt,
		Overall:    overall,
		Installation: operations.Installation{
			Environment: runtime.Environment, BuildVersion: runtime.BuildVersion,
			APIVersion: runtime.APIVersion, Uptime: uptime,
		},
		Components: []operations.Component{
			{Key: operations.ComponentAPI, State: operations.StateReady, Required: true, LastObservedAt: observedAt},
			{Key: operations.ComponentPostgreSQL, State: operations.StateReady, Required: true, LastObservedAt: observedAt},
			{Key: operations.ComponentWorker, State: workerState, Required: runtime.KafkaEnabled, LastObservedAt: utcTime(row.WorkerLastSeenAt)},
			{Key: operations.ComponentKafka, State: kafkaState, Required: runtime.KafkaEnabled, LastObservedAt: utcTime(row.KafkaObservedAt)},
		},
		Queues: []operations.Queue{
			{
				Key: operations.QueueDomainEvents, State: domainState,
				Pending: uint64(row.OutboxPending), Processing: uint64(row.OutboxProcessing),
				Retry: uint64(row.OutboxRetry), Dead: uint64(row.OutboxDead),
				OldestOutstandingAt: utcTime(row.OutboxOldestOutstandingAt),
			},
			{
				Key: operations.QueueSecurityAuditExport, State: auditState,
				Pending: uint64(row.AuditPending), Processing: uint64(row.AuditProcessing),
				Retry: uint64(row.AuditRetry), Dead: uint64(row.AuditDead),
				OldestOutstandingAt: utcTime(row.AuditOldestOutstandingAt),
			},
		},
	}
	if err := coreSummary.Validate(); err != nil {
		return OperationsSummaryView{}, err
	}

	eventTransport := "durable-queue-only"
	if runtime.KafkaEnabled {
		eventTransport = "kafka"
	}
	view := OperationsSummaryView{
		ObservedAt: observedAt,
		Overall:    string(overall),
		Installation: OperationsInstallationView{
			Environment: runtime.Environment, BuildVersion: runtime.BuildVersion,
			APIVersion: runtime.APIVersion, APIUptimeSeconds: int64(uptime / time.Second),
			TransportSecurity: runtime.TransportSecurity, DomainEventTransport: eventTransport,
		},
		Components: []OperationsComponentView{
			{Key: string(operations.ComponentAPI), State: string(operations.StateReady), Required: true, BuildVersion: runtime.BuildVersion, LastObservedAt: timePointer(observedAt)},
			{Key: string(operations.ComponentPostgreSQL), State: string(operations.StateReady), Required: true, LastObservedAt: timePointer(observedAt)},
			{Key: string(operations.ComponentWorker), State: string(workerState), Required: runtime.KafkaEnabled, BuildVersion: textValue(row.WorkerBuildVersion), StartedAt: timestampPointer(row.WorkerStartedAt), LastObservedAt: timestampPointer(row.WorkerLastSeenAt)},
			{Key: string(operations.ComponentKafka), State: string(kafkaState), Required: runtime.KafkaEnabled, LastObservedAt: timestampPointer(row.KafkaObservedAt)},
		},
		Queues: []OperationsQueueView{
			{Key: string(operations.QueueDomainEvents), State: string(domainState), Pending: uint64(row.OutboxPending), Processing: uint64(row.OutboxProcessing), Retry: uint64(row.OutboxRetry), Dead: uint64(row.OutboxDead), OldestOutstandingAt: timestampPointer(row.OutboxOldestOutstandingAt)},
			{Key: string(operations.QueueSecurityAuditExport), State: string(auditState), Pending: uint64(row.AuditPending), Processing: uint64(row.AuditProcessing), Retry: uint64(row.AuditRetry), Dead: uint64(row.AuditDead), OldestOutstandingAt: timestampPointer(row.AuditOldestOutstandingAt)},
		},
		Migrations: OperationsMigrationView{
			AppliedCount: uint64(row.MigrationCount), LatestName: textValue(row.LatestMigrationName),
			LatestAppliedAt: timestampPointer(row.LatestMigrationAppliedAt),
		},
		Execution: OperationsExecutionView{
			ExecutorConnected: false, RestartAvailable: false, UpdateAvailable: false,
			Reason: "separate-ops-executor-not-configured",
		},
	}
	return view, nil
}

func deliveryQueueState(kafkaEnabled bool, kafkaState operations.State, retry, dead int64) operations.State {
	if dead > 0 {
		return operations.StateAttention
	}
	if retry > 0 || (kafkaEnabled && kafkaState != operations.StateReady) {
		return operations.StateDegraded
	}
	if !kafkaEnabled {
		return operations.StateDisabled
	}
	return operations.StateReady
}

func kafkaComponentState(enabled, workerActive bool, observed string) operations.State {
	if !enabled {
		return operations.StateDisabled
	}
	if !workerActive {
		return operations.StateUnknown
	}
	state := operations.State(observed)
	if state == operations.StateReady || state == operations.StateDegraded {
		return state
	}
	return operations.StateDegraded
}

func utcTime(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time.UTC()
}

func timestampPointer(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	timestamp := value.Time.UTC()
	return &timestamp
}

func timePointer(value time.Time) *time.Time {
	value = value.UTC()
	return &value
}

func textValue(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
