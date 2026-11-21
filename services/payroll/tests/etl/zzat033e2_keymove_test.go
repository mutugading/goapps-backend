package etl

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/etl"
)

// T033e2: periode/event uji key-move. 209801 = CLOSED (sumber/tujuan terlarang), 209802 = OPEN.
const kmComp = "KM-T1"

var kmGuardTriggers = [][2]string{
	{"pyr_addition", "trg_guard_addition"}, {"pyr_salary_head", "trg_guard_header"}, {"pyr_salary_detail", "trg_guard_line"},
	{"pyr_thr_event", "trg_guard_thr_event"}, {"pyr_thr_run", "trg_guard_thr_run"}, {"pyr_thr_head", "trg_guard_thr_head"},
	{"pyr_thr_detail", "trg_guard_thr_detail"}, {"pyr_thr_input", "trg_guard_thr_input"}, {"pyr_thr_override", "trg_guard_thr_override"},
}

// dropKeyMove menghapus semua baris uji key-move (periode 209801/209802, event THR 209801/209802, baris KM-*) dengan guard
// dimatikan sementara; dipakai sebelum seed dan sebagai cleanup, jadi aman dipanggil berulang.
func dropKeyMove(t *testing.T) {
	t.Helper()
	var qs []string
	for _, g := range kmGuardTriggers {
		qs = append(qs, "ALTER TABLE payroll."+g[0]+" DISABLE TRIGGER "+g[1])
	}
	qs = append(qs,
		`DELETE FROM payroll.pyr_addition WHERE pa_nik LIKE 'KM-%'`,
		`DELETE FROM payroll.pyr_salary_detail WHERE psd_run_id IN (SELECT pr_id FROM payroll.pyr_run WHERE pr_yymm IN (209801,209802))`,
		`DELETE FROM payroll.pyr_salary_head WHERE psh_run_id IN (SELECT pr_id FROM payroll.pyr_run WHERE pr_yymm IN (209801,209802))`,
		`UPDATE payroll.pyr_period SET pp_active_run_id = NULL WHERE pp_yymm IN (209801,209802)`,
		`DELETE FROM payroll.pyr_run WHERE pr_yymm IN (209801,209802)`,
		`DELETE FROM payroll.pyr_thr_override WHERE pto_event_id IN (SELECT pte_id FROM payroll.pyr_thr_event WHERE pte_yymm IN (209801,209802))`,
		`DELETE FROM payroll.pyr_thr_input WHERE pti_event_id IN (SELECT pte_id FROM payroll.pyr_thr_event WHERE pte_yymm IN (209801,209802))`,
		`UPDATE payroll.pyr_thr_event SET pte_active_run_id = NULL WHERE pte_yymm IN (209801,209802)`,
		`DELETE FROM payroll.pyr_thr_run WHERE ptr_event_id IN (SELECT pte_id FROM payroll.pyr_thr_event WHERE pte_yymm IN (209801,209802))`,
		`DELETE FROM payroll.pyr_thr_event WHERE pte_yymm IN (209801,209802)`,
		`DELETE FROM payroll.pyr_period WHERE pp_yymm IN (209801,209802)`,
		`DELETE FROM payroll.pyr_employee WHERE pe_nik LIKE 'KM-%'`,
		`DELETE FROM payroll.pyr_component WHERE pc_code = '`+kmComp+`'`,
	)
	for _, g := range kmGuardTriggers {
		qs = append(qs, "ALTER TABLE payroll."+g[0]+" ENABLE TRIGGER "+g[1])
	}
	for _, q := range qs {
		if _, err := ownDB.Exec(q); err != nil {
			t.Errorf("cleanup key-move: %s: %v", q, err)
		}
	}
}

func execAll(t *testing.T, qs ...string) {
	t.Helper()
	for _, q := range qs {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatalf("seed: %s: %v", q, err)
		}
	}
}

type kmCase struct{ name, sql string }

// wantClosed menegaskan setiap UPDATE key-move ditolak guard CLOSED.
func wantClosed(t *testing.T, cases []kmCase) {
	t.Helper()
	for _, c := range cases {
		_, err := ownDB.Exec(c.sql)
		if err == nil || !strings.Contains(err.Error(), "CLOSED") {
			t.Errorf("%s: key-move harus ditolak guard CLOSED, err=%v", c.name, err)
		}
	}
}

// T033e2 (1a): fn_guard_thr_closed memeriksa event OLD dan NEW untuk head/detail/input/override/run.
func TestGuardThrClosed_KeyMoveRejected(t *testing.T) {
	dropKeyMove(t)
	t.Cleanup(func() { dropKeyMove(t) })
	execAll(t,
		`INSERT INTO payroll.pyr_component (pc_company_id, pc_code, pc_type, pc_name, pc_valid_from) VALUES (1,'`+kmComp+`','Tunjangan','KM uji',202001)`,
		`INSERT INTO payroll.pyr_employee (pe_company_id, pe_nik, pe_division, pe_nama, pe_orion_sub_acnt)
		 VALUES (1,'KM-E1','SOLO','Km Satu','KM-E1'),(1,'KM-E2','SOLO','Km Dua','KM-E2')`,
		`INSERT INTO payroll.pyr_thr_event (pte_company_id, pte_division, pte_kind, pte_yymm, pte_event_date, pte_description, created_by)
		 VALUES (1,'SOLO','THR',209801,'2098-01-10','km closed','t'),(1,'SOLO','THR',209802,'2098-02-10','km open','t')`,
		`INSERT INTO payroll.pyr_thr_run (ptr_event_id, ptr_kind, ptr_status)
		 SELECT pte_id, 'NORMAL', 'ACTIVE' FROM payroll.pyr_thr_event WHERE pte_yymm IN (209801,209802) AND pte_kind='THR'`,
		// event 209801 (CLOSED nanti) memuat KM-E1; event 209802 (OPEN) memuat KM-E2, supaya PK tidak bentrok.
		`INSERT INTO payroll.pyr_thr_head (pth_run_id, pth_employee_id, pth_nik, pth_tot_tunjangan, pth_tot_potongan, pth_rounding)
		 SELECT r.ptr_id, e.pe_id, e.pe_nik, 0, 0, 0 FROM payroll.pyr_thr_run r
		 JOIN payroll.pyr_thr_event ev ON ev.pte_id = r.ptr_event_id JOIN payroll.pyr_employee e ON e.pe_nik = CASE ev.pte_yymm WHEN 209801 THEN 'KM-E1' ELSE 'KM-E2' END
		 WHERE ev.pte_yymm IN (209801,209802)`,
		`INSERT INTO payroll.pyr_thr_detail (ptd_run_id, ptd_employee_id, ptd_nik, ptd_component_sysid, ptd_type, ptd_id, ptd_amount)
		 SELECT r.ptr_id, e.pe_id, e.pe_nik, (SELECT pc_sysid FROM payroll.pyr_component WHERE pc_code='`+kmComp+`'), 'Tunjangan', 1, 1
		 FROM payroll.pyr_thr_run r JOIN payroll.pyr_thr_event ev ON ev.pte_id = r.ptr_event_id
		 JOIN payroll.pyr_employee e ON e.pe_nik = CASE ev.pte_yymm WHEN 209801 THEN 'KM-E1' ELSE 'KM-E2' END
		 WHERE ev.pte_yymm IN (209801,209802)`,
		`INSERT INTO payroll.pyr_thr_input (pti_event_id, pti_employee_id, pti_component_sysid, pti_amount)
		 SELECT ev.pte_id, e.pe_id, (SELECT pc_sysid FROM payroll.pyr_component WHERE pc_code='`+kmComp+`'), 1
		 FROM payroll.pyr_thr_event ev JOIN payroll.pyr_employee e ON e.pe_nik = CASE ev.pte_yymm WHEN 209801 THEN 'KM-E1' ELSE 'KM-E2' END
		 WHERE ev.pte_yymm IN (209801,209802)`,
		`INSERT INTO payroll.pyr_thr_override (pto_event_id, pto_employee_id, pto_action, pto_reason, created_by)
		 SELECT ev.pte_id, e.pe_id, 'EXCLUDE', 'km', 't'
		 FROM payroll.pyr_thr_event ev JOIN payroll.pyr_employee e ON e.pe_nik = CASE ev.pte_yymm WHEN 209801 THEN 'KM-E1' ELSE 'KM-E2' END
		 WHERE ev.pte_yymm IN (209801,209802)`,
		`UPDATE payroll.pyr_thr_event SET pte_status='CLOSED' WHERE pte_yymm=209801 AND pte_kind='THR'`,
	)
	closedEv := `(SELECT pte_id FROM payroll.pyr_thr_event WHERE pte_yymm=209801)`
	openEv := `(SELECT pte_id FROM payroll.pyr_thr_event WHERE pte_yymm=209802)`
	closedRun := `(SELECT ptr_id FROM payroll.pyr_thr_run WHERE ptr_event_id=` + closedEv + `)`
	openRun := `(SELECT ptr_id FROM payroll.pyr_thr_run WHERE ptr_event_id=` + openEv + `)`
	wantClosed(t, []kmCase{
		// OLD di event CLOSED, NEW di event OPEN (keluar).
		{"run keluar", `UPDATE payroll.pyr_thr_run SET ptr_event_id=` + openEv + ` WHERE ptr_id=` + closedRun},
		{"head keluar", `UPDATE payroll.pyr_thr_head SET pth_run_id=` + openRun + ` WHERE pth_run_id=` + closedRun},
		{"detail keluar", `UPDATE payroll.pyr_thr_detail SET ptd_run_id=` + openRun + ` WHERE ptd_run_id=` + closedRun},
		{"input keluar", `UPDATE payroll.pyr_thr_input SET pti_event_id=` + openEv + ` WHERE pti_event_id=` + closedEv},
		{"override keluar", `UPDATE payroll.pyr_thr_override SET pto_event_id=` + openEv + ` WHERE pto_event_id=` + closedEv},
		// OLD di event OPEN, NEW di event CLOSED (masuk).
		{"run masuk", `UPDATE payroll.pyr_thr_run SET ptr_event_id=` + closedEv + ` WHERE ptr_id=` + openRun},
		{"head masuk", `UPDATE payroll.pyr_thr_head SET pth_run_id=` + closedRun + ` WHERE pth_run_id=` + openRun},
		{"detail masuk", `UPDATE payroll.pyr_thr_detail SET ptd_run_id=` + closedRun + ` WHERE ptd_run_id=` + openRun},
		{"input masuk", `UPDATE payroll.pyr_thr_input SET pti_event_id=` + closedEv + ` WHERE pti_event_id=` + openEv},
		{"override masuk", `UPDATE payroll.pyr_thr_override SET pto_event_id=` + closedEv + ` WHERE pto_event_id=` + openEv},
	})
	// Kontrol: ubah biasa di event OPEN tetap boleh.
	if _, err := ownDB.Exec(`UPDATE payroll.pyr_thr_input SET pti_amount=2 WHERE pti_event_id=` + openEv); err != nil {
		t.Fatalf("update input di event OPEN: %v", err)
	}
	if n := scalar(t, `SELECT count(*)::text FROM payroll.pyr_thr_input WHERE pti_event_id=`+closedEv); n != "1" {
		t.Errorf("input event CLOSED berubah setelah key-move ditolak: %s", n)
	}
}

// T033e2 (1b): fn_guard_closed (salary_head/detail) memeriksa periode OLD dan NEW lewat run.
func TestGuardClosed_SalaryKeyMoveRejected(t *testing.T) {
	dropKeyMove(t)
	t.Cleanup(func() { dropKeyMove(t) })
	execAll(t,
		`INSERT INTO payroll.pyr_period (pp_company_id, pp_yymm, pp_division, pp_date_from, pp_date_to, pp_status)
		 VALUES (1,209801,'SOLO','2097-12-26','2098-01-25','OPEN'),(1,209802,'SOLO','2098-01-26','2098-02-25','OPEN')`,
		`INSERT INTO payroll.pyr_run (pr_company_id, pr_yymm, pr_division, pr_pay_date, pr_rule_set_id, pr_snapshot_id, pr_status, pr_kind)
		 VALUES (1,209801,'SOLO','2098-01-25',0,NULL,'ACTIVE','LEGACY'),(1,209802,'SOLO','2098-02-25',0,NULL,'ACTIVE','LEGACY')`,
		// run 209801 memuat KM-S1; run 209802 memuat KM-S2.
		`INSERT INTO payroll.pyr_salary_head (psh_run_id, psh_company_id, psh_nik, psh_yymm, psh_division, psh_tot_tunjangan, psh_tot_potongan, psh_rounding)
		 SELECT pr_id, 1, CASE pr_yymm WHEN 209801 THEN 'KM-S1' ELSE 'KM-S2' END, pr_yymm, 'SOLO', 0, 0, 0 FROM payroll.pyr_run WHERE pr_yymm IN (209801,209802)`,
		`INSERT INTO payroll.pyr_salary_detail (psd_run_id, psd_nik, psd_type, psd_id, psd_amount)
		 SELECT pr_id, CASE pr_yymm WHEN 209801 THEN 'KM-S1' ELSE 'KM-S2' END, 'Tunjangan', 1, 1 FROM payroll.pyr_run WHERE pr_yymm IN (209801,209802)`,
		`UPDATE payroll.pyr_period SET pp_status='CLOSED' WHERE pp_company_id=1 AND pp_yymm=209801 AND pp_division='SOLO'`,
	)
	closedRun := `(SELECT pr_id FROM payroll.pyr_run WHERE pr_yymm=209801)`
	openRun := `(SELECT pr_id FROM payroll.pyr_run WHERE pr_yymm=209802)`
	wantClosed(t, []kmCase{
		{"head keluar", `UPDATE payroll.pyr_salary_head SET psh_run_id=` + openRun + ` WHERE psh_run_id=` + closedRun},
		{"detail keluar", `UPDATE payroll.pyr_salary_detail SET psd_run_id=` + openRun + ` WHERE psd_run_id=` + closedRun},
		{"head masuk", `UPDATE payroll.pyr_salary_head SET psh_run_id=` + closedRun + ` WHERE psh_run_id=` + openRun},
		{"detail masuk", `UPDATE payroll.pyr_salary_detail SET psd_run_id=` + closedRun + ` WHERE psd_run_id=` + openRun},
	})
	if _, err := ownDB.Exec(`UPDATE payroll.pyr_salary_detail SET psd_amount=2 WHERE psd_run_id=` + openRun); err != nil {
		t.Fatalf("update detail di periode OPEN: %v", err)
	}
}

// --- ETag (pe_version) -----------------------------------------------------------------------------------------

const etagEncKey = "test-enc-key-0123456789abcdef0123"

func etagOpts() etl.EmployeeOptions {
	return etl.EmployeeOptions{CompanyID: 1, EncKey: etagEncKey, HMACKey: "test-hmac-key-0123456789abcdef012", ExpectEmp: 4, ExpectCompE: 7}
}

// psvWithBankAcnt menyalin testdata ke folder sementara dan mengganti PE_BANK_ACNT satu NIK ("" = NULL).
func psvWithBankAcnt(t *testing.T, nik, val string) string {
	t.Helper()
	dir := t.TempDir()
	ents, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		b, rerr := os.ReadFile(filepath.Join("testdata", e.Name()))
		if rerr != nil {
			t.Fatal(rerr)
		}
		if strings.EqualFold(e.Name(), "MGT_PAYROLL_EMP.psv") {
			b = []byte(setPSVCol(t, string(b), nik, "PE_BANK_ACNT", val))
		}
		if werr := os.WriteFile(filepath.Join(dir, e.Name()), b, 0o600); werr != nil {
			t.Fatal(werr)
		}
	}
	return dir
}

func setPSVCol(t *testing.T, content, nik, col, val string) string {
	t.Helper()
	lines := strings.Split(content, "\n")
	ci := -1
	for i, h := range strings.Split(strings.TrimRight(lines[0], "\r"), "|") {
		if h == col {
			ci = i
		}
	}
	if ci < 0 {
		t.Fatalf("kolom %s tidak ada", col)
	}
	found := false
	for i := 1; i < len(lines); i++ {
		f := strings.Split(strings.TrimRight(lines[i], "\r"), "|")
		if len(f) > ci && f[0] == nik {
			f[ci] = val
			lines[i] = strings.Join(f, "|")
			found = true
		}
	}
	if !found {
		t.Fatalf("NIK %s tidak ada di sumber", nik)
	}
	return strings.Join(lines, "\n")
}

// empVersions membaca pe_version semua karyawan numerik company 1 (1001, 1002, 2001), kunci = NIK.
func empVersions(t *testing.T) map[string]string {
	t.Helper()
	rows, err := ownDB.Query(`SELECT pe_nik, pe_version::text FROM payroll.pyr_employee WHERE pe_company_id=1 AND pe_nik ~ '^[0-9]+$'`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err = rows.Scan(&k, &v); err != nil {
			t.Fatal(err)
		}
		out[k] = v
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, nik := range []string{"1001", "1002", "2001"} {
		if out[nik] == "" {
			t.Fatalf("karyawan %s tidak termuat: %v", nik, out)
		}
	}
	return out
}

func bankAcnt(t *testing.T, nik string) string {
	t.Helper()
	var s string
	if err := ownDB.QueryRow(`SELECT COALESCE(payroll.pgp_sym_decrypt(pe_bank_acnt_enc, $1::text), 'NULL') FROM payroll.pyr_employee WHERE pe_company_id=1 AND pe_nik=$2`,
		etagEncKey, nik).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

// wantVersions: hanya NIK di `bumped` yang naik tepat 1; semua karyawan lain (termasuk 1002 yang banyak NULL) tetap.
func wantVersions(t *testing.T, step string, before, after map[string]string, bumped ...string) {
	t.Helper()
	up := map[string]bool{}
	for _, n := range bumped {
		up[n] = true
	}
	for nik, b := range before {
		a := after[nik]
		if up[nik] {
			if a == b {
				t.Errorf("%s: NIK %s berubah tetapi pe_version tetap %s", step, nik, b)
			}
		} else if a != b {
			t.Errorf("%s: NIK %s tidak berubah tetapi pe_version %s→%s", step, nik, b, a)
		}
	}
}

// T033e2 (2): ETag semua karyawan; run yang hanya mengubah PE_BANK_ACNT (nilai↔nilai, NULL↔nilai) menaikkan version.
func TestETL_EmployeeETagAllEmployeesAndBankAcnt(t *testing.T) {
	ctx := context.Background()
	co := etl.Options{CompanyID: 1, ValidFrom: 202001}
	run := func(dir string) {
		t.Helper()
		src := etl.PSVDir{Dir: dir}
		if _, err := etl.Run(ctx, etlDB, src, co); err != nil {
			t.Fatal(err)
		}
		if _, err := etl.RunEmployees(ctx, etlDB, src, etagOpts()); err != nil {
			t.Fatal(err)
		}
	}
	run("testdata")
	v0 := empVersions(t)
	if bankAcnt(t, "1001") != "1234567890" || bankAcnt(t, "1002") != "NULL" {
		t.Fatalf("data awal tidak sesuai: 1001=%s 1002=%s", bankAcnt(t, "1001"), bankAcnt(t, "1002"))
	}
	run("testdata")
	wantVersions(t, "run ulang identik", v0, empVersions(t))

	// nilai → nilai lain (hanya PE_BANK_ACNT).
	run(psvWithBankAcnt(t, "1001", "9999999999"))
	v1 := empVersions(t)
	wantVersions(t, "1001 nilai→nilai", v0, v1, "1001")
	if got := bankAcnt(t, "1001"); got != "9999999999" {
		t.Errorf("PE_BANK_ACNT 1001 tidak diperbarui: %s", got)
	}
	// NULL → nilai (1002 awalnya kosong); sumber ini memakai testdata asli, jadi 1001 kembali ke 1234567890 dan ikut naik.
	run(psvWithBankAcnt(t, "1002", "5550001"))
	v2 := empVersions(t)
	wantVersions(t, "1002 NULL→nilai (+1001 kembali ke asli)", v1, v2, "1001", "1002")
	if got := bankAcnt(t, "1002"); got != "5550001" {
		t.Errorf("PE_BANK_ACNT 1002 tidak terisi: %s", got)
	}
	// nilai → NULL (sumber 1001 dikosongkan; 1002 kembali ke sumber asli = NULL, jadi ikut berubah).
	run(psvWithBankAcnt(t, "1001", ""))
	v3 := empVersions(t)
	wantVersions(t, "1001 nilai→NULL (+1002 kembali NULL)", v2, v3, "1001", "1002")
	if bankAcnt(t, "1001") != "NULL" || bankAcnt(t, "1002") != "NULL" {
		t.Errorf("setelah dikosongkan: 1001=%s 1002=%s", bankAcnt(t, "1001"), bankAcnt(t, "1002"))
	}
	// run ulang dengan sumber yang sama: tidak ada yang naik.
	run(psvWithBankAcnt(t, "1001", ""))
	wantVersions(t, "run ulang sesudah NULL", v3, empVersions(t))
	// kembalikan ke data awal supaya test lain tidak terpengaruh.
	run("testdata")
}

// T033e2 (3): kunci PII yang salah/dirotasi TIDAK ada fallback: ETL gagal cepat, transaksi dibatalkan, data & version utuh (D-75).
func TestETL_WrongPIIKeyFailsAndLeavesDataIntact(t *testing.T) {
	ctx := context.Background()
	src := etl.PSVDir{Dir: "testdata"}
	if _, err := etl.Run(ctx, etlDB, src, etl.Options{CompanyID: 1, ValidFrom: 202001}); err != nil {
		t.Fatal(err)
	}
	if _, err := etl.RunEmployees(ctx, etlDB, src, etagOpts()); err != nil {
		t.Fatal(err)
	}
	before := empVersions(t)
	o := etagOpts()
	o.EncKey = "rotated-enc-key-0123456789abcdef01"
	_, err := etl.RunEmployees(ctx, etlDB, src, o)
	if err == nil {
		t.Fatal("ETL dengan EncKey yang tidak cocok dengan ciphertext harus gagal (tanpa fallback ke kunci lama)")
	}
	if strings.Contains(err.Error(), o.EncKey) || strings.Contains(err.Error(), etagEncKey) {
		t.Errorf("pesan error membocorkan kunci: %v", err)
	}
	wantVersions(t, "ETL kunci salah", before, empVersions(t))
	if got := bankAcnt(t, "1001"); got != "1234567890" {
		t.Errorf("ciphertext 1001 tidak lagi terbaca dengan kunci asli: %s", got)
	}
}
