package e2e

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/report"
	auditinfra "github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/audit"
)

// T028l: GL journal (F-2 upload) against PG18 as payroll_app: group from the level frozen in the run, Salary Payables,
// rounding on the 'Gaji Solo' line, BOOK rate required, pyr_gl_export PREVIEW (repeatable) then FINAL for a CLOSED period, second FINAL
// rejected, actor required, audit without NIK.
const glYYMM = 203608

func TestReport_GL(t *testing.T) {
	seedReportRun(t, glYYMM, "APPROVED")
	y := itoa(glYYMM)
	for _, q := range []string{
		`INSERT INTO payroll.pyr_employee (pe_company_id, pe_nik, pe_division, pe_nama, pe_orion_sub_acnt, pe_level, pe_cost_group)
		 VALUES (1,'ZREP0001','SOLO','Rep Satu','ZREP0001','Executive','MGT'),(1,'ZREP0002','SOLO','Rep Dua','ZREP0002','Executive','MGT')
		 ON CONFLICT (pe_company_id, pe_nik, pe_division) DO UPDATE SET pe_cost_group = EXCLUDED.pe_cost_group`,
		// own legacy ids (911/912): 901/902 of seedReportRun are mapped by the reco test in the same database
		`UPDATE payroll.pyr_period SET pp_status = 'OPEN' WHERE pp_yymm = ` + y + ` AND pp_division = 'SOLO'`,
		`INSERT INTO payroll.pyr_component_master (pcm_type, pcm_id, pcm_name, pcm_serial) VALUES ('Tunjangan',911,'GL Gaji',2),('Potongan',912,'GL BPJS',1)
		 ON CONFLICT DO NOTHING`,
		`UPDATE payroll.pyr_salary_detail d SET psd_id = d.psd_id + 10 FROM payroll.pyr_run r
		 WHERE r.pr_id = d.psd_run_id AND r.pr_yymm = ` + y + ` AND r.pr_kind = 'ENGINE'`,
		`UPDATE payroll.pyr_salary_head SET psh_emp_type = 'ZGL Level', psh_dept_code = 'ZGL', psh_rounding = CASE psh_nik WHEN 'ZREP0001' THEN 50 ELSE 0 END
		 WHERE psh_yymm = ` + y,
		`INSERT INTO payroll.pyr_gl_group_map (pggm_company_id, pggm_division, pggm_level, pggm_gl_group) VALUES (1,'SOLO','ZGL Level','ZGL Group')
		 ON CONFLICT DO NOTHING`,
		`INSERT INTO payroll.pyr_component (pc_company_id, pc_code, pc_type, pc_name, pc_valid_from) VALUES
		 (1,'TGL901','Tunjangan','GL Gaji',200001),(1,'PGL902','Potongan','GL BPJS',200001) ON CONFLICT DO NOTHING`,
		`INSERT INTO payroll.pyr_component_legacy_map (pclm_legacy_type, pclm_legacy_id, pclm_legacy_division, pclm_sysid, pclm_company_id)
		 SELECT CASE pc_code WHEN 'TGL901' THEN 'Tunjangan' ELSE 'Potongan' END::payroll.comp_type,
		   CASE pc_code WHEN 'TGL901' THEN 911 ELSE 912 END,'SOLO',pc_sysid,1
		 FROM payroll.pyr_component WHERE pc_company_id = 1 AND pc_code IN ('TGL901','PGL902')
		 ON CONFLICT (pclm_company_id, pclm_legacy_type, pclm_legacy_id, pclm_legacy_division) DO UPDATE SET pclm_sysid = EXCLUDED.pclm_sysid`,
		`INSERT INTO payroll.pyr_gl_setup (pgs_company_id, pgs_division, pgs_journal_type, pgs_gl_group, pgs_cost_group, pgs_component_sysid,
		   pgs_line_kind, pgs_account_main, pgs_account_sub, pgs_drcr, pgs_description, pgs_valid_from)
		 SELECT 1,'SOLO','GAJI','ZGL Group','MGT',lm.pclm_sysid,'COMPONENT',
		   CASE lm.pclm_legacy_id WHEN 911 THEN '403000' ELSE '205037' END, CASE lm.pclm_legacy_id WHEN 911 THEN '403001' END,
		   CASE lm.pclm_legacy_id WHEN 911 THEN 'D' ELSE 'C' END, CASE lm.pclm_legacy_id WHEN 911 THEN 'Gaji Solo' ELSE 'Hutang BPJS' END, 200001
		 FROM payroll.pyr_component_legacy_map lm WHERE lm.pclm_company_id = 1 AND lm.pclm_legacy_division = 'SOLO'
		   AND (lm.pclm_legacy_type, lm.pclm_legacy_id) IN (('Tunjangan',911),('Potongan',912))
		 UNION ALL SELECT 1,'SOLO','GAJI','ZGL Group',NULL,NULL,'PAYABLE','205012',NULL,'C','Salary Payables',200001`,
		`UPDATE payroll.pyr_period SET pp_status = 'APPROVED' WHERE pp_yymm = ` + y + ` AND pp_division = 'SOLO'`,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatalf("seed: %v\n%s", err, q)
		}
	}
	svc := report.NewService(appDB, auditinfra.NewWriter())
	ctx := ctxFor(t, userRW)
	q := report.Query{CompanyID: 1, YYMM: glYYMM, Division: "SOLO"}
	write := func(j report.GLJournal) ([]byte, error) {
		var buf bytes.Buffer
		err := report.WriteGL(j, &buf)
		return buf.Bytes(), err
	}

	// No BOOK rate → rejected, nothing recorded (11§4.4).
	if _, err := svc.ExportGL(ctx, q, write); !errors.Is(err, report.ErrNoBookRate) {
		t.Fatalf("no rate: %v", err)
	}
	if _, err := ownDB.Exec(`INSERT INTO payroll.pyr_fx_rate (pfr_rate_date, pfr_from_ccy, pfr_to_ccy, pfr_rate_type, pfr_rate, pfr_source)
	  VALUES ('2036-08-01','IDR','USD','BOOK',0.000055561729,'TEST')`); err != nil {
		t.Fatal(err)
	}
	// No actor → rejected before any read.
	if _, err := svc.ExportGL(context.Background(), q, write); !errors.Is(err, report.ErrNoActor) {
		t.Fatalf("no actor: %v", err)
	}
	// APPROVED → PREVIEW, may repeat.
	for range 2 {
		if _, err := svc.ExportGL(ctx, q, write); err != nil {
			t.Fatalf("preview: %v", err)
		}
	}
	if _, err := ownDB.Exec(`UPDATE payroll.pyr_period SET pp_status = 'CLOSED' WHERE pp_yymm = ` + y + ` AND pp_division = 'SOLO'`); err != nil {
		t.Fatal(err)
	}
	var file []byte
	j, err := svc.ExportGL(ctx, q, func(j report.GLJournal) ([]byte, error) { b, err := write(j); file = b; return b, err })
	if err != nil {
		t.Fatal(err)
	}
	// Gaji Solo = 1500 − Σ rounding 50 = 1450 D; BPJS 100 C; Salary Payables = Σ (T − P − R) = 850 + 500 = 1350 C.
	want := map[string]string{"403000/D": "1450", "205037/C": "100", "205012/C": "1350"}
	if len(j.Rows) != 3 || !j.Debit.Equal(j.Credit) || !j.Debit.Equal(decimal.NewFromInt(1450)) || j.NIKCount != 2 {
		t.Fatalf("journal %+v", j)
	}
	for _, r := range j.Rows {
		if w := want[r.Main+"/"+r.DrCr]; w == "" || !r.FC.Equal(decimal.RequireFromString(w)) || r.Group != "ZGL Group" || r.Dept != "ZGL" {
			t.Errorf("row %+v", r)
		}
	}
	if _, err := excelize.OpenReader(bytes.NewReader(file)); err != nil {
		t.Fatal(err)
	}
	if got := scalar(t, `SELECT pge_status||'|'||pge_row_count||'|'||pge_total_debit||'|'||pge_total_credit||'|'||length(pge_file_sha256)
	  FROM payroll.pyr_gl_export x JOIN payroll.pyr_run r ON r.pr_id = x.pge_run_id WHERE r.pr_yymm = `+y+` AND pge_status = 'FINAL'`); got != "FINAL|3|1450.00|1450.00|64" {
		t.Fatalf("gl export = %s", got)
	}
	if got := scalar(t, `SELECT count(*)::text FROM payroll.pyr_audit_log WHERE pal_action='REPORT_EXPORT' AND pal_after->>'jenis'='gl'
	  AND pal_after->>'yymm'='`+y+`' AND pal_after::text NOT LIKE '%ZREP%'`); got != "3" {
		t.Fatalf("audit rows = %s", got)
	}
	// A second FINAL for the same run is rejected (the file was already handed to Orion).
	if _, err := svc.ExportGL(ctx, q, write); !errors.Is(err, report.ErrGLFinalExists) {
		t.Fatalf("second final: %v", err)
	}
	if _, err := ownDB.Exec(`UPDATE payroll.pyr_gl_export x SET pge_status = 'VOID' FROM payroll.pyr_run r
	  WHERE r.pr_id = x.pge_run_id AND r.pr_yymm = ` + y + ` AND pge_status = 'FINAL'`); err != nil {
		t.Fatal(err)
	}

	// A line without setup fails hard (GL_UNMAPPED), nothing recorded.
	if _, err := ownDB.Exec(`DELETE FROM payroll.pyr_gl_setup WHERE pgs_gl_group = 'ZGL Group' AND pgs_description = 'Hutang BPJS'`); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ExportGL(ctx, q, write); !errors.Is(err, report.ErrGL) {
		t.Fatalf("unmapped: %v", err)
	}
	if got := scalar(t, `SELECT count(*)::text FROM payroll.pyr_gl_export x JOIN payroll.pyr_run r ON r.pr_id = x.pge_run_id WHERE r.pr_yymm = `+y); got != "3" {
		t.Fatalf("gl export rows = %s", got)
	}
}
