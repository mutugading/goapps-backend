package etl

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/rs/zerolog/log"
)

// AttSnapshotQuery selects the period of `cmd/sync snapshot`. Division "" = SOLO and JAKARTA.
type AttSnapshotQuery struct {
	Company  int16
	YYMM     int
	Division string
}

// AttSnapshotReport is the outcome of one division. Counts only, no names (I25).
type AttSnapshotReport struct {
	Division       string
	YYMM           int
	SnapshotID     int64
	BatchIDs       []int64 // pas_source_batch_ids: last OK batch per source
	Rekap, Add     int
	Sch            int
	PremiFailNIKs  int // REKAP rows with premi_fail > 0
	BerasFailNIKs  int // REKAP rows with beras_fail > 0
	Checksum       string
	PeriodDayCount int // days of [26th previous month .. 25th]: premi_fail of a NIK without daily rows (05§7)
	// Unchanged: staging and source batches equal the newest snapshot of the division, so none was written and
	// SnapshotID is that existing snapshot (no-op, not an error).
	Unchanged bool
}

// Sources whose last OK batch is mandatory: REKAP is the recap itself and DAILY feeds PREMI/BERAS_FAIL (05§7), so a
// snapshot without either would silently give every NIK premi_fail = days of the period. ADD and SCH only exist in
// phase 0–2 (05§2) and are taken when synced.
var (
	snapRequiredSources = []string{tblRekap.source, tblDaily.source}
	snapSources         = []string{tblRekap.source, tblAdd.source, tblSch.source, tblDaily.source}
)

// RunAttSnapshot freezes the current staging of each division into a new pyr_att_snapshot (05§6): one transaction
// per division, refused when the period is CLOSED or has a locked snapshot (same advisory lock + FOR SHARE as
// RunAttSync, so a sync of the same period waits or runs first). The snapshot holds the whole staging of
// (company, yymm, division) at that moment, not only the rows written by the batches in pas_source_batch_ids:
// an upsert-if-changed keeps the old batch id on unchanged rows. Rows use the LEGACY jsonb format (MPAR_*,
// MPARA_*, SCHEDULES), so the run loader reads both alike; PREMI/BERAS_FAIL are computed set-based (05§7).
// Existing snapshots are never updated. db must connect as payroll_etl (D-58 (4)).
func RunAttSnapshot(ctx context.Context, db *sql.DB, q AttSnapshotQuery) ([]AttSnapshotReport, error) {
	_, divs, err := AttSyncQuery{Company: q.Company, YYMM: q.YYMM, Division: q.Division, Source: AttSrcAll}.plan()
	if err != nil {
		return nil, err
	}
	reps := make([]AttSnapshotReport, 0, len(divs))
	var errs []error
	for _, div := range divs { // one blocked division does not stop the other
		r, err := snapshotDivision(ctx, db, q, div)
		if err != nil {
			errs = append(errs, fmt.Errorf("snapshot %s/%d: %w", div, q.YYMM, err))
			continue
		}
		reps = append(reps, r)
		log.Info().Str("division", div).Int("yymm", q.YYMM).Int64("snapshot", r.SnapshotID).Bool("unchanged", r.Unchanged).Int("rekap", r.Rekap).
			Int("add", r.Add).Int("sch", r.Sch).Int("premi_fail_niks", r.PremiFailNIKs).Int("beras_fail_niks", r.BerasFailNIKs).
			Str("checksum", r.Checksum).Msg("att snapshot")
	}
	return reps, errors.Join(errs...)
}

func snapshotDivision(ctx context.Context, db *sql.DB, q AttSnapshotQuery, div string) (rep AttSnapshotReport, err error) {
	rep = AttSnapshotReport{Division: div, YYMM: q.YYMM}
	conn, err := db.Conn(ctx)
	if err != nil {
		return rep, err
	}
	defer closeQuiet(conn)
	err = conn.Raw(func(dc any) error {
		sc, ok := dc.(*stdlib.Conn)
		if !ok {
			return errors.New("driver bukan pgx")
		}
		tx, err := sc.Conn().Begin(ctx)
		if err != nil {
			return err
		}
		defer func() {
			if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
				log.Warn().Err(rbErr).Msg("att snapshot: rollback failed")
			}
		}()
		if err := lockAndCheckPeriod(ctx, tx, AttSyncQuery{Company: q.Company, YYMM: q.YYMM}, div); err != nil {
			return err
		}
		if err := writeSnapshot(ctx, tx, q, div, &rep); err != nil {
			return err
		}
		return tx.Commit(ctx)
	})
	return rep, err
}

func writeSnapshot(ctx context.Context, tx pgx.Tx, q AttSnapshotQuery, div string, rep *AttSnapshotReport) error {
	start, end := DailyWindow(q.YYMM)
	rep.PeriodDayCount = int(end.Sub(start).Hours()/24) + 1
	var err error
	if rep.BatchIDs, err = lastOKBatches(ctx, tx, q, div); err != nil {
		return err
	}
	if err := checkStagingFilled(ctx, tx, q, div); err != nil {
		return err
	}
	if err := buildSnapshotRows(ctx, tx, q, div, rep); err != nil {
		return err
	}
	if rep.Unchanged, rep.SnapshotID, err = sameAsLatest(ctx, tx, q, div, rep); err != nil || rep.Unchanged {
		return err
	}
	if err := tx.QueryRow(ctx, `INSERT INTO payroll.pyr_att_snapshot (pas_company_id, pas_yymm, pas_division, pas_source_batch_ids, pas_checksum, pas_locked, created_by)
VALUES ($1,$2,$3,$4,$5,false,'etl') RETURNING pas_snapshot_id`, q.Company, q.YYMM, div, rep.BatchIDs, rep.Checksum).Scan(&rep.SnapshotID); err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO payroll.pyr_att_snapshot_row (pasr_snapshot_id, pasr_kind, pasr_nik, pasr_data, pasr_premi_fail, pasr_beras_fail)
SELECT $1, kind, nik, data, premi_fail, beras_fail FROM `+tmpSnap, rep.SnapshotID); err != nil {
		return fmt.Errorf("snapshot rows: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO payroll.pyr_audit_log (pal_actor, pal_action, pal_entity, pal_entity_key, pal_division, pal_after, pal_company_id)
VALUES ('etl','ETL_ATT_SNAPSHOT','pyr_att_snapshot',$1,$2,jsonb_build_object('yymm',$3::int,'source_batch_ids',$4::bigint[],'checksum',$5::text,
  'rekap',$6::int,'add',$7::int,'sch',$8::int,'premi_fail_niks',$9::int,'beras_fail_niks',$10::int),$11)`,
		fmt.Sprint(rep.SnapshotID), div, q.YYMM, rep.BatchIDs, rep.Checksum, rep.Rekap, rep.Add, rep.Sch, rep.PremiFailNIKs, rep.BerasFailNIKs,
		q.Company); err != nil {
		return fmt.Errorf("audit: %w", err)
	}
	return nil
}

const tmpSnap = "tmp_att_snapshot"

// pasr_data keys of the values payroll derives itself (05§5 option A); recon reads them (ATT_DERIVED).
const (
	DerivedShiftKey = "derived_shift"
	DerivedOtiwKey  = "derived_otiw_decr"
)

// checkStagingFilled refuses an empty recap, and for SOLO an empty daily window: a sync batch can be OK with zero rows
// (HRIS view late), and an empty daily window would give every NIK premi_fail = days of the period (05§7 (4)).
// JAKARTA is not checked: whether JAKARTA has daily attendance at all is still open (question in the T021c report).
func checkStagingFilled(ctx context.Context, tx pgx.Tx, q AttSnapshotQuery, div string) error {
	start, end := DailyWindow(q.YYMM)
	var rekap, daily int
	if err := tx.QueryRow(ctx, `SELECT
  (SELECT count(*) FROM payroll_stg.pyr_stg_att_rekap_hm WHERE psar_company_id=$1 AND psar_division=$2 AND psar_yymm=$3),
  (SELECT count(*) FROM payroll_stg.pyr_stg_att_daily WHERE psad_company_id=$1 AND psad_division=$2 AND psad_work_date BETWEEN $4 AND $5)`,
		q.Company, div, q.YYMM, start, end).Scan(&rekap, &daily); err != nil {
		return fmt.Errorf("cek staging: %w", err)
	}
	if rekap == 0 {
		return fmt.Errorf("staging rekap %d %s kosong: jalankan `sync att` dulu", q.YYMM, div)
	}
	if daily == 0 && div == DivSolo {
		return fmt.Errorf("staging harian %d %s kosong (%s..%s): premi_fail semua NIK akan = jumlah hari; sync daily dulu",
			q.YYMM, div, start.Format("2006-01-02"), end.Format("2006-01-02"))
	}
	return nil
}

// buildSnapshotRows fills the temp table with the REKAP/ADD/SCH rows and computes counts and checksum.
func buildSnapshotRows(ctx context.Context, tx pgx.Tx, q AttSnapshotQuery, div string, rep *AttSnapshotReport) error {
	start, end := DailyWindow(q.YYMM)
	if _, err := tx.Exec(ctx, "CREATE TEMP TABLE "+tmpSnap+" (kind text, nik text, data jsonb, premi_fail int, beras_fail int) ON COMMIT DROP"); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "INSERT INTO "+tmpSnap+" "+sqlSnapRekap, q.Company, div, start, end, q.YYMM); err != nil {
		return fmt.Errorf("rekap + premi/beras fail: %w", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO "+tmpSnap+" "+sqlSnapAdd, q.Company, div, q.YYMM); err != nil {
		return fmt.Errorf("rekap add: %w", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO "+tmpSnap+" "+sqlSnapSch, q.Company, div, q.YYMM); err != nil {
		return fmt.Errorf("sch rekap: %w", err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FILTER (WHERE kind='REKAP'), count(*) FILTER (WHERE kind='ADD'), count(*) FILTER (WHERE kind='SCH'),
  count(*) FILTER (WHERE kind='REKAP' AND premi_fail > 0), count(*) FILTER (WHERE kind='REKAP' AND beras_fail > 0),
  encode(sha256(convert_to(coalesce(string_agg(kind||'|'||nik||'|'||data::text||'|'||coalesce(premi_fail::text,'')||'|'||coalesce(beras_fail::text,''),
    E'\n' ORDER BY kind, nik), ''), 'UTF8')), 'hex')
FROM `+tmpSnap).Scan(&rep.Rekap, &rep.Add, &rep.Sch, &rep.PremiFailNIKs, &rep.BerasFailNIKs, &rep.Checksum); err != nil {
		return fmt.Errorf("checksum: %w", err)
	}
	return nil
}

// sameAsLatest reports whether the newest snapshot of the division has the same checksum and source batches.
func sameAsLatest(ctx context.Context, tx pgx.Tx, q AttSnapshotQuery, div string, rep *AttSnapshotReport) (bool, int64, error) {
	var id int64
	var sum string
	var batches []int64
	err := tx.QueryRow(ctx, `SELECT pas_snapshot_id, pas_checksum, pas_source_batch_ids FROM payroll.pyr_att_snapshot
WHERE pas_company_id=$1 AND pas_yymm=$2 AND pas_division=$3 ORDER BY pas_snapshot_id DESC LIMIT 1`, q.Company, q.YYMM, div).Scan(&id, &sum, &batches)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, fmt.Errorf("snapshot terakhir: %w", err)
	}
	if sum != rep.Checksum || len(batches) != len(rep.BatchIDs) {
		return false, 0, nil
	}
	for i := range batches {
		if batches[i] != rep.BatchIDs[i] {
			return false, 0, nil
		}
	}
	return true, id, nil
}

// lastOKBatches returns the newest OK batch id per synced source (05§6 source_batch_ids), REKAP and DAILY required.
func lastOKBatches(ctx context.Context, tx pgx.Tx, q AttSnapshotQuery, div string) ([]int64, error) {
	// ok = newest OK batch; bad = newest non-OK (FAILED / RUNNING) batch after it, which makes staging suspect.
	rows, err := tx.Query(ctx, `SELECT s.psb_source, s.ok, (SELECT b.psb_status||' #'||b.psb_id FROM payroll_stg.pyr_stg_sync_batch b
  WHERE b.psb_company_id=$1 AND b.psb_yymm=$2 AND b.psb_division=$3 AND b.psb_source=s.psb_source AND b.psb_id > s.ok AND b.psb_status <> 'OK'
  ORDER BY b.psb_id DESC LIMIT 1)
FROM (SELECT psb_source, coalesce(max(psb_id) FILTER (WHERE psb_status='OK'), 0) AS ok FROM payroll_stg.pyr_stg_sync_batch
  WHERE psb_company_id=$1 AND psb_yymm=$2 AND psb_division=$3 AND psb_source = ANY($4::text[]) GROUP BY psb_source) s`,
		q.Company, q.YYMM, div, snapSources)
	if err != nil {
		return nil, fmt.Errorf("batch sync: %w", err)
	}
	got := map[string]int64{}
	var bad []string
	for rows.Next() {
		var src string
		var id int64
		var newer *string
		if err := rows.Scan(&src, &id, &newer); err != nil {
			rows.Close()
			return nil, err
		}
		got[src] = id
		if newer != nil {
			bad = append(bad, src+" "+*newer)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return nil, fmt.Errorf("ada batch sync tidak OK sesudah batch OK terakhir (%s) untuk %d %s: ulangi `sync att` sampai OK",
			strings.Join(bad, ", "), q.YYMM, div)
	}
	for _, s := range snapRequiredSources {
		if got[s] == 0 {
			return nil, fmt.Errorf("belum ada batch sync OK %s untuk %d %s: jalankan `sync att` dulu", s, q.YYMM, div)
		}
	}
	ids := make([]int64, 0, len(got))
	for _, s := range snapSources { // fixed order: rekap, add, sch, daily
		if id := got[s]; id != 0 {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// recapJSON renders a staging recap row as the LEGACY pasr_data object: <prefix><COL> keys, numbers without trailing
// zeros (staging numeric(6,2) 20.00 → 20, like the Oracle spool), NULL stays JSON null.
func recapJSON(prefix string, cols []string) string {
	parts := []string{"'" + prefix + "NAMA', psar_nama"}
	for _, c := range cols {
		parts = append(parts, "'"+prefix+strings.ToUpper(c)+"', trim_scale(psar_"+c+")")
	}
	return "jsonb_build_object(" + strings.Join(parts, ", ") + ")"
}

// sqlSnapRekap: REKAP rows + PREMI/BERAS_FAIL per 05§7 ($1 company, $2 division, $3..$4 work-date window, $5 yymm).
//   - premi (all divisions) = (1) batal & hours = 0 + (2) batal & abnormal & hours <> 0 + (3) manual abnormal rows
//   - (4) greatest(days − scheduled days, 0). NULL hours count in neither (1) nor (2): no coalesce. A NIK of the
//     recap without daily rows gets (4) = days of the period, as legacy COUNT(*) = 0.
//   - beras SOLO = the same (1)+(2) limited to pbp_beras = 'BERAS'; JAKARTA = trunc(alpa + ijin) of the recap when
//     the NIK has T1 (any frz flag), else 0 (legacy C_BERAS_FAIL). The JAKARTA value is stored for audit only: the
//     run loader recomputes it from the recap + T1 and ignores pasr_beras_fail for JAKARTA
//     (internal/application/run/loader.go dailyFails, 05§7 "Beras JAKARTA").
//   - An empty schedule code counts as NULL (Oracle treats the empty string as NULL).
//   - ATT_DERIVED (05§5 option A, T021e): derived_shift / derived_otiw_decr are the VIEW_ATT_REKAP_HM formulas of
//     MPAR_SHIFT / MPAR_OTIW_DECR with the eligibility read from pyr_employee_component (T34/T35, T6; Tunjangan,
//     frz_flag = 2, inside the validity window) instead of MGT_PAYROLL_COMPNT_EMP. The day categories of otiw_decr
//     come from the recap (day_work, dayoff, cuti, dirumahkan, alpa, ijin, NULL = 0). They are extra pasr_data keys
//     without the MPAR_ prefix: the run loader ignores them (engine v1 keeps MPAR_*), recon compares them. A NIK
//     without daily rows in the window gets no derived keys (not checked): the view derives MPAR from the same daily
//     rows, so an empty staging (JAKARTA, late sync) would only give false ATT_DERIVED (D-60).
var sqlSnapRekap = `
WITH d AS (
  SELECT a.psad_nik AS nik, a.psad_work_hours AS work_hours, nullif(a.psad_schedule_code, '') AS schedule_code,
         a.psad_sch_type AS sch_type, a.psad_ot_inwork AS ot_inwork,
         bp.pbp_code IS NOT NULL AS is_batal, coalesce(bp.pbp_abnormal, 'N') = 'Y' AS is_abn, bp.pbp_beras = 'BERAS' AS is_beras
  FROM   payroll_stg.pyr_stg_att_daily a
  LEFT   JOIN payroll.pyr_batal_premi bp ON bp.pbp_code = nullif(a.psad_schedule_code, '')
  WHERE  a.psad_company_id = $1::smallint AND a.psad_division = $2::text AND a.psad_work_date BETWEEN $3::date AND $4::date
), agg AS (
  SELECT nik,
         count(*) FILTER (WHERE is_batal AND work_hours = 0) + count(*) FILTER (WHERE is_batal AND is_abn AND work_hours <> 0) AS p12,
         count(*) FILTER (WHERE schedule_code IS NOT NULL) AS scheduled,
         count(*) FILTER (WHERE is_batal AND is_beras AND work_hours = 0)
       + count(*) FILTER (WHERE is_batal AND is_beras AND is_abn AND work_hours <> 0) AS beras,
         count(*) FILTER (WHERE sch_type = 'Shift' AND work_hours IS NOT NULL) AS shift_days,
         count(*) FILTER (WHERE ot_inwork = 'No') AS otiw_no_days
  FROM d GROUP BY nik
), ab AS (
  SELECT pap_nik AS nik, count(*) AS n FROM payroll.pyr_abnormal_premi
  WHERE pap_company_id = $1::smallint AND pap_yymm = $5::int AND pap_division = $2::text GROUP BY pap_nik
), t1 AS (
  SELECT DISTINCT e.pec_nik AS nik FROM payroll.pyr_employee_component e
  JOIN payroll.pyr_component c ON c.pc_sysid = e.pec_component_sysid
  WHERE e.pec_company_id = $1::smallint AND e.pec_division = $2::text AND c.pc_code = 'T001'
), elig AS (
  SELECT e.pec_nik AS nik, bool_or(c.pc_code IN ('T034','T035')) AS shift, bool_or(c.pc_code = 'T006') AS otiw
  FROM payroll.pyr_employee_component e
  JOIN payroll.pyr_component c ON c.pc_sysid = e.pec_component_sysid
  WHERE e.pec_company_id = $1::smallint AND e.pec_division = $2::text AND c.pc_type = 'Tunjangan'
    AND c.pc_code IN ('T006','T034','T035') AND e.pec_frz_flag = 2
    AND (e.pec_valid_from IS NULL OR e.pec_valid_from <= $5::int) AND (e.pec_valid_to IS NULL OR e.pec_valid_to >= $5::int)
  GROUP BY e.pec_nik
)
SELECT 'REKAP', r.psar_nik, ` + recapJSON("MPAR_", RekapNumCols) + ` || CASE WHEN agg.nik IS NULL THEN '{}'::jsonb ELSE jsonb_build_object(
         '` + DerivedShiftKey + `', CASE WHEN coalesce(elig.shift, false) THEN coalesce(agg.shift_days, 0) ELSE 0 END,
         '` + DerivedOtiwKey + `', trim_scale(CASE WHEN coalesce(elig.otiw, false)
              THEN coalesce(agg.otiw_no_days, 0) + ($4::date - $3::date + 1)
                 - (coalesce(r.psar_day_work, 0) + coalesce(r.psar_dayoff, 0) + coalesce(r.psar_cuti, 0)
                  + coalesce(r.psar_dirumahkan, 0) + coalesce(r.psar_alpa, 0) + coalesce(r.psar_ijin, 0))
              ELSE 0 END)) END,
       (coalesce(agg.p12, 0) + coalesce(ab.n, 0) + greatest(($4::date - $3::date + 1) - coalesce(agg.scheduled, 0), 0))::int,
       CASE WHEN $2::text = 'JAKARTA'
            THEN CASE WHEN t1.nik IS NOT NULL THEN trunc(coalesce(r.psar_alpa, 0) + coalesce(r.psar_ijin, 0))::int ELSE 0 END
            ELSE coalesce(agg.beras, 0)::int END
FROM   payroll_stg.pyr_stg_att_rekap_hm r
LEFT   JOIN agg ON agg.nik = r.psar_nik
LEFT   JOIN ab  ON ab.nik = r.psar_nik
LEFT   JOIN t1  ON t1.nik = r.psar_nik
LEFT   JOIN elig ON elig.nik = r.psar_nik
WHERE  r.psar_company_id = $1::smallint AND r.psar_division = $2::text AND r.psar_yymm = $5::int`

// sqlSnapAdd: ADD rows (MPARA_*, only the S3 columns day_work..shift), $1 company, $2 division, $3 yymm.
var sqlSnapAdd = `SELECT 'ADD', psar_nik, ` + recapJSON("MPARA_", RekapAddNumCols) + `, NULL::int, NULL::int
FROM payroll_stg.pyr_stg_att_rekap_add WHERE psar_company_id = $1::smallint AND psar_division = $2::text AND psar_yymm = $3::int`

// sqlSnapSch: one SCH row per NIK, {"SCHEDULES": {code: no_sch}} like the LEGACY snapshot.
const sqlSnapSch = `SELECT 'SCH', pssr_nik, jsonb_build_object('SCHEDULES', jsonb_object_agg(pssr_schedule, trim_scale(pssr_no_sch))), NULL::int, NULL::int
FROM payroll_stg.pyr_stg_sch_rekap WHERE pssr_company_id = $1::smallint AND pssr_division = $2::text AND pssr_yymm = $3::int
GROUP BY pssr_nik`
