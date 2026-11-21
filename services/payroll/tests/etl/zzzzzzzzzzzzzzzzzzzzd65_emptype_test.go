package etl

import (
	"context"
	"strings"
	"testing"

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/etl"
)

// T028r (D-65 C): scripts/fix/d65_emptype.sql membetulkan psh_emp_type MAIN sintetis D-35.2 di run LEGACY CLOSED lewat
// owner. Hanya baris sintetis ber-emp_type numerik dan pe_level terisi yang berubah; guard CLOSED tetap menolak
// sesudahnya; run kedua 0 baris; DB `payroll` di host non-lokal ditolak.
func TestD65_EmptyTypeFix(t *testing.T) {
	ctx := context.Background()
	const syn = etl.SyntheticMainRemark
	for _, q := range []string{
		`INSERT INTO payroll.pyr_employee (pe_company_id, pe_nik, pe_division, pe_nama, pe_orion_sub_acnt, pe_level)
		 VALUES (1,'D65-1','SOLO','Tes Satu','D65-1','LVL-A'),(1,'D65-2','SOLO','Tes Dua','D65-2',NULL)`,
		`INSERT INTO payroll.pyr_period (pp_company_id, pp_yymm, pp_division, pp_date_from, pp_date_to, pp_status)
		 VALUES (1,203001,'SOLO','2029-12-26','2030-01-25','OPEN'),(1,203002,'SOLO','2030-01-26','2030-02-25','OPEN')`,
		`INSERT INTO payroll.pyr_att_snapshot (pas_company_id, pas_yymm, pas_division, pas_source_batch_ids, pas_checksum)
		 VALUES (1,203002,'SOLO','{1}','d65')`,
		`INSERT INTO payroll.pyr_run (pr_company_id, pr_yymm, pr_division, pr_kind, pr_status, pr_rule_set_id, pr_pay_date, pr_snapshot_id)
		 VALUES (1,203001,'SOLO','LEGACY','ACTIVE',0,'2030-01-25',NULL),
		        (1,203002,'SOLO','ENGINE','ACTIVE',0,'2030-02-25',(SELECT pas_snapshot_id FROM payroll.pyr_att_snapshot WHERE pas_yymm=203002 AND pas_division='SOLO'))`,
		// LEGACY 203001: D65-1 sintetis '2' (diperbaiki); D65-2 sintetis '2' tanpa pe_level (tetap); 7777 MAIN asli '2'
		// (tetap); 8888 sintetis sudah benar (tetap). ENGINE 203002: D65-1 ber-penanda sintetis '2' (tetap, bukan LEGACY).
		`INSERT INTO payroll.pyr_salary_head (psh_run_id, psh_company_id, psh_nik, psh_yymm, psh_division, psh_emp_type, psh_remarks, psh_tot_tunjangan, psh_tot_potongan, psh_rounding)
		 SELECT pr_id, 1, v.nik, pr_yymm, 'SOLO', v.et, v.rm, 0, 0, 0 FROM payroll.pyr_run r,
		 (VALUES ('D65-1','2','` + syn + `'),('D65-2','2','` + syn + `'),('7777','2',NULL),('8888','Driver','` + syn + `')) v(nik,et,rm)
		 WHERE r.pr_yymm=203001 AND r.pr_division='SOLO'`,
		`INSERT INTO payroll.pyr_salary_head (psh_run_id, psh_company_id, psh_nik, psh_yymm, psh_division, psh_emp_type, psh_remarks, psh_tot_tunjangan, psh_tot_potongan, psh_rounding)
		 SELECT pr_id, 1, 'D65-1', pr_yymm, 'SOLO', '2', '` + syn + `', 0, 0, 0 FROM payroll.pyr_run WHERE pr_yymm=203002 AND pr_division='SOLO'`,
		`UPDATE payroll.pyr_period p SET pp_status='CLOSED', pp_active_run_id=(SELECT pr_id FROM payroll.pyr_run r
		 WHERE r.pr_yymm=p.pp_yymm AND r.pr_division=p.pp_division AND r.pr_company_id=p.pp_company_id) WHERE pp_yymm=203001 AND pp_division='SOLO'`,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	state := func() string {
		return scalar(t, `SELECT string_agg(psh_yymm||':'||psh_nik||'='||psh_emp_type, ',' ORDER BY psh_yymm, psh_nik) FROM payroll.pyr_salary_head WHERE psh_yymm IN (203001,203002)`)
	}
	const before = "203001:7777=2,203001:8888=Driver,203001:D65-1=2,203001:D65-2=2,203002:D65-1=2"
	if got := state(); got != before {
		t.Fatalf("data awal = %s", got)
	}

	if err := pgCtr.CopyFileToContainer(ctx, "../../scripts/fix/d65_emptype.sql", "/tmp/d65_emptype.sql", 0o644); err != nil {
		t.Fatal(err)
	}
	// payroll_migrator dibatasi 2 koneksi (bootstrap.sql): lepaskan koneksi idle pool ownDB (dipakai test lain di paket
	// ini) supaya psql bisa masuk; dikembalikan sesudah test.
	ownDB.SetMaxIdleConns(0)
	defer ownDB.SetMaxIdleConns(2)
	// psql(host, flags): host "" = IP container (non-lokal); flags mis. "-v CONFIRM_DEV=yes".
	psql := func(host, flags string) (int, string) {
		h := `"` + host + `"`
		if host == "" {
			h = `"$(hostname -i | cut -d' ' -f1)"`
		}
		code, r, err := pgCtr.Exec(ctx, []string{"sh", "-c",
			`PGPASSWORD=payroll_mig123 psql -h ` + h + ` -U payroll_migrator -d payroll_db -v ON_ERROR_STOP=1 ` + flags + ` -f /tmp/d65_emptype.sql 2>&1`})
		if err != nil {
			t.Fatal(err)
		}
		var sb strings.Builder
		buf := make([]byte, 4096)
		for {
			n, e := r.Read(buf)
			sb.Write(buf[:n])
			if e != nil {
				break
			}
		}
		return code, sb.String()
	}

	// penjaga allowlist: host non-lokal ditolak walau ada konfirmasi; host lokal tanpa konfirmasi ditolak juga.
	for name, c := range map[string]struct{ host, flags string }{
		"host non-lokal + konfirmasi": {"", "-v CONFIRM_DEV=yes"},
		"host non-lokal":              {"", ""},
		"lokal tanpa konfirmasi":      {"127.0.0.1", ""},
		"lokal konfirmasi salah":      {"127.0.0.1", "-v CONFIRM_DEV=no"},
		"localhost tanpa konfirmasi":  {"localhost", ""},
	} {
		if code, out := psql(c.host, c.flags); code == 0 || !strings.Contains(out, "DITOLAK") {
			t.Fatalf("%s tidak ditolak: exit %d\n%s", name, code, out)
		}
		if got := state(); got != before {
			t.Fatalf("penolakan (%s) mengubah data: %s", name, got)
		}
	}

	// gagal di tengah skrip (audit ditolak CHECK): transaksi batal, data utuh, guard CLOSED tetap aktif ('O').
	if _, err := ownDB.Exec(`ALTER TABLE payroll.pyr_audit_log ADD CONSTRAINT tmp_d65_fail CHECK (pal_action <> 'D65_EMPTYPE_FIX')`); err != nil {
		t.Fatal(err)
	}
	code, out := psql("127.0.0.1", "-v CONFIRM_DEV=yes")
	if _, err := ownDB.Exec(`ALTER TABLE payroll.pyr_audit_log DROP CONSTRAINT tmp_d65_fail`); err != nil {
		t.Fatal(err)
	}
	if code == 0 || !strings.Contains(out, "tmp_d65_fail") {
		t.Fatalf("kegagalan di tengah skrip tidak terjadi: exit %d\n%s", code, out)
	}
	if got := state(); got != before {
		t.Fatalf("skrip gagal meninggalkan perubahan: %s", got)
	}
	if got := scalar(t, `SELECT tgenabled::text FROM pg_trigger WHERE tgname='trg_guard_header' AND tgrelid='payroll.pyr_salary_head'::regclass`); got != "O" {
		t.Fatalf("setelah skrip gagal trg_guard_header tgenabled = %q, want O", got)
	}

	// host lokal + konfirmasi: hanya D65-1 (LEGACY, sintetis, pe_level terisi) berubah.
	if code, out := psql("127.0.0.1", "-v CONFIRM_DEV=yes"); code != 0 {
		t.Fatalf("skrip gagal: exit %d\n%s", code, out)
	}
	const after = "203001:7777=2,203001:8888=Driver,203001:D65-1=LVL-A,203001:D65-2=2,203002:D65-1=2"
	if got := state(); got != after {
		t.Fatalf("setelah skrip = %s, want %s", got, after)
	}
	// guard CLOSED tetap aktif dan masih menolak mutasi (juga untuk owner).
	if got := scalar(t, `SELECT tgenabled::text FROM pg_trigger WHERE tgname='trg_guard_header' AND tgrelid='payroll.pyr_salary_head'::regclass`); got != "O" {
		t.Fatalf("trg_guard_header tgenabled = %q, want O", got)
	}
	if _, err := ownDB.Exec(`UPDATE payroll.pyr_salary_head SET psh_emp_type='X' WHERE psh_yymm=203001`); err == nil || !strings.Contains(err.Error(), "CLOSED") {
		t.Fatalf("periode CLOSED tidak menolak UPDATE sesudah skrip dengan pesan guard: %v", err)
	}
	// audit: satu baris, jumlah baris 1, tanpa NIK.
	if got := scalar(t, `SELECT count(*)||'/'||coalesce(max(pal_after->>'rows'),'')||'/'||coalesce(bool_and(pal_after::text !~ 'D65-1' AND pal_entity_key !~ 'D65-1')::text,'') FROM payroll.pyr_audit_log WHERE pal_action='D65_EMPTYPE_FIX'`); got != "1/1/true" {
		t.Fatalf("audit = %s, want 1/1/true", got)
	}

	// run kedua: 0 baris, tanpa audit baru.
	if code, out := psql("127.0.0.1", "-v CONFIRM_DEV=yes"); code != 0 {
		t.Fatalf("run kedua gagal: exit %d\n%s", code, out)
	}
	if got := state(); got != after {
		t.Fatalf("run kedua mengubah data: %s", got)
	}
	if got := scalar(t, `SELECT count(*)::text FROM payroll.pyr_audit_log WHERE pal_action='D65_EMPTYPE_FIX'`); got != "1" {
		t.Fatalf("audit setelah run kedua = %s, want 1", got)
	}
}
