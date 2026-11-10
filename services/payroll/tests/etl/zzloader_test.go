package etl

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/etl"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/postgres"
)

const loaderYYMM = 203003

type stubRules struct{ legacy bool }

func (stubRules) ID() int                                 { return 1 }
func (s stubRules) LegacyCompat() bool                    { return s.legacy }
func (stubRules) Constant(string) (decimal.Decimal, bool) { return decimal.Zero, false }

func seedLoader(t *testing.T) {
	t.Helper()
	for _, q := range []string{
		`INSERT INTO payroll.pyr_component (pc_company_id, pc_code, pc_type, pc_name, pc_status, pc_valid_from)
		 VALUES (1,'P004','Potongan','Absensi','ACTIVE',202001), (1,'P008','Potongan','Deduction Impact','ACTIVE',202001), (1,'T074','Tunjangan','Beras','ACTIVE',202001)
		 ON CONFLICT (pc_company_id, pc_code) DO NOTHING`,
		`INSERT INTO payroll.pyr_employee (pe_company_id, pe_nik, pe_division, pe_nama, pe_level, pe_tax_id, pe_sts_payroll, pe_orion_sub_acnt) VALUES
		  (1,'L-SOLO-F','SOLO','Solo F','Driver','K1',2,'L-SOLO-F'),
		  (1,'L-SOLO-N','SOLO','Solo tanpa rekap','Driver','TK',2,'L-SOLO-N'),
		  (1,'L-SOLO-X','SOLO','Solo nonaktif','Driver','TK',1,'L-SOLO-X'),
		  (1,'L-JKT','JAKARTA','Jakarta','Staff','TK',2,'L-JKT'),
		  (1,'L-JKT-FRZ','JAKARTA','Jakarta T1 beku','Staff','TK',2,'L-JKT-FRZ'),
		  (1,'L-JKT-NOT1','JAKARTA','Jakarta tanpa T1','Staff','TK',2,'L-JKT-NOT1')`,
		`INSERT INTO payroll.pyr_employee_component (pec_company_id, pec_nik, pec_division, pec_component_sysid, pec_sts, pec_tax, pec_amount, pec_frz_flag)
		 SELECT 1, v.nik, v.div, c.pc_sysid, v.sts, v.tax, v.amt, v.frz
		 FROM (VALUES ('L-SOLO-F','SOLO','T001','F',2,3000000,2), ('L-SOLO-F','SOLO','T074','D',0,100000,2),
		              ('L-SOLO-F','SOLO','P004','F',0,50000,1),   -- frozen P4 loads (D-28d)
		              ('L-SOLO-F','SOLO','T006','D',2,10,1),      -- frozen T6 does not
		              ('L-SOLO-N','SOLO','T001','D',2,2500000,2),
		              ('L-JKT','JAKARTA','T001','F',2,5000000,2),
		              ('L-JKT-FRZ','JAKARTA','T001','F',2,5000000,1)) v(nik,div,code,sts,tax,amt,frz)
		 JOIN payroll.pyr_component c ON c.pc_company_id = 1 AND c.pc_code = v.code`,
		// validity window (yymm): expired, not yet valid and open-ended rows; the frozen P4 of L-SOLO-N
		// is outside the window too, so it must not load (D-28d only widens frz_flag, not the window).
		`INSERT INTO payroll.pyr_employee_component (pec_company_id, pec_nik, pec_division, pec_component_sysid, pec_sts, pec_tax, pec_amount, pec_frz_flag, pec_valid_from, pec_valid_to)
		 SELECT 1, 'L-SOLO-N', 'SOLO', c.pc_sysid, 'F', 0, v.amt, v.frz, v.vf, v.vt
		 FROM (VALUES ('T074',1,2,202901,NULL),     -- from 2029-01: valid at 203003
		              ('P004',10,1,201901,202912),  -- frozen P4 expired before the period
		              ('P008',20,1,203004,NULL)     -- frozen P8 not valid yet
		      ) v(code,amt,frz,vf,vt)
		 JOIN payroll.pyr_component c ON c.pc_company_id = 1 AND c.pc_code = v.code`,
		`INSERT INTO payroll.pyr_attd_days (pad_company_id, pad_yymm, pad_division, pad_days) VALUES (1,203003,'SOLO',26),(1,203003,'JAKARTA',22)`,
		`INSERT INTO payroll.pyr_att_snapshot (pas_company_id, pas_yymm, pas_division, pas_source_batch_ids, pas_checksum, pas_locked) VALUES
		  (1,203003,'SOLO','{1}','a',true), (1,203003,'JAKARTA','{1}','b',true)`,
		`INSERT INTO payroll.pyr_att_snapshot_row (pasr_snapshot_id, pasr_kind, pasr_nik, pasr_data, pasr_premi_fail, pasr_beras_fail)
		 SELECT s.pas_snapshot_id, v.kind, v.nik, v.data::jsonb, v.pf, v.bf
		 FROM (VALUES ('SOLO','REKAP','L-SOLO-F','{"MPAR_DAY_WORK":24,"MPAR_ALPA":null,"MPAR_LEMBUR_1":1.5}',2,3),
		              ('SOLO','ADD','L-SOLO-F','{"MPARA_SHIFT_ADD":3}',NULL,NULL),
		              ('SOLO','SCH','L-SOLO-F','{"SCHEDULES":{"NW":20}}',NULL,NULL),
		              ('SOLO','SCH','L-SOLO-N','{"SCHEDULES":{"NW":5}}',NULL,NULL),
		              ('JAKARTA','REKAP','L-JKT','{"MPAR_DAY_WORK":20,"MPAR_ALPA":1,"MPAR_IJIN":2}',4,9),
		              ('JAKARTA','REKAP','L-JKT-FRZ','{"MPAR_ALPA":1,"MPAR_IJIN":1}',0,0),
		              ('JAKARTA','REKAP','L-JKT-NOT1','{"MPAR_ALPA":1,"MPAR_IJIN":1}',0,0)
		      ) v(div,kind,nik,data,pf,bf)
		 JOIN payroll.pyr_att_snapshot s ON s.pas_company_id = 1 AND s.pas_yymm = 203003 AND s.pas_division = v.div`,
		`INSERT INTO payroll.pyr_addition (pa_company_id, pa_yymm, pa_nik, pa_division, pa_type, pa_id, pa_amount, pa_remarks, pa_source) VALUES
		  (1,203003,'L-SOLO-F','SOLO','Potongan',32,250000.75,'cicil','LOAN'),
		  (1,203003,'L-SOLO-F','SOLO','Tunjangan',34,1500000,'bonus','MANUAL')`,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
}

func TestRunLoader_ThreeNIKFixture(t *testing.T) {
	ctx := context.Background()
	if _, err := etl.Run(ctx, etlDB, etl.PSVDir{Dir: "testdata"}, etl.Options{CompanyID: 1, ValidFrom: 202001}); err != nil {
		t.Fatal(err)
	}
	seedLoader(t)
	raw, err := sql.Open("pgx", appDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	app := postgres.NewDBFromSQL(raw)
	load := func(q run.Query, legacy bool) (res run.Result, err error) {
		err = app.ScopedTx(asScopes(t, "SOLO", "JAKARTA"), func(tx *sql.Tx) error {
			res, err = run.NewLoader().Load(ctx, tx, q, run.Rules{Set: stubRules{legacy}})
			return err
		})
		return res, err
	}

	solo := run.Query{CompanyID: 1, Division: "SOLO", YYMM: loaderYYMM, NIKs: []string{"L-SOLO-F", "L-SOLO-N", "L-SOLO-X"}}
	res, err := load(solo, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Inputs) != 2 || res.Inputs[0].Emp.NIK != "L-SOLO-F" || res.Inputs[1].Emp.NIK != "L-SOLO-N" {
		t.Fatalf("inputs (nonaktif left out): %+v", res.Inputs)
	}
	for _, in := range res.Inputs {
		if err := in.Validate(); err != nil {
			t.Fatalf("%s: %v", in.Emp.NIK, err)
		}
		if in.Workdays == nil || *in.Workdays != 26 || in.Period.Start.Format("2006-01-02") != "2030-02-26" || in.Period.End.Day() != 25 {
			t.Fatalf("%s workdays/period: %v %+v", in.Emp.NIK, in.Workdays, in.Period)
		}
	}

	f := res.Inputs[0]
	if f.Emp.GapokSts != "F" || len(f.Components) != 3 { // T1, T74, frozen P4; frozen T6 left out
		t.Fatalf("components: %+v", f.Components)
	}
	var p4 *engine.EmpComponent
	for i, c := range f.Components {
		if c.Key == (engine.CompKey{Type: engine.CompP, ID: 4}) {
			p4 = &f.Components[i]
		}
	}
	if p4 == nil || !p4.Frozen || !p4.Amount.Equal(dec("50000")) {
		t.Fatalf("frozen P4: %+v", p4)
	}
	if !f.Att.Get("day_work").Equal(dec("24")) || !f.Att.Get("lembur_1").Equal(dec("1.5")) || !f.Att.Get("alpa").IsZero() {
		t.Fatalf("att: %v", f.Att)
	}
	if _, has := f.Att["alpa"]; has {
		t.Fatal("null recap column must be absent")
	}
	if !f.AttAdd.Get("shift_add").Equal(dec("3")) || !f.Sch.Get("NW").Equal(dec("20")) {
		t.Fatalf("add/sch: %v %v", f.AttAdd, f.Sch)
	}
	if f.Daily.PremiFail != 2 || f.Daily.BerasFail != 3 {
		t.Fatalf("daily: %+v", f.Daily)
	}
	loan := f.Additions[engine.CompKey{Type: engine.CompP, ID: 32}]
	if len(loan) != 1 || loan[0].Source != engine.AdditionSourceLoan || !loan[0].Amount.Equal(dec("250000.75")) {
		t.Fatalf("additions: %+v", f.Additions)
	}

	n := res.Inputs[1] // no REKAP: Att nil (D2) + warning
	if n.HasRekap() || n.Att != nil || n.AttAdd != nil {
		t.Fatalf("no-rekap input: att=%v add=%v", n.Att, n.AttAdd)
	}
	if len(res.Warnings) != 1 || res.Warnings[0].NIK != "L-SOLO-N" || res.Warnings[0].Code != run.CodeMissingAttendance {
		t.Fatalf("warnings: %+v", res.Warnings)
	}

	t.Run("NIK filter", func(t *testing.T) {
		r, err := load(run.Query{CompanyID: 1, Division: "SOLO", YYMM: loaderYYMM, NIKs: []string{"L-SOLO-N"}}, true)
		if err != nil || len(r.Inputs) != 1 || r.Inputs[0].Emp.NIK != "L-SOLO-N" || len(r.Inputs[0].Components) != 2 {
			t.Fatalf("filter: %v %+v", err, r.Inputs)
		}
		// Validity window: T1 + open-ended T74 load; the expired frozen P4 and the not-yet-valid frozen P8 do not.
		for _, c := range r.Inputs[0].Components {
			if c.Frozen || (c.Key != (engine.CompKey{Type: engine.CompT, ID: 1}) && c.Key != (engine.CompKey{Type: engine.CompT, ID: 74})) {
				t.Fatalf("component outside window loaded: %+v", c)
			}
		}
	})
	t.Run("JAKARTA beras = alpa + ijin", func(t *testing.T) {
		r, err := load(run.Query{CompanyID: 1, Division: "JAKARTA", YYMM: loaderYYMM, NIKs: []string{"L-JKT", "L-JKT-FRZ", "L-JKT-NOT1"}}, true)
		if err != nil || len(r.Inputs) != 3 {
			t.Fatal(err)
		}
		// T1 frozen still counts (legacy EXISTS, no frz); no T1 at all means no beras.
		if r.Inputs[0].Emp.NIK != "L-JKT" || r.Inputs[1].Daily.BerasFail != 2 || r.Inputs[2].Daily.BerasFail != 0 {
			t.Fatalf("beras: %+v %+v", r.Inputs[1].Daily, r.Inputs[2].Daily)
		}
		if d := r.Inputs[0].Daily; d.BerasFail != 3 || d.PremiFail != 4 || *r.Inputs[0].Workdays != 22 {
			t.Fatalf("daily/workdays: %+v", d)
		}
		if err := r.Inputs[0].Validate(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("rule v2: missing attendance is an error", func(t *testing.T) {
		if _, err := load(solo, false); !errors.Is(err, run.ErrMissingAttendance) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("missing ATTD_DAYS and snapshot", func(t *testing.T) {
		if _, err := load(run.Query{CompanyID: 1, Division: "SOLO", YYMM: 203004}, false); !errors.Is(err, run.ErrMissingAttdDays) {
			t.Fatalf("attd days v2: %v", err)
		}
		// Rule v1 outside JAKARTA: no ATTD_DAYS row = NULL workdays, so the loader moves on to the snapshot (D-45).
		if _, err := load(run.Query{CompanyID: 1, Division: "SOLO", YYMM: 203004}, true); !errors.Is(err, run.ErrNoSnapshot) {
			t.Fatalf("attd days v1 SOLO: %v", err)
		}
		// JAKARTA P4 divides by ATTD_DAYS (legacy ORA-01476): still rejected, so a commit cannot drop NIKs silently.
		if _, err := load(run.Query{CompanyID: 1, Division: "JAKARTA", YYMM: 203004}, true); !errors.Is(err, run.ErrMissingAttdDays) {
			t.Fatalf("attd days v1 JAKARTA: %v", err)
		}
		if _, err := ownDB.Exec(`INSERT INTO payroll.pyr_attd_days (pad_company_id, pad_yymm, pad_division, pad_days) VALUES (1,203004,'SOLO',26)`); err != nil {
			t.Fatal(err)
		}
		if _, err := load(run.Query{CompanyID: 1, Division: "SOLO", YYMM: 203004}, true); !errors.Is(err, run.ErrNoSnapshot) {
			t.Fatalf("snapshot: %v", err)
		}
	})
}

// T023f / D-55: v1 keeps legacy parity (out-of-period employees stay), v2 skips them with OUT_OF_PERIOD.
// Also: Load inside a REPEATABLE READ tx does not see changes committed after its first statement.
func TestRunLoader_OutOfPeriodAndSnapshot(t *testing.T) {
	ctx := context.Background()
	for _, q := range []string{
		`INSERT INTO payroll.pyr_employee (pe_company_id, pe_nik, pe_division, pe_nama, pe_level, pe_tax_id, pe_sts_payroll, pe_orion_sub_acnt, pe_date_join, pe_resign_date) VALUES
		  (1,'L-SOLO-LATE','SOLO','Join sesudah periode','Driver','TK',2,'L-SOLO-LATE','2030-04-01',NULL),
		  (1,'L-SOLO-GONE','SOLO','Resign sebelum periode','Driver','TK',2,'L-SOLO-GONE','2019-01-01','2030-01-31'),
		  (1,'L-B-JOIN25','SOLO','Join tepat akhir periode','Driver','TK',2,'L-B-JOIN25','2030-03-25',NULL),
		  (1,'L-B-JOIN26','SOLO','Join sehari sesudah akhir','Driver','TK',2,'L-B-JOIN26','2030-03-26',NULL),
		  (1,'L-B-RES26','SOLO','Resign tepat awal periode','Driver','TK',2,'L-B-RES26','2019-01-01','2030-02-26'),
		  (1,'L-B-RES25','SOLO','Resign sehari sebelum awal','Driver','TK',2,'L-B-RES25','2019-01-01','2030-02-25')
		 ON CONFLICT DO NOTHING`,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = ownDB.Exec(`DELETE FROM payroll.pyr_employee WHERE pe_company_id = 1 AND pe_nik IN ('L-SOLO-LATE','L-SOLO-GONE','L-B-JOIN25','L-B-JOIN26','L-B-RES25','L-B-RES26')`)
	})
	raw, err := sql.Open("pgx", appDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	app := postgres.NewDBFromSQL(raw)
	load := func(q run.Query, legacy bool) (res run.Result, err error) {
		err = app.ScopedTx(asScopes(t, "SOLO", "JAKARTA"), func(tx *sql.Tx) error {
			res, err = run.NewLoader().Load(ctx, tx, q, run.Rules{Set: stubRules{legacy}})
			return err
		})
		return res, err
	}
	q := run.Query{CompanyID: 1, Division: "SOLO", YYMM: loaderYYMM, NIKs: []string{"L-SOLO-F", "L-SOLO-LATE", "L-SOLO-GONE"}}

	t.Run("boundaries: join <= end and resign >= start stay (period 2030-02-26..2030-03-25)", func(t *testing.T) {
		bq := run.Query{CompanyID: 1, Division: "SOLO", YYMM: loaderYYMM, NIKs: []string{"L-B-JOIN25", "L-B-JOIN26", "L-B-RES25", "L-B-RES26"}}
		// v2 without attendance rows would error for kept NIKs; use v1 only for the kept set check via warnings of v2:
		_, err := load(bq, false)
		if !errors.Is(err, run.ErrMissingAttendance) {
			t.Fatalf("kept NIKs have no recap, want ErrMissingAttendance, got %v", err)
		}
		if _, err := ownDB.Exec(`UPDATE payroll.pyr_employee SET pe_sts_payroll = 1 WHERE pe_nik IN ('L-B-JOIN25','L-B-RES26')`); err != nil {
			t.Fatal(err)
		}
		// now only the two outside ones stay active: both skipped, no error
		r, err := load(bq, false)
		if err != nil || len(r.Inputs) != 0 {
			t.Fatalf("outside boundary: %v %d", err, len(r.Inputs))
		}
		got := map[string]bool{}
		for _, w := range r.Warnings {
			if w.Code == run.CodeOutOfPeriod {
				got[w.NIK] = true
			}
		}
		if len(got) != 2 || !got["L-B-JOIN26"] || !got["L-B-RES25"] {
			t.Fatalf("out of period: %+v", r.Warnings)
		}
	})

	t.Run("v1 keeps them (paritas)", func(t *testing.T) {
		r, err := load(q, true)
		if err != nil || len(r.Inputs) != 3 {
			t.Fatalf("v1: %v %d inputs", err, len(r.Inputs))
		}
		for _, w := range r.Warnings {
			if w.Code == run.CodeOutOfPeriod {
				t.Fatalf("v1 must not warn OUT_OF_PERIOD: %+v", w)
			}
		}
	})
	t.Run("v2 skips them with warning", func(t *testing.T) {
		r, err := load(q, false)
		if err != nil {
			t.Fatalf("v2: %v", err) // no ErrMissingAttendance for the skipped ones
		}
		if len(r.Inputs) != 1 || r.Inputs[0].Emp.NIK != "L-SOLO-F" {
			t.Fatalf("v2 inputs: %+v", r.Inputs)
		}
		got := map[string]string{}
		for _, w := range r.Warnings {
			got[w.NIK] = w.Code
		}
		if len(got) != 2 || got["L-SOLO-LATE"] != run.CodeOutOfPeriod || got["L-SOLO-GONE"] != run.CodeOutOfPeriod {
			t.Fatalf("warnings: %+v", r.Warnings)
		}
	})
	t.Run("REPEATABLE READ: change after snapshot not read", func(t *testing.T) {
		q := run.Query{CompanyID: 1, Division: "SOLO", YYMM: loaderYYMM, NIKs: []string{"L-SOLO-F", "L-SOLO-N"}}
		var first, second int
		err := app.ScopedTxOpts(asScopes(t, "SOLO"), run.SnapshotTx, func(tx *sql.Tx) error {
			r, err := run.NewLoader().Load(ctx, tx, q, run.Rules{Set: stubRules{true}})
			if err != nil {
				return err
			}
			first = len(r.Inputs)
			if _, err := ownDB.Exec(`UPDATE payroll.pyr_employee SET pe_sts_payroll = 1 WHERE pe_company_id = 1 AND pe_nik = 'L-SOLO-N'`); err != nil {
				return err
			}
			r, err = run.NewLoader().Load(ctx, tx, q, run.Rules{Set: stubRules{true}})
			second = len(r.Inputs)
			return err
		})
		defer func() {
			_, _ = ownDB.Exec(`UPDATE payroll.pyr_employee SET pe_sts_payroll = 2 WHERE pe_company_id = 1 AND pe_nik = 'L-SOLO-N'`)
		}()
		if err != nil || first != 2 || second != 2 {
			t.Fatalf("snapshot: %v first=%d second=%d (want 2,2)", err, first, second)
		}
		// a fresh tx sees the change (read committed default)
		if r, err := load(q, true); err != nil || len(r.Inputs) != 1 {
			t.Fatalf("after commit: %v %d", err, len(r.Inputs))
		}
	})
}
