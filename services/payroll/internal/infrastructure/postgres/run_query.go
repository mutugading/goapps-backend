package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
)

// T022b: read side of the run and period API (D-51). Every query runs as payroll_app in a scoped
// transaction, so RLS (p_scope) is the second layer behind the handler scope check.

const (
	maxRuns      = 200
	defaultPage  = 100
	maxPageLines = 500
)

// RunQuery implements run.Reader.
type RunQuery struct{ db *DB }

// NewRunQuery creates a RunQuery.
func NewRunQuery(db *DB) *RunQuery { return &RunQuery{db: db} }

var _ run.Reader = (*RunQuery)(nil)

const runCols = `pr_id, pr_yymm, pr_division, to_char(pr_pay_date, 'YYYY-MM-DD'), pr_status::text, COALESCE(pr_emp_count, 0),
  COALESCE(pr_line_count, 0), COALESCE(pr_tot_tunjangan, 0)::text, COALESCE(pr_tot_potongan, 0)::text, pr_rule_set_id, pr_snapshot_id,
  COALESCE(array_to_string(pr_nik_filter, ','), ''), COALESCE(pr_warnings, '[]'::jsonb)::text, COALESCE(pr_started_by, ''),
  pr_started_at, pr_finished_at`

type scanner interface{ Scan(dest ...any) error }

func scanRun(s scanner) (run.Info, error) {
	var (
		r         run.Info
		niks, wrn string
		fin       sql.NullTime
	)
	if err := s.Scan(&r.ID, &r.YYMM, &r.Division, &r.PayDate, &r.Status, &r.EmpCount, &r.LineCount, &r.TotT, &r.TotP,
		&r.RuleSetID, &r.SnapshotID, &niks, &wrn, &r.StartedBy, &r.StartedAt, &fin); err != nil {
		return r, err
	}
	if niks != "" {
		r.NIKFilter = strings.Split(niks, ",")
	}
	if err := json.Unmarshal([]byte(wrn), &r.Warnings); err != nil {
		return r, fmt.Errorf("decode run warnings: %w", err)
	}
	if fin.Valid {
		r.FinishedAt = fin.Time
		r.DurationMs = fin.Time.Sub(r.StartedAt).Milliseconds()
	}
	return r, nil
}

// GetRun reads one run; a run outside the caller scope is invisible (NOT_FOUND).
func (q *RunQuery) GetRun(ctx context.Context, id int64) (out run.Info, err error) {
	err = q.db.ScopedTx(ctx, func(tx *sql.Tx) error {
		r, e := scanRun(tx.QueryRowContext(ctx, `SELECT `+runCols+` FROM payroll.pyr_run WHERE pr_id = $1`, id))
		if errors.Is(e, sql.ErrNoRows) {
			return apperr.New(apperr.CodeNotFound, "run tidak ditemukan", nil)
		}
		out = r
		return wrapRead("run", e)
	})
	return out, err
}

// ListRuns lists runs newest first; yymm 0 and division "" mean all (within scope).
func (q *RunQuery) ListRuns(ctx context.Context, yymm int, division string) (out []run.Info, err error) {
	err = q.db.ScopedTx(ctx, func(tx *sql.Tx) error {
		rows, e := tx.QueryContext(ctx, `SELECT `+runCols+` FROM payroll.pyr_run
WHERE ($1 = 0 OR pr_yymm = $1) AND ($2 = '' OR pr_division = $2) ORDER BY pr_id DESC LIMIT $3`, yymm, division, maxRuns)
		if e != nil {
			return fmt.Errorf("list runs: %w", e)
		}
		defer func() { _ = rows.Close() }() //nolint:errcheck // read-only rows
		for rows.Next() {
			r, e := scanRun(rows)
			if e != nil {
				return fmt.Errorf("scan run: %w", e)
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}

// ListLines pages the component lines of a run. A run outside the scope or unknown is NOT_FOUND.
func (q *RunQuery) ListLines(ctx context.Context, runID int64, nik string, page, size int) (out []run.LineInfo, total int64, err error) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = defaultPage
	}
	if size > maxPageLines {
		size = maxPageLines
	}
	err = q.db.ScopedTx(ctx, func(tx *sql.Tx) error {
		var exists bool
		if e := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM payroll.pyr_run WHERE pr_id = $1)`, runID).Scan(&exists); e != nil {
			return fmt.Errorf("check run: %w", e)
		}
		if !exists {
			return apperr.New(apperr.CodeNotFound, "run tidak ditemukan", nil)
		}
		if e := tx.QueryRowContext(ctx, `SELECT count(*) FROM payroll.pyr_salary_detail WHERE psd_run_id = $1 AND ($2 = '' OR psd_nik = $2)`,
			runID, nik).Scan(&total); e != nil {
			return fmt.Errorf("count lines: %w", e)
		}
		rows, e := tx.QueryContext(ctx, `SELECT psd_nik, CASE psd_type WHEN 'Tunjangan' THEN 'T' ELSE 'P' END, psd_id,
  COALESCE(psd_description, ''), COALESCE(psd_amount::text, ''), COALESCE(psd_basis_amount::text, ''), COALESCE(psd_qty::text, ''),
  COALESCE(psd_rate::text, ''), COALESCE(psd_formula, ''), COALESCE(psd_remarks, '')
FROM payroll.pyr_salary_detail WHERE psd_run_id = $1 AND ($2 = '' OR psd_nik = $2)
ORDER BY psd_nik, psd_type, psd_id LIMIT $3 OFFSET $4`, runID, nik, size, (page-1)*size)
		if e != nil {
			return fmt.Errorf("list lines: %w", e)
		}
		defer func() { _ = rows.Close() }() //nolint:errcheck // read-only rows
		for rows.Next() {
			var l run.LineInfo
			if e := rows.Scan(&l.NIK, &l.CompType, &l.CompID, &l.Description, &l.Amount, &l.Base, &l.Qty, &l.Rate, &l.Formula, &l.Remarks); e != nil {
				return fmt.Errorf("scan line: %w", e)
			}
			out = append(out, l)
		}
		return rows.Err()
	})
	return out, total, err
}

const periodCols = `pp_yymm, pp_division, to_char(pp_date_from, 'YYYY-MM-DD'), to_char(pp_date_to, 'YYYY-MM-DD'), pp_status::text,
  COALESCE(pp_active_run_id, 0), COALESCE(approved_by, ''), COALESCE(to_char(approved_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), ''),
  COALESCE(closed_by, ''), COALESCE(to_char(closed_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), ''), COALESCE(pp_reopen_reason, '')`

func scanPeriod(s scanner) (p run.PeriodInfo, err error) {
	err = s.Scan(&p.YYMM, &p.Division, &p.DateFrom, &p.DateTo, &p.Status, &p.ActiveRunID, &p.ApprovedBy, &p.ApprovedAt,
		&p.ClosedBy, &p.ClosedAt, &p.ReopenReason)
	return p, err
}

// ListPeriods lists periods newest first; division "" = all in scope.
func (q *RunQuery) ListPeriods(ctx context.Context, division string) (out []run.PeriodInfo, err error) {
	err = q.db.ScopedTx(ctx, func(tx *sql.Tx) error {
		rows, e := tx.QueryContext(ctx, `SELECT `+periodCols+` FROM payroll.pyr_period
WHERE ($1 = '' OR pp_division = $1) ORDER BY pp_yymm DESC, pp_division LIMIT $2`, division, maxRuns)
		if e != nil {
			return fmt.Errorf("list periods: %w", e)
		}
		defer func() { _ = rows.Close() }() //nolint:errcheck // read-only rows
		for rows.Next() {
			p, e := scanPeriod(rows)
			if e != nil {
				return fmt.Errorf("scan period: %w", e)
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	return out, err
}

// GetPeriod reads one period.
func (q *RunQuery) GetPeriod(ctx context.Context, companyID int16, yymm int, division string) (out run.PeriodInfo, err error) {
	err = q.db.ScopedTx(ctx, func(tx *sql.Tx) error {
		p, e := scanPeriod(tx.QueryRowContext(ctx, `SELECT `+periodCols+` FROM payroll.pyr_period
WHERE pp_company_id = $1 AND pp_yymm = $2 AND pp_division = $3`, companyID, yymm, division))
		if errors.Is(e, sql.ErrNoRows) {
			return apperr.New(apperr.CodeNotFound, "periode tidak ditemukan", nil)
		}
		out = p
		return wrapRead("period", e)
	})
	return out, err
}

func wrapRead(what string, err error) error {
	if err != nil {
		return fmt.Errorf("read %s: %w", what, err)
	}
	return nil
}
