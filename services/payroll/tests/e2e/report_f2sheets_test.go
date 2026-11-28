package e2e

import (
	"testing"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/report"
	auditinfra "github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/audit"
)

// T028d: additional / reg / OT payroll sheet data against PG18 as payroll_app. Overtime rupiah = Σ OT components in
// the detail (T21-T31), for an Executive (NAS x2) and a non-Executive.
const f2YYMM = 203609

func TestReport_AdditionalRegOvertime(t *testing.T) {
	seedReportRun(t, f2YYMM, "APPROVED")
	y := itoa(f2YYMM)
	for _, q := range []string{
		`INSERT INTO payroll.pyr_employee (pe_company_id, pe_nik, pe_division, pe_nama, pe_orion_sub_acnt, pe_level)
		 VALUES (1,'ZREP0001','SOLO','Rep Satu','ZREP0001','Executive'),(1,'ZREP0002','SOLO','Rep Dua','ZREP0002','Staff')
		 ON CONFLICT (pe_company_id, pe_nik, pe_division) DO UPDATE SET pe_level = EXCLUDED.pe_level`,
		`UPDATE payroll.pyr_salary_head SET psh_rate_perjam = 10000.50,
		   psh_jam_lembur = CASE psh_nik WHEN 'ZREP0001' THEN '{"lembur1": 3, "nas1": 4, "nas2": 4, "nas3": 4}'::jsonb
		     ELSE '{"lembur1": 3, "nas1": 4, "nas2": 6, "nas3": 8}'::jsonb END
		 WHERE psh_yymm = ` + y,
		// OT components of the detail as the engine would write them: Σ round(paid x rate)
		`INSERT INTO payroll.pyr_salary_detail (psd_run_id, psd_nik, psd_type, psd_id, psd_amount)
		 SELECT psh_run_id, psh_nik, 'Tunjangan'::payroll.comp_type, c.id, ROUND(c.h * 10000.50, 2)
		 FROM payroll.pyr_salary_head, (VALUES (21,3::numeric),(26,4),(27,4),(28,4)) c(id,h)
		 WHERE psh_yymm = ` + y + ` AND psh_nik = 'ZREP0001'
		 UNION ALL SELECT psh_run_id, psh_nik, 'Tunjangan'::payroll.comp_type, c.id, ROUND(c.h * 10000.50, 2)
		 FROM payroll.pyr_salary_head, (VALUES (21,3::numeric),(26,4),(27,6),(28,8)) c(id,h)
		 WHERE psh_yymm = ` + y + ` AND psh_nik = 'ZREP0002'
		 UNION ALL SELECT psh_run_id, psh_nik, 'Tunjangan'::payroll.comp_type, 1, 7000000
		 FROM payroll.pyr_salary_head WHERE psh_yymm = ` + y + ` AND psh_nik = 'ZREP0002'`,
		`INSERT INTO payroll.pyr_addition (pa_company_id, pa_yymm, pa_nik, pa_division, pa_type, pa_id, pa_nama, pa_compnt_name, pa_compnt_sts, pa_compnt_tax, pa_amount, pa_remarks)
		 VALUES (1,` + y + `,'ZREP0002','SOLO','Tunjangan',100,'Rep Dua','Additional Earning','F',2,250000,'Award')`,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatalf("seed: %v\n%s", err, q)
		}
	}
	svc := report.NewService(appDB, auditinfra.NewWriter())
	var d report.RecoData
	err := svc.ExportReco(ctxFor(t, userRW), report.Query{CompanyID: 1, YYMM: f2YYMM, Division: "SOLO"}, encKey,
		func(r report.RecoData) (int, error) { d = r; return len(r.Rows), nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Additions) != 1 || !d.Additions[0].Amount.Equal(decimal.NewFromInt(250000)) || d.Additions[0].Remarks != "Award" {
		t.Fatalf("additions %+v", d.Additions)
	}
	if len(d.Reg) != 2 || !d.Reg[1].Amt[0].Equal(decimal.NewFromInt(7000000)) || !d.Reg[1].Total().Equal(decimal.NewFromInt(7000000)) {
		t.Fatalf("reg %+v", d.Reg)
	}
	if len(d.Overtime) != 2 {
		t.Fatalf("overtime rows %d", len(d.Overtime))
	}
	for _, o := range d.Overtime {
		var sum decimal.Decimal
		if err := ownDB.QueryRow(`SELECT COALESCE(SUM(d.psd_amount),0)::text FROM payroll.pyr_salary_detail d JOIN payroll.pyr_run r ON r.pr_id = d.psd_run_id
		  WHERE r.pr_yymm = $1 AND r.pr_kind='ENGINE' AND d.psd_nik = $2 AND d.psd_type='Tunjangan' AND d.psd_id BETWEEN 21 AND 31`, f2YYMM, o.NIK).Scan(&sum); err != nil {
			t.Fatal(err)
		}
		if !o.TotalRupiah().Equal(sum) {
			t.Fatalf("%s OT rupiah %s != detail %s", o.NIK, o.TotalRupiah(), sum)
		}
	}
	// Executive NAS x2: paid 4 each -> actual 2 each; non-Executive paid 4/6/8 -> actual 2/2/2.
	for _, o := range d.Overtime {
		if !o.Actual[2].Equal(decimal.NewFromInt(2)) || (o.NIK == "ZREP0001" && !o.Actual[4].Equal(decimal.NewFromInt(2))) ||
			(o.NIK == "ZREP0002" && !o.Actual[4].Equal(decimal.NewFromInt(2))) {
			t.Fatalf("%s actual %v", o.NIK, o.Actual)
		}
	}
}
