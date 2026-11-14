package e2e

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	payrollv1 "github.com/mutugading/goapps-backend/gen/payroll/v1"
	"github.com/mutugading/goapps-backend/services/payroll/internal/application/recon"
	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	grpcdelivery "github.com/mutugading/goapps-backend/services/payroll/internal/delivery/grpc"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
)

// T030c: GET /runs/{id}/diff (07§4) against PG18 as payroll_app. Own period (203505) so run_api_test.go is untouched;
// the stub engine of runHandler gives every ZRUN employee one T001 line of 1.000.000.
const diffYYMM = 203505

func diffHandler(t *testing.T) *grpcdelivery.RunHandler {
	t.Helper()
	rules := func(context.Context, *sql.Tx, run.Query, int) (run.Rules, error) { return run.Rules{Set: stubRules{}}, nil }
	return runHandler(t).WithDiff(recon.RunDiffSource{DB: appDB, Rules: rules})
}

func TestRunDiff(t *testing.T) {
	h := diffHandler(t)
	for _, div := range []string{"SOLO", "JAKARTA"} {
		chk := fmt.Sprintf("rundiff-%s", div)
		for _, q := range []string{
			`INSERT INTO payroll.pyr_attd_days (pad_company_id, pad_yymm, pad_division, pad_days) VALUES (1,203505,'` + div + `',26)`,
			`INSERT INTO payroll.pyr_att_snapshot (pas_company_id, pas_yymm, pas_division, pas_source_batch_ids, pas_checksum, pas_locked)
			 VALUES (1,203505,'` + div + `','{1}','` + chk + `',true)`,
			`INSERT INTO payroll.pyr_att_snapshot_row (pasr_snapshot_id, pasr_kind, pasr_nik, pasr_data)
			 SELECT s.pas_snapshot_id, 'REKAP', 'ZRUN' || left('` + div + `',1) || lpad(g::text, 4, '0'), '{"MPAR_DAY_WORK":26,"MPAR_PAY_TOT":26}'::jsonb
			 FROM payroll.pyr_att_snapshot s, generate_series(1, 3) g WHERE s.pas_checksum = '` + chk + `'`,
		} {
			if _, err := ownDB.Exec(q); err != nil {
				t.Fatalf("seed: %v\n%s", err, q)
			}
		}
	}
	solo := ctxFor(t, userRW, permsRun...)
	commit := func(ctx context.Context, div string) int64 {
		t.Helper()
		req := &payrollv1.CreateRunRequest{Yymm: diffYYMM, Division: div, CompanyId: 1, ProcessDate: "2035-05-25", Mode: payrollv1.RunMode_RUN_MODE_COMMIT}
		cm, err := h.CreateRun(ctx, req)
		if err != nil {
			t.Fatalf("commit %s: %v", div, err)
		}
		return cm.GetData().GetRunId()
	}
	first, active := commit(solo, "SOLO"), commit(solo, "SOLO") // first is SUPERSEDED by active
	jkt := commit(ctxFor(t, userJkt, permsRun...), "JAKARTA")

	// Diff 0 against itself: the ACTIVE run vs against=active, and an identical earlier run.
	for _, against := range []string{"active", fmt.Sprintf("run:%d", first)} {
		r, err := h.GetRunDiff(solo, &payrollv1.GetRunDiffRequest{RunId: active, Against: against})
		if err != nil {
			t.Fatalf("%s: %v", against, err)
		}
		s := r.GetData().GetSummary()
		if !s.GetParity() || len(r.GetData().GetLines()) != 0 || s.GetCategories()[recon.CatMatch] != 3 || s.GetHeadMismatch() != 0 ||
			s.GetTakeHomeDelta() != "0.00" || r.GetPagination().GetTotalItems() != 0 {
			t.Fatalf("%s: self diff = %v", against, r.GetData())
		}
	}

	// ≥ 1 category on synthetic data: the earlier run paid ZRUNS0001 less and had an extra line for ZRUNS0002.
	for _, q := range []string{
		`UPDATE payroll.pyr_salary_detail SET psd_amount = 900000 WHERE psd_run_id = $1 AND psd_nik = 'ZRUNS0001'`,
		`UPDATE payroll.pyr_salary_head SET psh_tot_tunjangan = 900000 WHERE psh_run_id = $1 AND psh_nik = 'ZRUNS0001'`,
		`INSERT INTO payroll.pyr_salary_detail (psd_run_id, psd_nik, psd_type, psd_id, psd_amount) VALUES ($1, 'ZRUNS0002', 'Potongan', 5, 1000)`,
	} {
		if _, err := ownDB.Exec(q, first); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
	r, err := h.GetRunDiff(solo, &payrollv1.GetRunDiffRequest{RunId: active, Against: fmt.Sprintf("run:%d", first)})
	if err != nil {
		t.Fatal(err)
	}
	d := r.GetData()
	s := d.GetSummary()
	if s.GetParity() || s.GetCategories()[recon.CatFormula] != 1 || s.GetCategories()[recon.CatOnlyLegacy] != 1 || s.GetHeadMismatch() != 1 ||
		s.GetTakeHomeDelta() != "100000.00" || d.GetAgainstRunId() != first || len(d.GetLines()) != 2 {
		t.Fatalf("diff vs earlier run = %v", d)
	}
	if l := d.GetLines()[0]; l.GetNik() != "ZRUNS0001" || l.GetComponent() != "T001" || l.GetLegacyAmount() != "900000.00" ||
		l.GetTargetAmount() != "1000000.00" || l.GetDeltaAmount() != "100000.00" || l.GetCategory() != recon.CatFormula {
		t.Fatalf("formula line = %v", l)
	}
	page, err := h.GetRunDiff(solo, &payrollv1.GetRunDiffRequest{RunId: active, Against: fmt.Sprintf("run:%d", first), PageSize: 1, Page: 2})
	if err != nil || len(page.GetData().GetLines()) != 1 || page.GetData().GetLines()[0].GetCategory() != recon.CatOnlyLegacy ||
		page.GetPagination().GetTotalPages() != 2 {
		t.Fatalf("page 2: %v %v", page, err)
	}

	// Masked without payroll.salary.amount.view: nominals "***", codes and categories stay.
	m, err := h.GetRunDiff(ctxFor(t, userRW, permsRunNoAmount...), &payrollv1.GetRunDiffRequest{RunId: active, Against: fmt.Sprintf("run:%d", first)})
	if err != nil {
		t.Fatal(err)
	}
	ml := m.GetData().GetLines()[0]
	if ml.GetLegacyAmount() != "***" || ml.GetTargetAmount() != "***" || ml.GetDeltaAmount() != "***" || ml.GetCategory() != recon.CatFormula ||
		m.GetData().GetSummary().GetTakeHomeDelta() != "***" || m.GetData().GetHeads()[0].GetTargetTakeHome() != "***" {
		t.Fatalf("masked diff = %v", m.GetData())
	}

	// Another scope is refused: a JAKARTA run is invisible to the SOLO user (NOT_FOUND like GetRun), as is a
	// SOLO run to the JAKARTA user; an against run of another period/division is refused even when visible.
	_, err = h.GetRunDiff(solo, &payrollv1.GetRunDiffRequest{RunId: jkt, Against: "active"})
	wantCode(t, "solo user, jakarta run", err, apperr.CodeNotFound)
	_, err = h.GetRunDiff(ctxFor(t, userJkt, permsRun...), &payrollv1.GetRunDiffRequest{RunId: active, Against: "active"})
	wantCode(t, "jakarta user, solo run", err, apperr.CodeNotFound)
	_, err = h.GetRunDiff(solo, &payrollv1.GetRunDiffRequest{RunId: active, Against: fmt.Sprintf("run:%d", jkt)})
	wantCode(t, "against run outside scope", err, apperr.CodeNotFound)
	_, err = h.GetRunDiff(ctxFor(t, userBoth, permsRun...), &payrollv1.GetRunDiffRequest{RunId: active, Against: fmt.Sprintf("run:%d", jkt)})
	wantCode(t, "against run of another division", err, apperr.CodeValidation)

	// against=legacy without a LEGACY run of the period: NOT_FOUND (the recon path itself is covered by tests/etl).
	_, err = h.GetRunDiff(solo, &payrollv1.GetRunDiffRequest{RunId: active, Against: "legacy"})
	wantCode(t, "no legacy run", err, apperr.CodeNotFound)
}
