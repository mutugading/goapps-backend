package etl

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/etl"
)

// fakeAtt is a settable etl.AttReader.
type fakeAtt struct {
	rekap, add []etl.RekapRow
	daily      []etl.DailyRow
	sch        []etl.SchRekapRow
	err        error
	calls      int
	failFirst  int // fail this many calls before succeeding
	gotStart   time.Time
	gotEnd     time.Time
}

func (f *fakeAtt) fail() error {
	f.calls++
	if f.calls <= f.failFirst {
		return errors.New("oracle down")
	}
	return f.err
}

func (f *fakeAtt) Rekap(context.Context, int, string) ([]etl.RekapRow, error) {
	return f.rekap, f.fail()
}
func (f *fakeAtt) RekapAdd(context.Context, int, string) ([]etl.RekapRow, error) {
	return f.add, f.fail()
}
func (f *fakeAtt) SchRekap(context.Context, int, string) ([]etl.SchRekapRow, error) {
	return f.sch, f.fail()
}
func (f *fakeAtt) Daily(_ context.Context, s, e time.Time, _ string) ([]etl.DailyRow, error) {
	f.gotStart, f.gotEnd = s, e
	return f.daily, f.fail()
}

func rrow(yymm int, nik, div, dayWork string) etl.RekapRow {
	return etl.RekapRow{Yymm: yymm, NIK: nik, Nama: "Nama " + nik, Division: div,
		Nums: map[string]decimal.NullDecimal{"day_work": decimal.NullDecimal{Decimal: decimal.RequireFromString(dayWork), Valid: true}}}
}

func attQ(yymm int, div string) etl.AttSyncQuery {
	return etl.AttSyncQuery{Company: 1, YYMM: yymm, Division: div, Source: etl.AttSrcRekap, Backoff: time.Millisecond}
}

func stgCount(t *testing.T, yymm int, div string) string {
	return scalar(t, fmt.Sprintf(`SELECT count(*)::text FROM payroll_stg.pyr_stg_att_rekap_hm WHERE psar_yymm=%d AND psar_division='%s'`, yymm, div))
}

func stgDayWork(t *testing.T, yymm int, nik string) string {
	return scalar(t, fmt.Sprintf(`SELECT coalesce(psar_day_work::text,'null') FROM payroll_stg.pyr_stg_att_rekap_hm WHERE psar_yymm=%d AND psar_nik='%s'`, yymm, nik))
}

func lastBatch(t *testing.T, yymm int) string {
	return scalar(t, fmt.Sprintf(`SELECT psb_status||'/'||coalesce(psb_rows_in,-1)||'/'||coalesce(psb_rows_upserted,-1)||'/'||coalesce(psb_rows_deleted,-1)
		FROM payroll_stg.pyr_stg_sync_batch WHERE psb_yymm=%d ORDER BY psb_id DESC LIMIT 1`, yymm))
}

func one(t *testing.T, reps []etl.AttSyncReport, err error) etl.AttSyncReport {
	t.Helper()
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(reps) != 1 {
		t.Fatalf("reports = %d", len(reps))
	}
	return reps[0]
}

func TestAttSync_UpsertDeleteIdempotent(t *testing.T) {
	const ym = 203801
	f := &fakeAtt{rekap: []etl.RekapRow{rrow(ym, "A1", "SOLO", "20"), rrow(ym, "A2", "SOLO", "21"), rrow(ym, "A3", "SOLO", "22")}}

	r := run1(t, f, attQ(ym, "SOLO"))
	if r.Upserted != 3 || r.Deleted != 0 || r.Status != "OK" || r.Checksum == "" {
		t.Fatalf("first run: %+v", r)
	}
	if got := lastBatch(t, ym); got != "OK/3/3/0" {
		t.Fatalf("batch = %s", got)
	}
	first := r.Checksum

	r = run1(t, f, attQ(ym, "SOLO")) // idempotent: nothing changed in HRIS
	if r.Upserted != 0 || r.Deleted != 0 || r.Checksum != first {
		t.Fatalf("second run must be a no-op: %+v", r)
	}

	// HRIS correction + a NIK that disappeared.
	f.rekap = []etl.RekapRow{rrow(ym, "A1", "SOLO", "20"), rrow(ym, "A2", "SOLO", "25")}
	r = run1(t, f, attQ(ym, "SOLO"))
	if r.Upserted != 1 || r.Deleted != 1 || r.Checksum == first {
		t.Fatalf("correction run: %+v", r)
	}
	if stgDayWork(t, ym, "A2") != "25.00" || stgCount(t, ym, "SOLO") != "2" {
		t.Fatalf("staging not updated: A2=%s n=%s", stgDayWork(t, ym, "A2"), stgCount(t, ym, "SOLO"))
	}
	// checksum = md5(string_agg(row_hash ORDER BY nik))
	if got := scalar(t, fmt.Sprintf(`SELECT md5(string_agg(psar_row_hash,'' ORDER BY psar_nik)) FROM payroll_stg.pyr_stg_att_rekap_hm WHERE psar_yymm=%d`, ym)); got != r.Checksum {
		t.Fatalf("checksum %s != recomputed %s", r.Checksum, got)
	}

	// Dry run: reports the diff, changes nothing, leaves no batch row.
	nBatch := scalar(t, `SELECT count(*)::text FROM payroll_stg.pyr_stg_sync_batch`)
	f.rekap = []etl.RekapRow{rrow(ym, "A1", "SOLO", "1")}
	q := attQ(ym, "SOLO")
	q.Dry = true
	r = run1(t, f, q)
	if r.Status != "DRY" || r.Upserted != 1 || r.Deleted != 1 {
		t.Fatalf("dry: %+v", r)
	}
	if stgDayWork(t, ym, "A1") != "20.00" || stgCount(t, ym, "SOLO") != "2" || scalar(t, `SELECT count(*)::text FROM payroll_stg.pyr_stg_sync_batch`) != nBatch {
		t.Fatal("dry run changed the database")
	}
}

func mustRun(ctx context.Context, f etl.AttReader, q etl.AttSyncQuery) ([]etl.AttSyncReport, error) {
	return etl.RunAttSync(ctx, etlDB, f, q)
}

func TestAttSync_InsertOnly(t *testing.T) {
	const ym = 203802
	f := &fakeAtt{rekap: []etl.RekapRow{rrow(ym, "B1", "SOLO", "10"), rrow(ym, "B2", "SOLO", "11")}}
	run1(t, f, attQ(ym, "SOLO"))

	f.rekap = []etl.RekapRow{rrow(ym, "B1", "SOLO", "99"), rrow(ym, "B3", "SOLO", "12")} // B1 changed, B2 gone, B3 new
	q := attQ(ym, "SOLO")
	q.InsertOnly = true
	r := run1(t, f, q)
	if r.Upserted != 1 || r.Deleted != 0 {
		t.Fatalf("insert-only: %+v", r)
	}
	if stgDayWork(t, ym, "B1") != "10.00" || stgCount(t, ym, "SOLO") != "3" {
		t.Fatalf("insert-only touched existing rows: B1=%s n=%s", stgDayWork(t, ym, "B1"), stgCount(t, ym, "SOLO"))
	}
}

func TestAttSync_ClosedAndLockedRefused(t *testing.T) {
	const closed, locked = 203803, 203804
	for _, q := range []string{
		fmt.Sprintf(`INSERT INTO payroll.pyr_period (pp_company_id, pp_yymm, pp_division, pp_date_from, pp_date_to, pp_status)
			VALUES (1,%d,'SOLO','2038-02-26','2038-03-25','CLOSED')`, closed),
		fmt.Sprintf(`INSERT INTO payroll.pyr_att_snapshot (pas_company_id, pas_yymm, pas_division, pas_source_batch_ids, pas_checksum, pas_locked)
			VALUES (1,%d,'SOLO','{1}','attsync-lock',true)`, locked),
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	f := &fakeAtt{rekap: []etl.RekapRow{rrow(closed, "C1", "SOLO", "20")}}
	for _, ym := range []int{closed, locked} {
		f.rekap = []etl.RekapRow{rrow(ym, "C1", "SOLO", "20")}
		reps, err := mustRun(context.Background(), f, attQ(ym, "SOLO"))
		if err == nil || !strings.Contains(err.Error(), "ditolak") {
			t.Fatalf("%d: want refusal, got %v", ym, err)
		}
		if len(reps) != 1 || reps[0].Status == "OK" {
			t.Fatalf("%d: report %+v", ym, reps)
		}
		if stgCount(t, ym, "SOLO") != "0" {
			t.Fatalf("%d: staging written despite refusal", ym)
		}
		if got := lastBatch(t, ym); !strings.HasPrefix(got, "FAILED/") {
			t.Fatalf("%d: batch = %s", ym, got)
		}
	}
	if f.calls != 0 {
		t.Fatalf("Oracle was read %d times for a refused period", f.calls)
	}
}

func TestAttSync_FailureKeepsStaging(t *testing.T) {
	const ym = 203805
	f := &fakeAtt{rekap: []etl.RekapRow{rrow(ym, "D1", "SOLO", "20"), rrow(ym, "D2", "SOLO", "21")}}
	first := run1(t, f, attQ(ym, "SOLO"))

	// (a) Oracle down for every attempt: FAILED, staging untouched.
	f.err = errors.New("ORA-12170 timeout")
	f.calls = 0
	if _, err := mustRun(context.Background(), f, attQ(ym, "SOLO")); err == nil {
		t.Fatal("want error")
	}
	if f.calls != 3 {
		t.Fatalf("attempts = %d, want 3", f.calls)
	}
	if got := lastBatch(t, ym); !strings.HasPrefix(got, "FAILED/") {
		t.Fatalf("batch = %s", got)
	}
	// (b) transient failure: the retry succeeds.
	f.err, f.calls, f.failFirst = nil, 0, 2
	f.rekap = []etl.RekapRow{rrow(ym, "D1", "SOLO", "20"), rrow(ym, "D2", "SOLO", "21")}
	if r := run1(t, f, attQ(ym, "SOLO")); r.Status != "OK" || f.calls != 3 {
		t.Fatalf("retry: %+v calls=%d", r, f.calls)
	}
	f.failFirst, f.calls = 0, 0

	// (c) failure in the middle of the write transaction (duplicate key in the source): new data must not leak in.
	f.rekap = []etl.RekapRow{rrow(ym, "D1", "SOLO", "77"), rrow(ym, "D9", "SOLO", "1"), rrow(ym, "D9", "SOLO", "2")}
	if _, err := mustRun(context.Background(), f, attQ(ym, "SOLO")); err == nil {
		t.Fatal("duplicate source key must fail")
	}
	if stgDayWork(t, ym, "D1") != "20.00" || stgCount(t, ym, "SOLO") != "2" {
		t.Fatalf("old staging not intact: D1=%s n=%s", stgDayWork(t, ym, "D1"), stgCount(t, ym, "SOLO"))
	}
	if got := lastBatch(t, ym); !strings.HasPrefix(got, "FAILED/") {
		t.Fatalf("batch = %s", got)
	}

	// (d) empty HRIS answer must not wipe a populated staging.
	f.rekap = nil
	if _, err := mustRun(context.Background(), f, attQ(ym, "SOLO")); err == nil || stgCount(t, ym, "SOLO") != "2" {
		t.Fatalf("empty source: err=%v n=%s", err, stgCount(t, ym, "SOLO"))
	}
	f.rekap = []etl.RekapRow{rrow(ym, "D1", "SOLO", "20"), rrow(ym, "D2", "SOLO", "21")}
	if r := run1(t, f, attQ(ym, "SOLO")); r.Checksum != first.Checksum || r.Upserted != 0 {
		t.Fatalf("after failures the data must be as before: %+v", r)
	}
}

func TestAttSync_V6DivisionRejected(t *testing.T) {
	const ym = 203806
	f := &fakeAtt{rekap: []etl.RekapRow{rrow(ym, "E1", "SOLO", "20"), rrow(ym, "E2", "BANDUNG", "21"), rrow(ym, "E3", "SOLO", "22")}}
	r := run1(t, f, attQ(ym, "SOLO"))
	if r.Rejected != 1 || r.RowsIn != 3 || r.Upserted != 2 || len(r.RejectedNIKs) != 1 || r.RejectedNIKs[0] != "E2" {
		t.Fatalf("V6: %+v", r)
	}
	if stgCount(t, ym, "SOLO") != "2" || stgCount(t, ym, "BANDUNG") != "0" {
		t.Fatal("rejected row reached staging")
	}
	if n := scalar(t, fmt.Sprintf(`SELECT count(*)::text FROM payroll_stg.pyr_stg_sync_batch WHERE psb_yymm=%d AND psb_error LIKE 'V6:%%'`, ym)); n != "1" {
		t.Fatalf("rejection not recorded in the batch (%s)", n)
	}
	// An unsupported division on the command line is refused outright.
	if _, err := mustRun(context.Background(), f, attQ(ym, "BANDUNG")); err == nil {
		t.Fatal("division BANDUNG must be refused")
	}
}

func TestAttSync_AllSourcesAndDailyWindow(t *testing.T) {
	const ym = 203807
	in := time.Date(2038, 6, 26, 7, 0, 0, 0, time.UTC)
	hours := decimal.NullDecimal{Decimal: decimal.RequireFromString("7.5"), Valid: true}
	f := &fakeAtt{
		rekap: []etl.RekapRow{rrow(ym, "F1", "JAKARTA", "20")},
		add:   []etl.RekapRow{rrow(ym, "F1", "JAKARTA", "3")},
		sch: []etl.SchRekapRow{{Yymm: ym, NIK: "F1", Schedule: "NW", Division: "JAKARTA", NoSch: hours},
			{Yymm: ym, NIK: "F1", Schedule: "NWS", Division: "JAKARTA", NoSch: hours}},
		daily: []etl.DailyRow{
			{WorkDate: time.Date(2038, 6, 26, 0, 0, 0, 0, time.UTC), NIK: "F1", Division: "JAKARTA", ScheduleCode: "NW", CheckIn: &in, WorkHours: hours},
			{WorkDate: time.Date(2038, 7, 25, 0, 0, 0, 0, time.UTC), NIK: "F1", Division: "JAKARTA", ScheduleCode: "NW"},
			{WorkDate: time.Date(2038, 7, 26, 0, 0, 0, 0, time.UTC), NIK: "F1", Division: "JAKARTA", ScheduleCode: "NW"}, // outside the window
		},
	}
	q := attQ(ym, "JAKARTA")
	q.Source = etl.AttSrcAll
	reps, err := mustRun(context.Background(), f, q)
	if err != nil || len(reps) != 4 {
		t.Fatalf("all: %v %+v", err, reps)
	}
	if f.gotStart.Format("2006-01-02") != "2038-06-26" || f.gotEnd.Format("2006-01-02") != "2038-07-25" {
		t.Fatalf("daily window = %s..%s", f.gotStart, f.gotEnd)
	}
	for tbl, c := range map[string][2]string{
		"pyr_stg_att_rekap_hm": {"psar_yymm=203807", "1"}, "pyr_stg_att_rekap_add": {"psar_yymm=203807", "1"},
		"pyr_stg_sch_rekap": {"pssr_yymm=203807", "2"}, "pyr_stg_att_daily": {"psad_nik='F1'", "2"},
	} {
		if got := scalar(t, `SELECT count(*)::text FROM payroll_stg.`+tbl+` WHERE `+c[0]); got != c[1] {
			t.Fatalf("%s rows = %s, want %s", tbl, got, c[1])
		}
	}
	for _, r := range reps {
		if r.Source == etl.AttSrcDaily && r.Rejected != 1 {
			t.Fatalf("daily outside window must be rejected: %+v", r)
		}
	}
	// second run: all four idempotent.
	reps, err = mustRun(context.Background(), f, q)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range reps {
		if r.Upserted != 0 || r.Deleted != 0 {
			t.Fatalf("second run not idempotent: %+v", r)
		}
	}
}

func run1(t *testing.T, f etl.AttReader, q etl.AttSyncQuery) etl.AttSyncReport {
	t.Helper()
	reps, err := mustRun(context.Background(), f, q)
	return one(t, reps, err)
}

func TestAttSync_DivisionIsolation(t *testing.T) {
	const ym = 203809
	f := &fakeAtt{rekap: []etl.RekapRow{rrow(ym, "G1", "SOLO", "20"), rrow(ym, "G2", "SOLO", "21")}}
	run1(t, f, attQ(ym, "SOLO"))
	f.rekap = []etl.RekapRow{rrow(ym, "G1", "JAKARTA", "10"), rrow(ym, "G3", "JAKARTA", "11")}
	run1(t, f, attQ(ym, "JAKARTA"))
	// SOLO loses G2 and gets a correction; JAKARTA must stay as it was.
	f.rekap = []etl.RekapRow{rrow(ym, "G1", "SOLO", "22")}
	r := run1(t, f, attQ(ym, "SOLO"))
	if r.Deleted != 1 || r.Upserted != 1 {
		t.Fatalf("solo: %+v", r)
	}
	if stgCount(t, ym, "JAKARTA") != "2" || stgCount(t, ym, "SOLO") != "1" {
		t.Fatalf("solo=%s jakarta=%s", stgCount(t, ym, "SOLO"), stgCount(t, ym, "JAKARTA"))
	}
	if got := scalar(t, fmt.Sprintf(`SELECT psar_day_work::text FROM payroll_stg.pyr_stg_att_rekap_hm WHERE psar_yymm=%d AND psar_nik='G1' AND psar_division='JAKARTA'`, ym)); got != "10.00" {
		t.Fatalf("JAKARTA G1 = %s", got)
	}
}

func TestAttSync_DailyWindowJanuary(t *testing.T) {
	s, e := etl.DailyWindow(203801)
	if s.Format("2006-01-02") != "2037-12-26" || e.Format("2006-01-02") != "2038-01-25" {
		t.Fatalf("window = %s..%s", s, e)
	}
}

func TestAttSync_DraftApprovedUnlockedPass(t *testing.T) {
	const draft, approved = 203810, 203811
	for ym, st := range map[int]string{draft: "DRAFT", approved: "APPROVED"} {
		if _, err := ownDB.Exec(fmt.Sprintf(`INSERT INTO payroll.pyr_period (pp_company_id, pp_yymm, pp_division, pp_date_from, pp_date_to, pp_status)
			VALUES (1,%d,'SOLO','2038-01-26','2038-02-25','%s')`, ym, st)); err != nil {
			t.Fatal(err)
		}
		f := &fakeAtt{rekap: []etl.RekapRow{rrow(ym, "H1", "SOLO", "20")}}
		if r := run1(t, f, attQ(ym, "SOLO")); r.Status != "OK" || r.Upserted != 1 {
			t.Fatalf("%s: %+v", st, r)
		}
	}
}

func TestAttSync_BlockedDivisionDoesNotStopOther(t *testing.T) {
	const ym = 203812
	if _, err := ownDB.Exec(fmt.Sprintf(`INSERT INTO payroll.pyr_period (pp_company_id, pp_yymm, pp_division, pp_date_from, pp_date_to, pp_status)
		VALUES (1,%d,'SOLO','2038-11-26','2038-12-25','CLOSED')`, ym)); err != nil {
		t.Fatal(err)
	}
	f := &fakeAtt{rekap: []etl.RekapRow{rrow(ym, "K1", "SOLO", "20"), rrow(ym, "K1", "JAKARTA", "20")}}
	reps, err := mustRun(context.Background(), f, attQ(ym, ""))
	if err == nil || !strings.Contains(err.Error(), "CLOSED") {
		t.Fatalf("want CLOSED error, got %v", err)
	}
	if len(reps) != 2 || reps[0].Division != "SOLO" || reps[0].Status == "OK" || reps[1].Division != "JAKARTA" || reps[1].Status != "OK" {
		t.Fatalf("reports: %+v", reps)
	}
	if stgCount(t, ym, "JAKARTA") != "1" || stgCount(t, ym, "SOLO") != "0" {
		t.Fatal("JAKARTA must be written, SOLO not")
	}
}
