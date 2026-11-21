package e2e

import (
	"strconv"
	"testing"

	payrollv1 "github.com/mutugading/goapps-backend/gen/payroll/v1"
)

// T023a: an IAM UserID is a 36-character UUID. Every actor column (pr_started_by, approved_by, closed_by,
// pal_actor, created_by/updated_by on master) must hold it; with varchar(30) each write failed with 22001.
// Own period (203503) so the commit → approve → close flow in run_api_test.go is untouched; sorts after it.
const (
	uuidUser  = "0b8f3c2e-6a1d-4f7b-9c3e-5d2a1b4c6e8f"
	uuidYYMM  = 203503
	uuidCheck = "uuid-actor"
)

func TestActorColumns_UUIDUserID(t *testing.T) {
	h := runHandler(t)
	for _, q := range []string{
		`INSERT INTO payroll.pyr_user_scope (pus_user_id, pus_company_id, pus_division) VALUES ('` + uuidUser + `',1,'SOLO')`,
		`INSERT INTO payroll.pyr_attd_days (pad_company_id, pad_yymm, pad_division, pad_days) VALUES (1,203503,'SOLO',26)`,
		`INSERT INTO payroll.pyr_att_snapshot (pas_company_id, pas_yymm, pas_division, pas_source_batch_ids, pas_checksum, pas_locked, created_by)
		 VALUES (1,203503,'SOLO','{1}','` + uuidCheck + `',true,'` + uuidUser + `')`,
		`INSERT INTO payroll.pyr_att_snapshot_row (pasr_snapshot_id, pasr_kind, pasr_nik, pasr_data)
		 SELECT s.pas_snapshot_id, 'REKAP', 'ZRUNS' || lpad(g::text, 4, '0'), '{"MPAR_DAY_WORK":26,"MPAR_PAY_TOT":26}'::jsonb
		 FROM payroll.pyr_att_snapshot s, generate_series(1, 3) g WHERE s.pas_checksum = '` + uuidCheck + `'`,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatalf("seed: %v\n%s", err, q)
		}
	}

	ctx := ctxFor(t, uuidUser, permsRun...)
	req := &payrollv1.CreateRunRequest{Yymm: uuidYYMM, Division: "SOLO", CompanyId: 1, ProcessDate: "2035-03-25", Mode: payrollv1.RunMode_RUN_MODE_COMMIT}
	cm, err := h.CreateRun(ctx, req)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if _, err = h.ApprovePeriod(ctx, &payrollv1.ApprovePeriodRequest{Yymm: uuidYYMM, Division: "SOLO"}); err != nil {
		t.Fatalf("approve: %v", err)
	}
	cl, err := h.ClosePeriod(ctx, &payrollv1.ClosePeriodRequest{Yymm: uuidYYMM, Division: "SOLO"})
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	if cl.GetData().GetClosedBy() != uuidUser {
		t.Fatalf("closed_by = %q, want %q", cl.GetData().GetClosedBy(), uuidUser)
	}
	id := cm.GetData().GetRunId()
	if got := scalar(t, `SELECT pr_started_by FROM payroll.pyr_run WHERE pr_id = `+strconv.FormatInt(id, 10)); got != uuidUser {
		t.Fatalf("pr_started_by = %q", got)
	}
	if got := scalar(t, `SELECT approved_by || '|' || closed_by FROM payroll.pyr_period
		WHERE pp_company_id = 1 AND pp_yymm = 203503 AND pp_division = 'SOLO'`); got != uuidUser+"|"+uuidUser {
		t.Fatalf("period actors = %q", got)
	}

	// Master mutation: create + update an employee, then the audit trail of the whole flow.
	mctx := ctxFor(t, uuidUser, permsRW...)
	e, err := handler.CreateEmployee(mctx, &payrollv1.CreateEmployeeRequest{CompanyId: 1, Division: "SOLO", Nik: "UUID01", Input: input("Uuid Aktor")})
	if err != nil {
		t.Fatalf("create employee: %v", err)
	}
	if _, err = handler.UpdateEmployee(mctx, &payrollv1.UpdateEmployeeRequest{Id: e.GetData().GetId(), Input: input("Uuid Aktor 2"), IfMatch: e.GetData().GetEtag()}); err != nil {
		t.Fatalf("update employee: %v", err)
	}
	if got := scalar(t, `SELECT created_by || '|' || updated_by FROM payroll.pyr_employee WHERE pe_nik = 'UUID01'`); got != uuidUser+"|"+uuidUser {
		t.Fatalf("employee actors = %q", got)
	}
	for _, want := range []string{"RUN_COMMIT", "PERIOD_APPROVE", "PERIOD_CLOSE", "CREATE", "UPDATE"} {
		if n := scalar(t, `SELECT count(*) FROM payroll.pyr_audit_log WHERE pal_actor = '`+uuidUser+`' AND pal_action = '`+want+`'`); n == "0" {
			t.Fatalf("no %s audit row with the UUID actor", want)
		}
	}
}

// Guard for new migrations: every actor column (*_by, pal_actor) must hold a 36-character UUID.
func TestActorColumns_AllWideEnough(t *testing.T) {
	got := scalar(t, `SELECT COALESCE(string_agg(table_name || '.' || column_name || '=' || character_maximum_length, ', '), '')
		FROM information_schema.columns
		WHERE table_schema = 'payroll' AND data_type = 'character varying' AND character_maximum_length < 64
		  AND (column_name LIKE '%\_by' OR column_name = 'pal_actor')`)
	if got != "" {
		t.Fatalf("actor columns narrower than varchar(64): %s", got)
	}
}
