// Package postgres provides PostgreSQL implementations for domain repositories.
package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// Transition atomically persists a workflow-state change: updates mst_mb_head's
// entry_status/current_version/state_reason (and, when params is non-nil, the frozen
// mbh_param_* snapshot columns), inserts a mst_mb_workflow_log audit row, and — only when
// toState is StatusValidated — snapshots the current composition into
// mst_mb_composition_version. All writes commit or roll back together.
func (r *MBHeadRepository) Transition(ctx context.Context, id uuid.UUID, fromState, toState string, currentVersion int32, stateReason, actorUserID string, params *mbhead.ParamSnapshot) error {
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		if err := r.updateEntryStatusTx(ctx, tx, id, toState, currentVersion, stateReason, params); err != nil {
			return err
		}
		if err := r.insertWorkflowLogTx(ctx, tx, id, fromState, toState, actorUserID, stateReason, currentVersion); err != nil {
			return err
		}
		if toState == mbhead.StatusValidated {
			if err := r.compositionRepo.SnapshotVersion(ctx, tx, id.String(), currentVersion, actorUserID); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *MBHeadRepository) updateEntryStatusTx(ctx context.Context, tx *sql.Tx, id uuid.UUID, toState string, currentVersion int32, stateReason string, params *mbhead.ParamSnapshot) error {
	q := `
		UPDATE mst_mb_head
		SET mbh_entry_status = $2, mbh_current_version = $3, mbh_state_reason = NULLIF($4, ''), updated_at = NOW()`
	args := []any{id, toState, currentVersion, stateReason}
	if params != nil {
		q += `,
		    mbh_param_waste = NULLIF($5, '')::numeric, mbh_param_quality_loss = NULLIF($6, '')::numeric,
		    mbh_param_efficiency = NULLIF($7, '')::numeric, mbh_param_dev_expense = NULLIF($8, '')::numeric,
		    mbh_param_packing = NULLIF($9, '')::numeric, mbh_param_mb_prod_per_day = NULLIF($10, '')::numeric,
		    mbh_param_throughput_per_hour = $11, mbh_param_no_of_process = $12`
		args = append(args, params.Waste, params.QualityLoss, params.Efficiency, params.DevExpense,
			params.Packing, params.MBProdPerDay, params.ThroughputPerHour, params.NoOfProcess)
	}
	q += ` WHERE mbh_id = $1 AND deleted_at IS NULL`

	result, err := tx.ExecContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("mb_head_transition: update entry status: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mb_head_transition: rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return mbhead.ErrNotFound
	}
	return nil
}

func (r *MBHeadRepository) insertWorkflowLogTx(ctx context.Context, tx *sql.Tx, id uuid.UUID, fromState, toState, actorUserID, reason string, version int32) error {
	const q = `
		INSERT INTO mst_mb_workflow_log
			(mbwl_mbh_id, mbwl_from_state, mbwl_to_state, mbwl_actor_user_id, mbwl_reason, mbwl_version)
		VALUES ($1, NULLIF($2, ''), $3, $4, NULLIF($5, ''), $6)`
	_, err := tx.ExecContext(ctx, q, id, fromState, toState, actorUserID, reason, version)
	if err != nil {
		return fmt.Errorf("mb_head_transition: insert workflow log: %w", err)
	}
	return nil
}
