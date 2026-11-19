package etl

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/etl"
)

// T033g5 (D-77 (3), 000049): p_etl writes only ETL-owned rows. A source row whose key is held by an app-made row must become
// an Exception and leave that row untouched; it must not fail RLS and roll back the whole `sync inputs` / GL load.

// inputsDir copies testdata and overrides the given tables.
func inputsDir(t *testing.T, override map[string]string) etl.PSVDir {
	t.Helper()
	dir := t.TempDir()
	ents, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		name := strings.TrimSuffix(e.Name(), ".psv")
		b, err := os.ReadFile(filepath.Join("testdata", e.Name())) //nolint:gosec // repo fixture
		if err != nil {
			t.Fatal(err)
		}
		if o, ok := override[name]; ok {
			b = []byte(o)
		}
		if err := os.WriteFile(filepath.Join(dir, e.Name()), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return etl.PSVDir{Dir: dir}
}

// ensureComponents runs the component ETL only when P000 (subsidy target) is missing: earlier tests of this package leave
// extra components behind, which would make a second full run fail its source=destination count check.
func ensureComponents(t *testing.T) {
	t.Helper()
	if scalar(t, `SELECT count(*) FROM payroll.pyr_component WHERE pc_company_id=1 AND pc_code='P000'`) != "0" {
		return
	}
	if _, err := etl.Run(context.Background(), etlDB, etl.PSVDir{Dir: "testdata"}, etl.Options{CompanyID: 1, ValidFrom: 202001}); err != nil {
		t.Fatal(err)
	}
}

func TestETL_InputsSkipAppOwnedRows(t *testing.T) {
	ctx := context.Background()
	ensureComponents(t)
	t.Cleanup(func() {
		for _, q := range []string{
			`DELETE FROM payroll.pyr_addition WHERE pa_yymm = 209801`,
			`DELETE FROM payroll.pyr_subsidy_rule WHERE psr_valid_from = 209801`,
		} {
			if _, err := ownDB.Exec(q); err != nil {
				t.Errorf("cleanup %s: %v", q, err)
			}
		}
	})
	for _, q := range []string{
		`INSERT INTO payroll.pyr_addition (pa_company_id, pa_yymm, pa_nik, pa_division, pa_type, pa_id, pa_amount, pa_source, created_by)
		 VALUES (1, 209801, 'G5A', 'SOLO', 'Tunjangan', 1, 111, 'MANUAL', 'user')`,
		`INSERT INTO payroll.pyr_subsidy_rule (psr_company_id, psr_name, psr_valid_from, psr_basis_kind, psr_limit_amount,
		   psr_subsidy_kind, psr_target_component_sysid, psr_status, created_by)
		 SELECT 1, 'ETL subsidi SOLO 209801', 209801, 'COMPONENTS_PERIOD', 5, 'FULL_PPH', pc_sysid, 'DRAFT', 'maker'
		 FROM payroll.pyr_component WHERE pc_company_id = 1 AND pc_code = 'P000'`,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatalf("fixture: %v\n%s", err, q)
		}
	}
	src := inputsDir(t, map[string]string{
		etl.TableAddition: "MPA_YYMM|MPA_NIK|MPA_NAMA|MPA_COMPNT_TYPE|MPA_COMPNT_ID|MPA_COMPNT_NAME|MPA_COMPNT_STS|MPA_COMPNT_TAX|MPA_AMOUNT|MPA_REMARKS|MPA_DIVISION\n" +
			"209801|G5A|Budi|Tunjangan|1|Bonus|F|2|999|bentrok app|SOLO\n" +
			"209801|G5B|Ani|Tunjangan|1|Bonus|F|2|222|baru|SOLO\n",
		etl.TableSubsidiPPh: "MPSP_YYMM|MPSP_YN|MPSP_LIMIT_AMT|MPSP_DIVISION|MPSP_DEPT\n209801|Y|10000000|SOLO|PROD\n",
	})
	reps, err := etl.RunInputs(ctx, etlDB, src, etl.InputOptions{CompanyID: 1})
	if err != nil {
		t.Fatalf("baris milik app harus jadi exception, bukan menggagalkan load: %v", err)
	}
	for _, r := range reps {
		switch r.Table {
		case etl.TableAddition:
			if r.DstRows != 1 || len(r.Exceptions) != 1 || !strings.Contains(r.Exceptions[0], "milik app") || !r.OK() {
				t.Errorf("addition dst=%d exc=%v", r.DstRows, r.Exceptions)
			}
		case "pyr_subsidy_rule":
			if r.DstRows != 0 || len(r.Exceptions) != 1 || !strings.Contains(r.Exceptions[0], "milik app") || !r.OK() {
				t.Errorf("subsidy dst=%d exc=%v", r.DstRows, r.Exceptions)
			}
		}
	}
	for q, want := range map[string]string{
		`SELECT pa_amount::text||pa_source FROM payroll.pyr_addition WHERE pa_nik='G5A' AND pa_yymm=209801`:         "111.00MANUAL",
		`SELECT pa_amount::text||pa_source FROM payroll.pyr_addition WHERE pa_nik='G5B' AND pa_yymm=209801`:         "222.00LEGACY",
		`SELECT count(*) FROM payroll.pyr_subsidy_rule WHERE psr_name='ETL subsidi SOLO 209801'`:                    "1",
		`SELECT created_by||psr_status||psr_limit_amount FROM payroll.pyr_subsidy_rule WHERE psr_valid_from=209801`: "makerDRAFT5.00",
	} {
		if got := scalar(t, q); got != want {
			t.Errorf("%s\n  got %q, want %q", q, got, want)
		}
	}
}

func TestETL_GlSetupSkipsAppOwnedRow(t *testing.T) {
	ctx := context.Background()
	ensureComponents(t)
	dir := writePSV(t, "MGT_PAYROLL_SETUP", glHdr,
		glRow("Operator", "", "Salary Payables", "", "205012", "205012", "", "C", "Salary Payables", "GAJI", "SOLO"),
		glRow("Driver Contract", "", "Salary Payables", "", "205000", "205000", "", "C", "Salary Payables", "GAJI", "SOLO"),
	)
	o := etl.GlSetupOptions{CompanyID: 1}
	if _, err := etl.RunGlSetup(ctx, etlDB, dir, o); err != nil {
		t.Fatal(err)
	}
	const sel = `SELECT pgs_account_main FROM payroll.pyr_gl_setup WHERE pgs_division='SOLO' AND pgs_journal_type='GAJI' AND pgs_line_kind='PAYABLE' AND pgs_gl_group=`
	t.Cleanup(func() {
		if _, err := ownDB.Exec(`UPDATE payroll.pyr_gl_setup SET pgs_legacy_row='{}'::jsonb WHERE pgs_legacy_row IS NULL AND pgs_gl_group='Operator'`); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
	// The app takes over the Operator row (no pgs_legacy_row) with its own account.
	if _, err := ownDB.Exec(`UPDATE payroll.pyr_gl_setup SET pgs_legacy_row=NULL, pgs_account_main='999999'
	  WHERE pgs_division='SOLO' AND pgs_journal_type='GAJI' AND pgs_line_kind='PAYABLE' AND pgs_gl_group='Operator'`); err != nil {
		t.Fatal(err)
	}
	rep, err := etl.RunGlSetup(ctx, etlDB, dir, o)
	if err != nil {
		t.Fatalf("baris setup milik app harus jadi exception, bukan menggagalkan load: %v", err)
	}
	if len(rep.Exceptions) != 1 || !strings.Contains(rep.Exceptions[0], "milik app") || rep.DstRows != 1 {
		t.Fatalf("dst=%d exceptions=%v", rep.DstRows, rep.Exceptions)
	}
	if got := scalar(t, sel+`'Operator'`); got != "999999" {
		t.Errorf("baris app tertimpa ETL: %s", got)
	}
	if got := scalar(t, sel+`'Driver Contract'`); got != "205000" {
		t.Errorf("baris ETL lain tidak dimuat: %s", got)
	}
}
