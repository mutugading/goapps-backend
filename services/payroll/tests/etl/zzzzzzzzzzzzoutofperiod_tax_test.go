package etl

import (
	"context"
	"database/sql"
	"testing"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/postgres"
)

// T023h / D-55: rule v2 filters OUT_OF_PERIOD employees before loadComponents/loadTax, so a skipped NIK gets
// no per-NIK YTD_* issue (December reads YTD); rule v1 keeps legacy parity (every NIK is read and warned).
func TestLoader_OutOfPeriod_NoTaxIssueForSkipped(t *testing.T) {
	const yymm = 203412 // period 2034-11-26 .. 2034-12-25 (December: loadYTD runs)
	raw, err := sql.Open("pgx", appDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	app := postgres.NewDBFromSQL(raw)
	for _, q := range []string{
		`INSERT INTO payroll.pyr_employee (pe_company_id, pe_nik, pe_division, pe_nama, pe_level, pe_tax_id, pe_sts_payroll, pe_orion_sub_acnt, pe_date_join, pe_resign_date) VALUES
		  (1,'OOPT-LATE','SOLO','Join sesudah periode','Staff','TK',2,'OOPT-LATE','2035-03-01',NULL),
		  (1,'OOPT-GONE','SOLO','Resign sebelum periode','Staff','TK',2,'OOPT-GONE','2019-01-01','2034-06-30'),
		  (1,'OOPT-KEEP','SOLO','Dalam periode','Staff','TK',2,'OOPT-KEEP','2020-01-01',NULL)
		 ON CONFLICT DO NOTHING`,
		`INSERT INTO payroll.pyr_attd_days (pad_company_id, pad_yymm, pad_division, pad_days) VALUES (1,203412,'SOLO',26) ON CONFLICT DO NOTHING`,
		`INSERT INTO payroll.pyr_att_snapshot (pas_company_id, pas_yymm, pas_division, pas_source_batch_ids, pas_checksum, pas_locked)
		 VALUES (1,203412,'SOLO','{1}','t023h',false)`,
		`INSERT INTO payroll.pyr_att_snapshot_row (pasr_snapshot_id, pasr_kind, pasr_nik, pasr_data)
		 SELECT s.pas_snapshot_id, 'REKAP', n, '{"MPAR_DAY_WORK":26}'::jsonb
		 FROM payroll.pyr_att_snapshot s, unnest(ARRAY['OOPT-LATE','OOPT-GONE','OOPT-KEEP']) n WHERE s.pas_checksum = 't023h'`,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
	load := func(legacy bool) run.Result {
		var res run.Result
		q := run.Query{CompanyID: 1, Division: "SOLO", YYMM: yymm, NIKs: []string{"OOPT-LATE", "OOPT-GONE", "OOPT-KEEP"}}
		err := app.ScopedTx(asScopes(t, "SOLO"), func(tx *sql.Tx) error {
			var lerr error
			res, lerr = run.NewLoader().Load(context.Background(), tx, q, run.Rules{Set: stubRules{legacy}})
			return lerr
		})
		if err != nil {
			t.Fatalf("load legacy=%v: %v", legacy, err)
		}
		return res
	}
	count := func(res run.Result, code string) map[string]bool {
		m := map[string]bool{}
		for _, w := range res.Warnings {
			if w.Code == code {
				m[w.NIK] = true
			}
		}
		return m
	}

	v1 := load(true)
	if len(v1.Inputs) != 3 || len(count(v1, run.CodeOutOfPeriod)) != 0 {
		t.Fatalf("v1 must keep parity: inputs=%d warnings=%+v", len(v1.Inputs), v1.Warnings)
	}
	if got := count(v1, run.CodeYTDNoNPWP); len(got) != 3 {
		t.Fatalf("v1 YTD_NO_NPWP for all three NIKs, got %v", got)
	}

	v2 := load(false)
	if len(v2.Inputs) != 1 || v2.Inputs[0].Emp.NIK != "OOPT-KEEP" {
		t.Fatalf("v2 inputs: %+v", v2.Inputs)
	}
	if got := count(v2, run.CodeOutOfPeriod); len(got) != 2 || !got["OOPT-LATE"] || !got["OOPT-GONE"] {
		t.Fatalf("v2 OUT_OF_PERIOD: %v", got)
	}
	if got := count(v2, run.CodeYTDNoNPWP); len(got) != 1 || !got["OOPT-KEEP"] {
		t.Fatalf("v2 YTD_NO_NPWP must only cover the kept NIK, got %v", got)
	}
}
