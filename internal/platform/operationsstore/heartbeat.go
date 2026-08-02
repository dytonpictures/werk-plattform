// Package operationsstore persists minimized technical observations used by
// platform operations. Heartbeats are not authority, readiness or fachliche
// truth; they only prove that one worker refreshed an expiring observation.
package operationsstore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/dytonpictures/werk/internal/core/operations"
	"github.com/dytonpictures/werk/internal/platform/database"
)

const (
	minimumHeartbeatExpiry = 5 * time.Second
	maximumHeartbeatExpiry = 10 * time.Minute
	defaultStaleRetention  = 24 * time.Hour
)

var ErrInvalidHeartbeat = errors.New("invalid worker heartbeat")

type globalWriter interface {
	WithinGlobalWrite(context.Context, database.TenantOperation) error
}

// WorkerHeartbeatWriter owns one random run-scoped worker heartbeat. Its
// identifier has no hostname, PID or reusable host identity embedded in it.
type WorkerHeartbeatWriter struct {
	database       globalWriter
	runID          string
	buildVersion   string
	expiry         time.Duration
	staleRetention time.Duration
}

// NewWorkerHeartbeatWriter creates an in-memory writer. No database row is
// created until Beat succeeds, allowing startup to fail closed explicitly.
func NewWorkerHeartbeatWriter(workerDatabase *database.WorkerDB, buildVersion string, expiry time.Duration) (*WorkerHeartbeatWriter, error) {
	if workerDatabase == nil {
		return nil, ErrInvalidHeartbeat
	}
	runID, err := newRunID(time.Now().UTC(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate worker heartbeat run ID: %w", err)
	}
	return newWorkerHeartbeatWriter(workerDatabase, runID, buildVersion, expiry, defaultStaleRetention)
}

func newWorkerHeartbeatWriter(writer globalWriter, runID, buildVersion string, expiry, staleRetention time.Duration) (*WorkerHeartbeatWriter, error) {
	if writer == nil || !validRunID(runID) || !operations.ValidBuildVersion(buildVersion) ||
		expiry < minimumHeartbeatExpiry || expiry > maximumHeartbeatExpiry ||
		expiry%time.Second != 0 ||
		staleRetention < expiry || staleRetention > 30*24*time.Hour ||
		staleRetention%time.Second != 0 {
		return nil, ErrInvalidHeartbeat
	}
	return &WorkerHeartbeatWriter{
		database: writer, runID: runID, buildVersion: buildVersion,
		expiry: expiry, staleRetention: staleRetention,
	}, nil
}

// Beat invokes the worker-only database contract which removes long-expired
// observations and atomically creates or refreshes this run. PostgreSQL derives
// start, last-seen and expiry from one database-clock observation.
func (writer *WorkerHeartbeatWriter) Beat(ctx context.Context, kafkaState operations.State) error {
	if writer == nil || writer.database == nil || ctx == nil ||
		(kafkaState != operations.StateDisabled && kafkaState != operations.StateReady && kafkaState != operations.StateDegraded) {
		return ErrInvalidHeartbeat
	}
	return writer.database.WithinGlobalWrite(ctx, func(ctx context.Context, tx database.TenantTx) error {
		command, err := tx.Exec(ctx, `
			SELECT *
			FROM werk_security.beat_worker_heartbeat(
				$1::uuid, $2, $3::integer, $4::integer, $5::text
			)
		`, writer.runID, writer.buildVersion, int32(writer.expiry/time.Second), int32(writer.staleRetention/time.Second), string(kafkaState))
		if err != nil {
			return fmt.Errorf("write worker heartbeat: %w", err)
		}
		if command.RowsAffected() != 1 {
			return errors.New("worker heartbeat identity changed")
		}
		return nil
	})
}

// Remove withdraws this run's observation during a graceful shutdown. It is
// intentionally idempotent; an already-pruned or expired row is not an error.
func (writer *WorkerHeartbeatWriter) Remove(ctx context.Context) error {
	if writer == nil || writer.database == nil || ctx == nil {
		return ErrInvalidHeartbeat
	}
	return writer.database.WithinGlobalWrite(ctx, func(ctx context.Context, tx database.TenantTx) error {
		command, err := tx.Exec(ctx, `
			SELECT *
			FROM werk_security.remove_worker_heartbeat($1::uuid)
		`, writer.runID)
		if err != nil {
			return fmt.Errorf("remove worker heartbeat: %w", err)
		}
		if command.RowsAffected() != 1 {
			return errors.New("worker heartbeat removal result changed")
		}
		return nil
	})
}

func newRunID(at time.Time, randomness io.Reader) (string, error) {
	if at.IsZero() || randomness == nil {
		return "", ErrInvalidHeartbeat
	}
	var identifier [16]byte
	milliseconds := uint64(at.UnixMilli())
	identifier[0] = byte(milliseconds >> 40)
	identifier[1] = byte(milliseconds >> 32)
	identifier[2] = byte(milliseconds >> 24)
	identifier[3] = byte(milliseconds >> 16)
	identifier[4] = byte(milliseconds >> 8)
	identifier[5] = byte(milliseconds)
	if _, err := io.ReadFull(randomness, identifier[6:]); err != nil {
		return "", err
	}
	identifier[6] = (identifier[6] & 0x0f) | 0x70
	identifier[8] = (identifier[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", identifier[0:4], identifier[4:6], identifier[6:8], identifier[8:10], identifier[10:16]), nil
}

func validRunID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' || value[14] != '7' {
		return false
	}
	variant := value[19]
	if variant != '8' && variant != '9' && variant != 'a' && variant != 'b' && variant != 'A' && variant != 'B' {
		return false
	}
	compact := strings.ReplaceAll(value, "-", "")
	if len(compact) != 32 {
		return false
	}
	_, err := hex.DecodeString(compact)
	return err == nil
}
