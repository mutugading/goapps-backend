package etl

import (
	"context"
	"strings"
	"testing"

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/etl"
)

// T028q (D-63): FM_DEPARTMENT.DEPT_FIELD_01 -> pyr_employee.pe_cost_group per dept; CR/LF dibuang, nilai asing dan dept
// ganda-beda jadi exception, dept tanpa group jadi warning, idempoten.
func TestETL_CostGroup(t *testing.T) {
	ctx := context.Background()
	if _, err := etl.Run(ctx, etlDB, etl.PSVDir{Dir: "testdata"}, etl.Options{CompanyID: 1, ValidFrom: 202001}); err != nil {
		t.Fatal(err)
	}
	const dept = "ZZD"
	if _, err := ownDB.Exec(`UPDATE payroll.pyr_employee SET pe_dept_id=$1 WHERE pe_company_id=1 AND pe_nik IN ('1001','1002')`, dept); err != nil {
		t.Fatal(err)
	}
	hdr := "DEPT_COMP_CODE|DEPT_CODE|DEPT_FIELD_01\n"
	row := func(code, g string) string { return "002|" + code + "|" + g }
	dir := writePSV(t, "FM_DEPARTMENT", hdr, row(dept, "contract\r"), row("ZZ1", "MGT"), row("ZZ2", ""), row("ZZ3", "XX"), row("ZZ4", "MGT"), row("ZZ4", "MB"))
	rep, err := etl.RunCostGroup(ctx, etlDB, dir, etl.CostGroupOptions{CompanyID: 1, ExpectRows: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Exceptions) != 2 { // ZZ3 nilai asing, ZZ4 ganda-beda
		t.Fatalf("exception = %v", rep.Exceptions)
	}
	if got := scalar(t, `SELECT count(*)::text FROM payroll.pyr_employee WHERE pe_company_id=1 AND pe_dept_id='ZZD' AND pe_cost_group IS DISTINCT FROM 'CONTRACT'`); got != "0" {
		t.Fatalf("%s karyawan dept ZZD tidak CONTRACT", got)
	}
	first := scalar(t, `SELECT coalesce(string_agg(pe_nik||coalesce(pe_cost_group,'-'),',' ORDER BY pe_nik),'') FROM payroll.pyr_employee`)
	rep2, err := etl.RunCostGroup(ctx, etlDB, dir, etl.CostGroupOptions{CompanyID: 1, ExpectRows: 5})
	if err != nil {
		t.Fatal(err)
	}
	if rep2.DstRows != 0 || scalar(t, `SELECT coalesce(string_agg(pe_nik||coalesce(pe_cost_group,'-'),',' ORDER BY pe_nik),'') FROM payroll.pyr_employee`) != first {
		t.Fatal("run kedua mengubah hasil (tidak idempotent)")
	}
	if _, err = etl.RunCostGroup(ctx, etlDB, dir, etl.CostGroupOptions{CompanyID: 1, ExpectRows: 9}); err == nil || !strings.Contains(err.Error(), "diharapkan") {
		t.Fatalf("expect-rows tidak dicek: %v", err)
	}
}
