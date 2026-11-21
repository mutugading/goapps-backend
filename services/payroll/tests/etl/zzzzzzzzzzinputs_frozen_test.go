package etl

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/etl"
)

// T023e (D-54): LoadTaxAdj/LoadAttdDays never write to an APPROVED/CLOSED period. A source row identical to the stored
// one counts as loaded, a different or missing one is an exception, and the load does not fail with P0001.
func TestETL_TaxAdjAttdDaysFrozenPeriods(t *testing.T) {
	ctx := context.Background()
	src := etl.PSVDir{Dir: t.TempDir()}
	dir := src.Dir
	ents, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}
	extra := map[string]string{
		etl.TableAttdDays: "203401|SOLO|20\n203402|SOLO|22\n203402|JAKARTA|18\n",
		etl.TableTaxAdj:   "203401|SOLO|2026-0001|Budi|KURANG|koreksi|100.50\n203402|SOLO|2026-0001|Budi|KURANG|koreksi|200\n203402|JAKARTA|2026-0001|Budi|KURANG|baru|1\n",
	}
	for _, e := range ents {
		b, err := os.ReadFile(filepath.Join("testdata", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if add, ok := extra[strings.TrimSuffix(e.Name(), ".psv")]; ok {
			b = []byte(strings.TrimRight(string(b), "\n") + "\n" + add)
		}
		if err := os.WriteFile(filepath.Join(dir, e.Name()), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := etl.Run(ctx, etlDB, src, etl.Options{CompanyID: 1, ValidFrom: 202001}); err != nil {
		t.Fatal(err)
	}
	// 203401/SOLO APPROVED, 203402/SOLO and /JAKARTA CLOSED. Rows are seeded while DRAFT, then the period is frozen.
	for _, q := range []string{
		`INSERT INTO payroll.pyr_period (pp_company_id, pp_yymm, pp_division, pp_date_from, pp_date_to, pp_status) VALUES
		 (1,203401,'SOLO','2033-12-26','2034-01-25','DRAFT'),(1,203402,'SOLO','2034-01-26','2034-02-25','DRAFT'),(1,203402,'JAKARTA','2034-01-26','2034-02-25','DRAFT')`,
		`INSERT INTO payroll.pyr_attd_days (pad_company_id, pad_yymm, pad_division, pad_days) VALUES (1,203401,'SOLO',20),(1,203402,'SOLO',21)`,
		`INSERT INTO payroll.pyr_tax_adj (pta_company_id, pta_yymm, pta_division, pta_nik, pta_type, pta_nama, pta_reason, pta_amount)
		 VALUES (1,203401,'SOLO','2026-0001','KURANG','Budi','koreksi',100.50),(1,203402,'SOLO','2026-0001','KURANG','Budi','koreksi',100)`,
		`UPDATE payroll.pyr_period SET pp_status='APPROVED', approved_by='t' WHERE pp_yymm=203401`,
		`UPDATE payroll.pyr_period SET pp_status='CLOSED', closed_by='t' WHERE pp_yymm=203402`,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
	snap := func() string {
		return scalar(t, `SELECT (SELECT md5(string_agg(pad_yymm||pad_division||pad_days, ',' ORDER BY pad_yymm, pad_division)) FROM payroll.pyr_attd_days WHERE pad_yymm >= 203401)
||(SELECT md5(string_agg(pta_yymm||pta_division||pta_nik||pta_amount, ',' ORDER BY pta_yymm, pta_division)) FROM payroll.pyr_tax_adj WHERE pta_yymm >= 203401)`)
	}
	before := snap()
	reps, err := etl.RunInputs(ctx, etlDB, src, etl.InputOptions{CompanyID: 1})
	if err != nil {
		t.Fatalf("inputs ke periode APPROVED/CLOSED harus lolos tanpa P0001: %v", err)
	}
	if snap() != before {
		t.Fatal("periode APPROVED/CLOSED berubah")
	}
	for _, r := range reps {
		if r.Table != etl.TableAttdDays && r.Table != etl.TableTaxAdj {
			continue
		}
		// Identical 203401 row is loaded; the different 203402/SOLO and missing 203402/JAKARTA rows are exceptions.
		if len(r.Exceptions) != 2 {
			t.Errorf("%s: want 2 exceptions, got %v", r.Table, r.Exceptions)
			continue
		}
		for _, ex := range r.Exceptions {
			if !strings.Contains(ex, "CLOSED") || strings.Contains(ex, "203401") {
				t.Errorf("%s: exception tidak sesuai: %s", r.Table, ex)
			}
		}
	}
}
