package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	cprapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductrequest"
)

// ParamEditLogEntry is a single audit record for a param value override.
type ParamEditLogEntry struct {
	ID         int64
	RequestID  int64
	RouteLevel int
	ParamCode  string
	OldValue   string
	NewValue   string
	ChangedBy  string
	ChangedAt  time.Time
}

// CostParamEditLogRepository writes and reads param value override audit records.
type CostParamEditLogRepository struct {
	db *sql.DB
}

// NewCostParamEditLogRepository constructs a CostParamEditLogRepository.
func NewCostParamEditLogRepository(db *sql.DB) *CostParamEditLogRepository {
	return &CostParamEditLogRepository{db: db}
}

// BulkInsert inserts multiple audit entries in a single transaction.
func (r *CostParamEditLogRepository) BulkInsert(ctx context.Context, entries []ParamEditLogEntry) error {
	if len(entries) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cost_param_edit_log begin tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			_ = rbErr
		}
	}()

	const q = `
INSERT INTO cost_param_edit_log
    (cpel_request_id, cpel_route_level, cpel_param_code, cpel_old_value, cpel_new_value, cpel_changed_by, cpel_changed_at)
VALUES ($1, $2, $3, $4, $5, $6, NOW())`

	for i := range entries {
		e := &entries[i]
		if _, err := tx.ExecContext(ctx, q,
			e.RequestID, e.RouteLevel, e.ParamCode,
			nullableString(e.OldValue), nullableString(e.NewValue),
			e.ChangedBy,
		); err != nil {
			return fmt.Errorf("cost_param_edit_log insert entry %d: %w", i, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cost_param_edit_log commit: %w", err)
	}
	return nil
}

// GetLastEditInfoPerLevel implements cprapp.ParamEditLogLoader.
// It returns the user + timestamp of the most recent override per route level.
func (r *CostParamEditLogRepository) GetLastEditInfoPerLevel(ctx context.Context, requestID int64) (map[int]cprapp.LevelEditInfo, error) {
	raw, err := r.GetLastEditPerLevel(ctx, requestID)
	if err != nil {
		return nil, err
	}
	result := make(map[int]cprapp.LevelEditInfo, len(raw))
	for level, e := range raw {
		result[level] = cprapp.LevelEditInfo{
			ChangedBy: e.ChangedBy,
			ChangedAt: e.ChangedAt.UTC().Format("2006-01-02T15:04:05Z"),
		}
	}
	return result, nil
}

// ListByRequestLevel returns all edit log entries for one fill level of a request,
// ordered newest-first. It satisfies cprapp.ParamEditLogByLevelReader.
func (r *CostParamEditLogRepository) ListByRequestLevel(ctx context.Context, requestID int64, routeLevel int) ([]cprapp.ParamEditLogRow, error) {
	const q = `
SELECT cpel_id, cpel_request_id, cpel_route_level, cpel_param_code,
       cpel_old_value, cpel_new_value, cpel_changed_by, cpel_changed_at
FROM cost_param_edit_log
WHERE cpel_request_id = $1
  AND cpel_route_level = $2
ORDER BY cpel_changed_at DESC`

	rows, err := r.db.QueryContext(ctx, q, requestID, routeLevel)
	if err != nil {
		return nil, fmt.Errorf("list_param_edit_log query: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()

	var rows2 []cprapp.ParamEditLogRow
	for rows.Next() {
		var id, requestID int64
		var routeLvl int
		var paramCode string
		var oldVal, newVal sql.NullString
		var changedBy string
		var changedAt time.Time
		if err := rows.Scan(
			&id, &requestID, &routeLvl, &paramCode,
			&oldVal, &newVal, &changedBy, &changedAt,
		); err != nil {
			return nil, fmt.Errorf("list_param_edit_log scan: %w", err)
		}
		rows2 = append(rows2, cprapp.ParamEditLogRow{
			ParamCode: paramCode,
			OldValue:  oldVal.String,
			NewValue:  newVal.String,
			ChangedBy: changedBy,
			ChangedAt: changedAt.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list_param_edit_log rows: %w", err)
	}
	return rows2, nil
}

// GetLastEditPerLevel returns the most recent edit log entry per route level for
// a given request. The map key is route_level.
func (r *CostParamEditLogRepository) GetLastEditPerLevel(ctx context.Context, requestID int64) (map[int]ParamEditLogEntry, error) {
	const q = `
SELECT DISTINCT ON (cpel_route_level)
    cpel_id, cpel_request_id, cpel_route_level, cpel_param_code,
    cpel_old_value, cpel_new_value, cpel_changed_by, cpel_changed_at
FROM cost_param_edit_log
WHERE cpel_request_id = $1
ORDER BY cpel_route_level, cpel_changed_at DESC`

	rows, err := r.db.QueryContext(ctx, q, requestID)
	if err != nil {
		return nil, fmt.Errorf("get_last_edit_per_level query: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()

	result := make(map[int]ParamEditLogEntry)
	for rows.Next() {
		var e ParamEditLogEntry
		var oldVal, newVal sql.NullString
		if err := rows.Scan(
			&e.ID, &e.RequestID, &e.RouteLevel, &e.ParamCode,
			&oldVal, &newVal, &e.ChangedBy, &e.ChangedAt,
		); err != nil {
			return nil, fmt.Errorf("get_last_edit_per_level scan: %w", err)
		}
		e.OldValue = oldVal.String
		e.NewValue = newVal.String
		result[e.RouteLevel] = e
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get_last_edit_per_level rows: %w", err)
	}
	return result, nil
}
