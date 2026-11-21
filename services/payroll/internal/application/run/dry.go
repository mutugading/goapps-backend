package run

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
)

// T022b: dry run (07§4, D-51). Same load and calculation as Commit, but nothing is written: no period row,
// no advisory lock (a dry run never blocks or is blocked by a commit) and no audit row.

// DryCommand is a dry run of a division, optionally limited to NIKs (single-employee check).
type DryCommand struct {
	CompanyID int16
	Division  string
	YYMM      int
	RuleSetID int
	NIKs      []string
}

// DryResult is the calculated, unsaved outcome.
type DryResult struct {
	Summary    Summary
	InputHash  string
	Warnings   []Issue
	Employees  []EmployeeTotal
	SnapshotID int64
	Duration   time.Duration
}

// Dry calculates the division and returns the totals per employee. Rows are read with the caller scope.
func (s *Service) Dry(ctx context.Context, cmd DryCommand) (out DryResult, err error) {
	start := time.Now()
	if err = InScope(ctx, cmd.CompanyID, cmd.Division); err != nil {
		return out, err
	}
	if _, err = validateRun(cmd.YYMM, cmd.RuleSetID); err != nil {
		return out, err
	}
	q := Query{CompanyID: cmd.CompanyID, Division: cmd.Division, YYMM: cmd.YYMM, NIKs: cmd.NIKs}
	err = s.db.ScopedTxOpts(ctx, SnapshotTx, func(tx *sql.Tx) error {
		rules, e := s.rules(ctx, tx, q, cmd.RuleSetID)
		if e != nil {
			return ruleErr(e)
		}
		loaded, e := s.loader.Load(ctx, tx, q, rules)
		if e != nil {
			return loadErr(e)
		}
		res, e := s.runner.DryRun(ctx, loaded)
		if e != nil {
			return e
		}
		if e := checkDry(ctx, tx, q, &res); e != nil {
			return e
		}
		names := make(map[string]string, len(loaded.Inputs))
		for _, in := range loaded.Inputs {
			names[in.Emp.NIK] = in.Emp.Name
		}
		out = DryResult{Summary: res.Summary, InputHash: res.InputHash, Warnings: res.Warnings,
			SnapshotID: loaded.SnapshotID, Employees: employeeTotals(res.Results, names)}
		return nil
	})
	if err != nil {
		return DryResult{}, err
	}
	out.Duration = time.Since(start)
	return out, nil
}

// employeeTotals takes the header totals; a result without header (legacy compat, D2) is summed from its lines.
func employeeTotals(results []engine.Result, names map[string]string) []EmployeeTotal {
	out := make([]EmployeeTotal, 0, len(results))
	for _, r := range results {
		var t, p, rounding decimal.Decimal
		if r.Header != nil {
			t, p, rounding = r.Header.TotTunjangan, r.Header.TotPotongan, r.Header.Rounding
		} else {
			for _, l := range r.Lines {
				if l.Key.Type == engine.CompT {
					t = t.Add(l.AmountOrZero())
				} else {
					p = p.Add(l.AmountOrZero())
				}
			}
		}
		out = append(out, EmployeeTotal{NIK: r.NIK, Name: names[r.NIK], TotT: t.StringFixed(2), TotP: p.StringFixed(2),
			TakeHome: t.Sub(p).Sub(rounding).StringFixed(2)})
	}
	return out
}

// ResolveRuleSet picks the rule set of a run (the API has no rule_set_id, 07§4, D-53): the rule set of the
// period's ACTIVE engine run when there is one (reprocess keeps the rules of the run, 03§2.3), otherwise the
// ACTIVE rule set with the latest effective_from <= yymm.
func (s *Service) ResolveRuleSet(ctx context.Context, companyID int16, division string, yymm int) (id int, err error) {
	err = s.db.ScopedTx(ctx, func(tx *sql.Tx) error {
		e := tx.QueryRowContext(ctx, `SELECT pr_rule_set_id FROM payroll.pyr_run
WHERE pr_company_id = $1 AND pr_division = $2 AND pr_yymm = $3 AND pr_status = 'ACTIVE' AND pr_kind = 'ENGINE'
  AND pr_parent_run_id IS NULL AND pr_rule_set_id > 0`, companyID, division, yymm).Scan(&id)
		if e == nil {
			return nil
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return fmt.Errorf("resolve rule set: %w", e)
		}
		e = tx.QueryRowContext(ctx, `SELECT prs_id FROM payroll.pyr_rule_set
WHERE prs_status = 'ACTIVE' AND prs_effective_from <= $1 ORDER BY prs_effective_from DESC, prs_id DESC LIMIT 1`, yymm).Scan(&id)
		if errors.Is(e, sql.ErrNoRows) {
			return apperr.New(apperr.CodeRuleIncomplete, "tidak ada rule set ACTIVE untuk periode ini",
				map[string]string{"period": fmt.Sprint(yymm)})
		}
		if e != nil {
			return fmt.Errorf("resolve rule set: %w", e)
		}
		return nil
	})
	return id, err
}

// validateRun checks the period and rule set of a run command and returns the period.
func validateRun(yymm, ruleSetID int) (engine.Period, error) {
	period, err := periodOf(yymm)
	if err != nil {
		return period, apperr.New(apperr.CodeValidation, "yymm tidak valid", map[string]string{"field": "period"})
	}
	if ruleSetID <= 0 {
		return period, apperr.New(apperr.CodeValidation, "rule set wajib diisi", map[string]string{"field": "rule_set_id"})
	}
	return period, nil
}
