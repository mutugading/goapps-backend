package etl

import (
	"context"
	"strings"
	"testing"

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/etl"
)

// Runs after the component tests (file order) and loads master/template itself (idempotent).
func TestETL_Employees(t *testing.T) {
	ctx := context.Background()
	src := etl.PSVDir{Dir: "testdata"}
	if _, err := etl.Run(ctx, etlDB, src, etl.Options{CompanyID: 1, ValidFrom: 202001}); err != nil {
		t.Fatal(err)
	}
	o := etl.EmployeeOptions{CompanyID: 1, EncKey: "test-enc-key-0123456789abcdef0123", HMACKey: "test-hmac-key-0123456789abcdef012", ExpectEmp: 4, ExpectCompE: 7}

	snap := func() string {
		return scalar(t, `SELECT md5(
		  (SELECT string_agg(pe_nik||pe_division||coalesce(pe_hris_sys_id::text,'')||coalesce(pe_birth_date::text,'')||pe_orion_sub_acnt, ',' ORDER BY 1) FROM payroll.pyr_employee) ||
		  (SELECT string_agg(pec_nik||pec_component_sysid||pec_source||pec_frz_flag||coalesce(pec_amount::text,''), ',' ORDER BY 1) FROM payroll.pyr_employee_component))`)
	}
	reps, err := etl.RunEmployees(ctx, etlDB, src, o)
	if err != nil {
		t.Fatal(err)
	}
	first := snap()
	reps2, err := etl.RunEmployees(ctx, etlDB, src, o)
	if err != nil {
		t.Fatal(err)
	}
	if snap() != first {
		t.Fatal("run kedua mengubah hasil (tidak idempotent)")
	}
	for i, r := range reps2 {
		if reps[i].Checksum != r.Checksum || !r.OK() {
			t.Errorf("%s: checksum/hitungan berbeda", r.Table)
		}
	}
	if n := scalar(t, `SELECT count(*) FROM payroll.pyr_employee`); n != "3" {
		t.Errorf("pyr_employee = %s, want 3 (divisi BEKASI tidak dimigrasi)", n)
	}
	if n := scalar(t, `SELECT count(*) FROM payroll.pyr_employee_nik_hist WHERE penh_source='PAYROLL_NIK'`); n != "3" {
		t.Errorf("nik_hist = %s", n)
	}
	// I21: PII hanya ciphertext, tidak ada plaintext di kolom.
	if n := scalar(t, `SELECT count(*) FROM payroll.pyr_employee WHERE pe_nik='1001' AND pe_npwp_enc IS NOT NULL AND pe_ktp_enc IS NOT NULL
	  AND pe_bank_acnt_enc IS NOT NULL AND position('1234567890'::bytea in pe_bank_acnt_enc)=0 AND position('3372010101900001'::bytea in pe_ktp_enc)=0`); n != "1" {
		t.Errorf("PII 1001 tidak terenkripsi benar: %s", n)
	}
	if dec := scalar(t, `SELECT payroll.pgp_sym_decrypt(pe_bank_acnt_enc, '`+o.EncKey+`') FROM payroll.pyr_employee WHERE pe_nik='1001'`); dec != "1234567890" {
		t.Errorf("dekripsi rekening = %q", dec)
	}
	if n := scalar(t, `SELECT count(*) FROM payroll.pyr_employee WHERE pe_nik='1001' AND pe_npwp_hash IS NOT NULL`); n != "1" {
		t.Errorf("npwp_hash 1001 kosong")
	}
	if v := scalar(t, `SELECT pe_orion_sub_acnt FROM payroll.pyr_employee WHERE pe_nik='1001'`); v != "OLD1001" {
		t.Errorf("orion_sub_acnt = %s", v)
	}
	if v := scalar(t, `SELECT pe_orion_sub_acnt FROM payroll.pyr_employee WHERE pe_nik='1002'`); v != "1002" {
		t.Errorf("orion_sub_acnt default = %s", v)
	}
	if v := scalar(t, `SELECT coalesce(pe_hris_sys_id::text,'-') FROM payroll.pyr_employee WHERE pe_nik IN ('1001','1002') ORDER BY pe_nik`); v != "MGTSLO5001" {
		t.Errorf("hris_sys_id 1001 = %s", v)
	}
	// birth date dari PE_PASSWORD: Andi kosong → 1985-06-05; Siti (31021991) tidak valid → NULL + warning.
	if v := scalar(t, `SELECT pe_birth_date::text FROM payroll.pyr_employee WHERE pe_nik='2001'`); v != "1985-06-05" {
		t.Errorf("birth 2001 = %s", v)
	}
	if v := scalar(t, `SELECT coalesce(pe_birth_date::text,'NULL') FROM payroll.pyr_employee WHERE pe_nik='1002'`); v != "NULL" {
		t.Errorf("birth 1002 = %s", v)
	}
	if v := scalar(t, `SELECT pe_date_join::text FROM payroll.pyr_employee WHERE pe_nik='1002'`); v != "2021-01-15" {
		t.Errorf("date_join 1002 (format Oracle) = %s", v)
	}
	// Attachment: 4 baris termuat (1001×2, 1002, 2001), 3 exception (hantu, 7777, BEKASI skip tidak dihitung).
	var comp, exc int
	for _, r := range reps2 {
		if strings.HasPrefix(r.Table, etl.TableEmployeeComponent) {
			comp += r.DstRows
			exc += len(r.Exceptions)
		}
	}
	if comp != 4 || exc != 2 {
		t.Errorf("attachment dst=%d exc=%d, want 4/2", comp, exc)
	}
	if v := scalar(t, `SELECT string_agg(pec_source||pec_frz_flag, ',' ORDER BY pec_id) FROM payroll.pyr_employee_component WHERE pec_nik='1001'`); v != "TEMPLATE2,MANUAL1" {
		t.Errorf("source/frz 1001 = %s (frz 1 harus dipertahankan, amount beda = MANUAL)", v)
	}
	if n := scalar(t, `SELECT count(*) FROM payroll.pyr_employee_component WHERE pec_component_sysid IS NULL`); n != "0" {
		t.Errorf("sysid null: %s", n)
	}
}

func TestETL_EmployeesRequirePIIKey(t *testing.T) {
	if _, err := etl.RunEmployees(context.Background(), etlDB, etl.PSVDir{Dir: "testdata"}, etl.EmployeeOptions{CompanyID: 1}); err == nil {
		t.Fatal("tanpa kunci PII harus gagal")
	}
}
