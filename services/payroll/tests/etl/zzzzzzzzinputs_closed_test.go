package etl

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/etl"
)

// T009e8: `sync inputs` must be re-runnable after salary-legacy closed a period. A CLOSED period's ADDITION rows are
// skipped when identical to what is loaded (idempotent) and reported as exceptions when different or missing, never
// written (fn_guard_closed would reject them with P0001 and roll back the whole inputs transaction).
func TestETL_InputsAfterSalaryLegacyClosed(t *testing.T) {
	ctx := context.Background()
	// fixture bersama punya ADDITION hanya di 202607; salin dan tambah satu baris di 202606/SOLO (ditutup salary-legacy).
	copyData := func(mutate func(add string) string) etl.PSVDir {
		dir := t.TempDir()
		ents, err := os.ReadDir("testdata")
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range ents {
			b, err := os.ReadFile(filepath.Join("testdata", e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			if e.Name() == etl.TableAddition+".psv" {
				b = []byte(mutate(strings.TrimRight(string(b), "\n") + "\n202606|2026-0001|Budi|Tunjangan|34|Bonus|F|2|900000.004|lama|SOLO\n202606|2026-0002|Sari|Tunjangan|34|Bonus|F|2|1|baru|SOLO\n"))
			}
			if err := os.WriteFile(filepath.Join(dir, e.Name()), b, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		return etl.PSVDir{Dir: dir}
	}
	src := copyData(func(a string) string { return a })
	if _, err := etl.Run(ctx, etlDB, src, etl.Options{CompanyID: 1, ValidFrom: 202001}); err != nil {
		t.Fatal(err)
	}
	if _, err := etl.RunAttLegacy(ctx, etlDB, src, etl.AttLegacyOptions{CompanyID: 1}); err != nil {
		t.Fatal(err)
	}
	o := etl.InputOptions{CompanyID: 1}
	if _, err := etl.RunSalaryLegacy(ctx, etlDB, src, etl.SalaryLegacyOptions{CompanyID: 1}); err != nil {
		t.Fatalf("salary-legacy: %v", err)
	}
	// DB tes dipakai bersama: 202606/SOLO sudah CLOSED oleh tes lain. Baris "yang sudah dimuat" disemai lewat pemilik
	// skema dengan guard dimatikan dalam satu transaksi (meniru ADDITION yang dimuat sebelum periode ditutup).
	tx, err := ownDB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`ALTER TABLE payroll.pyr_addition DISABLE TRIGGER trg_guard_addition`,
		`INSERT INTO payroll.pyr_addition (pa_company_id, pa_yymm, pa_nik, pa_division, pa_type, pa_id, pa_nama, pa_compnt_name, pa_compnt_sts, pa_compnt_tax, pa_amount, pa_remarks, pa_source)
VALUES (1,202606,'2026-0001','SOLO','Tunjangan',34,'Budi','Bonus','F',2,900000,'lama','LEGACY')
ON CONFLICT DO NOTHING`,
		`ALTER TABLE payroll.pyr_addition ENABLE TRIGGER trg_guard_addition`,
	} {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			_ = tx.Rollback()
			t.Fatalf("seed: %v", err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	closedQ := `SELECT count(*) FROM payroll.pyr_addition a JOIN payroll.pyr_period p
ON p.pp_company_id=a.pa_company_id AND p.pp_yymm=a.pa_yymm AND p.pp_division=a.pa_division WHERE p.pp_status='CLOSED'`
	nClosed := scalar(t, closedQ)
	if nClosed == "0" {
		t.Fatal("fixture: tidak ada ADDITION di periode CLOSED, tes tidak bermakna")
	}
	snap := func() string {
		return scalar(t, `SELECT md5(string_agg(pa_yymm||pa_nik||pa_type||pa_id||pa_division||pa_amount||coalesce(pa_remarks,''), ',' ORDER BY 1)) FROM payroll.pyr_addition`)
	}
	if _, err := etl.RunInputs(ctx, etlDB, src, o); err != nil { // memuat periode OPEN (202607); 202606 CLOSED dilewati
		t.Fatalf("inputs pertama setelah periode CLOSED: %v", err)
	}
	before := snap()
	reps, err := etl.RunInputs(ctx, etlDB, src, o)
	if err != nil {
		t.Fatalf("inputs setelah periode CLOSED harus lolos: %v", err)
	}
	if snap() != before {
		t.Fatal("run ulang mengubah pyr_addition")
	}
	for _, r := range reps {
		if r.Table == etl.TableAddition && len(r.Exceptions) != 1 { // hanya baris 2026-0002 (belum dimuat)
			t.Errorf("sumber identik: hanya baris yang belum ada boleh jadi exception: %v", r.Exceptions)
		}
	}

	// sumber beda untuk periode CLOSED: dilaporkan, tidak diubah.
	diff := copyData(func(a string) string { return strings.Replace(a, "|900000.004|lama|", "|950000|lama|", 1) })
	reps, err = etl.RunInputs(ctx, etlDB, diff, o)
	if err != nil {
		t.Fatalf("sumber beda: %v", err)
	}
	found := false
	for _, r := range reps {
		if r.Table == etl.TableAddition {
			found = len(r.Exceptions) == 2 && strings.Contains(r.Exceptions[0], "CLOSED") && strings.Contains(r.Exceptions[1], "CLOSED")
		}
	}
	if !found {
		t.Fatalf("baris beda di periode CLOSED harus dilaporkan: %+v", reps)
	}
	if snap() != before {
		t.Fatal("baris CLOSED berubah")
	}
}
