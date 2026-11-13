package e2e

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/report"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/scope"
	auditinfra "github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/audit"
)

// T028b: cmd/report data path against PG18 as payroll_app: synthetic ENGINE run, OPEN refused, scope, audit row.
const reportYYMM = 203601

func seedReportRun(t *testing.T, yymm int, status string) {
	t.Helper()
	for _, q := range []string{
		`INSERT INTO payroll.pyr_period (pp_company_id, pp_yymm, pp_division, pp_date_from, pp_date_to, pp_status)
		 VALUES (1,` + itoa(yymm) + `,'SOLO','2035-12-26','2036-01-25','APPROVED')`,
		`INSERT INTO payroll.pyr_rule_set (prs_id, prs_name, prs_effective_from, prs_status) VALUES (1,'v1',200001,'ACTIVE') ON CONFLICT DO NOTHING`,
		`INSERT INTO payroll.pyr_att_snapshot (pas_company_id, pas_yymm, pas_division, pas_source_batch_ids, pas_checksum, pas_locked)
		 VALUES (1,` + itoa(yymm) + `,'SOLO','{1}','rep-` + itoa(yymm) + `',true)`,
		`INSERT INTO payroll.pyr_component_master (pcm_type, pcm_id, pcm_name, pcm_serial) VALUES ('Tunjangan',901,'Rep Gaji',2),('Potongan',902,'Rep BPJS',1)
		 ON CONFLICT DO NOTHING`,
		`INSERT INTO payroll.pyr_run (pr_company_id, pr_yymm, pr_division, pr_pay_date, pr_rule_set_id, pr_snapshot_id, pr_status, pr_kind)
		 SELECT 1,` + itoa(yymm) + `,'SOLO','2036-01-26',1,pas_snapshot_id,'ACTIVE','ENGINE' FROM payroll.pyr_att_snapshot WHERE pas_checksum='rep-` + itoa(yymm) + `'`,
		`INSERT INTO payroll.pyr_salary_head (psh_run_id, psh_company_id, psh_nik, psh_yymm, psh_division, psh_nama, psh_tot_tunjangan, psh_tot_potongan, psh_rounding)
		 SELECT pr_id,1,'ZREP0001',` + itoa(yymm) + `,'SOLO','Rep Satu',1000,100,0 FROM payroll.pyr_run WHERE pr_yymm=` + itoa(yymm) + ` AND pr_kind='ENGINE'
		 UNION ALL SELECT pr_id,1,'ZREP0002',` + itoa(yymm) + `,'SOLO','Rep Dua',500,0,0 FROM payroll.pyr_run WHERE pr_yymm=` + itoa(yymm) + ` AND pr_kind='ENGINE'`,
		`INSERT INTO payroll.pyr_salary_detail (psd_run_id, psd_nik, psd_type, psd_id, psd_amount)
		 SELECT pr_id,'ZREP0001','Tunjangan'::payroll.comp_type,901,1000 FROM payroll.pyr_run WHERE pr_yymm=` + itoa(yymm) + ` AND pr_kind='ENGINE'
		 UNION ALL SELECT pr_id,'ZREP0001','Potongan'::payroll.comp_type,902,100 FROM payroll.pyr_run WHERE pr_yymm=` + itoa(yymm) + ` AND pr_kind='ENGINE'
		 UNION ALL SELECT pr_id,'ZREP0002','Tunjangan'::payroll.comp_type,901,500 FROM payroll.pyr_run WHERE pr_yymm=` + itoa(yymm) + ` AND pr_kind='ENGINE'
		 UNION ALL SELECT pr_id,'ZREP0002','Potongan'::payroll.comp_type,902,0 FROM payroll.pyr_run WHERE pr_yymm=` + itoa(yymm) + ` AND pr_kind='ENGINE'`,
		`UPDATE payroll.pyr_period SET pp_status = '` + status + `' WHERE pp_yymm = ` + itoa(yymm) + ` AND pp_division = 'SOLO'`,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatalf("seed: %v\n%s", err, q)
		}
	}
}

func TestReport_ReconKomponen(t *testing.T) {
	seedReportRun(t, reportYYMM, "APPROVED")
	svc := report.NewService(appDB, auditinfra.NewWriter())
	ctx := ctxFor(t, userRW)
	q := report.Query{CompanyID: 1, YYMM: reportYYMM, Division: "SOLO"}

	var buf bytes.Buffer
	var sum report.Summary
	err := svc.Export(ctx, "recon-komponen", q, func(d report.RunData) (int, error) {
		sum = report.Summarize(d)
		if len(d.Comps) != 2 || d.Comps[0].Key() != "P902" || d.Comps[1].Key() != "T901" {
			t.Errorf("serial order: %+v", d.Comps)
		}
		return report.WriteReconKomponen(d, &buf)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !sum.SumT.Equal(decimal.NewFromInt(1500)) || !sum.SumT.Equal(sum.HeadTunj) || !sum.SumP.Equal(sum.HeadPot) || sum.NIKCount != 2 {
		t.Fatalf("summary %+v", sum)
	}
	if _, err := excelize.OpenReader(&buf); err != nil {
		t.Fatal(err)
	}
	if got := scalar(t, `SELECT count(*)::text FROM payroll.pyr_audit_log WHERE pal_action='REPORT_EXPORT' AND pal_actor='`+userRW+`'
	  AND pal_after->>'jenis'='recon-komponen' AND (pal_after->>'baris')::int = 2 AND pal_after->>'yymm'='`+itoa(reportYYMM)+`'`); got != "1" {
		t.Fatalf("audit rows = %s", got)
	}
	if got := scalar(t, `SELECT count(*)::text FROM payroll.pyr_audit_log WHERE pal_action='REPORT_EXPORT' AND pal_after::text LIKE '%ZREP%'`); got != "0" {
		t.Fatal("audit holds NIK")
	}
}

func TestReport_OpenRefusedUnlessAllowed(t *testing.T) {
	seedReportRun(t, reportYYMM+1, "OPEN")
	svc := report.NewService(appDB, auditinfra.NewWriter())
	ctx := ctxFor(t, userRW)
	q := report.Query{CompanyID: 1, YYMM: reportYYMM + 1, Division: "SOLO"}
	noop := func(d report.RunData) (int, error) { return len(d.Emps), nil }
	if err := svc.Export(ctx, "recon-komponen", q, noop); !errors.Is(err, report.ErrPeriodNotApproved) {
		t.Fatalf("OPEN: err = %v", err)
	}
	if got := scalar(t, `SELECT count(*)::text FROM payroll.pyr_audit_log WHERE pal_after->>'yymm'='`+itoa(reportYYMM+1)+`'`); got != "0" {
		t.Fatalf("refused export audited: %s", got)
	}
	q.AllowOpen = true
	if err := svc.Export(ctx, "recon-komponen", q, noop); err != nil {
		t.Fatal(err)
	}
	if got := scalar(t, `SELECT count(*)::text FROM payroll.pyr_audit_log WHERE pal_after->>'yymm'='`+itoa(reportYYMM+1)+`' AND pal_after->>'allow_open'='true'`); got != "1" {
		t.Fatalf("allow_open not audited: %s", got)
	}
}

func TestReport_ScopeEnforced(t *testing.T) {
	seedReportRun(t, reportYYMM+2, "CLOSED")
	svc := report.NewService(appDB, auditinfra.NewWriter())
	ctx := ctxFor(t, userJkt) // JAKARTA only
	err := svc.Export(ctx, "recon-komponen", report.Query{CompanyID: 1, YYMM: reportYYMM + 2, Division: "SOLO"},
		func(report.RunData) (int, error) { return 0, nil })
	var ae *apperr.Error
	if !errors.As(err, &ae) || ae.Code != apperr.CodeNotFound {
		t.Fatalf("foreign scope: err = %v, want NOT_FOUND", err)
	}
	if _, ok := scope.FromContext(context.Background()); ok {
		t.Fatal("unexpected identity")
	}
	if err := svc.Export(context.Background(), "x", report.Query{}, nil); !errors.Is(err, scope.ErrNoIdentity) {
		t.Fatalf("no identity: %v", err)
	}
}
