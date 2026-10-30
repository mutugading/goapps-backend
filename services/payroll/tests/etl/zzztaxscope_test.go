package etl

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/postgres"
)

// seedTaxScope builds tax year 2032: Z-S (SOLO, December target) moved from JAKARTA (Z-J, same NPWP).
// Both placements have an ACTIVE LEGACY run in January; Z-N never left SOLO.
func seedTaxScope(t *testing.T) {
	t.Helper()
	for _, q := range []string{
		`INSERT INTO payroll.pyr_component (pc_company_id, pc_code, pc_type, pc_name, pc_status, pc_valid_from)
		 VALUES (1,'T001','Tunjangan','Gaji Pokok','ACTIVE',202001) ON CONFLICT (pc_company_id, pc_code) DO NOTHING`,
		`INSERT INTO payroll.pyr_employee (pe_company_id, pe_nik, pe_division, pe_nama, pe_level, pe_tax_id, pe_sts_payroll, pe_orion_sub_acnt, pe_npwp_hash) VALUES
		  (1,'Z-S','SOLO','S','Staff','TK',2,'Z-S','\x7a'), (1,'Z-J','JAKARTA','S lama','Staff','TK',1,'Z-J','\x7a'),
		  (1,'Z-N','SOLO','N','Staff','TK',2,'Z-N','\x7b'),
		  (1,'Z-X','SOLO','X','Staff','TK',2,'Z-X','\x7c'), (1,'Z-XJ','JAKARTA','X lama','Staff','TK',1,'Z-XJ','\x7c')`,
		`INSERT INTO payroll.pyr_addition (pa_company_id, pa_yymm, pa_nik, pa_division, pa_type, pa_id, pa_amount, pa_remarks, pa_source)
		 VALUES (1,203203,'Z-XJ','JAKARTA','Potongan',7,-1000,'PPh THR','MANUAL')`,
		`INSERT INTO payroll.pyr_period (pp_company_id, pp_yymm, pp_division, pp_date_from, pp_date_to)
		 VALUES (1,203212,'SOLO','2032-11-26','2032-12-25'), (1,203201,'SOLO','2031-12-26','2032-01-25'),
		        (1,203201,'JAKARTA','2031-12-26','2032-01-25')`,
		`INSERT INTO payroll.pyr_attd_days (pad_company_id, pad_yymm, pad_division, pad_days) VALUES (1,203212,'SOLO',26)`,
		`INSERT INTO payroll.pyr_att_snapshot (pas_company_id, pas_yymm, pas_division, pas_source_batch_ids, pas_checksum, pas_locked)
		 VALUES (1,203212,'SOLO','{1}','taxscope',true)`,
		`INSERT INTO payroll.pyr_att_snapshot_row (pasr_snapshot_id, pasr_kind, pasr_nik, pasr_data)
		 SELECT s.pas_snapshot_id, 'REKAP', n, '{"MPAR_DAY_WORK":26,"MPAR_PAY_TOT":26}'::jsonb
		 FROM payroll.pyr_att_snapshot s, unnest(ARRAY['Z-S','Z-N','Z-X']) n
		 WHERE s.pas_company_id = 1 AND s.pas_division = 'SOLO' AND s.pas_yymm = 203212`,
		`INSERT INTO payroll.pyr_run (pr_company_id, pr_yymm, pr_division, pr_pay_date, pr_rule_set_id, pr_snapshot_id, pr_status, pr_kind)
		 VALUES (1,203201,'SOLO','2032-01-25',0,NULL,'ACTIVE','LEGACY'), (1,203201,'JAKARTA','2032-01-25',0,NULL,'ACTIVE','LEGACY')`,
		`INSERT INTO payroll.pyr_salary_detail (psd_run_id, psd_nik, psd_type, psd_id, psd_amount, psd_tax, psd_tax_paid_company, psd_component_sysid)
		 SELECT r.pr_id, v.nik, 'Tunjangan', 1, v.amt, 2, 'N', c.pc_sysid
		 FROM (VALUES ('SOLO','Z-S',2000000), ('JAKARTA','Z-J',1000000), ('SOLO','Z-N',500000)) v(div,nik,amt)
		 JOIN payroll.pyr_run r ON r.pr_company_id = 1 AND r.pr_yymm = 203201 AND r.pr_division = v.div
		 JOIN payroll.pyr_component c ON c.pc_company_id = 1 AND c.pc_code = 'T001'`,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
}

func TestRunLoader_YTDOutOfScope(t *testing.T) {
	ctx := context.Background()
	seedTaxScope(t)
	raw, err := sql.Open("pgx", appDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	app := postgres.NewDBFromSQL(raw)
	load := func(legacy bool, divs ...string) (map[string]engine.Input, []run.Issue, error) {
		var res run.Result
		err := app.ScopedTx(asScopes(t, divs...), func(tx *sql.Tx) error {
			var lerr error
			res, lerr = run.NewLoader().Load(ctx, tx, run.Query{CompanyID: 1, Division: "SOLO", YYMM: 203212, NIKs: []string{"Z-S", "Z-N", "Z-X"}},
				run.Rules{Set: stubRules{legacy}, Subsidy: func(string) engine.Subsidy { return engine.Subsidy{} }})
			return lerr
		})
		out := map[string]engine.Input{}
		for _, in := range res.Inputs {
			out[in.Emp.NIK] = in
		}
		return out, res.Warnings, err
	}
	outOfScope := func(ws []run.Issue) (niks []string) {
		for _, w := range ws {
			if w.Code == run.CodeYTDOutOfScope {
				niks = append(niks, w.NIK)
			}
		}
		return niks
	}

	t.Run("v1 scope SOLO only: warning, JAKARTA not summed", func(t *testing.T) {
		ins, warns, err := load(true, "SOLO")
		if err != nil {
			t.Fatal(err)
		}
		if got := outOfScope(warns); len(got) != 2 || got[0] != "Z-S" || got[1] != "Z-X" { // Z-X: only a P7 addition in JAKARTA
			t.Errorf("out-of-scope warnings: %v (%+v)", got, warns)
		}
		if g := ins["Z-S"].YTD.Gross; !g.Equal(dec("2000000")) {
			t.Errorf("YTD.Gross = %s, want 2000000", g)
		}
	})
	t.Run("v2 scope SOLO only: error", func(t *testing.T) {
		if _, _, err := load(false, "SOLO"); !errors.Is(err, run.ErrYTDOutOfScope) {
			t.Errorf("got %v", err)
		}
	})
	t.Run("both divisions in scope: summed, no signal", func(t *testing.T) {
		for _, legacy := range []bool{true, false} {
			ins, warns, err := load(legacy, "SOLO", "JAKARTA")
			if err != nil {
				t.Fatalf("legacy=%v: %v", legacy, err)
			}
			if got := outOfScope(warns); len(got) != 0 {
				t.Errorf("legacy=%v warnings: %v", legacy, got)
			}
			if g := ins["Z-S"].YTD.Gross; !g.Equal(dec("3000000")) {
				t.Errorf("legacy=%v YTD.Gross = %s, want 3000000", legacy, g)
			}
		}
	})
}
