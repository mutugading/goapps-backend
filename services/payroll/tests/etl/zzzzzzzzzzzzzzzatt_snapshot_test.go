package etl

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/audit"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/etl"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/postgres"
)

// T021c: snapshot from staging + PREMI_FAIL/BERAS_FAIL (05§6–7). Period 203902 = 2039-01-26 .. 2039-02-25 (31 days).
const snapYM = 203902

func nd2(s string) decimal.NullDecimal {
	if s == "" {
		return decimal.NullDecimal{}
	}
	return decimal.NullDecimal{Decimal: decimal.RequireFromString(s), Valid: true}
}

// dayRows gives n consecutive daily rows from the window start, schedule NW, 8 hours.
func dayRows(nik, div string, n int) []etl.DailyRow {
	out := make([]etl.DailyRow, n)
	for i := range out {
		out[i] = etl.DailyRow{WorkDate: time.Date(2039, 1, 26+i, 0, 0, 0, 0, time.UTC), NIK: nik, Division: div, ScheduleCode: "NW", WorkHours: nd2("8")}
	}
	return out
}

func seedSnapshotMasters(t *testing.T) {
	t.Helper()
	for _, q := range []string{
		// BX: batal + beras; BA: batal + beras + abnormal; NB: batal only (pbp_abnormal NULL = 'N').
		`INSERT INTO payroll.pyr_batal_premi (pbp_code, pbp_beras, pbp_abnormal) VALUES ('BX','BERAS','N'), ('BA','BERAS','Y'), ('NB',NULL,NULL)
		 ON CONFLICT (pbp_code) DO NOTHING`,
		`INSERT INTO payroll.pyr_abnormal_premi (pap_company_id, pap_yymm, pap_nik, pap_date, pap_code, pap_division) VALUES
		  (1,203902,'SN-S1','2039-02-01','X1','SOLO'), (1,203902,'SN-S1','2039-02-02','X1','SOLO'),
		  (1,203901,'SN-S1','2039-01-02','X1','SOLO'), (1,203902,'SN-S1','2039-02-01','X1','JAKARTA')`,
		`INSERT INTO payroll.pyr_component (pc_company_id, pc_code, pc_type, pc_name, pc_status, pc_valid_from)
		 VALUES (1,'T001','Tunjangan','Gaji Pokok','ACTIVE',202001) ON CONFLICT (pc_company_id, pc_code) DO NOTHING`,
		`INSERT INTO payroll.pyr_employee (pe_company_id, pe_nik, pe_division, pe_nama, pe_level, pe_tax_id, pe_sts_payroll, pe_orion_sub_acnt) VALUES
		  (1,'SN-S1','SOLO','S1','Driver','TK',2,'SN-S1'), (1,'SN-S2','SOLO','S2','Driver','TK',2,'SN-S2'),
		  (1,'SN-S3','SOLO','S3','Driver','TK',2,'SN-S3'), (1,'SN-J1','JAKARTA','J1','Staff','TK',2,'SN-J1'),
		  (1,'SN-J2','JAKARTA','J2','Staff','TK',2,'SN-J2')`,
		`INSERT INTO payroll.pyr_employee_component (pec_company_id, pec_nik, pec_division, pec_component_sysid, pec_sts, pec_tax, pec_amount, pec_frz_flag)
		 SELECT 1, 'SN-J1', 'JAKARTA', c.pc_sysid, 'F', 2, 5000000, 1 FROM payroll.pyr_component c WHERE c.pc_company_id = 1 AND c.pc_code = 'T001'`,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
}

// snapFixture is the HRIS answer for 203902.
func snapFixture() (solo, jkt *fakeAtt) {
	rek := func(nik, div, dayWork, alpa, ijin string) etl.RekapRow {
		r := rrow(snapYM, nik, div, dayWork)
		r.Nums["alpa"], r.Nums["ijin"] = nd2(alpa), nd2(ijin)
		return r
	}
	// SN-S1: 31 scheduled days with every 05§7 branch.
	s1 := dayRows("SN-S1", "SOLO", 31)
	s1[0].ScheduleCode, s1[0].WorkHours = "BX", nd2("0")    // (1) premi + beras
	s1[1].ScheduleCode, s1[1].WorkHours = "BA", nd2("4")    // (2) premi + beras (abnormal, hours <> 0)
	s1[2].ScheduleCode, s1[2].WorkHours = "BX", nd2("4")    // not abnormal, hours <> 0: nothing
	s1[3].ScheduleCode, s1[3].WorkHours = "BX", nd2("")     // NULL hours: neither (1) nor (2)
	s1[4].ScheduleCode, s1[4].WorkHours = "NB", nd2("0")    // (1) premi only (not BERAS)
	s1[5].ScheduleCode, s1[5].WorkHours = "BA", nd2("")     // NULL hours, abnormal: nothing
	s1[6].ScheduleCode, s1[6].WorkHours = "NB", nd2("0.00") // (1) again: 0.00 = 0
	// SN-S2: 20 rows, 2 without schedule → (4) = 31 − 18 = 13.
	s2 := dayRows("SN-S2", "SOLO", 20)
	s2[18].ScheduleCode, s2[19].ScheduleCode = "", ""
	s2 = append(s2, dayRows("SN-SD", "SOLO", 3)...) // daily rows, no recap
	solo = &fakeAtt{
		rekap: []etl.RekapRow{rek("SN-S1", "SOLO", "24", "", ""), rek("SN-S2", "SOLO", "18", "1", "0"), rek("SN-S3", "SOLO", "0", "", "")},
		add:   []etl.RekapRow{rrow(snapYM, "SN-S1", "SOLO", "2.5")},
		sch:   []etl.SchRekapRow{{Yymm: snapYM, NIK: "SN-S1", Schedule: "NW", Division: "SOLO", NoSch: nd2("20")}, {Yymm: snapYM, NIK: "SN-S1", Schedule: "NWS", Division: "SOLO", NoSch: nd2("4.50")}},
		daily: append(s1, s2...),
	}
	j1 := dayRows("SN-J1", "JAKARTA", 31)
	j1[0].ScheduleCode, j1[0].WorkHours = "BX", nd2("0") // JAKARTA premi counts batal days too; beras comes from the recap
	jkt = &fakeAtt{
		rekap: []etl.RekapRow{rek("SN-J1", "JAKARTA", "20", "1.5", "2"), rek("SN-J2", "JAKARTA", "20", "1", "1")},
		add:   []etl.RekapRow{rrow(snapYM, "SN-J1", "JAKARTA", "1")},
		daily: j1,
	}
	return solo, jkt
}

func syncAll(t *testing.T, f etl.AttReader, ym int, div string) []etl.AttSyncReport {
	t.Helper()
	q := attQ(ym, div)
	q.Source = etl.AttSrcAll
	reps, err := mustRun(context.Background(), f, q)
	if err != nil {
		t.Fatalf("sync %s: %v", div, err)
	}
	return reps
}

func snapOne(t *testing.T, ym int, div string) etl.AttSnapshotReport {
	t.Helper()
	reps, err := etl.RunAttSnapshot(context.Background(), etlDB, etl.AttSnapshotQuery{Company: 1, YYMM: ym, Division: div})
	if err != nil || len(reps) != 1 {
		t.Fatalf("snapshot %d %s: %v %+v", ym, div, err, reps)
	}
	return reps[0]
}

func snapFail(t *testing.T, id int64, nik string) string {
	return scalar(t, fmt.Sprintf(`SELECT coalesce(pasr_premi_fail::text,'null')||'/'||coalesce(pasr_beras_fail::text,'null')
		FROM payroll.pyr_att_snapshot_row WHERE pasr_snapshot_id=%d AND pasr_kind='REKAP' AND pasr_nik='%s'`, id, nik))
}

func TestAttSnapshot_FromStaging(t *testing.T) {
	seedSnapshotMasters(t)
	solo, jkt := snapFixture()
	soloReps := syncAll(t, solo, snapYM, "SOLO")
	syncAll(t, jkt, snapYM, "JAKARTA")

	s := snapOne(t, snapYM, "SOLO")
	if s.Rekap != 3 || s.Add != 1 || s.Sch != 1 || s.PeriodDayCount != 31 || s.Checksum == "" || len(s.BatchIDs) != 4 {
		t.Fatalf("solo report: %+v", s)
	}
	for i, r := range soloReps { // rekap, add, sch, daily: the batches just written
		if s.BatchIDs[i] != r.BatchID {
			t.Fatalf("source_batch_ids %v, sync batches %+v", s.BatchIDs, soloReps)
		}
	}
	if got := scalar(t, fmt.Sprintf(`SELECT pas_source_batch_ids::text||'/'||pas_locked||'/'||pas_checksum FROM payroll.pyr_att_snapshot WHERE pas_snapshot_id=%d`, s.SnapshotID)); got != fmt.Sprintf("{%d,%d,%d,%d}/false/%s", s.BatchIDs[0], s.BatchIDs[1], s.BatchIDs[2], s.BatchIDs[3], s.Checksum) {
		t.Fatalf("snapshot header %s", got)
	}

	t.Run("PREMI/BERAS_FAIL 05§7 SOLO", func(t *testing.T) {
		// S1: (1) BX/0 + NB/0 + NB/0.00 = 3, (2) BA/4 = 1, (3) 2 manual rows of 203902 SOLO, (4) 0; NULL hours not counted.
		// Beras: BX/0 + BA/4 = 2. S2: (4) 31 − 18 = 13. S3 without daily rows: 31 (legacy COUNT(*) = 0).
		for nik, want := range map[string]string{"SN-S1": "6/2", "SN-S2": "13/0", "SN-S3": "31/0"} {
			if got := snapFail(t, s.SnapshotID, nik); got != want {
				t.Errorf("%s premi/beras = %s, want %s", nik, got, want)
			}
		}
	})
	t.Run("JAKARTA beras = alpa + ijin with T1", func(t *testing.T) {
		j := snapOne(t, snapYM, "JAKARTA")
		if j.Sch != 0 || len(j.BatchIDs) != 4 { // the sch batch is OK with zero rows
			t.Fatalf("jakarta report %+v", j)
		}
		// J1: premi (1) BX/0 = 1, beras trunc(1.5 + 2) = 3 (T1 frozen counts, no frz condition). J2 no T1 → 0, no daily → 31.
		for nik, want := range map[string]string{"SN-J1": "1/3", "SN-J2": "31/0"} {
			if got := snapFail(t, j.SnapshotID, nik); got != want {
				t.Errorf("%s premi/beras = %s, want %s", nik, got, want)
			}
		}
	})
	t.Run("jsonb format = LEGACY", func(t *testing.T) {
		row := func(kind, nik string) string {
			return scalar(t, fmt.Sprintf(`SELECT pasr_data::text FROM payroll.pyr_att_snapshot_row WHERE pasr_snapshot_id=%d AND pasr_kind='%s' AND pasr_nik='%s'`, s.SnapshotID, kind, nik))
		}
		rk := row("REKAP", "SN-S2")
		for _, want := range []string{`"MPAR_DAY_WORK": 18`, `"MPAR_ALPA": 1`, `"MPAR_IJIN": 0`, `"MPAR_LEMBUR_1": null`, `"MPAR_OTIW_DECR": null`, `"MPAR_NAMA": "Nama SN-S2"`} {
			if !strings.Contains(rk, want) {
				t.Errorf("REKAP %s lacks %s", rk, want)
			}
		}
		if strings.Contains(rk, "ROW_HASH") || strings.Contains(rk, "YYMM") || strings.Contains(rk, "DIVISION") {
			t.Errorf("REKAP has key/technical columns: %s", rk)
		}
		if ad := row("ADD", "SN-S1"); !strings.Contains(ad, `"MPARA_DAY_WORK": 2.5`) || !strings.Contains(ad, `"MPARA_SHIFT": null`) || strings.Contains(ad, "MPARA_DAYOFF") {
			t.Errorf("ADD %s", ad)
		}
		if sc := row("SCH", "SN-S1"); sc != `{"SCHEDULES": {"NW": 20, "NWS": 4.5}}` {
			t.Errorf("SCH %s", sc)
		}
		if n := scalar(t, fmt.Sprintf(`SELECT count(*) FROM payroll.pyr_att_snapshot_row WHERE pasr_snapshot_id=%d AND pasr_kind<>'REKAP' AND (pasr_premi_fail IS NOT NULL OR pasr_beras_fail IS NOT NULL)`, s.SnapshotID)); n != "0" {
			t.Errorf("ADD/SCH rows carry fail counts: %s", n)
		}
	})
	t.Run("loader + dry run read the new snapshot", func(t *testing.T) {
		raw, err := sql.Open("pgx", appDSN)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = raw.Close() }()
		app := postgres.NewDBFromSQL(raw)
		q := run.Query{CompanyID: 1, Division: "SOLO", YYMM: snapYM, NIKs: []string{"SN-S1", "SN-S2", "SN-S3"}}
		var res run.Result
		if err := app.ScopedTx(asScopes(t, "SOLO"), func(tx *sql.Tx) error {
			res, err = run.NewLoader().Load(context.Background(), tx, q, run.Rules{Set: stubRules{true}})
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if res.SnapshotID != s.SnapshotID || len(res.Inputs) != 3 {
			t.Fatalf("loader snapshot %d (want %d), inputs %d", res.SnapshotID, s.SnapshotID, len(res.Inputs))
		}
		in := res.Inputs[0]
		if in.Daily != (engine.DailyFails{PremiFail: 6, BerasFail: 2}) || !in.Att.Get("day_work").Equal(dec("24")) ||
			!in.AttAdd.Get("day_work").Equal(dec("2.5")) || !in.Sch.Get("NWS").Equal(dec("4.5")) {
			t.Fatalf("S1 input: daily %+v att %v add %v sch %v", in.Daily, in.Att, in.AttAdd, in.Sch)
		}
		if res.Inputs[2].Daily.PremiFail != 31 {
			t.Fatalf("S3 premi %+v", res.Inputs[2].Daily)
		}
		// SN-SD has daily rows but no recap: no snapshot row (the recap drives the snapshot).
		if n := scalar(t, fmt.Sprintf(`SELECT count(*) FROM payroll.pyr_att_snapshot_row WHERE pasr_snapshot_id=%d AND pasr_nik='SN-SD'`, s.SnapshotID)); n != "0" {
			t.Fatalf("daily-only NIK in snapshot: %s", n)
		}

		stub := func(in engine.Input) (engine.Result, error) {
			a := dec("1000")
			return engine.Result{NIK: in.Emp.NIK, Lines: []engine.Line{{Key: engine.CompKey{Type: engine.CompT, ID: 1}, Amount: &a, Tax: true}},
				Header: &engine.Header{TotTunjangan: a}}, nil
		}
		// The rule set is stubbed so the test runs alone (rule set 1 only exists after TestRuleSetLoader_FromETL).
		rules := func(context.Context, *sql.Tx, run.Query, int) (run.Rules, error) {
			return run.Rules{Set: stubRules{true}}, nil
		}
		dry, err := run.NewServiceWith(app, rules, audit.NewWriter(), run.Runner{Calc: stub}).Dry(asScopes(t, "SOLO"),
			run.DryCommand{CompanyID: 1, Division: "SOLO", YYMM: snapYM, RuleSetID: 1, NIKs: q.NIKs})
		if err != nil || dry.SnapshotID != s.SnapshotID || dry.Summary.EmpCount != 3 {
			t.Fatalf("dry: %v snapshot %d (want %d) %+v", err, dry.SnapshotID, s.SnapshotID, dry.Summary)
		}
	})
	t.Run("snapshot is never updated; a correction is a new snapshot", func(t *testing.T) {
		before := scalar(t, fmt.Sprintf(`SELECT md5(string_agg(pasr_kind||pasr_nik||pasr_data::text||coalesce(pasr_premi_fail,-1)||coalesce(pasr_beras_fail,-1), ',' ORDER BY pasr_kind, pasr_nik))
			FROM payroll.pyr_att_snapshot_row WHERE pasr_snapshot_id=%d`, s.SnapshotID))
		// HRIS correction on S2 only, synced rekap only: unchanged rows keep the older batch id, the snapshot still holds them all.
		solo.rekap[1].Nums["day_work"] = nd2("19")
		r := run1(t, solo, attQ(snapYM, "SOLO"))
		if r.Upserted != 1 {
			t.Fatalf("correction sync %+v", r)
		}
		s2 := snapOne(t, snapYM, "SOLO")
		if s2.SnapshotID <= s.SnapshotID || s2.Rekap != 3 || s2.Checksum == s.Checksum || s2.BatchIDs[0] != r.BatchID || s2.BatchIDs[3] != s.BatchIDs[3] {
			t.Fatalf("second snapshot %+v (first %+v, rekap batch %d)", s2, s, r.BatchID)
		}
		if got := scalar(t, fmt.Sprintf(`SELECT pasr_data->>'MPAR_DAY_WORK' FROM payroll.pyr_att_snapshot_row WHERE pasr_snapshot_id=%d AND pasr_kind='REKAP' AND pasr_nik='SN-S2'`, s2.SnapshotID)); got != "19" {
			t.Fatalf("new snapshot S2 day_work %s", got)
		}
		after := scalar(t, fmt.Sprintf(`SELECT md5(string_agg(pasr_kind||pasr_nik||pasr_data::text||coalesce(pasr_premi_fail,-1)||coalesce(pasr_beras_fail,-1), ',' ORDER BY pasr_kind, pasr_nik))
			FROM payroll.pyr_att_snapshot_row WHERE pasr_snapshot_id=%d`, s.SnapshotID))
		if before != after || scalar(t, fmt.Sprintf(`SELECT pas_checksum FROM payroll.pyr_att_snapshot WHERE pas_snapshot_id=%d`, s.SnapshotID)) != s.Checksum {
			t.Fatal("first snapshot changed")
		}
		// Same staging and batches again: unchanged, no new snapshot, no audit row, no error.
		s3 := snapOne(t, snapYM, "SOLO")
		if !s3.Unchanged || s3.SnapshotID != s2.SnapshotID || s3.Checksum != s2.Checksum {
			t.Fatalf("third snapshot must be unchanged: %+v vs %+v", s3, s2)
		}
		if n := scalar(t, fmt.Sprintf(`SELECT count(*) FROM payroll.pyr_att_snapshot WHERE pas_yymm=%d AND pas_division='SOLO'`, snapYM)); n != "2" {
			t.Fatalf("snapshots after dedup %s", n)
		}
		if n := scalar(t, fmt.Sprintf(`SELECT count(*) FROM payroll.pyr_audit_log WHERE pal_action='ETL_ATT_SNAPSHOT' AND pal_entity_key IN ('%d','%d')`, s.SnapshotID, s2.SnapshotID)); n != "2" {
			t.Fatalf("audit rows %s", n)
		}
		if got := scalar(t, fmt.Sprintf(`SELECT pal_after->>'beras_fail_niks' FROM payroll.pyr_audit_log WHERE pal_action='ETL_ATT_SNAPSHOT' AND pal_entity_key='%d'`, s.SnapshotID)); got != "1" {
			t.Fatalf("audit beras_fail_niks %s", got)
		}
		// Same content but a new OK batch (re-sync without change): batches differ, so a new snapshot.
		run1(t, solo, attQ(snapYM, "SOLO"))
		if s4 := snapOne(t, snapYM, "SOLO"); s4.Unchanged || s4.SnapshotID <= s2.SnapshotID || s4.Checksum != s2.Checksum {
			t.Fatalf("new batch, same content: %+v", s4)
		}
	})
}

func TestAttSnapshot_Refused(t *testing.T) {
	const closed, locked, nosync, noDaily, zeroDaily, failedAfter = 203903, 203904, 203905, 203906, 203907, 203908
	for _, q := range []string{
		fmt.Sprintf(`INSERT INTO payroll.pyr_period (pp_company_id, pp_yymm, pp_division, pp_date_from, pp_date_to, pp_status)
			VALUES (1,%d,'SOLO','2039-02-26','2039-03-25','OPEN')`, closed),
		fmt.Sprintf(`INSERT INTO payroll.pyr_att_snapshot (pas_company_id, pas_yymm, pas_division, pas_source_batch_ids, pas_checksum, pas_locked)
			VALUES (1,%d,'SOLO','{1}','snap-lock',false)`, locked),
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	// Staging for every refused period first (sync is allowed while OPEN / unlocked).
	for _, ym := range []int{closed, locked} {
		syncAll(t, &fakeAtt{rekap: []etl.RekapRow{rrow(ym, "SN-R1", "SOLO", "20")}, daily: []etl.DailyRow{{WorkDate: time.Date(ym/100, time.Month(ym%100), 1, 0, 0, 0, 0, time.UTC), NIK: "SN-R1", Division: "SOLO", ScheduleCode: "NW"}}}, ym, "SOLO")
	}
	run1(t, &fakeAtt{rekap: []etl.RekapRow{rrow(noDaily, "SN-R1", "SOLO", "20")}}, attQ(noDaily, "SOLO"))  // rekap only
	syncAll(t, &fakeAtt{rekap: []etl.RekapRow{rrow(zeroDaily, "SN-R1", "SOLO", "20")}}, zeroDaily, "SOLO") // daily batch OK, 0 rows
	// failedAfter: everything OK, then a daily re-sync that fails (Oracle down): the newer FAILED batch blocks.
	okDaily := []etl.DailyRow{{WorkDate: time.Date(2039, 8, 1, 0, 0, 0, 0, time.UTC), NIK: "SN-R1", Division: "SOLO", ScheduleCode: "NW"}}
	fa := &fakeAtt{rekap: []etl.RekapRow{rrow(failedAfter, "SN-R1", "SOLO", "20")}, daily: okDaily}
	syncAll(t, fa, failedAfter, "SOLO")
	fa.err = fmt.Errorf("ORA-12170")
	dq := attQ(failedAfter, "SOLO")
	dq.Source = etl.AttSrcDaily
	if _, err := mustRun(context.Background(), fa, dq); err == nil {
		t.Fatal("daily sync must fail")
	}
	for _, q := range []string{
		fmt.Sprintf(`UPDATE payroll.pyr_period SET pp_status='CLOSED' WHERE pp_company_id=1 AND pp_yymm=%d AND pp_division='SOLO'`, closed),
		fmt.Sprintf(`UPDATE payroll.pyr_att_snapshot SET pas_locked=true WHERE pas_company_id=1 AND pas_yymm=%d AND pas_division='SOLO'`, locked),
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	for ym, want := range map[int]string{closed: "CLOSED", locked: "terkunci", nosync: "belum ada batch sync OK", noDaily: "HM_ATT_DAILY", zeroDaily: "staging harian", failedAfter: "FAILED"} {
		n0 := scalar(t, fmt.Sprintf(`SELECT count(*) FROM payroll.pyr_att_snapshot WHERE pas_yymm=%d`, ym))
		reps, err := etl.RunAttSnapshot(context.Background(), etlDB, etl.AttSnapshotQuery{Company: 1, YYMM: ym, Division: "SOLO"})
		if err == nil || !strings.Contains(err.Error(), want) || len(reps) != 0 {
			t.Fatalf("%d: want refusal %q, got %v %+v", ym, want, err, reps)
		}
		if n := scalar(t, fmt.Sprintf(`SELECT count(*) FROM payroll.pyr_att_snapshot WHERE pas_yymm=%d`, ym)); n != n0 {
			t.Fatalf("%d: snapshot written despite refusal", ym)
		}
	}
	// Both divisions: SOLO refused (CLOSED) does not stop JAKARTA.
	syncAll(t, &fakeAtt{rekap: []etl.RekapRow{rrow(closed, "SN-R2", "JAKARTA", "20")}, daily: []etl.DailyRow{{WorkDate: time.Date(2039, 3, 1, 0, 0, 0, 0, time.UTC), NIK: "SN-R2", Division: "JAKARTA", ScheduleCode: "NW"}}}, closed, "JAKARTA")
	reps, err := etl.RunAttSnapshot(context.Background(), etlDB, etl.AttSnapshotQuery{Company: 1, YYMM: closed})
	if err == nil || len(reps) != 1 || reps[0].Division != "JAKARTA" || reps[0].SnapshotID == 0 {
		t.Fatalf("both divisions: %v %+v", err, reps)
	}
}

// The work-date window crosses the year: January starts on 26 December, December ends on 25 December.
func TestAttSnapshot_YearBoundary(t *testing.T) {
	for _, c := range []struct {
		ym         int
		first, end time.Time
		days       int
	}{
		{204001, time.Date(2039, 12, 26, 0, 0, 0, 0, time.UTC), time.Date(2040, 1, 25, 0, 0, 0, 0, time.UTC), 31},
		{204012, time.Date(2040, 11, 26, 0, 0, 0, 0, time.UTC), time.Date(2040, 12, 25, 0, 0, 0, 0, time.UTC), 30},
	} {
		// One scheduled day at each edge, one just outside each edge (rejected by the sync window).
		daily := []etl.DailyRow{
			{WorkDate: c.first, NIK: "SN-Y1", Division: "SOLO", ScheduleCode: "NW", WorkHours: nd2("8")},
			{WorkDate: c.end, NIK: "SN-Y1", Division: "SOLO", ScheduleCode: "NW", WorkHours: nd2("8")},
			{WorkDate: c.first.AddDate(0, 0, -1), NIK: "SN-Y1", Division: "SOLO", ScheduleCode: "NW", WorkHours: nd2("8")},
			{WorkDate: c.end.AddDate(0, 0, 1), NIK: "SN-Y1", Division: "SOLO", ScheduleCode: "NW", WorkHours: nd2("8")},
		}
		syncAll(t, &fakeAtt{rekap: []etl.RekapRow{rrow(c.ym, "SN-Y1", "SOLO", "20")}, daily: daily}, c.ym, "SOLO")
		r := snapOne(t, c.ym, "SOLO")
		if r.PeriodDayCount != c.days {
			t.Fatalf("%d: days %d, want %d", c.ym, r.PeriodDayCount, c.days)
		}
		if got, want := snapFail(t, r.SnapshotID, "SN-Y1"), fmt.Sprintf("%d/0", c.days-2); got != want {
			t.Fatalf("%d: premi/beras %s, want %s", c.ym, got, want)
		}
	}
}
