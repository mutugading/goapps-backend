package etl

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
)

// DummyMarker labels every synthetic row of the D-64 rehearsal (created_by / updated_by). It differs from the '{}'
// LEGACY marker of pas_source_batch_ids so uix_pas_legacy and the loaders never mistake a dummy for history.
const DummyMarker = "DUMMY-D64"

// dummyBatchIDs marks the dummy snapshot source batches: non-empty (not LEGACY) and impossible for a real batch.
const dummyBatchIDs = "{-64}"

// tetapCodes are the TETAP components of the basis (06§1): T001, T002, T003, T004, T007, T009, T055.
var tetapCodes = []string{"T001", "T002", "T003", "T004", "T007", "T009", "T055"}

// DummyPeriodQuery selects the clone of `cmd/sync dummy-period` (D-64 (1)-(3)). Division is required (SOLO or JAKARTA).
type DummyPeriodQuery struct {
	Company       int16
	From, To      int // source (real) period and synthetic target period, YYYYMM
	Division      string
	WorkdaysDelta int // added to pyr_attd_days of the source
}

// DummyReport is the outcome of one dummy operation. Counts only (I25).
type DummyReport struct {
	Division   string
	YYMM       int
	SnapshotID int64
	Rows       int // snapshot rows copied (clone) / deleted (purge)
	Additions  int
	AttdDays   int  // pyr_attd_days value of the target (clone; 0 = SOLO source without a row, D-45) or rows deleted (purge)
	Unchanged  bool // dummy period already cloned: snapshot kept, attd_days and additions topped up
}

func validDummyYYMM(y int) error {
	if y < 200001 || y > 299912 || y%100 < 1 || y%100 > 12 {
		return fmt.Errorf("yymm %d tidak valid (YYYYMM)", y)
	}
	return nil
}

func validDummyDivision(d string) error {
	if d != DivSolo && d != DivJakarta {
		return fmt.Errorf("-division wajib %s atau %s, bukan %q", DivSolo, DivJakarta, d)
	}
	return nil
}

// affected returns the row count of a statement (the pgx driver always knows it).
func affected(res sql.Result) int {
	n, err := res.RowsAffected()
	if err != nil {
		log.Warn().Err(err).Msg("etl dummy: rows affected unknown")
	}
	return int(n)
}

func withTx(ctx context.Context, db *sql.DB, f func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			log.Warn().Err(rbErr).Msg("etl dummy: rollback failed")
		}
	}()
	if err := f(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// lockDummyPeriod (plain read of the status, no FOR SHARE: the p_etl update policy would hide non-OPEN periods from a locking read) takes the same advisory lock as att sync/snapshot and refuses an APPROVED/CLOSED period or a
// snapshot that is locked or not dummy (a real period is never overwritten, D-64 (1)). It returns the id of the
// existing dummy snapshot (0 = none).
func lockDummyPeriod(ctx context.Context, tx *sql.Tx, co int16, yymm int, div string) (int64, error) {
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		fmt.Sprintf("att-sync:%d:%d:%s", co, yymm, div)); err != nil {
		return 0, fmt.Errorf("advisory lock: %w", err)
	}
	var status sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT pp_status::text FROM payroll.pyr_period WHERE pp_company_id=$1 AND pp_yymm=$2 AND pp_division=$3`,
		co, yymm, div).Scan(&status); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("cek periode: %w", err)
	}
	if status.String == "APPROVED" || status.String == "CLOSED" {
		return 0, fmt.Errorf("periode %d %s %s: dummy ditolak", yymm, div, status.String)
	}
	var nonDummy, locked int
	var dummyID sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FILTER (WHERE created_by IS DISTINCT FROM $4), count(*) FILTER (WHERE pas_locked),
  max(pas_snapshot_id) FILTER (WHERE created_by = $4) FROM payroll.pyr_att_snapshot WHERE pas_company_id=$1 AND pas_yymm=$2 AND pas_division=$3`,
		co, yymm, div, DummyMarker).Scan(&nonDummy, &locked, &dummyID); err != nil {
		return 0, fmt.Errorf("cek snapshot: %w", err)
	}
	if nonDummy > 0 {
		return 0, fmt.Errorf("periode %d %s punya %d snapshot non-dummy (periode riil): dummy ditolak", yymm, div, nonDummy)
	}
	if locked > 0 {
		return 0, fmt.Errorf("periode %d %s punya snapshot terkunci: dummy ditolak", yymm, div)
	}
	return dummyID.Int64, nil
}

// RunDummyPeriod clones the newest non-dummy snapshot, pyr_attd_days (+delta) and pyr_addition (without THR T75/P75/P7) of
// q.From into q.To as dummy rows (D-64 (2)(3)). It refuses a target with a non-dummy snapshot or addition, an APPROVED/CLOSED
// target and a non-dummy pyr_attd_days row. Idempotent: an existing dummy snapshot is kept and attd_days / additions are
// brought in line. db must connect as payroll_etl.
func RunDummyPeriod(ctx context.Context, db *sql.DB, q DummyPeriodQuery) (DummyReport, error) {
	rep := DummyReport{Division: q.Division, YYMM: q.To}
	if q.Company <= 0 {
		return rep, errors.New("company wajib")
	}
	if err := validDummyYYMM(q.From); err != nil {
		return rep, err
	}
	if err := validDummyYYMM(q.To); err != nil {
		return rep, err
	}
	if q.From == q.To {
		return rep, errors.New("-from dan -to tidak boleh sama")
	}
	if err := validDummyDivision(q.Division); err != nil {
		return rep, err
	}
	err := withTx(ctx, db, func(tx *sql.Tx) error { return cloneDummy(ctx, tx, q, &rep) })
	if err == nil {
		log.Info().Str("division", q.Division).Int("from", q.From).Int("to", q.To).Int64("snapshot", rep.SnapshotID).Int("rows", rep.Rows).
			Int("additions", rep.Additions).Int("attd_days", rep.AttdDays).Bool("unchanged", rep.Unchanged).Msg("dummy period")
	}
	return rep, err
}

func cloneDummy(ctx context.Context, tx *sql.Tx, q DummyPeriodQuery, rep *DummyReport) error {
	dummyID, err := lockDummyPeriod(ctx, tx, q.Company, q.To, q.Division)
	if err != nil {
		return err
	}
	srcID, srcSum, srcDays, err := dummySource(ctx, tx, q)
	if err != nil {
		return err
	}
	if srcDays > 0 {
		rep.AttdDays = srcDays + q.WorkdaysDelta
		if rep.AttdDays < 1 {
			return fmt.Errorf("pad_days target %d < 1 (sumber %d, delta %d)", rep.AttdDays, srcDays, q.WorkdaysDelta)
		}
	}
	// pyr_attd_days has no marker: a row of the target is only ours when the dummy snapshot exists.
	var existing int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM payroll.pyr_attd_days WHERE pad_company_id=$1 AND pad_yymm=$2 AND pad_division=$3`,
		q.Company, q.To, q.Division).Scan(&existing); err != nil {
		return fmt.Errorf("cek attd_days: %w", err)
	}
	if existing > 0 && dummyID == 0 {
		return fmt.Errorf("periode %d %s sudah punya pyr_attd_days non-dummy: dummy ditolak", q.To, q.Division)
	}
	var foreign int
	// Scheduled LEGACY additions (e.g. installments migrated ahead) stay as they are: the clone skips their keys (ON CONFLICT DO NOTHING).
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM payroll.pyr_addition WHERE pa_company_id=$1 AND pa_yymm=$2 AND pa_division=$3 AND created_by IS DISTINCT FROM $4
  AND pa_source IS DISTINCT FROM 'LEGACY'`,
		q.Company, q.To, q.Division, DummyMarker).Scan(&foreign); err != nil {
		return fmt.Errorf("cek addition: %w", err)
	}
	if foreign > 0 {
		return fmt.Errorf("periode %d %s punya %d pyr_addition non-dummy di luar LEGACY: dummy ditolak", q.To, q.Division, foreign)
	}
	if rep.AttdDays > 0 {
		if _, err := tx.ExecContext(ctx, `INSERT INTO payroll.pyr_attd_days (pad_company_id, pad_yymm, pad_division, pad_days) VALUES ($1,$2,$3,$4)
ON CONFLICT (pad_company_id, pad_yymm, pad_division) DO UPDATE SET pad_days = EXCLUDED.pad_days`, q.Company, q.To, q.Division, rep.AttdDays); err != nil {
			return fmt.Errorf("attd_days: %w", err)
		}
	}
	if dummyID != 0 {
		rep.Unchanged, rep.SnapshotID = true, dummyID
	} else if err := insertDummySnapshot(ctx, tx, q, srcID, srcSum, rep); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO payroll.pyr_addition (pa_company_id, pa_yymm, pa_nik, pa_division, pa_type, pa_id, pa_nama, pa_compnt_name,
  pa_compnt_sts, pa_compnt_tax, pa_amount, pa_remarks, pa_source, pa_loan_id, pa_generated, created_by)
SELECT pa_company_id, $4::int, pa_nik, pa_division, pa_type, pa_id, pa_nama, pa_compnt_name, pa_compnt_sts, pa_compnt_tax, pa_amount, pa_remarks,
  pa_source, pa_loan_id, pa_generated, $5::text
FROM payroll.pyr_addition WHERE pa_company_id=$1 AND pa_yymm=$2 AND pa_division=$3 AND created_by IS DISTINCT FROM $5::text
  AND NOT ((pa_type='Tunjangan' AND pa_id=75) OR (pa_type='Potongan' AND pa_id IN (75, 7)))
ON CONFLICT DO NOTHING`, q.Company, q.From, q.Division, q.To, DummyMarker)
	if err != nil {
		return fmt.Errorf("addition: %w", err)
	}
	rep.Additions = affected(res)
	return dummyAudit(ctx, tx, q.Company, "ETL_DUMMY_PERIOD", q.To, q.Division, map[string]any{"from": q.From, "rows": rep.Rows, "additions": rep.Additions,
		"attd_days": rep.AttdDays, "snapshot": rep.SnapshotID, "unchanged": rep.Unchanged})
}

// dummySource reads the newest non-dummy snapshot (id, checksum) and pyr_attd_days of the source period (0 = SOLO without a row).
func dummySource(ctx context.Context, tx *sql.Tx, q DummyPeriodQuery) (srcID int64, srcSum string, srcDays int, err error) {
	if err := tx.QueryRowContext(ctx, `SELECT pas_snapshot_id, pas_checksum FROM payroll.pyr_att_snapshot
WHERE pas_company_id=$1 AND pas_yymm=$2 AND pas_division=$3 AND created_by IS DISTINCT FROM $4 ORDER BY pas_snapshot_id DESC LIMIT 1`,
		q.Company, q.From, q.Division, DummyMarker).Scan(&srcID, &srcSum); errors.Is(err, sql.ErrNoRows) {
		return 0, "", 0, fmt.Errorf("periode sumber %d %s tidak punya snapshot", q.From, q.Division)
	} else if err != nil {
		return 0, "", 0, fmt.Errorf("snapshot sumber: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `SELECT pad_days FROM payroll.pyr_attd_days WHERE pad_company_id=$1 AND pad_yymm=$2 AND pad_division=$3`,
		q.Company, q.From, q.Division).Scan(&srcDays); errors.Is(err, sql.ErrNoRows) {
		if q.Division == DivSolo {
			return srcID, srcSum, 0, nil // SOLO never had ATTD_DAYS rows: the target stays without one (workdays NULL, D-45)
		}
		return 0, "", 0, fmt.Errorf("periode sumber %d %s tidak punya pyr_attd_days", q.From, q.Division)
	} else if err != nil {
		return 0, "", 0, fmt.Errorf("attd_days sumber: %w", err)
	}
	return srcID, srcSum, srcDays, nil
}

func insertDummySnapshot(ctx context.Context, tx *sql.Tx, q DummyPeriodQuery, srcID int64, srcSum string, rep *DummyReport) error {
	if err := tx.QueryRowContext(ctx, `INSERT INTO payroll.pyr_att_snapshot (pas_company_id, pas_yymm, pas_division, pas_source_batch_ids, pas_checksum, pas_locked, created_by)
VALUES ($1,$2,$3,$4::bigint[],$5,false,$6) RETURNING pas_snapshot_id`, q.Company, q.To, q.Division, dummyBatchIDs, "dummy-d64:"+srcSum, DummyMarker).Scan(&rep.SnapshotID); err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO payroll.pyr_att_snapshot_row (pasr_snapshot_id, pasr_kind, pasr_nik, pasr_data, pasr_premi_fail, pasr_beras_fail)
SELECT $1, pasr_kind, pasr_nik, pasr_data, pasr_premi_fail, pasr_beras_fail FROM payroll.pyr_att_snapshot_row WHERE pasr_snapshot_id = $2`, rep.SnapshotID, srcID)
	if err != nil {
		return fmt.Errorf("snapshot rows: %w", err)
	}
	rep.Rows = affected(res)
	return nil
}

func dummyAudit(ctx context.Context, tx *sql.Tx, co int16, action string, yymm int, div string, after map[string]any) error {
	b, err := json.Marshal(after)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO payroll.pyr_audit_log (pal_actor, pal_action, pal_entity, pal_entity_key, pal_division, pal_company_id, pal_after)
VALUES ('etl', $1, 'dummy_d64', $2, $3, $4, $5::jsonb)`, action, fmt.Sprintf("%d/%s", yymm, div), div, co, string(b)); err != nil {
		return fmt.Errorf("audit: %w", err)
	}
	return nil
}

// RunDummyPurge deletes the dummy rows of one period: dummy snapshots (rows by cascade), additions marked DUMMY-D64 and
// pyr_attd_days (only when a dummy snapshot exists). It needs the table-owner DSN (payroll_etl has no DELETE; D-64 (3)
// amended, user decision batch T029a). It refuses a non-dummy snapshot, an APPROVED/CLOSED period and a period whose dummy
// snapshot a run references; a period without dummy rows is a no-op.
func RunDummyPurge(ctx context.Context, db *sql.DB, company int16, yymm int, division string) (DummyReport, error) {
	rep := DummyReport{Division: division, YYMM: yymm}
	if company <= 0 {
		return rep, errors.New("company wajib")
	}
	if err := validDummyYYMM(yymm); err != nil {
		return rep, err
	}
	if err := validDummyDivision(division); err != nil {
		return rep, err
	}
	err := withTx(ctx, db, func(tx *sql.Tx) error {
		id, err := lockDummyPeriod(ctx, tx, company, yymm, division)
		if err != nil || id == 0 {
			return err // no dummy snapshot: nothing of ours to delete (additions/attd_days are only ours with a dummy snapshot)
		}
		var runs int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM payroll.pyr_run r JOIN payroll.pyr_att_snapshot s ON s.pas_snapshot_id = r.pr_snapshot_id
WHERE s.pas_company_id=$1 AND s.pas_yymm=$2 AND s.pas_division=$3`, company, yymm, division).Scan(&runs); err != nil {
			return fmt.Errorf("cek run: %w", err)
		}
		if runs > 0 {
			return fmt.Errorf("periode %d %s: %d run memakai snapshot dummy; hapus run dulu", yymm, division, runs)
		}
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM payroll.pyr_att_snapshot_row WHERE pasr_snapshot_id = $1`, id).Scan(&rep.Rows); err != nil {
			return err
		}
		for _, d := range []struct {
			q string
			n *int
		}{
			{`DELETE FROM payroll.pyr_addition WHERE pa_company_id=$1 AND pa_yymm=$2 AND pa_division=$3 AND created_by = $4`, &rep.Additions},
			{`DELETE FROM payroll.pyr_attd_days WHERE pad_company_id=$1 AND pad_yymm=$2 AND pad_division=$3 AND $4::text IS NOT NULL`, &rep.AttdDays},
			{`DELETE FROM payroll.pyr_att_snapshot WHERE pas_company_id=$1 AND pas_yymm=$2 AND pas_division=$3 AND created_by = $4`, nil},
		} {
			res, err := tx.ExecContext(ctx, d.q, company, yymm, division, DummyMarker)
			if err != nil {
				return fmt.Errorf("purge: %w", err)
			}
			if d.n != nil {
				*d.n = affected(res)
			}
		}
		rep.SnapshotID = id
		return dummyAudit(ctx, tx, company, "ETL_DUMMY_PURGE", yymm, division, map[string]any{"rows": rep.Rows, "additions": rep.Additions, "attd_days": rep.AttdDays, "snapshot": id})
	})
	return rep, err
}

// JanBaseReport is the outcome of RunDummyJanBase.
type JanBaseReport struct {
	YYMM, Source int
	Written      int // T901 rows inserted or updated
}

func prevYYMM(y int) int {
	if y%100 == 1 {
		return y - 100 + 11
	}
	return y - 1
}

// RunDummyJanBase seeds T901 (JAN_BASE) per NIK = Σ TETAP (T001, T002, T003, T004, T007, T009, T055) valid in the previous
// month, valid only in yymm (D-64 (4)). It refuses when a non-dummy T901 exists in the company; dummy rows are updated
// (idempotent). db must connect as payroll_etl.
func RunDummyJanBase(ctx context.Context, db *sql.DB, company int16, yymm int) (JanBaseReport, error) {
	rep := JanBaseReport{YYMM: yymm, Source: prevYYMM(yymm)}
	if company <= 0 {
		return rep, errors.New("company wajib")
	}
	if err := validDummyYYMM(yymm); err != nil {
		return rep, err
	}
	err := withTx(ctx, db, func(tx *sql.Tx) error {
		var sysid int64
		if err := tx.QueryRowContext(ctx, `SELECT pc_sysid FROM payroll.pyr_component WHERE pc_company_id=$1 AND pc_code='T901' AND pc_type='Tunjangan'`,
			company).Scan(&sysid); errors.Is(err, sql.ErrNoRows) {
			return errors.New("komponen T901 belum ada di pyr_component")
		} else if err != nil {
			return fmt.Errorf("komponen T901: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, fmt.Sprintf("dummy-janbase:%d:%d", company, yymm)); err != nil {
			return fmt.Errorf("advisory lock: %w", err)
		}
		var foreign int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM payroll.pyr_employee_component WHERE pec_company_id=$1 AND pec_component_sysid=$2 AND updated_by IS DISTINCT FROM $3`,
			company, sysid, DummyMarker).Scan(&foreign); err != nil {
			return fmt.Errorf("cek T901: %w", err)
		}
		if foreign > 0 {
			return fmt.Errorf("ada %d baris T901 non-dummy di company %d: dummy-janbase ditolak", foreign, company)
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO payroll.pyr_employee_component (pec_company_id, pec_nik, pec_division, pec_component_sysid, pec_type, pec_id, pec_name,
  pec_sts, pec_tax, pec_amount, pec_frz_flag, pec_tax_paid_company, pec_source, pec_valid_from, pec_valid_to, updated_by, updated_at)
SELECT e.pec_company_id, e.pec_nik, e.pec_division, $2, 'Tunjangan', 901, 'JAN_BASE (dummy D-64)', 'F', 0, sum(coalesce(e.pec_amount, 0)), 2, 'N', 'MANUAL', $3, $3, $5, now()
FROM payroll.pyr_employee_component e
JOIN payroll.pyr_component c ON c.pc_sysid = e.pec_component_sysid
JOIN payroll.pyr_employee p ON p.pe_company_id = e.pec_company_id AND p.pe_nik = e.pec_nik AND p.pe_division = e.pec_division
WHERE e.pec_company_id = $1 AND c.pc_type = 'Tunjangan' AND c.pc_code = ANY($4::text[]) AND e.pec_frz_flag = 2 AND p.pe_sts_payroll = 2
  AND (e.pec_valid_from IS NULL OR e.pec_valid_from <= $6) AND (e.pec_valid_to IS NULL OR e.pec_valid_to >= $6)
GROUP BY e.pec_company_id, e.pec_nik, e.pec_division
HAVING sum(coalesce(e.pec_amount, 0)) > 0
ON CONFLICT (pec_company_id, pec_nik, pec_division, pec_component_sysid) DO UPDATE SET pec_amount = EXCLUDED.pec_amount, pec_valid_from = EXCLUDED.pec_valid_from,
  pec_valid_to = EXCLUDED.pec_valid_to, pec_frz_flag = 2, updated_by = EXCLUDED.updated_by, updated_at = now()`,
			company, sysid, yymm, "{"+strings.Join(tetapCodes, ",")+"}", DummyMarker, rep.Source)
		if err != nil {
			return fmt.Errorf("T901: %w", err)
		}
		rep.Written = affected(res)
		_, err = tx.ExecContext(ctx, `INSERT INTO payroll.pyr_audit_log (pal_actor, pal_action, pal_entity, pal_entity_key, pal_company_id, pal_after)
VALUES ('etl','ETL_DUMMY_JANBASE','dummy_d64',$1,$2,jsonb_build_object('source',$3::int,'written',$4::int))`, fmt.Sprint(yymm), company, rep.Source, rep.Written)
		return err
	})
	return rep, err
}
