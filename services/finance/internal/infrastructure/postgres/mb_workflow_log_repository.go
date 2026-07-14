// Package postgres provides PostgreSQL implementations for domain repositories.
package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbworkflowlog"
)

// MBWorkflowLogRepository implements mbworkflowlog.Repository using PostgreSQL.
type MBWorkflowLogRepository struct {
	db *DB
}

// NewMBWorkflowLogRepository creates a new MBWorkflowLogRepository instance.
func NewMBWorkflowLogRepository(db *DB) *MBWorkflowLogRepository {
	return &MBWorkflowLogRepository{db: db}
}

// Verify interface implementation at compile time.
var _ mbworkflowlog.Repository = (*MBWorkflowLogRepository)(nil)

// Create records one workflow transition audit row.
func (r *MBWorkflowLogRepository) Create(ctx context.Context, e *mbworkflowlog.Entity) error {
	const q = `
		INSERT INTO mst_mb_workflow_log
			(mbwl_mbh_id, mbwl_from_state, mbwl_to_state, mbwl_actor_user_id, mbwl_reason, mbwl_version)
		VALUES ($1, NULLIF($2, ''), $3, $4, NULLIF($5, ''), $6)`
	_, err := r.db.ExecContext(ctx, q,
		e.MbhID(), e.FromState(), e.ToState(), e.ActorUserID(), e.Reason(), e.Version(),
	)
	if err != nil {
		return fmt.Errorf("mb_workflow_log_repository: create: %w", err)
	}
	return nil
}

// ListByMbhID returns all workflow transitions for one MB head, newest first.
func (r *MBWorkflowLogRepository) ListByMbhID(ctx context.Context, mbhID string) ([]*mbworkflowlog.Entity, error) {
	const q = `
		SELECT mbwl_id, mbwl_mbh_id, COALESCE(mbwl_from_state, ''), mbwl_to_state,
		       mbwl_actor_user_id, mbwl_actor_at, COALESCE(mbwl_reason, ''), COALESCE(mbwl_version, 0)
		FROM mst_mb_workflow_log
		WHERE mbwl_mbh_id = $1
		ORDER BY mbwl_actor_at DESC`
	rows, err := r.db.QueryContext(ctx, q, mbhID)
	if err != nil {
		return nil, fmt.Errorf("mb_workflow_log_repository: list: %w", err)
	}
	defer closeRows(rows)

	var out []*mbworkflowlog.Entity
	for rows.Next() {
		e, scanErr := r.scanRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mb_workflow_log_repository: iterate: %w", err)
	}
	return out, nil
}

type mbWorkflowLogDTO struct {
	ID          string
	MbhID       string
	FromState   string
	ToState     string
	ActorUserID string
	ActorAt     string
	Reason      string
	Version     int32
}

func (d *mbWorkflowLogDTO) toEntity() *mbworkflowlog.Entity {
	return mbworkflowlog.Reconstruct(
		d.ID, d.MbhID, d.FromState, d.ToState, d.ActorUserID, d.ActorAt, d.Reason, d.Version,
	)
}

func (r *MBWorkflowLogRepository) scanRow(rows *sql.Rows) (*mbworkflowlog.Entity, error) {
	var d mbWorkflowLogDTO
	err := rows.Scan(&d.ID, &d.MbhID, &d.FromState, &d.ToState, &d.ActorUserID, &d.ActorAt,
		&d.Reason, &d.Version)
	if err != nil {
		return nil, fmt.Errorf("mb_workflow_log_repository: scan row: %w", err)
	}
	return d.toEntity(), nil
}
