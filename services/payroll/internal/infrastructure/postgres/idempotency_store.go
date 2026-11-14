package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/idempotency"
)

// staleInProgress is how long an IN_PROGRESS row blocks a retry before it is
// treated as left behind by a crashed request (D-19).
const staleInProgress = "2 minutes"

// IdempotencyStore persists Idempotency-Key results in pyr_idempotency_key
// (D-19). Rows are per user (RLS p_self), so app.user_id is set on every call.
type IdempotencyStore struct {
	db *DB
}

var _ idempotency.Store = (*IdempotencyStore)(nil)

// NewIdempotencyStore creates an IdempotencyStore.
func NewIdempotencyStore(db *DB) *IdempotencyStore {
	return &IdempotencyStore{db: db}
}

// Begin claims the key or reports what already exists for it. Expired rows
// and stale IN_PROGRESS rows are taken over.
func (s *IdempotencyStore) Begin(ctx context.Context, k idempotency.Key, requestHash, scopeHash string) (idempotency.State, string, []byte, error) {
	state := idempotency.Started
	var claimID string
	var resp []byte
	err := s.db.Transaction(ctx, func(tx *sql.Tx) error {
		if err := setSession(ctx, tx, k.UserID, ""); err != nil {
			return err
		}
		const claim = `
INSERT INTO payroll.pyr_idempotency_key
  (pik_user_id, pik_method, pik_key, pik_request_hash, pik_scope_hash, pik_status)
VALUES ($1, $2, $3, $4, $5, 'IN_PROGRESS')
ON CONFLICT (pik_user_id, pik_method, pik_key) DO UPDATE
   SET pik_request_hash = EXCLUDED.pik_request_hash, pik_scope_hash = EXCLUDED.pik_scope_hash,
       pik_status = 'IN_PROGRESS', pik_response = NULL, pik_claim_id = gen_random_uuid(),
       pik_created_at = now(), pik_expires_at = now() + interval '24 hours'
 WHERE payroll.pyr_idempotency_key.pik_expires_at <= now()
    OR (payroll.pyr_idempotency_key.pik_status = 'IN_PROGRESS'
        AND payroll.pyr_idempotency_key.pik_created_at <= now() - interval '` + staleInProgress + `')
RETURNING pik_claim_id::text`
		// A fresh claim gets a new token; DO UPDATE re-evaluates the default via SET.
		err := tx.QueryRowContext(ctx, claim, k.UserID, k.Method, k.Key, requestHash, scopeHash).Scan(&claimID)
		if err == nil {
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("failed to claim idempotency key: %w", err)
		}

		var reqHash, scHash, status string
		err = tx.QueryRowContext(ctx, `
SELECT pik_request_hash, pik_scope_hash, pik_status, pik_response
FROM payroll.pyr_idempotency_key WHERE pik_user_id = $1 AND pik_method = $2 AND pik_key = $3`,
			k.UserID, k.Method, k.Key).Scan(&reqHash, &scHash, &status, &resp)
		if err != nil {
			return fmt.Errorf("failed to read idempotency key: %w", err)
		}
		switch {
		case reqHash != requestHash || scHash != scopeHash:
			state = idempotency.Conflict
		case status == "DONE":
			state = idempotency.Replay
		default:
			state = idempotency.InProgress
		}
		return nil
	})
	if err != nil {
		return 0, "", nil, err
	}
	if state != idempotency.Started {
		claimID = ""
	}
	return state, claimID, resp, nil
}

// Complete stores the successful response of a claimed key.
func (s *IdempotencyStore) Complete(ctx context.Context, k idempotency.Key, claim string, response []byte) error {
	return s.db.Transaction(ctx, func(tx *sql.Tx) error {
		if err := setSession(ctx, tx, k.UserID, ""); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
UPDATE payroll.pyr_idempotency_key SET pik_status = 'DONE', pik_response = $5
WHERE pik_user_id = $1 AND pik_method = $2 AND pik_key = $3 AND pik_claim_id = $4::uuid`,
			k.UserID, k.Method, k.Key, claim, response)
		if err != nil {
			return fmt.Errorf("failed to complete idempotency key: %w", err)
		}
		return nil
	})
}

// Abort releases a claimed key after a failed request so the client can retry.
func (s *IdempotencyStore) Abort(ctx context.Context, k idempotency.Key, claim string) error {
	return s.db.Transaction(ctx, func(tx *sql.Tx) error {
		if err := setSession(ctx, tx, k.UserID, ""); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
DELETE FROM payroll.pyr_idempotency_key
WHERE pik_user_id = $1 AND pik_method = $2 AND pik_key = $3 AND pik_claim_id = $4::uuid AND pik_status = 'IN_PROGRESS'`,
			k.UserID, k.Method, k.Key, claim)
		if err != nil {
			return fmt.Errorf("failed to abort idempotency key: %w", err)
		}
		return nil
	})
}

// Purge deletes keys that expired more than grace ago, across all users
// (SECURITY DEFINER function, T007f), and returns how many rows went.
func (s *IdempotencyStore) Purge(ctx context.Context, grace time.Duration) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, `SELECT payroll.fn_purge_idempotency(make_interval(secs => $1))`,
		grace.Seconds()).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("failed to purge idempotency keys: %w", err)
	}
	return n, nil
}

// RunPurgeLoop purges expired keys every interval until ctx is done. Failures
// are logged and retried on the next tick.
func (s *IdempotencyStore) RunPurgeLoop(ctx context.Context, interval, grace time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			n, err := s.Purge(ctx, grace)
			if err != nil {
				log.Error().Err(err).Msg("idempotency purge failed")
				continue
			}
			if n > 0 {
				log.Info().Int64("rows", n).Msg("idempotency keys purged")
			}
		}
	}
}
