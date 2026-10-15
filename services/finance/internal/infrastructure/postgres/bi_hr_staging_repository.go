package postgres

import (
	"context"
	"fmt"
	"time"

	oracleinfra "github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
)

// BiHRStagingRepository writes HRIS staging tables (bi_hr_stg_*, bi_hr_employee).
type BiHRStagingRepository struct {
	db *DB
}

// NewBiHRStagingRepository constructs the repository.
func NewBiHRStagingRepository(db *DB) *BiHRStagingRepository {
	return &BiHRStagingRepository{db: db}
}

// ReplaceOTDaily deletes staging rows with work_date in [d1, d2] and inserts rows in one
// transaction (HD-04), and upserts the employee label table (HD-07). Rows with empty NIK are skipped.
// Returns rows inserted.
func (r *BiHRStagingRepository) ReplaceOTDaily(ctx context.Context, d1, d2 time.Time, rows []oracleinfra.OTDailyRow) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err = tx.ExecContext(ctx, `DELETE FROM bi_hr_stg_ot_daily WHERE work_date BETWEEN $1 AND $2`, d1, d2); err != nil {
		return 0, fmt.Errorf("delete window: %w", err)
	}

	const ins = `INSERT INTO bi_hr_stg_ot_daily (
    work_date, week_start, att_period, week_att_period, emp_sys_id, nik, division, dept_id, dept_name,
    schedule_code, ot_type, ot_tier_1, ot_tier_2, ot_tier_3, ot_actual_hours, ot_calc_hours, ot_limit_hours
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`
	const emp = `INSERT INTO bi_hr_employee (nik, emp_sys_id, emp_name, dept_id, dept_name, updated_at)
VALUES ($1,$2,$3,$4,$5,NOW())
ON CONFLICT (nik) DO UPDATE SET emp_sys_id = EXCLUDED.emp_sys_id, emp_name = EXCLUDED.emp_name,
    dept_id = EXCLUDED.dept_id, dept_name = EXCLUDED.dept_name, updated_at = NOW()`

	seen := make(map[string]struct{}, len(rows))
	inserted := 0
	for _, x := range rows {
		if x.NIK == "" { // cannot be keyed to an employee; skipped (count = len(rows) - inserted)
			continue
		}
		if _, err = tx.ExecContext(ctx, ins,
			x.WorkDate, x.WeekStart, x.AttPeriod, x.WeekAttPeriod, x.EmpSysID, x.NIK, x.Division, x.DeptID, x.DeptName,
			x.ScheduleCode, x.OTType, x.Tier1.String(), x.Tier2.String(), x.Tier3.String(),
			x.ActualHours.String(), x.CalcHours.String(), x.LimitHours.String()); err != nil {
			return 0, fmt.Errorf("insert staging OT: %w", err)
		}
		inserted++
		if _, ok := seen[x.NIK]; ok {
			continue
		}
		seen[x.NIK] = struct{}{}
		if _, err = tx.ExecContext(ctx, emp, x.NIK, x.EmpSysID, x.EmpName, x.DeptID, x.DeptName); err != nil {
			return 0, fmt.Errorf("upsert employee: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return inserted, nil
}
