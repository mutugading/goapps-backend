package e2e

import (
	"bytes"
	"errors"
	"testing"

	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/report"
	auditinfra "github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/audit"
)

// T028m: reco workbook data path (F-2 detail) against PG18 as payroll_app: decrypted bank account, GL setup join,
// line without setup, key required, audit without PII.
const recoYYMM = 203611

func TestReport_Reco(t *testing.T) {
	seedReportRun(t, recoYYMM, "APPROVED")
	y := itoa(recoYYMM)
	for _, q := range []string{
		`INSERT INTO payroll.pyr_employee (pe_company_id, pe_nik, pe_division, pe_nama, pe_orion_sub_acnt, pe_level, pe_cost_group, pe_bank_acnt_enc, pe_birth_date)
		 VALUES (1,'ZREP0001','SOLO','Rep Satu','ZREP0001','Executive','MGT', payroll.pgp_sym_encrypt('0012345678','` + encKey + `'), '1990-03-07')
		 ON CONFLICT (pe_company_id, pe_nik, pe_division) DO UPDATE SET pe_bank_acnt_enc = EXCLUDED.pe_bank_acnt_enc, pe_level = EXCLUDED.pe_level,
		   pe_cost_group = EXCLUDED.pe_cost_group, pe_birth_date = EXCLUDED.pe_birth_date`,
		`INSERT INTO payroll.pyr_gl_group_map (pggm_company_id, pggm_division, pggm_level, pggm_gl_group) VALUES (1,'SOLO','Executive','Executive')
		 ON CONFLICT DO NOTHING`,
		`INSERT INTO payroll.pyr_component (pc_company_id, pc_code, pc_type, pc_name, pc_valid_from) VALUES (1,'TREP901','Tunjangan','Rep Gaji',200001)`,
		`INSERT INTO payroll.pyr_component_legacy_map (pclm_legacy_type, pclm_legacy_id, pclm_legacy_division, pclm_sysid, pclm_company_id)
		 SELECT 'Tunjangan',901,'SOLO',pc_sysid,1 FROM payroll.pyr_component WHERE pc_code='TREP901'`,
		`INSERT INTO payroll.pyr_gl_setup (pgs_company_id, pgs_division, pgs_journal_type, pgs_gl_group, pgs_component_sysid, pgs_account_main,
		   pgs_account_sub, pgs_drcr, pgs_description, pgs_valid_from)
		 SELECT 1,'SOLO','GAJI','Executive',pc_sysid,'405300','405301','D','Gaji Rep',200001 FROM payroll.pyr_component WHERE pc_code='TREP901'`,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatalf("seed: %v\n%s", err, q)
		}
	}
	svc := report.NewService(appDB, auditinfra.NewWriter())
	ctx := ctxFor(t, userRW)
	q := report.Query{CompanyID: 1, YYMM: recoYYMM, Division: "SOLO"}

	if err := svc.ExportReco(ctx, q, "", func(report.RecoData) (int, error) { return 0, nil }); !errors.Is(err, report.ErrNoPIIKey) {
		t.Fatalf("no key: %v", err)
	}
	var buf bytes.Buffer
	var d report.RecoData
	err := svc.ExportReco(ctx, q, encKey, func(r report.RecoData) (int, error) { d = r; return report.WriteReco(r, &buf) })
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Rows) != 4 {
		t.Fatalf("rows = %d", len(d.Rows))
	}
	var withSetup, without int
	for _, r := range d.Rows {
		if r.NIK == "ZREP0001" && r.Type == "Tunjangan" {
			if !r.HasSetup || r.Akun != "405300-405301" || r.DrCr != "D" || r.BankAcnt != "0012345678" || r.LevelStatus != "Executive" || r.MGTMB != "MGT" {
				t.Fatalf("setup row %+v", r)
			}
			withSetup++
		}
		if !r.HasSetup {
			without++
		}
	}
	if withSetup != 1 || without != 3 || len(report.PreChecks(d.Rows)) != 2 {
		t.Fatalf("withSetup=%d without=%d pre=%d", withSetup, without, len(report.PreChecks(d.Rows)))
	}
	if _, err := excelize.OpenReader(&buf); err != nil {
		t.Fatal(err)
	}
	if got := scalar(t, `SELECT count(*)::text FROM payroll.pyr_audit_log WHERE pal_action='REPORT_EXPORT' AND pal_after->>'jenis'='reco'
	  AND (pal_after->>'baris')::int = 4 AND pal_after->>'yymm'='`+y+`'`); got != "1" {
		t.Fatalf("audit rows = %s", got)
	}
	if got := scalar(t, `SELECT count(*)::text FROM payroll.pyr_audit_log WHERE pal_action='REPORT_EXPORT' AND (pal_after::text LIKE '%ZREP%' OR pal_after::text LIKE '%0012345678%')`); got != "0" {
		t.Fatal("audit holds PII")
	}
}
