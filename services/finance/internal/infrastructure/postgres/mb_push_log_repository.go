// Package postgres provides PostgreSQL implementations for domain repositories.
package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbpushlog"
)

// MBPushLogRepository implements mbpushlog.Repository using PostgreSQL.
type MBPushLogRepository struct {
	db *DB
}

// NewMBPushLogRepository creates a new MBPushLogRepository instance.
func NewMBPushLogRepository(db *DB) *MBPushLogRepository {
	return &MBPushLogRepository{db: db}
}

// Verify interface implementation at compile time.
var _ mbpushlog.Repository = (*MBPushLogRepository)(nil)

// Create persists one push-execution audit row.
func (r *MBPushLogRepository) Create(ctx context.Context, e *mbpushlog.Entity) error {
	const q = `
		INSERT INTO mst_mb_push_log
			(mbpl_period, mbpl_pushed_by, mbpl_mb_count, mbpl_row_count, mbpl_cost_types,
			 mbpl_previous_period, mbpl_notes)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), NULLIF($7, ''))
		RETURNING mbpl_id`
	var id string
	err := r.db.QueryRowContext(ctx, q,
		e.Period(), e.PushedBy(), e.MBCount(), e.RowCount(), e.CostTypes(),
		e.PreviousPeriod(), e.Notes(),
	).Scan(&id)
	if err != nil {
		return fmt.Errorf("mb_push_log_repository: create: %w", err)
	}
	return nil
}

// List returns paginated push-log rows, newest first, optionally filtered by exact period.
func (r *MBPushLogRepository) List(ctx context.Context, page, pageSize int32, period string) ([]*mbpushlog.Entity, int64, error) {
	where := "WHERE TRUE"
	args := []any{}
	if period != "" {
		where += fmt.Sprintf(" AND mbpl_period = $%d", len(args)+1)
		args = append(args, period)
	}

	var total int64
	countQ := "SELECT COUNT(*) FROM mst_mb_push_log " + where
	if err := r.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("mb_push_log_repository: count: %w", err)
	}

	offset := (page - 1) * pageSize
	listQ := fmt.Sprintf("%s %s ORDER BY mbpl_pushed_at DESC LIMIT $%d OFFSET $%d",
		r.selectCols(), where, len(args)+1, len(args)+2)
	args = append(args, pageSize, offset)

	rows, err := r.db.QueryContext(ctx, listQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("mb_push_log_repository: list: %w", err)
	}
	defer closeRows(rows)

	var out []*mbpushlog.Entity
	for rows.Next() {
		e, scanErr := r.scanRow(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("mb_push_log_repository: iterate: %w", err)
	}
	return out, total, nil
}

func (r *MBPushLogRepository) selectCols() string {
	return `
		SELECT mbpl_id, mbpl_period, mbpl_pushed_at, mbpl_pushed_by, mbpl_mb_count, mbpl_row_count,
		       mbpl_cost_types, COALESCE(mbpl_previous_period, ''), COALESCE(mbpl_notes, '')
		FROM mst_mb_push_log
	`
}

type mbPushLogDTO struct {
	ID             string
	Period         string
	PushedAt       string
	PushedBy       string
	MBCount        int32
	RowCount       int32
	CostTypes      string
	PreviousPeriod string
	Notes          string
}

func (d *mbPushLogDTO) toEntity() *mbpushlog.Entity {
	return mbpushlog.Reconstruct(
		d.ID, d.Period, d.PushedAt, d.PushedBy, d.MBCount, d.RowCount, d.CostTypes,
		d.PreviousPeriod, d.Notes,
	)
}

func (r *MBPushLogRepository) scanRow(rows *sql.Rows) (*mbpushlog.Entity, error) {
	var d mbPushLogDTO
	err := rows.Scan(&d.ID, &d.Period, &d.PushedAt, &d.PushedBy, &d.MBCount, &d.RowCount,
		&d.CostTypes, &d.PreviousPeriod, &d.Notes)
	if err != nil {
		return nil, fmt.Errorf("mb_push_log_repository: scan row: %w", err)
	}
	return d.toEntity(), nil
}
