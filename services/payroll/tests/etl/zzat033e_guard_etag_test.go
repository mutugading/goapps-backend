package etl

import (
	"context"
	"strings"
	"testing"

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/etl"
)

// T033e (#3a): guard CLOSED memeriksa kunci OLD dan NEW; memindahkan baris keluar dari periode CLOSED ditolak.
func TestGuardClosed_KeyMoveRejected(t *testing.T) {
	ctx := context.Background()
	for _, q := range []string{
		`INSERT INTO payroll.pyr_period (pp_company_id, pp_yymm, pp_division, pp_date_from, pp_date_to, pp_status)
		 VALUES (1,209801,'SOLO','2097-12-26','2098-01-25','OPEN'),(1,209802,'SOLO','2098-01-26','2098-02-25','OPEN')
		 ON CONFLICT DO NOTHING`,
		`INSERT INTO payroll.pyr_addition (pa_company_id, pa_yymm, pa_nik, pa_division, pa_type, pa_id, pa_nama, pa_compnt_name, pa_compnt_sts, pa_compnt_tax, pa_amount, pa_remarks, pa_source)
		 VALUES (1,209801,'KM-0001','SOLO','Tunjangan',34,'Budi','Bonus','F',2,100,'key-move','MANUAL')`,
		`UPDATE payroll.pyr_period SET pp_status='CLOSED' WHERE pp_company_id=1 AND pp_yymm=209801 AND pp_division='SOLO'`,
	} {
		if _, err := ownDB.ExecContext(ctx, q); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	// OLD di periode CLOSED, NEW di periode OPEN: sebelumnya lolos karena hanya NEW yang dicek.
	_, err := ownDB.ExecContext(ctx, `UPDATE payroll.pyr_addition SET pa_yymm=209802 WHERE pa_nik='KM-0001' AND pa_yymm=209801`)
	if err == nil || !strings.Contains(err.Error(), "CLOSED") {
		t.Fatalf("key-move keluar dari periode CLOSED harus ditolak, err=%v", err)
	}
	// NEW di periode CLOSED (arah sebaliknya) tetap ditolak.
	if _, err = ownDB.ExecContext(ctx, `INSERT INTO payroll.pyr_addition (pa_company_id, pa_yymm, pa_nik, pa_division, pa_type, pa_id, pa_nama, pa_compnt_name, pa_compnt_sts, pa_compnt_tax, pa_amount, pa_source)
		 VALUES (1,209802,'KM-0002','SOLO','Tunjangan',34,'Sari','Bonus','F',2,1,'MANUAL')`); err != nil {
		t.Fatal(err)
	}
	_, err = ownDB.ExecContext(ctx, `UPDATE payroll.pyr_addition SET pa_yymm=209801 WHERE pa_nik='KM-0002'`)
	if err == nil || !strings.Contains(err.Error(), "CLOSED") {
		t.Fatalf("key-move masuk ke periode CLOSED harus ditolak, err=%v", err)
	}
	// UPDATE biasa di periode OPEN tetap boleh.
	if _, err = ownDB.ExecContext(ctx, `UPDATE payroll.pyr_addition SET pa_amount=2 WHERE pa_nik='KM-0002'`); err != nil {
		t.Fatalf("update di periode OPEN: %v", err)
	}
}

// T033e (#3b): ETL hanya menaikkan version (ETag) bila nilai berubah; run ulang identik tidak menyentuhnya.
func TestETL_BumpsVersionOnlyOnChange(t *testing.T) {
	ctx := context.Background()
	src := etl.PSVDir{Dir: "testdata"}
	co := etl.Options{CompanyID: 1, ValidFrom: 202001}
	eo := etl.EmployeeOptions{CompanyID: 1, EncKey: "test-enc-key-0123456789abcdef0123", HMACKey: "test-hmac-key-0123456789abcdef012", ExpectEmp: 4, ExpectCompE: 7}
	if _, err := etl.Run(ctx, etlDB, src, co); err != nil {
		t.Fatal(err)
	}
	if _, err := etl.RunEmployees(ctx, etlDB, src, eo); err != nil {
		t.Fatal(err)
	}
	empV := func() string {
		return scalar(t, `SELECT pe_version::text FROM payroll.pyr_employee WHERE pe_nik='1001'`)
	}
	compV := func() string {
		return scalar(t, `SELECT string_agg(pc_sysid||':'||pc_version, ',' ORDER BY pc_sysid) FROM payroll.pyr_component`)
	}
	ev, cv := empV(), compV()
	if _, err := etl.Run(ctx, etlDB, src, co); err != nil {
		t.Fatal(err)
	}
	if _, err := etl.RunEmployees(ctx, etlDB, src, eo); err != nil {
		t.Fatal(err)
	}
	if empV() != ev || compV() != cv {
		t.Fatalf("run ulang identik menaikkan version: employee %s→%s, component %s→%s", ev, empV(), cv, compV())
	}
	// nilai diubah di luar ETL (mis. edit manual), lalu ETL mengembalikannya dari sumber: version naik.
	if _, err := ownDB.Exec(`UPDATE payroll.pyr_employee SET pe_nama='EDIT MANUAL' WHERE pe_nik='1001';
		UPDATE payroll.pyr_component SET pc_name='EDIT MANUAL' WHERE pc_sysid=(SELECT min(pc_sysid) FROM payroll.pyr_component)`); err != nil {
		t.Fatal(err)
	}
	if _, err := etl.Run(ctx, etlDB, src, co); err != nil {
		t.Fatal(err)
	}
	if _, err := etl.RunEmployees(ctx, etlDB, src, eo); err != nil {
		t.Fatal(err)
	}
	if empV() == ev {
		t.Errorf("ETL mengubah pe_nama tetapi pe_version tetap %s (ETag lama masih valid)", ev)
	}
	if compV() == cv {
		t.Errorf("ETL mengubah pc_name tetapi pc_version tetap (ETag lama masih valid): %s", cv)
	}
	ev2, cv2 := empV(), compV()
	if _, err := etl.Run(ctx, etlDB, src, co); err != nil {
		t.Fatal(err)
	}
	if _, err := etl.RunEmployees(ctx, etlDB, src, eo); err != nil {
		t.Fatal(err)
	}
	if empV() != ev2 || compV() != cv2 {
		t.Error("run ulang sesudah koreksi menaikkan version lagi")
	}
}
