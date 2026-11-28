package etl

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/audit"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/postgres"
)

// T017c: extra commit cases (JAKARTA, STS D, empty division, batch > 5000 rows, employee without header,
// NULL basis, header = Σ line guard, stale LOCK_NOW warning).
func seedPeriodSnap(t *testing.T, div string, yymm, emps int, tag string) {
	t.Helper()
	for _, q := range []string{
		fmt.Sprintf(`INSERT INTO payroll.pyr_attd_days (pad_company_id, pad_yymm, pad_division, pad_days) VALUES (1,%d,'%s',26) ON CONFLICT DO NOTHING`, yymm, div),
		fmt.Sprintf(`INSERT INTO payroll.pyr_att_snapshot (pas_company_id, pas_yymm, pas_division, pas_source_batch_ids, pas_checksum, pas_locked)
			VALUES (1,%d,'%s','{1}','%s',true)`, yymm, div, tag),
		fmt.Sprintf(`INSERT INTO payroll.pyr_att_snapshot_row (pasr_snapshot_id, pasr_kind, pasr_nik, pasr_data)
			SELECT s.pas_snapshot_id, 'REKAP', 'J' || lpad(g::text, 5, '0'), '{"MPAR_DAY_WORK":26,"MPAR_PAY_TOT":26}'::jsonb
			FROM payroll.pyr_att_snapshot s, generate_series(1, %d) g WHERE s.pas_checksum = '%s'`, emps, tag),
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
}

func TestRunCommit_More(t *testing.T) {
	raw, err := sql.Open("pgx", appDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	app := postgres.NewDBFromSQL(raw)
	// The T016 fixture rule "JKT tanpa limit" (valid from 202601, open-ended) would fail every JAKARTA run (D-31):
	// end it before the periods used here (2033xx).
	if _, err := ownDB.Exec(`UPDATE payroll.pyr_subsidy_rule SET psr_valid_to = 202612 WHERE psr_name = 'JKT tanpa limit'`); err != nil {
		t.Fatal(err)
	}

	t.Run("0 employees calculated is rejected, nothing saved", func(t *testing.T) {
		seedPeriodSnap(t, "JAKARTA", 203301, 0, "t17c-empty")
		before := scalar(t, `SELECT count(*) FROM payroll.pyr_run WHERE pr_division='JAKARTA'`)
		// Every NIK fails in the engine (per-NIK warning, D-27), so no employee is calculated.
		none := run.Runner{Calc: func(engine.Input) (engine.Result, error) { return engine.Result{}, errors.New("boom") }}
		_, err := run.NewServiceWith(app, ruleFunc, audit.NewWriter(), none).Commit(asScopes(t, "JAKARTA"),
			run.CommitCommand{CompanyID: 1, Division: "JAKARTA", YYMM: 203301, RuleSetID: 1})
		var ae *apperr.Error
		if !errors.As(err, &ae) || ae.Code != apperr.CodeBusinessRule {
			t.Fatalf("got %v, want BUSINESS_RULE", err)
		}
		if after := scalar(t, `SELECT count(*) FROM payroll.pyr_run WHERE pr_division='JAKARTA'`); after != before {
			t.Fatalf("run tersimpan: %s → %s", before, after)
		}
	})

	// 1.300 employees × 6 lines = 7.800 detail rows (> persistBatch 5000); 2 employees have STS D / no header.
	stub := func(in engine.Input) (engine.Result, error) {
		var lines []engine.Line
		var tot decimal.Decimal
		for id := 1; id <= 6; id++ {
			a := decimal.NewFromInt(int64(id) * 1000)
			tot = tot.Add(a)
			lines = append(lines, engine.Line{Key: engine.CompKey{Type: engine.CompT, ID: id}, Amount: &a,
				Base: decimal.NewFromInt(int64(id%2) * 500)}) // even ids: zero base -> psd_basis_amount NULL
		}
		res := engine.Result{NIK: in.Emp.NIK, Lines: lines, Header: &engine.Header{TotTunjangan: tot}}
		if in.Emp.NIK == "J00002" {
			res.Header = nil // legacy D2: lines without header
		}
		return res, nil
	}
	t.Run("JAKARTA, STS D, batch > 5000, no header, NULL basis", func(t *testing.T) {
		const n = 1300
		for _, q := range []string{
			fmt.Sprintf(`INSERT INTO payroll.pyr_employee (pe_company_id, pe_nik, pe_division, pe_nama, pe_level, pe_tax_id, pe_sts_payroll, pe_orion_sub_acnt)
				SELECT 1, 'J' || lpad(g::text, 5, '0'), 'JAKARTA', 'Jkt ' || g, 'Staff', 'TK', 2, 'J' || g FROM generate_series(1, %d) g`, n),
			`INSERT INTO payroll.pyr_employee_component (pec_company_id, pec_nik, pec_division, pec_component_sysid, pec_sts, pec_tax, pec_amount, pec_frz_flag)
				SELECT 1, 'J' || lpad(g::text, 5, '0'), 'JAKARTA', c.pc_sysid, CASE WHEN g = 1 THEN 'D' ELSE 'F' END, 2, 1000, 2
				FROM generate_series(1, 1300) g JOIN payroll.pyr_component c ON c.pc_company_id = 1 AND c.pc_code = 'T001'`,
		} {
			if _, err := ownDB.Exec(q); err != nil {
				t.Fatalf("%v\n%s", err, q)
			}
		}
		seedPeriodSnap(t, "JAKARTA", 203302, n, "t17c-jkt")
		svc := run.NewServiceWith(app, ruleFunc, audit.NewWriter(), run.Runner{Calc: stub})
		res, err := svc.Commit(asScopes(t, "JAKARTA"), run.CommitCommand{CompanyID: 1, Division: "JAKARTA", YYMM: 203302, RuleSetID: 1})
		if err != nil {
			t.Fatal(err)
		}
		id := fmt.Sprint(res.RunID)
		if res.Summary.LineCount < 5001 || res.Summary.EmpCount < n {
			t.Fatalf("summary %+v", res.Summary)
		}
		if got := scalar(t, `SELECT count(*) FROM payroll.pyr_salary_detail WHERE psd_run_id=`+id); got != fmt.Sprint(res.Summary.LineCount) {
			t.Fatalf("detail %s, want %d", got, res.Summary.LineCount)
		}
		if got := scalar(t, `SELECT count(*) FROM payroll.pyr_salary_head WHERE psh_run_id=`+id+` AND psh_nik LIKE 'J%'`); got != fmt.Sprint(n-1) {
			t.Fatalf("head %s, want %d (J00002 tanpa header)", got, n-1)
		}
		if got := scalar(t, `SELECT string_agg(DISTINCT psd_sts, ',') FROM payroll.pyr_salary_detail WHERE psd_run_id=`+id+` AND psd_nik='J00001' AND psd_id=1`); got != "D" {
			t.Fatalf("psd_sts = %q, want D", got)
		}
		if got := scalar(t, `SELECT count(*) FILTER (WHERE psd_basis_amount IS NULL)||'/'||count(*) FILTER (WHERE psd_basis_amount = 500)
			FROM payroll.pyr_salary_detail WHERE psd_run_id=`+id+` AND psd_nik='J00003'`); got != "3/3" {
			t.Fatalf("basis NULL/500 = %s, want 3/3", got)
		}
		if got := scalar(t, `SELECT count(*) FROM payroll.pyr_salary_head WHERE psh_run_id=`+id+` AND psh_division<>'JAKARTA'`); got != "0" {
			t.Fatalf("head division salah: %s", got)
		}
	})

	t.Run("header total != Σ line is refused before saving", func(t *testing.T) {
		bad := func(in engine.Input) (engine.Result, error) {
			a := decimal.NewFromInt(1000)
			return engine.Result{NIK: in.Emp.NIK, Lines: []engine.Line{{Key: engine.CompKey{Type: engine.CompT, ID: 1}, Amount: &a}},
				Header: &engine.Header{TotTunjangan: decimal.NewFromInt(999)}}, nil
		}
		before := scalar(t, `SELECT count(*) FROM payroll.pyr_run WHERE pr_yymm=203302 AND pr_division='JAKARTA'`)
		_, err := run.NewServiceWith(app, ruleFunc, audit.NewWriter(), run.Runner{Calc: bad}).Commit(asScopes(t, "JAKARTA"),
			run.CommitCommand{CompanyID: 1, Division: "JAKARTA", YYMM: 203302, RuleSetID: 1})
		if err == nil || !strings.Contains(err.Error(), "header total") {
			t.Fatalf("got %v", err)
		}
		if after := scalar(t, `SELECT count(*) FROM payroll.pyr_run WHERE pr_yymm=203302 AND pr_division='JAKARTA'`); after != before {
			t.Fatalf("run berubah: %s → %s", before, after)
		}
	})

	t.Run("re-run M while M+1 holds another lock warns", func(t *testing.T) {
		lock := func(basis string) func(engine.Input) (engine.Result, error) {
			return func(in engine.Input) (engine.Result, error) {
				a, z := decimal.NewFromInt(1000), decimal.Zero
				src := engine.SubsidySrcLockNow
				if in.Period.YYMM != 203303 {
					src = engine.SubsidySrcLocked
				}
				return engine.Result{NIK: in.Emp.NIK, Lines: []engine.Line{
					{Key: engine.CompKey{Type: engine.CompT, ID: 1}, Amount: &a},
					{Key: engine.CompKey{Type: engine.CompP, ID: 0}, Amount: &z, Meta: map[string]string{
						engine.MetaSubsidySource: src, engine.MetaSubsidyBasis: basis}},
				}, Header: &engine.Header{TotTunjangan: a}}, nil
			}
		}
		for _, q := range []string{
			`DELETE FROM payroll.pyr_employee_component WHERE pec_division='JAKARTA' AND pec_nik > 'J00005'`,
			`DELETE FROM payroll.pyr_employee WHERE pe_division='JAKARTA' AND pe_nik > 'J00005'`,
		} {
			if _, err := ownDB.Exec(q); err != nil {
				t.Fatal(err)
			}
		}
		seedPeriodSnap(t, "JAKARTA", 203303, 5, "t17c-m")
		seedPeriodSnap(t, "JAKARTA", 203304, 5, "t17c-m1")
		ctx := asScopes(t, "JAKARTA")
		cm := func(y int) run.CommitCommand {
			return run.CommitCommand{CompanyID: 1, Division: "JAKARTA", YYMM: y, RuleSetID: 1}
		}
		if _, err := run.NewServiceWith(app, ruleFunc, audit.NewWriter(), run.Runner{Calc: lock("3000000")}).Commit(ctx, cm(203303)); err != nil {
			t.Fatal(err)
		}
		// M+1 (203304) uses the lock of M.
		if _, err := run.NewServiceWith(app, ruleFunc, audit.NewWriter(), run.Runner{Calc: lock("3000000")}).Commit(ctx, cm(203304)); err != nil {
			t.Fatal(err)
		}
		same, err := run.NewServiceWith(app, ruleFunc, audit.NewWriter(), run.Runner{Calc: lock("3000000")}).Commit(ctx, cm(203303))
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range same.Warnings {
			if w.Code == run.CodeLockStale {
				t.Fatalf("warning padahal kunci sama: %+v", w)
			}
		}
		changed, err := run.NewServiceWith(app, ruleFunc, audit.NewWriter(), run.Runner{Calc: lock("3500000")}).Commit(ctx, cm(203303))
		if err != nil {
			t.Fatal(err)
		}
		var found bool
		for _, w := range changed.Warnings {
			if w.Code == run.CodeLockStale && strings.Contains(w.Message, "203304 (") {
				found = true
			}
		}
		if !found {
			t.Fatalf("warning SUBSIDY_LOCK_STALE tidak muncul: %+v", changed.Warnings)
		}
	})
}
