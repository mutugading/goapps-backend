package etl

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/postgres"
)

// T033n (D-80 (3)): pyr_employee_tax_hist ditulis trigger pyr_employee, dan loader membaca status per 1 Januari.
func TestT033n_TaxHistTriggerAndLoader(t *testing.T) {
	ctx := context.Background()
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := ownDB.Exec(q, args...); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
	t.Cleanup(func() {
		_, _ = ownDB.Exec(`DELETE FROM payroll.pyr_employee WHERE pe_nik LIKE 'T33N-%'`)
		_, _ = ownDB.Exec(`DELETE FROM payroll.pyr_att_snapshot WHERE pas_yymm = 203512`)
	})
	mustExec(`INSERT INTO payroll.pyr_employee (pe_company_id, pe_nik, pe_division, pe_nama, pe_level, pe_tax_id, pe_sts_payroll, pe_orion_sub_acnt) VALUES
	  (1,'T33N-MARRY','SOLO','Menikah Juni','Staff','TK',2,'T33N-MARRY'),
	  (1,'T33N-LATE','SOLO','Status diisi tengah tahun','Staff',NULL,2,'T33N-LATE'),
	  (1,'T33N-OLD','SOLO','Riwayat tahun lalu','Staff','K1',2,'T33N-OLD'),
	  (1,'T33N-NONE','SOLO','Tanpa riwayat','Staff',NULL,2,'T33N-NONE')`)

	// Trigger: INSERT → berlaku 1 Jan tahun berjalan; NULL tidak dicatat.
	if n := scalar(t, `SELECT count(*) FROM payroll.pyr_employee_tax_hist h JOIN payroll.pyr_employee e ON e.pe_id = h.peth_employee_id
	  WHERE e.pe_nik = 'T33N-MARRY' AND h.peth_valid_from = date_trunc('year', CURRENT_DATE)::date AND h.peth_tax_id = 'TK' AND h.peth_source = 'INSERT'`); n != "1" {
		t.Fatalf("insert row = %s, want 1", n)
	}
	if n := scalar(t, `SELECT count(*) FROM payroll.pyr_employee_tax_hist h JOIN payroll.pyr_employee e ON e.pe_id = h.peth_employee_id WHERE e.pe_nik = 'T33N-NONE'`); n != "0" {
		t.Fatalf("NULL tax_id recorded: %s rows", n)
	}
	// UPDATE pe_tax_id → berlaku hari ini; dua kali di hari yang sama = satu baris (yang terakhir); update kolom lain tidak mencatat.
	mustExec(`UPDATE payroll.pyr_employee SET pe_tax_id = 'K0', updated_by = 'hr1' WHERE pe_nik = 'T33N-MARRY'`)
	mustExec(`UPDATE payroll.pyr_employee SET pe_tax_id = 'K1', updated_by = 'hr2' WHERE pe_nik = 'T33N-MARRY'`)
	mustExec(`UPDATE payroll.pyr_employee SET pe_nama = 'Menikah Juni (edit)' WHERE pe_nik = 'T33N-MARRY'`)
	got := scalar(t, `SELECT string_agg(h.peth_tax_id || '/' || h.peth_source || '/' || h.created_by, ',' ORDER BY h.peth_valid_from)
	  FROM payroll.pyr_employee_tax_hist h JOIN payroll.pyr_employee e ON e.pe_id = h.peth_employee_id WHERE e.pe_nik = 'T33N-MARRY'`)
	if got != "TK/INSERT/postgres,K1/UPDATE/hr2" && got != "TK/INSERT/payroll_migrator,K1/UPDATE/hr2" {
		t.Fatalf("history = %s", got)
	}

	// Skenario tahun pajak 2035 (tanggal ditanam langsung oleh owner, trigger memakai CURRENT_DATE):
	// MARRY TK 1 Jan → K0 10 Jun; LATE baru diisi 10 Jun (K2); OLD K1 sejak 2034, K3 Jun 2035; NONE tanpa riwayat.
	mustExec(`DELETE FROM payroll.pyr_employee_tax_hist h USING payroll.pyr_employee e WHERE e.pe_id = h.peth_employee_id AND e.pe_nik LIKE 'T33N-%'`)
	mustExec(`UPDATE payroll.pyr_employee SET pe_tax_id = CASE pe_nik WHEN 'T33N-MARRY' THEN 'K0' WHEN 'T33N-LATE' THEN 'K2' WHEN 'T33N-OLD' THEN 'K3' END
	  WHERE pe_nik IN ('T33N-MARRY','T33N-LATE','T33N-OLD')`)
	mustExec(`DELETE FROM payroll.pyr_employee_tax_hist h USING payroll.pyr_employee e WHERE e.pe_id = h.peth_employee_id AND e.pe_nik LIKE 'T33N-%'`)
	mustExec(`INSERT INTO payroll.pyr_employee_tax_hist (peth_employee_id, peth_valid_from, peth_tax_id, peth_source)
	  SELECT e.pe_id, v.d::date, v.tax, 'UPDATE' FROM (VALUES
	    ('T33N-MARRY','2035-01-01','TK'), ('T33N-MARRY','2035-06-10','K0'),
	    ('T33N-LATE','2035-06-10','K2'),
	    ('T33N-OLD','2033-01-01','K0'), ('T33N-OLD','2034-03-01','K1'), ('T33N-OLD','2035-06-10','K3')) v(nik, d, tax)
	  JOIN payroll.pyr_employee e ON e.pe_company_id = 1 AND e.pe_nik = v.nik`)
	mustExec(`INSERT INTO payroll.pyr_att_snapshot (pas_company_id, pas_yymm, pas_division, pas_source_batch_ids, pas_checksum, pas_locked)
	  VALUES (1,203512,'SOLO','{1}','t033n',true)`)

	raw, err := sql.Open("pgx", appDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	app := postgres.NewDBFromSQL(raw)
	var res run.Result
	q := run.Query{CompanyID: 1, Division: "SOLO", YYMM: 203512, NIKs: []string{"T33N-MARRY", "T33N-LATE", "T33N-OLD", "T33N-NONE"}}
	if err = app.ScopedTx(asScopes(t, "SOLO"), func(tx *sql.Tx) error {
		res, err = run.NewLoader().Load(ctx, tx, q, run.Rules{Set: stubRules{true}})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	want := map[string][2]string{ // NIK → (TaxStatus terkini, TaxStatusJan1)
		"T33N-MARRY": {"K0", "TK"}, "T33N-LATE": {"K2", "K2"}, "T33N-OLD": {"K3", "K1"}, "T33N-NONE": {"", ""},
	}
	if len(res.Inputs) != len(want) {
		t.Fatalf("inputs %d, want %d", len(res.Inputs), len(want))
	}
	for _, in := range res.Inputs {
		if w := want[in.Emp.NIK]; in.Emp.TaxStatus != w[0] || in.Emp.TaxStatusJan1 != w[1] {
			t.Errorf("%s: status %q jan1 %q, want %v", in.Emp.NIK, in.Emp.TaxStatus, in.Emp.TaxStatusJan1, w)
		}
	}

	// payroll_app hanya membaca riwayat; tulis langsung ditolak (deploy/grants.sql).
	if err = app.ScopedTx(asScopes(t, "SOLO"), func(tx *sql.Tx) error {
		_, e := tx.Exec(`DELETE FROM payroll.pyr_employee_tax_hist`)
		return e
	}); err == nil {
		t.Fatal("payroll_app DELETE pyr_employee_tax_hist allowed")
	}
}

// Migrasi 000053 down → down → up dapat diulang; up mem-backfill status terkini per 1 Jan tahun berjalan.
func TestT033n_MigrationDownUp(t *testing.T) {
	read := func(f string) string {
		b, err := os.ReadFile("../../migrations/postgres/" + f) //nolint:gosec // repo migration
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	tx, err := ownDB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }() //nolint:errcheck // test cleanup
	if _, err = tx.Exec(`INSERT INTO payroll.pyr_employee (pe_company_id, pe_nik, pe_division, pe_nama, pe_tax_id, pe_orion_sub_acnt)
	  VALUES (1,'T33N-MIG','SOLO','Mig','TK2','T33N-MIG')`); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"000053_employee_tax_hist.down.sql", "000053_employee_tax_hist.down.sql", "000053_employee_tax_hist.up.sql"} {
		if _, err = tx.Exec(read(f)); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
	}
	var n int
	if err = tx.QueryRow(`SELECT count(*) FROM payroll.pyr_employee_tax_hist h JOIN payroll.pyr_employee e ON e.pe_id = h.peth_employee_id
	  WHERE e.pe_nik = 'T33N-MIG' AND h.peth_tax_id = 'TK2' AND h.peth_source = 'BACKFILL'
	    AND h.peth_valid_from = date_trunc('year', CURRENT_DATE)::date`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("backfill row = %d (%v)", n, err)
	}
	var missing int
	if err = tx.QueryRow(`SELECT count(*) FROM payroll.pyr_employee e WHERE e.pe_tax_id IS NOT NULL
	  AND NOT EXISTS (SELECT 1 FROM payroll.pyr_employee_tax_hist h WHERE h.peth_employee_id = e.pe_id)`).Scan(&missing); err != nil || missing != 0 {
		t.Fatalf("employees without backfill = %d (%v)", missing, err)
	}
	// Trigger aktif lagi sesudah up.
	if _, err = tx.Exec(`UPDATE payroll.pyr_employee SET pe_tax_id = 'K2' WHERE pe_nik = 'T33N-MIG'`); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(`SELECT count(*) FROM payroll.pyr_employee_tax_hist h JOIN payroll.pyr_employee e ON e.pe_id = h.peth_employee_id
	  WHERE e.pe_nik = 'T33N-MIG'`).Scan(&n); err != nil || n < 1 {
		t.Fatalf("after update rows = %d (%v)", n, err)
	}
}

// T033n2: migrasi 000054 down → down → up dapat diulang; down mempertahankan baris koreksi sebagai UPDATE.
func TestT033n2_MigrationDownUp(t *testing.T) {
	read := func(f string) string {
		b, err := os.ReadFile("../../migrations/postgres/" + f) //nolint:gosec // repo migration
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	tx, err := ownDB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }() //nolint:errcheck // test cleanup
	if _, err = tx.Exec(`INSERT INTO payroll.pyr_employee (pe_company_id, pe_nik, pe_division, pe_nama, pe_tax_id, pe_orion_sub_acnt)
	  VALUES (1,'T33N2-MIG','SOLO','Mig','TK','T33N2-MIG')`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`INSERT INTO payroll.pyr_employee_tax_hist (peth_employee_id, peth_valid_from, peth_tax_id, peth_source, peth_reason)
	  SELECT pe_id, '2035-01-01', 'K1', 'CORRECT', 'salah input' FROM payroll.pyr_employee WHERE pe_nik = 'T33N2-MIG'`); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"000054_employee_tax_correction.down.sql", "000054_employee_tax_correction.down.sql",
		"000054_employee_tax_correction.up.sql"} {
		if _, err = tx.Exec(read(f)); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
	}
	var src string
	if err = tx.QueryRow(`SELECT h.peth_source FROM payroll.pyr_employee_tax_hist h JOIN payroll.pyr_employee e ON e.pe_id = h.peth_employee_id
	  WHERE e.pe_nik = 'T33N2-MIG' AND h.peth_valid_from = '2035-01-01'`).Scan(&src); err != nil || src != "UPDATE" {
		t.Fatalf("source after down/up = %q (%v)", src, err)
	}
	// CORRECT tanpa alasan ditolak.
	if _, err = tx.Exec(`SAVEPOINT s; INSERT INTO payroll.pyr_employee_tax_hist (peth_employee_id, peth_valid_from, peth_tax_id, peth_source)
	  SELECT pe_id, '2035-02-01', 'K1', 'CORRECT' FROM payroll.pyr_employee WHERE pe_nik = 'T33N2-MIG'`); err == nil {
		t.Fatal("CORRECT without reason accepted")
	}
}
