package etl

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/recon"
	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/postgres"
)

const reconYYMM = 203305

// seedRecon: three SOLO employees, a legacy snapshot A and a newer snapshot B (R00002 recap changed), an ACTIVE
// LEGACY run on A and a SUPERSEDED ENGINE run on B. Legacy lines: R00001 matches, R00002 has another basis
// (INPUT_MASTER on the dry target), R00003 is 500 off (FORMULA), R00004 exists only in legacy.
func seedRecon(t *testing.T) (legacyRun, engineRun int64) {
	t.Helper()
	y := fmt.Sprint(reconYYMM)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := ownDB.Exec(q, args...); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
	exec(`INSERT INTO payroll.pyr_component (pc_company_id, pc_code, pc_type, pc_name, pc_status, pc_valid_from)
	 VALUES (1,'T001','Tunjangan','Gaji Pokok','ACTIVE',202001) ON CONFLICT (pc_company_id, pc_code) DO NOTHING`)
	exec(`INSERT INTO payroll.pyr_employee (pe_company_id, pe_nik, pe_division, pe_nama, pe_level, pe_tax_id, pe_sts_payroll, pe_orion_sub_acnt, pe_dept)
	 SELECT 1, 'R0000' || g, 'SOLO', 'Recon ' || g, 'Staff', 'TK', 2, 'R' || g, 'PROD' FROM generate_series(1, 3) g`)
	exec(`INSERT INTO payroll.pyr_employee_component (pec_company_id, pec_nik, pec_division, pec_component_sysid, pec_sts, pec_tax, pec_amount, pec_frz_flag)
	 SELECT 1, 'R0000' || g, 'SOLO', c.pc_sysid, 'F', 0, 3000000, 2
	 FROM generate_series(1, 3) g JOIN payroll.pyr_component c ON c.pc_company_id = 1 AND c.pc_code = 'T001'`)
	exec(`INSERT INTO payroll.pyr_attd_days (pad_company_id, pad_yymm, pad_division, pad_days) VALUES (1,` + y + `,'SOLO',26)`)
	exec(`INSERT INTO payroll.pyr_period (pp_company_id, pp_yymm, pp_division, pp_date_from, pp_date_to, pp_status)
	 VALUES (1,` + y + `,'SOLO','2033-04-26','2033-05-25','OPEN')`)
	exec(`INSERT INTO payroll.pyr_att_snapshot (pas_company_id, pas_yymm, pas_division, pas_source_batch_ids, pas_checksum)
	 VALUES (1,` + y + `,'SOLO','{}','recon-a'), (1,` + y + `,'SOLO','{9}','recon-b')`)
	exec(`INSERT INTO payroll.pyr_att_snapshot_row (pasr_snapshot_id, pasr_kind, pasr_nik, pasr_data)
	 SELECT s.pas_snapshot_id, 'REKAP', 'R0000' || g,
	   CASE WHEN s.pas_checksum = 'recon-b' AND g = 2 THEN '{"MPAR_DAY_WORK":20,"MPAR_PAY_TOT":20}' ELSE '{"MPAR_DAY_WORK":26,"MPAR_PAY_TOT":26}' END::jsonb
	 FROM payroll.pyr_att_snapshot s, generate_series(1, 3) g
	 WHERE s.pas_company_id = 1 AND s.pas_yymm = ` + y + ` AND s.pas_division = 'SOLO'`)
	snap := func(sum string) string {
		return `(SELECT pas_snapshot_id FROM payroll.pyr_att_snapshot WHERE pas_yymm=` + y + ` AND pas_checksum='` + sum + `')`
	}
	if err := ownDB.QueryRow(`INSERT INTO payroll.pyr_run (pr_company_id, pr_yymm, pr_division, pr_pay_date, pr_rule_set_id, pr_snapshot_id, pr_status, pr_kind)
	 VALUES (1,` + y + `,'SOLO','2033-05-25',0,` + snap("recon-a") + `,'ACTIVE','LEGACY') RETURNING pr_id`).Scan(&legacyRun); err != nil {
		t.Fatal(err)
	}
	if err := ownDB.QueryRow(`INSERT INTO payroll.pyr_run (pr_company_id, pr_yymm, pr_division, pr_pay_date, pr_rule_set_id, pr_snapshot_id, pr_status, pr_kind)
	 VALUES (1,` + y + `,'SOLO','2033-05-25',1,` + snap("recon-b") + `,'SUPERSEDED','ENGINE') RETURNING pr_id`).Scan(&engineRun); err != nil {
		t.Fatal(err)
	}
	lines := func(runID int64, rows string) {
		exec(`INSERT INTO payroll.pyr_salary_detail (psd_run_id, psd_nik, psd_type, psd_id, psd_amount, psd_basis_amount, psd_sts)
		 SELECT $1, v.nik, 'Tunjangan', 1, v.amt, v.basis, 'F' FROM (VALUES `+rows+`) v(nik, amt, basis)`, runID)
		exec(`INSERT INTO payroll.pyr_salary_head (psh_run_id, psh_nik, psh_yymm, psh_division, psh_tot_tunjangan, psh_tot_potongan, psh_rounding, psh_company_id)
		 SELECT $1, psd_nik, `+y+`, 'SOLO', sum(psd_amount), 0, 0, 1 FROM payroll.pyr_salary_detail WHERE psd_run_id = $1 GROUP BY psd_nik`, runID)
	}
	lines(legacyRun, `('R00001',3000000::numeric,3000000::numeric),('R00002',3100000,3100000),('R00003',3000500,3000000),('R00004',100,100)`)
	lines(engineRun, `('R00001',3000000::numeric,NULL::numeric),('R00002',2300000,NULL),('R00003',3000000,NULL)`)
	return legacyRun, engineRun
}

func TestRecon_DiffSourcePG(t *testing.T) {
	legacyRun, engineRun := seedRecon(t)
	raw, err := sql.Open("pgx", strings.Replace(appDSN, "user=payroll_app password=payroll_app123", "user=payroll_ro password=payroll_ro123", 1))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	src := recon.DiffSource{DB: postgres.NewDBFromSQL(raw), Rules: ruleFunc, Runner: run.NewRunner()}
	ctx := asScopes(t, "SOLO")
	q := recon.DiffQuery{CompanyID: 1, Division: "SOLO", YYMM: reconYYMM, RuleSetID: 1}
	runCount := scalar(t, `SELECT count(*) FROM payroll.pyr_run`)

	t.Run("dry uses the legacy snapshot", func(t *testing.T) {
		q.Target = recon.TargetDry
		in, err := src.Load(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		if in.Meta.LegacyRunID != legacyRun || in.Meta.TargetSnapshotID != in.Meta.LegacySnapshotID || len(in.AttChanged) != 0 {
			t.Fatalf("meta %+v att %v", in.Meta, in.AttChanged)
		}
		got := t001(recon.Diff(in))
		want := map[string]string{"R00002": recon.CatInputMaster, "R00003": recon.CatFormula, "R00004": recon.CatOnlyLegacy}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("T001 categories %v, want %v", got, want)
		}
	})
	t.Run("run target compares snapshots", func(t *testing.T) {
		q.Target = fmt.Sprintf("run:%d", engineRun)
		in, err := src.Load(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		if !in.AttChanged["R00002"] || len(in.AttChanged) != 1 || in.Meta.TargetSnapshotID == in.Meta.LegacySnapshotID {
			t.Fatalf("att changed %v meta %+v", in.AttChanged, in.Meta)
		}
		rep := recon.Diff(in)
		got := t001(rep)
		want := map[string]string{"R00002": recon.CatInputAtt, "R00003": recon.CatFormula, "R00004": recon.CatOnlyLegacy}
		if fmt.Sprint(got) != fmt.Sprint(want) || rep.Summary.Parity {
			t.Fatalf("T001 categories %v, want %v (parity %t)", got, want, rep.Summary.Parity)
		}
	})
	t.Run("errors", func(t *testing.T) {
		q.Target = fmt.Sprintf("run:%d", legacyRun) // a LEGACY run is not a target
		if _, err := src.Load(ctx, q); !errors.Is(err, recon.ErrTargetRun) {
			t.Fatalf("legacy run as target: %v", err)
		}
		q.Target, q.YYMM = recon.TargetDry, reconYYMM+1
		if _, err := src.Load(ctx, q); !errors.Is(err, recon.ErrNoLegacyRun) {
			t.Fatalf("no legacy run: %v", err)
		}
		if _, err := src.Load(asScopes(t, "JAKARTA"), recon.DiffQuery{CompanyID: 1, Division: "SOLO", YYMM: reconYYMM,
			RuleSetID: 1, Target: recon.TargetDry}); !errors.Is(err, recon.ErrNoLegacyRun) {
			t.Fatalf("out of scope must see nothing: %v", err)
		}
	})
	if got := scalar(t, `SELECT count(*) FROM payroll.pyr_run`); got != runCount {
		t.Fatalf("recon wrote runs: %s -> %s", runCount, got)
	}
	if _, err := raw.ExecContext(context.Background(), `DELETE FROM payroll.pyr_run WHERE pr_id = $1`, engineRun); err == nil {
		t.Fatal("payroll_ro must not write")
	}
}

// t001 maps NIK → category of the non-MATCH T001 lines of this fixture (other tests leave SOLO employees active).
func t001(rep recon.DiffReport) map[string]string {
	out := map[string]string{}
	for _, l := range rep.Lines {
		if l.Component == "T001" && strings.HasPrefix(l.NIK, "R0000") {
			out[l.NIK] = l.Category
		}
	}
	return out
}
