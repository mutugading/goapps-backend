package e2e

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	payrollv1 "github.com/mutugading/goapps-backend/gen/payroll/v1"
	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
)

// T033n2 (D-82 (5)): HR salah input status pajak (TK, seharusnya K1 sejak awal tahun) dan baru mengoreksi di
// Desember. Lewat UpdateEmployee, riwayat mencatat K1 berlaku hari koreksi → status 1 Jan tetap TK. Lewat koreksi
// mundur berlaku 1 Jan, status 1 Jan (PTKP tahunan Desember) menjadi K1. Tahun pajak memakai jam WIB sekarang
// karena fungsi DB membatasi valid_from ke tahun berjalan/lalu dan tidak di masa depan.
func TestT033n2_BackdatedTaxCorrection(t *testing.T) {
	ctx := context.Background()
	wib := time.FixedZone("WIB", 7*60*60)
	year := time.Now().In(wib).Year()
	jan1 := fmt.Sprintf("%d-01-01", year)
	dec := year*100 + 12
	rw := ctxFor(t, userRW, append(permsRW, "payroll.master.taxstatus.update")...)

	emp := mustCreate(t, "T33N2-A", "Koreksi mundur")
	in := input("Koreksi mundur")
	in.TaxId = "TK"
	upd, err := handler.UpdateEmployee(rw, &payrollv1.UpdateEmployeeRequest{Id: emp.GetId(), Input: in, IfMatch: emp.GetEtag()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = ownDB.Exec(`DELETE FROM payroll.pyr_period WHERE pp_yymm = $1 AND pp_division = 'SOLO'`, dec)
		_, _ = ownDB.Exec(`DELETE FROM payroll.pyr_att_snapshot WHERE pas_yymm = $1 AND pas_division = 'SOLO'`, dec)
	})
	// Riwayat buatan: TK berlaku 1 Jan (input awal tahun yang salah), seperti backfill 000053.
	if _, err = ownDB.Exec(`DELETE FROM payroll.pyr_employee_tax_hist WHERE peth_employee_id = $1`, emp.GetId()); err != nil {
		t.Fatal(err)
	}
	if _, err = ownDB.Exec(`INSERT INTO payroll.pyr_employee_tax_hist (peth_employee_id, peth_valid_from, peth_tax_id, peth_source)
	  VALUES ($1, $2::date, 'TK', 'BACKFILL')`, emp.GetId(), jan1); err != nil {
		t.Fatal(err)
	}

	correct := func(ctx context.Context, tax, from, etag string) (*payrollv1.CorrectEmployeeTaxStatusResponse, error) {
		return handler.CorrectEmployeeTaxStatus(ctx, &payrollv1.CorrectEmployeeTaxStatusRequest{
			Id: emp.GetId(), TaxId: tax, ValidFrom: from, Reason: "salah input HR, K1 sejak awal tahun", IfMatch: etag})
	}
	etag := upd.GetData().GetEtag()

	// Penolakan: tanggal di masa depan / dua tahun lalu, kode bukan EMPTAX, etag basi, di luar scope.
	for _, c := range []struct {
		name, tax, from, etag string
		ctx                   context.Context
		want                  apperr.Code
	}{
		{"masa depan", "K1", fmt.Sprintf("%d-01-01", year+1), etag, rw, apperr.CodeValidation},
		{"dua tahun lalu", "K1", fmt.Sprintf("%d-12-31", year-2), etag, rw, apperr.CodeValidation},
		{"kode tidak dikenal", "K9", jan1, etag, rw, apperr.CodeValidation},
		{"etag basi", "K1", jan1, emp.GetEtag(), rw, apperr.CodeVersionMismatch},
		{"di luar scope", "K1", jan1, etag, ctxFor(t, userJkt, "payroll.master.taxstatus.update"), apperr.CodeNotFound},
	} {
		if _, err = correct(c.ctx, c.tax, c.from, c.etag); code(t, err) != c.want {
			t.Errorf("%s: %v, want %s", c.name, err, c.want)
		}
	}

	// Tahun pajak sudah final (Desember APPROVED/CLOSED) → PERIOD_CLOSED, riwayat tidak berubah.
	if _, err = ownDB.Exec(`INSERT INTO payroll.pyr_period (pp_company_id, pp_yymm, pp_division, pp_date_from, pp_date_to, pp_status)
	  VALUES (1, $1, 'SOLO', $2::date, $3::date, 'CLOSED')`, dec, fmt.Sprintf("%d-11-26", year), fmt.Sprintf("%d-12-25", year)); err != nil {
		t.Fatal(err)
	}
	if _, err = correct(rw, "K1", jan1, etag); code(t, err) != apperr.CodePeriodClosed {
		t.Fatalf("closed December: %v, want PERIOD_CLOSED", err)
	}
	if _, err = ownDB.Exec(`UPDATE payroll.pyr_period SET pp_status = 'OPEN' WHERE pp_yymm = $1 AND pp_division = 'SOLO'`, dec); err != nil {
		t.Fatal(err)
	}

	// Koreksi K1 berlaku 1 Jan.
	resp, err := correct(rw, "K1", jan1, etag)
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetData().GetTaxId() != "K1" || resp.GetData().GetEtag() == etag {
		t.Fatalf("after correction tax_id %q etag %q (before %q)", resp.GetData().GetTaxId(), resp.GetData().GetEtag(), etag)
	}
	if got := ownScalar(t, `SELECT string_agg(peth_valid_from || '/' || peth_tax_id || '/' || peth_source || '/' || created_by, ',' ORDER BY peth_valid_from)
	  FROM payroll.pyr_employee_tax_hist WHERE peth_employee_id = $1`, emp.GetId()); got != jan1+"/K1/CORRECT/"+userRW {
		t.Errorf("history = %s", got)
	}
	if got := ownScalar(t, `SELECT pal_actor || '/' || (pal_before->>'taxid') || '/' || (pal_before->'changes'->0->>'TaxID') || '/' || (pal_after->>'taxid') || '/' || (pal_after->>'validfrom') || '/' || (pal_after->>'reason')
	  FROM payroll.pyr_audit_log WHERE pal_action = 'TAX_STATUS_CORRECT' AND pal_entity_key = $1`, fmt.Sprint(emp.GetId())); got != userRW+"/TK/TK/K1/"+jan1+"/salah input HR, K1 sejak awal tahun" {
		t.Errorf("audit = %s", got)
	}

	// Loader periode Desember: status 1 Jan = K1 (PTKP tahunan v2 memakai K1, formula_pph_taxhist_test).
	if _, err = ownDB.Exec(`INSERT INTO payroll.pyr_att_snapshot (pas_company_id, pas_yymm, pas_division, pas_source_batch_ids, pas_checksum, pas_locked)
	  VALUES (1, $1, 'SOLO', '{1}', 't033n2', true)`, dec); err != nil {
		t.Fatal(err)
	}
	var res run.Result
	if err = appDB.ScopedTx(rw, func(tx *sql.Tx) error {
		var e error
		res, e = run.NewLoader().Load(ctx, tx, run.Query{CompanyID: 1, Division: "SOLO", YYMM: dec, NIKs: []string{"T33N2-A"}}, run.Rules{Set: stubRules{}})
		return e
	}); err != nil {
		t.Fatal(err)
	}
	if len(res.Inputs) != 1 || res.Inputs[0].Emp.TaxStatus != "K1" || res.Inputs[0].Emp.TaxStatusJan1 != "K1" {
		t.Fatalf("loader inputs = %+v", res.Inputs)
	}

	// payroll_app tetap tidak bisa menulis riwayat langsung; hanya lewat fungsi.
	if err = appDB.ScopedTx(rw, func(tx *sql.Tx) error {
		_, e := tx.Exec(`UPDATE payroll.pyr_employee_tax_hist SET peth_tax_id = 'TK'`)
		return e
	}); err == nil {
		t.Fatal("payroll_app UPDATE pyr_employee_tax_hist allowed")
	}
}

// T033n2 review: koreksi tidak menghapus perubahan sungguhan sesudah D, dan guard memeriksa setiap tahun terdampak.
func TestT033n2_CorrectionKeepsLaterChangesAndGuardsYears(t *testing.T) {
	wib := time.FixedZone("WIB", 7*60*60)
	year := time.Now().In(wib).Year()
	jan1, mar1 := fmt.Sprintf("%d-01-01", year), fmt.Sprintf("%d-03-01", year)
	rw := ctxFor(t, userRW, append(permsRW, "payroll.master.taxstatus.update")...)
	emp := mustCreate(t, "T33N2-B", "Menikah Maret")
	hist := func() string {
		return ownScalar(t, `SELECT string_agg(peth_valid_from || '/' || peth_tax_id || '/' || peth_source, ',' ORDER BY peth_valid_from)
		  FROM payroll.pyr_employee_tax_hist WHERE peth_employee_id = $1`, emp.GetId())
	}
	t.Cleanup(func() {
		_, _ = ownDB.Exec(`DELETE FROM payroll.pyr_period WHERE pp_yymm IN ($1, $2) AND pp_division = 'SOLO'`, (year-1)*100+12, year*100+12)
	})
	// Riwayat: salah input K1 sejak 1 Jan (seharusnya TK), lalu perubahan sungguhan K1 pada 1 Maret (menikah + anak).
	for _, q := range []string{
		`DELETE FROM payroll.pyr_employee_tax_hist WHERE peth_employee_id = $1`,
		`UPDATE payroll.pyr_employee SET pe_tax_id = 'K1' WHERE pe_id = $1`,
		`DELETE FROM payroll.pyr_employee_tax_hist WHERE peth_employee_id = $1`,
	} {
		if _, err := ownDB.Exec(q, emp.GetId()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ownDB.Exec(`INSERT INTO payroll.pyr_employee_tax_hist (peth_employee_id, peth_valid_from, peth_tax_id, peth_source)
	  VALUES ($1, $2::date, 'K1', 'BACKFILL'), ($1, $3::date, 'K1', 'UPDATE')`, emp.GetId(), jan1, mar1); err != nil {
		t.Fatal(err)
	}
	etag := func() string {
		r, err := handler.GetEmployee(rw, &payrollv1.GetEmployeeRequest{Nik: "T33N2-B", CompanyId: 1, Division: "SOLO"})
		if err != nil {
			t.Fatal(err)
		}
		return r.GetData().GetEtag()
	}
	correct := func(from string) error {
		_, err := handler.CorrectEmployeeTaxStatus(rw, &payrollv1.CorrectEmployeeTaxStatusRequest{
			Id: emp.GetId(), TaxId: "TK", ValidFrom: from, Reason: "salah input awal tahun", IfMatch: etag()})
		return err
	}

	// Guard: Desember tahun berjalan APPROVED menolak koreksi bertanggal tahun lalu (tahun sesudahnya ikut terdampak).
	if _, err := ownDB.Exec(`INSERT INTO payroll.pyr_period (pp_company_id, pp_yymm, pp_division, pp_date_from, pp_date_to, pp_status)
	  VALUES (1, $1, 'SOLO', $2::date, $3::date, 'APPROVED')`, year*100+12, fmt.Sprintf("%d-11-26", year), fmt.Sprintf("%d-12-25", year)); err != nil {
		t.Fatal(err)
	}
	if err := correct(fmt.Sprintf("%d-06-01", year-1)); code(t, err) != apperr.CodePeriodClosed {
		t.Fatalf("previous-year correction with current December APPROVED: %v, want PERIOD_CLOSED", err)
	}
	if err := correct(jan1); code(t, err) != apperr.CodePeriodClosed {
		t.Fatalf("December APPROVED: %v, want PERIOD_CLOSED", err)
	}
	if _, err := ownDB.Exec(`UPDATE payroll.pyr_period SET pp_status = 'OPEN' WHERE pp_yymm = $1 AND pp_division = 'SOLO'`, year*100+12); err != nil {
		t.Fatal(err)
	}

	// Koreksi TK sejak 1 Jan: baris 1 Maret (K1) tetap, status terkini tetap K1.
	if err := correct(jan1); err != nil {
		t.Fatal(err)
	}
	if got := hist(); got != jan1+"/TK/CORRECT,"+mar1+"/K1/UPDATE" {
		t.Errorf("history = %s", got)
	}
	if got := ownScalar(t, `SELECT pe_tax_id FROM payroll.pyr_employee WHERE pe_id = $1`, emp.GetId()); got != "K1" {
		t.Errorf("current tax_id = %s, want K1 (later genuine change kept)", got)
	}
}

func ownScalar(t *testing.T, q string, args ...any) string {
	t.Helper()
	var s sql.NullString
	if err := ownDB.QueryRow(q, args...).Scan(&s); err != nil {
		t.Fatalf("%v\n%s", err, q)
	}
	return s.String
}

// T033n4 (D-82 (1)): riwayat status pajak tampil di UI lewat RPC baca; koreksi langsung terlihat, terbaru dulu,
// dan scope RLS dihormati (id di luar scope = NOT_FOUND, bukan daftar kosong).
func TestT033n4_ListEmployeeTaxHistory(t *testing.T) {
	wib := time.FixedZone("WIB", 7*60*60)
	year := time.Now().In(wib).Year()
	jan1, mar1 := fmt.Sprintf("%d-01-01", year), fmt.Sprintf("%d-03-01", year)
	rw := ctxFor(t, userRW, append(permsRW, "payroll.master.taxstatus.update")...)
	emp := mustCreate(t, "T33N4-A", "Riwayat tampil")
	list := func(c context.Context) (*payrollv1.ListEmployeeTaxHistoryResponse, error) {
		return handler.ListEmployeeTaxHistory(c, &payrollv1.ListEmployeeTaxHistoryRequest{Id: emp.GetId()})
	}
	// Riwayat buatan: TK sejak 1 Jan (backfill), K1 sejak 1 Maret (perubahan sungguhan).
	if _, err := ownDB.Exec(`DELETE FROM payroll.pyr_employee_tax_hist WHERE peth_employee_id = $1`, emp.GetId()); err != nil {
		t.Fatal(err)
	}
	if _, err := ownDB.Exec(`INSERT INTO payroll.pyr_employee_tax_hist (peth_employee_id, peth_valid_from, peth_tax_id, peth_source)
	  VALUES ($1, $2::date, 'TK', 'BACKFILL'), ($1, $3::date, 'K1', 'UPDATE')`, emp.GetId(), jan1, mar1); err != nil {
		t.Fatal(err)
	}
	r, err := list(rw)
	if err != nil {
		t.Fatal(err)
	}
	d := r.GetData()
	if len(d) != 2 || d[0].GetValidFrom() != mar1 || d[0].GetTaxId() != "K1" || d[0].GetSource() != "UPDATE" ||
		d[1].GetValidFrom() != jan1 || d[1].GetTaxId() != "TK" || d[1].GetSource() != "BACKFILL" || d[1].GetReason() != "" {
		t.Fatalf("history = %v", d)
	}
	if d[0].GetCreatedBy() == "" || d[0].GetCreatedAt() == "" {
		t.Errorf("created_by/created_at kosong: %v", d[0])
	}

	// Koreksi dari RPC langsung terlihat: baris CORRECT berlaku 1 Jan memuat alasan dan aktor.
	if _, err = handler.CorrectEmployeeTaxStatus(rw, &payrollv1.CorrectEmployeeTaxStatusRequest{
		Id: emp.GetId(), TaxId: "K1", ValidFrom: jan1, Reason: "salah input HR", IfMatch: emp.GetEtag()}); err != nil {
		t.Fatal(err)
	}
	if r, err = list(rw); err != nil {
		t.Fatal(err)
	}
	d = r.GetData()
	if len(d) != 2 || d[1].GetValidFrom() != jan1 || d[1].GetTaxId() != "K1" || d[1].GetSource() != "CORRECT" ||
		d[1].GetReason() != "salah input HR" || d[1].GetCreatedBy() != userRW || d[0].GetValidFrom() != mar1 {
		t.Fatalf("history after correction = %v", d)
	}

	// Scope: user Jakarta tidak melihat karyawan SOLO; id tidak ada juga NOT_FOUND.
	if _, err = list(ctxFor(t, userJkt, permsRW...)); code(t, err) != apperr.CodeNotFound {
		t.Errorf("di luar scope: %v, want NOT_FOUND", err)
	}
	if _, err = handler.ListEmployeeTaxHistory(rw, &payrollv1.ListEmployeeTaxHistoryRequest{Id: 999999999}); code(t, err) != apperr.CodeNotFound {
		t.Errorf("id tidak ada: %v, want NOT_FOUND", err)
	}
}
