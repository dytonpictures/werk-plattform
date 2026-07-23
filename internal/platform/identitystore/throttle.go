package identitystore

import (
	"context"
	"crypto/sha256"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dytonpictures/werk/internal/platform/database"
)

const (
	loginFailureLimit  = 8
	loginFailureWindow = 15 * time.Minute
	loginLockDuration  = 15 * time.Minute
)

func loginThrottleKey(normalizedLogin string) [32]byte {
	return sha256.Sum256([]byte("identity-login-v2:" + normalizedLogin))
}

// Unknown account names share one decoy bucket so arbitrary guesses cannot
// create an unbounded number of persistent rows.
func unknownLoginThrottleKey() [32]byte {
	return sha256.Sum256([]byte("identity-login-v1:unknown-account"))
}

func (service *Service) loginThrottled(ctx context.Context, key [32]byte) (bool, error) {
	var lockedUntil time.Time
	err := service.database.WithinRead(ctx, func(ctx context.Context, tx database.TenantTx) error {
		return tx.QueryRow(ctx, `
			SELECT locked_until FROM werk_core.identity_auth_throttles
			WHERE subject_hash = $1 AND locked_until > $2
		`, key[:], service.now()).Scan(&lockedUntil)
	})
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return service.now().Before(lockedUntil), nil
}

func (service *Service) recordLoginFailure(ctx context.Context, key [32]byte) error {
	now := service.now()
	return service.database.WithinWrite(ctx, func(ctx context.Context, tx database.TenantTx) error {
		if _, err := tx.Exec(ctx, `
			WITH expired AS (
				SELECT subject_hash
				FROM werk_core.identity_auth_throttles
				WHERE updated_at < $1
				ORDER BY updated_at
				LIMIT 128
				FOR UPDATE SKIP LOCKED
			)
			DELETE FROM werk_core.identity_auth_throttles AS throttle
			USING expired
			WHERE throttle.subject_hash = expired.subject_hash
		`, now.Add(-24*time.Hour)); err != nil {
			return err
		}
		var count int
		var windowStarted time.Time
		err := tx.QueryRow(ctx, `
			SELECT failure_count, window_started_at
			FROM werk_core.identity_auth_throttles WHERE subject_hash = $1
			FOR UPDATE
		`, key[:]).Scan(&count, &windowStarted)
		if err != nil && err != pgx.ErrNoRows {
			return err
		}
		if err == pgx.ErrNoRows || now.Sub(windowStarted) >= loginFailureWindow {
			count = 1
			windowStarted = now
		} else {
			count++
		}
		var lockedUntil any
		if count >= loginFailureLimit {
			lockedUntil = now.Add(loginLockDuration)
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO werk_core.identity_auth_throttles (
				subject_hash, failure_count, window_started_at, locked_until, updated_at
			) VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (subject_hash) DO UPDATE SET
				failure_count = EXCLUDED.failure_count,
				window_started_at = EXCLUDED.window_started_at,
				locked_until = EXCLUDED.locked_until,
				updated_at = EXCLUDED.updated_at
		`, key[:], count, windowStarted, lockedUntil, now)
		return err
	})
}

func (service *Service) clearLoginFailures(ctx context.Context, key [32]byte) error {
	return service.database.WithinWrite(ctx, func(ctx context.Context, tx database.TenantTx) error {
		_, err := tx.Exec(ctx, `DELETE FROM werk_core.identity_auth_throttles WHERE subject_hash = $1`, key[:])
		return err
	})
}
