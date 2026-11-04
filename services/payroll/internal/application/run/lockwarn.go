package run

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
)

// CodeLockStale warns that a re-run of period M produced a LOCK_NOW basis (D-30) that later periods of the
// same tax year, already ACTIVE, did not use (D-38/D-39): they keep the old lock until they are re-run.
const CodeLockStale = "SUBSIDY_LOCK_STALE"

// addLockStaleWarning appends the SUBSIDY_LOCK_STALE warning to the dry-run result, if any.
func addLockStaleWarning(ctx context.Context, tx *sql.Tx, q Query, res *DryRunResult) error {
	w, err := lockStaleWarning(ctx, tx, q, res.Results)
	if err != nil {
		return err
	}
	if w != nil {
		res.Warnings = append(res.Warnings, *w)
	}
	return nil
}

// lockStaleWarning returns one aggregated warning when an ACTIVE run of a later period in the same year holds a
// LOCKED basis different from the LOCK_NOW basis this run just calculated. nil = nothing to report.
func lockStaleWarning(ctx context.Context, tx *sql.Tx, q Query, results []engine.Result) (*Issue, error) {
	var niks, bases []string
	for _, r := range results {
		for _, l := range r.Lines {
			if l.Key.Type == engine.CompP && l.Key.ID == 0 && l.Meta[engine.MetaSubsidySource] == engine.SubsidySrcLockNow {
				if b, err := decimal.NewFromString(l.Meta[engine.MetaSubsidyBasis]); err == nil {
					niks, bases = append(niks, r.NIK), append(bases, b.String())
				}
				break
			}
		}
	}
	if len(niks) == 0 {
		return nil, nil //nolint:nilnil // no lock produced: no warning
	}
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT r.pr_yymm, d.psd_nik
FROM payroll.pyr_run r JOIN payroll.pyr_salary_detail d ON d.psd_run_id = r.pr_id
JOIN unnest($5::text[], $6::numeric[]) AS n(nik, basis) ON n.nik = d.psd_nik
WHERE r.pr_company_id = $1 AND r.pr_division = $2 AND r.pr_status = 'ACTIVE' AND r.pr_parent_run_id IS NULL
  AND r.pr_yymm > $3 AND r.pr_yymm <= $4 AND d.psd_type = 'Potongan' AND d.psd_id = 0
  AND d.psd_meta->>$7 = $8 AND (d.psd_meta->>$9)::numeric IS DISTINCT FROM n.basis`,
		q.CompanyID, q.Division, q.YYMM, q.YYMM/100*100+12, niks, bases,
		engine.MetaSubsidySource, engine.SubsidySrcLocked, engine.MetaSubsidyBasis)
	if err != nil {
		return nil, fmt.Errorf("check stale subsidy lock: %w", err)
	}
	defer closeRows(rows)
	periods := map[int]int{}
	for rows.Next() {
		var yymm int
		var nik string
		if err := rows.Scan(&yymm, &nik); err != nil {
			return nil, fmt.Errorf("scan stale subsidy lock: %w", err)
		}
		periods[yymm]++
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("check stale subsidy lock: %w", err)
	}
	if len(periods) == 0 {
		return nil, nil //nolint:nilnil // later periods agree with this lock
	}
	ys := make([]int, 0, len(periods))
	for y := range periods {
		ys = append(ys, y)
	}
	sort.Ints(ys)
	parts := make([]string, len(ys))
	for i, y := range ys {
		parts[i] = fmt.Sprintf("%d (%d karyawan)", y, periods[y])
	}
	return &Issue{Code: CodeLockStale, Message: "kunci basis subsidi LOCK_NOW periode " + fmt.Sprint(q.YYMM) +
		" berubah, periode berikut sudah ACTIVE dengan kunci lama: " + strings.Join(parts, ", ") + "; re-run periode tersebut (D-38)"}, nil
}
