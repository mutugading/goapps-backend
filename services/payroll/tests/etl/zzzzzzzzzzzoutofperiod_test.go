package etl

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/period"
	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/audit"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/postgres"
)

// T023g / D-55: Commit v2 with employees outside the period succeeds without result rows for them,
// approve/close recompute the same input hash (OUT_OF_PERIOD does not block), and recon's Load runs inside
// the same REPEATABLE READ snapshot option as Commit.
func TestCommitApprove_OutOfPeriod(t *testing.T) {
	const yymm = 203501 // period 2034-12-26 .. 2035-01-25
	raw, err := sql.Open("pgx", appDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	app := postgres.NewDBFromSQL(raw)
	ctx := asScopes(t, "SOLO")
	stub := func(in engine.Input) (engine.Result, error) {
		a := dec("1000")
		return engine.Result{NIK: in.Emp.NIK, Lines: []engine.Line{{Key: engine.CompKey{Type: engine.CompT, ID: 1}, Amount: &a, Tax: true}},
			Header: &engine.Header{TotTunjangan: a}}, nil
	}
	for _, q := range []string{
		`INSERT INTO payroll.pyr_employee (pe_company_id, pe_nik, pe_division, pe_nama, pe_level, pe_tax_id, pe_sts_payroll, pe_orion_sub_acnt, pe_date_join, pe_resign_date) VALUES
		  (1,'OOP-LATE','SOLO','Join sesudah periode','Staff','TK',2,'OOP-LATE','2035-03-01',NULL),
		  (1,'OOP-GONE','SOLO','Resign sebelum periode','Staff','TK',2,'OOP-GONE','2019-01-01','2034-06-30')
		 ON CONFLICT DO NOTHING`,
		`INSERT INTO payroll.pyr_attd_days (pad_company_id, pad_yymm, pad_division, pad_days) VALUES (1,203501,'SOLO',26) ON CONFLICT DO NOTHING`,
		`INSERT INTO payroll.pyr_att_snapshot (pas_company_id, pas_yymm, pas_division, pas_source_batch_ids, pas_checksum, pas_locked)
		 VALUES (1,203501,'SOLO','{1}','t023g',false)`,
		// recap for every in-period active SOLO employee; the two outside ones deliberately have none
		`INSERT INTO payroll.pyr_att_snapshot_row (pasr_snapshot_id, pasr_kind, pasr_nik, pasr_data)
		 SELECT s.pas_snapshot_id, 'REKAP', e.pe_nik, '{"MPAR_DAY_WORK":26}'::jsonb
		 FROM payroll.pyr_att_snapshot s JOIN payroll.pyr_employee e ON e.pe_company_id = 1 AND e.pe_division = 'SOLO'
		 AND e.pe_sts_payroll = 2 AND e.pe_nik NOT IN ('OOP-LATE','OOP-GONE')
		 AND (e.pe_date_join IS NULL OR e.pe_date_join <= DATE '2035-01-25') AND (e.pe_resign_date IS NULL OR e.pe_resign_date >= DATE '2034-12-26')
		 WHERE s.pas_checksum = 't023g'`,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
	// Rule set 1 is v1 (legacy parity); D-55 only applies to v2, so swap in a v2 rule view (the engine is stubbed).
	v2Rules := func(ctx context.Context, tx *sql.Tx, q run.Query, ruleSetID int) (run.Rules, error) {
		r, err := ruleFunc(ctx, tx, q, ruleSetID)
		r.Set = stubRules{legacy: false}
		return r, err
	}
	runs := run.NewServiceWith(app, v2Rules, audit.NewWriter(), run.Runner{Calc: stub})
	svc := period.NewService(app, audit.NewWriter(), runs)

	res, err := runs.Commit(ctx, run.CommitCommand{CompanyID: 1, Division: "SOLO", YYMM: yymm, RuleSetID: 1})
	if err != nil {
		t.Fatalf("commit v2 with out-of-period NIKs: %v", err)
	}
	id := fmt.Sprint(res.RunID)
	oop := map[string]bool{}
	for _, w := range res.Warnings {
		if w.Code == run.CodeOutOfPeriod {
			oop[w.NIK] = true
		}
	}
	if len(oop) != 2 || !oop["OOP-LATE"] || !oop["OOP-GONE"] {
		t.Fatalf("OUT_OF_PERIOD warnings: %+v", res.Warnings)
	}
	for _, tbl := range []string{"pyr_salary_head h WHERE h.psh_run_id=" + id + " AND h.psh_nik LIKE 'OOP-%'",
		"pyr_salary_detail d WHERE d.psd_run_id=" + id + " AND d.psd_nik LIKE 'OOP-%'"} {
		if got := scalar(t, `SELECT count(*) FROM payroll.`+tbl); got != "0" {
			t.Fatalf("result rows for skipped NIK in %s: %s", tbl, got)
		}
	}
	if got := scalar(t, `SELECT pr_input_hash FROM payroll.pyr_run WHERE pr_id=`+id); got != res.InputHash || got == "" {
		t.Fatalf("pr_input_hash %q != Commit InputHash %q", got, res.InputHash)
	}

	cmd := period.Command{CompanyID: 1, Division: "SOLO", YYMM: yymm}
	// Approve/close recompute InputHash over the same input (skipped NIKs included in the hash) and must match.
	if err := svc.Approve(ctx, cmd); err != nil {
		t.Fatalf("approve blocked by OUT_OF_PERIOD or hash mismatch: %v", err)
	}
	if got := scalar(t, fmt.Sprintf(`SELECT pp_status FROM payroll.pyr_period WHERE pp_company_id=1 AND pp_yymm=%d AND pp_division='SOLO'`, yymm)); got != "APPROVED" {
		t.Fatalf("status %s", got)
	}
	if err := svc.Close(ctx, cmd); err != nil {
		t.Fatalf("close: %v", err)
	}
	if got := scalar(t, `SELECT pr_input_hash FROM payroll.pyr_run WHERE pr_id=`+id); got != res.InputHash {
		t.Fatalf("input hash changed after approve/close: %s", got)
	}
}
