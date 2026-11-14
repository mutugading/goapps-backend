package etl

import (
	"bytes"
	"context"
	"crypto/md5" //nolint:gosec // change-detection hash (05§4), not security
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"
)

// Attendance sources of `cmd/sync att` (05§4).
const (
	AttSrcRekap = "rekap"
	AttSrcDaily = "daily"
	AttSrcAdd   = "add"
	AttSrcSch   = "sch"
	AttSrcAll   = "all"

	// AttBatchOK etc. are pyr_stg_sync_batch.psb_status values (DRY only appears in reports).
	AttBatchOK  = "OK"
	AttBatchDry = "DRY"

	attBatchFailed = "FAILED"
	maxRejectNIKs  = 20
)

// AttSyncQuery selects what one run pulls. Division "" = SOLO and JAKARTA; Source "" = all.
type AttSyncQuery struct {
	Company    int16
	YYMM       int
	Division   string
	Source     string
	InsertOnly bool // 05§4.1: rows already in staging are neither updated nor deleted (parallel-run only)
	Dry        bool // full pull and diff inside a transaction that is rolled back
	// Attempts/Backoff wrap each Oracle read (05§4: 3 tries, exponential backoff). Zero = 3 and 1s.
	Attempts int
	Backoff  time.Duration
}

// AttSyncReport is the outcome of one (source, division) batch. Only NIK and counts: no names (I25).
type AttSyncReport struct {
	Source, Division                    string
	YYMM                                int
	BatchID                             int64
	RowsIn, Rejected, Upserted, Deleted int
	RejectedNIKs                        []string
	Checksum, Status                    string
}

// attTable describes one staging table.
type attTable struct {
	name, source      string
	cols              []string // COPY/INSERT order, last = batch id
	pk                []string
	match             []string // key columns beyond (company, division, scope) used by the DELETE
	changed           string   // WHERE of DO UPDATE (t = target)
	sumExpr, sumOrder string   // batch checksum = md5(string_agg(sumExpr ORDER BY sumOrder))
	company, division string
	yymm, date        string // scope: exactly one is set
}

func rekapTable(name, source string) attTable {
	cols := make([]string, 0, 5+len(RekapNumCols)+2)
	cols = append(cols, colPsarCompany, colPsarYymm, colPsarNIK, colPsarDivision, "psar_nama")
	for _, c := range RekapNumCols {
		cols = append(cols, "psar_"+c)
	}
	cols = append(cols, "psar_row_hash", "psar_batch_id")
	return attTable{name: name, source: source, cols: cols,
		pk:    []string{colPsarCompany, colPsarYymm, colPsarNIK, colPsarDivision},
		match: []string{colPsarNIK}, changed: "t.psar_row_hash <> EXCLUDED.psar_row_hash",
		sumExpr: "psar_row_hash", sumOrder: colPsarNIK, company: colPsarCompany, division: colPsarDivision, yymm: colPsarYymm}
}

var (
	tblRekap = rekapTable("pyr_stg_att_rekap_hm", "MGTHRIS.VIEW_ATT_REKAP_HM")
	tblAdd   = rekapTable("pyr_stg_att_rekap_add", "MGTDAT.MGT_PAYROLL_ATT_REKAP_ADD")
	tblDaily = attTable{name: "pyr_stg_att_daily", source: "MGTHRIS.HM_ATT_DAILY",
		cols: []string{colPsadCompany, colPsadWorkDate, colPsadNIK, colPsadDivision, "psad_schedule_code",
			"psad_check_in", "psad_check_out", "psad_work_hours", "psad_sch_type", "psad_ot_inwork", "psad_row_hash", "psad_batch_id"},
		pk:    []string{colPsadCompany, colPsadWorkDate, colPsadNIK, colPsadDivision},
		match: []string{colPsadWorkDate, colPsadNIK}, changed: "t.psad_row_hash <> EXCLUDED.psad_row_hash",
		sumExpr: "psad_row_hash", sumOrder: "psad_nik, psad_work_date", company: colPsadCompany, division: colPsadDivision, date: colPsadWorkDate}
	tblSch = attTable{name: "pyr_stg_sch_rekap", source: "MGTDAT.MGT_PAYROLL_SCH_REKAP",
		cols:  []string{colPssrCompany, colPssrYymm, colPssrNIK, colPssrSchedule, colPssrDivision, "pssr_no_sch", "pssr_batch_id"},
		pk:    []string{colPssrCompany, colPssrYymm, colPssrNIK, colPssrSchedule, colPssrDivision},
		match: []string{colPssrNIK, colPssrSchedule}, changed: "t.pssr_no_sch IS DISTINCT FROM EXCLUDED.pssr_no_sch",
		sumExpr: "pssr_nik || ':' || pssr_schedule || '=' || coalesce(pssr_no_sch::text, '')", sumOrder: "pssr_nik, pssr_schedule",
		company: colPssrCompany, division: colPssrDivision, yymm: colPssrYymm}
)

// attPull is the mapped reader output: COPY rows without company/batch id.
type attPull struct {
	rows     [][]string // text cells in tb.cols order minus company (first) and batch id (last); "\N" = NULL
	rejected []string   // NIKs of rows refused by V6 (division / period mismatch)
}

// RunAttSync pulls HRIS attendance (S1–S4) into payroll_stg per 05§4: batch RUNNING, refuse CLOSED / locked
// periods, hash rows, one transaction (temp + COPY + upsert-if-changed + delete-missing), batch OK or FAILED with
// staging untouched on failure. db must connect as payroll_etl (D-58 (4)). Rows outside SOLO/JAKARTA (or not of the
// requested division/period) are rejected and counted, not fatal (V6, D-58 (6)).
func RunAttSync(ctx context.Context, db *sql.DB, reader AttReader, q AttSyncQuery) ([]AttSyncReport, error) {
	srcs, divs, err := q.plan()
	if err != nil {
		return nil, err
	}
	var reps []AttSyncReport
	var errs []error
	for _, div := range divs { // a failing division does not stop the other one; the errors are joined
	srcLoop:
		for _, src := range srcs {
			r, err := runOneAttSync(ctx, db, reader, q, src, div)
			reps = append(reps, r)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s %s/%d: %w", src, div, q.YYMM, err))
				break srcLoop
			}
		}
	}
	if len(errs) > 0 {
		return reps, errors.Join(errs...)
	}
	return reps, nil
}

func (q AttSyncQuery) plan() (srcs, divs []string, err error) {
	if q.Company <= 0 {
		return nil, nil, errors.New("company wajib")
	}
	if q.YYMM < 200001 || q.YYMM > 299912 || q.YYMM%100 < 1 || q.YYMM%100 > 12 {
		return nil, nil, fmt.Errorf("yymm %d tidak valid (YYYYMM)", q.YYMM)
	}
	switch q.Source {
	case "", AttSrcAll:
		srcs = []string{AttSrcRekap, AttSrcAdd, AttSrcSch, AttSrcDaily}
	case AttSrcRekap, AttSrcDaily, AttSrcAdd, AttSrcSch:
		srcs = []string{q.Source}
	default:
		return nil, nil, fmt.Errorf("source %q tidak dikenal (rekap|daily|add|sch|all)", q.Source)
	}
	switch q.Division {
	case "":
		divs = []string{DivSolo, DivJakarta}
	case DivSolo, DivJakarta:
		divs = []string{q.Division}
	default:
		return nil, nil, fmt.Errorf("divisi %q ditolak (hanya %s/%s, V6)", q.Division, DivSolo, DivJakarta)
	}
	return srcs, divs, nil
}

// DailyWindow is the work-date window of a payroll period: the 26th of the previous month .. the 25th (05§2 S2).
func DailyWindow(yymm int) (start, end time.Time) {
	y, m := yymm/100, time.Month(yymm%100)
	return time.Date(y, m-1, 26, 0, 0, 0, 0, time.UTC), time.Date(y, m, 25, 0, 0, 0, 0, time.UTC)
}

func runOneAttSync(ctx context.Context, db *sql.DB, reader AttReader, q AttSyncQuery, src, div string) (rep AttSyncReport, err error) {
	tb := map[string]attTable{AttSrcRekap: tblRekap, AttSrcAdd: tblAdd, AttSrcSch: tblSch, AttSrcDaily: tblDaily}[src]
	rep = AttSyncReport{Source: src, Division: div, YYMM: q.YYMM, Status: attBatchFailed}
	if !q.Dry {
		if err = db.QueryRowContext(ctx, `INSERT INTO payroll_stg.pyr_stg_sync_batch (psb_company_id, psb_source, psb_yymm, psb_division)
VALUES ($1,$2,$3,$4) RETURNING psb_id`, q.Company, tb.source, q.YYMM, div).Scan(&rep.BatchID); err != nil {
			return rep, fmt.Errorf("batch: %w", err)
		}
		defer func() {
			if err != nil {
				markBatchFailed(ctx, db, rep.BatchID, err)
			}
		}()
	}
	if err = attPeriodOpen(ctx, func(c context.Context, s string, a ...any) rowScanner { return db.QueryRowContext(c, s, a...) }, q, div, false); err != nil {
		return rep, err
	}
	p, err := pullAtt(ctx, reader, q, src, div)
	if err != nil {
		return rep, err
	}
	rep.Rejected, rep.RejectedNIKs = len(p.rejected), firstN(p.rejected, maxRejectNIKs)
	rep.RowsIn = len(p.rows) + rep.Rejected
	if err = writeAttStage(ctx, db, q, tb, div, p, &rep); err != nil {
		return rep, err
	}
	rep.Status = AttBatchOK
	if q.Dry {
		rep.Status = AttBatchDry
	}
	log.Info().Str("source", src).Str("division", div).Int("yymm", q.YYMM).Int64("batch", rep.BatchID).
		Int("rows_in", rep.RowsIn).Int("rejected", rep.Rejected).Int("upserted", rep.Upserted).Int("deleted", rep.Deleted).
		Str("status", rep.Status).Msg("att sync")
	return rep, nil
}

func firstN(s []string, n int) []string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func markBatchFailed(ctx context.Context, db *sql.DB, id int64, cause error) {
	msg := cause.Error()
	if len(msg) > 500 {
		msg = msg[:500]
	}
	if _, e := db.ExecContext(context.WithoutCancel(ctx), `UPDATE payroll_stg.pyr_stg_sync_batch
SET psb_status='FAILED', psb_error=$2, psb_finished_at=now() WHERE psb_id=$1`, id, msg); e != nil {
		log.Warn().Err(e).Int64("batch", id).Msg("att sync: cannot mark batch FAILED")
	}
}

type rowScanner interface{ Scan(dest ...any) error }

// attPeriodOpen refuses a CLOSED period or one with a locked snapshot (05§4 step 2). The --force-unlocked
// supervisor flag is not implemented (no consumer before B-UI3); CLOSED is never allowed anyway.
func attPeriodOpen(ctx context.Context, qr func(context.Context, string, ...any) rowScanner, q AttSyncQuery, div string, share bool) error {
	forShare := ""
	if share { // inside the write transaction: block a concurrent approve/close until we commit
		forShare = " FOR SHARE"
	}
	var status sql.NullString
	err := qr(ctx, `SELECT pp_status::text FROM payroll.pyr_period WHERE pp_company_id=$1 AND pp_yymm=$2 AND pp_division=$3`+forShare,
		q.Company, q.YYMM, div).Scan(&status)
	if err != nil && !errors.Is(err, sql.ErrNoRows) && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("cek periode: %w", err)
	}
	if status.String == "CLOSED" {
		return fmt.Errorf("periode %d %s CLOSED: sync ditolak", q.YYMM, div)
	}
	var locked bool
	if err := qr(ctx, `SELECT coalesce(bool_or(pas_locked), false) FROM (SELECT pas_locked FROM payroll.pyr_att_snapshot
WHERE pas_company_id=$1 AND pas_yymm=$2 AND pas_division=$3`+forShare+`) s`, q.Company, q.YYMM, div).Scan(&locked); err != nil {
		return fmt.Errorf("cek lock snapshot: %w", err)
	}
	if locked {
		return fmt.Errorf("periode %d %s punya snapshot terkunci: sync ditolak", q.YYMM, div)
	}
	return nil
}

// retry runs f up to attempts times with exponential backoff (05§4).
func retry[T any](ctx context.Context, q AttSyncQuery, f func() (T, error)) (T, error) {
	attempts, backoff := q.Attempts, q.Backoff
	if attempts <= 0 {
		attempts = 3
	}
	if backoff <= 0 {
		backoff = time.Second
	}
	var zero T
	var err error
	for i := 0; i < attempts; i++ {
		var v T
		if v, err = f(); err == nil {
			return v, nil
		}
		if i == attempts-1 || ctx.Err() != nil {
			break
		}
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(backoff << i):
		}
	}
	return zero, err
}

func pullAtt(ctx context.Context, reader AttReader, q AttSyncQuery, src, div string) (attPull, error) {
	switch src {
	case AttSrcRekap, AttSrcAdd:
		rows, err := retry(ctx, q, func() ([]RekapRow, error) {
			if src == AttSrcRekap {
				return reader.Rekap(ctx, q.YYMM, div)
			}
			return reader.RekapAdd(ctx, q.YYMM, div)
		})
		if err != nil {
			return attPull{}, fmt.Errorf("baca HRIS: %w", err)
		}
		return mapRekap(rows, q.YYMM, div, src == AttSrcAdd), nil
	case AttSrcSch:
		rows, err := retry(ctx, q, func() ([]SchRekapRow, error) { return reader.SchRekap(ctx, q.YYMM, div) })
		if err != nil {
			return attPull{}, fmt.Errorf("baca HRIS: %w", err)
		}
		return mapSch(rows, q.YYMM, div), nil
	default:
		start, end := DailyWindow(q.YYMM)
		rows, err := retry(ctx, q, func() ([]DailyRow, error) { return reader.Daily(ctx, start, end, div) })
		if err != nil {
			return attPull{}, fmt.Errorf("baca HRIS: %w", err)
		}
		return mapDaily(rows, start, end, div), nil
	}
}

const sqlNull = `\N`

func nd(d decimal.NullDecimal) string {
	if !d.Valid {
		return sqlNull
	}
	return d.Decimal.String()
}

func hashCells(cells []string) string {
	h := md5.New() //nolint:gosec // see import
	for _, c := range cells {
		h.Write([]byte(c))
		h.Write([]byte{0x1f})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func mapRekap(rows []RekapRow, yymm int, div string, add bool) attPull {
	var p attPull
	cols := RekapNumCols
	if add {
		cols = RekapAddNumCols
	}
	for _, r := range rows {
		if r.Division != div || r.Yymm != yymm || r.NIK == "" {
			p.rejected = append(p.rejected, r.NIK)
			continue
		}
		cells := make([]string, 0, 4+len(RekapNumCols)+1)
		cells = append(cells, fmt.Sprint(r.Yymm), r.NIK, r.Division, r.Nama)
		hashed := make([]string, 0, 2+len(RekapNumCols))
		hashed = append(hashed, r.NIK, r.Nama)
		for _, c := range RekapNumCols { // add tables keep the unused tail NULL (S3 only has day_work..shift)
			v := sqlNull
			if inCols(cols, c) {
				v = nd(r.Nums[c])
			}
			cells = append(cells, v)
			hashed = append(hashed, v)
		}
		p.rows = append(p.rows, append(cells, hashCells(hashed)))
	}
	return p
}

func inCols(cols []string, c string) bool {
	for _, x := range cols {
		if x == c {
			return true
		}
	}
	return false
}

func mapSch(rows []SchRekapRow, yymm int, div string) attPull {
	var p attPull
	for _, r := range rows {
		if r.Division != div || r.Yymm != yymm || r.NIK == "" || r.Schedule == "" {
			p.rejected = append(p.rejected, r.NIK)
			continue
		}
		p.rows = append(p.rows, []string{fmt.Sprint(r.Yymm), r.NIK, r.Schedule, r.Division, nd(r.NoSch)})
	}
	return p
}

// nullStr stores an empty text as NULL (Oracle has no empty string).
func nullStr(s string) string {
	if s == "" {
		return sqlNull
	}
	return s
}

func tm(t *time.Time) string {
	if t == nil {
		return sqlNull
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func mapDaily(rows []DailyRow, start, end time.Time, div string) attPull {
	var p attPull
	for _, r := range rows {
		d := time.Date(r.WorkDate.Year(), r.WorkDate.Month(), r.WorkDate.Day(), 0, 0, 0, 0, time.UTC)
		if r.Division != div || r.NIK == "" || d.Before(start) || d.After(end) {
			p.rejected = append(p.rejected, r.NIK)
			continue
		}
		cells := make([]string, 0, 10)
		cells = append(cells, d.Format("2006-01-02"), r.NIK, r.Division, r.ScheduleCode, tm(r.CheckIn), tm(r.CheckOut), nd(r.WorkHours),
			nullStr(r.SchType), nullStr(r.OtInwork))
		p.rows = append(p.rows, append(cells, hashCells(cells)))
	}
	return p
}

func copyEscape(s string) string {
	if s == sqlNull {
		return s
	}
	return strings.NewReplacer(`\`, `\\`, "\t", `\t`, "\n", `\n`, "\r", `\r`).Replace(s)
}

// copyData renders rows as COPY text: company id first, batch id last. Duplicate keys are an error (the upsert
// could not apply both); key = every cell up to and excluding the value cells.
func copyData(tb attTable, company int16, batch int64, p attPull) (*bytes.Buffer, error) {
	var buf bytes.Buffer
	seen := map[string]bool{}
	keyLen := len(tb.pk) - 1 // company is not a cell
	for _, r := range p.rows {
		k := tb.keyCells(r, keyLen)
		if seen[k] {
			return nil, fmt.Errorf("baris ganda di sumber: %s", strings.ReplaceAll(k, "\x1f", "/"))
		}
		seen[k] = true
		fmt.Fprint(&buf, company)
		for _, c := range r {
			buf.WriteByte('\t')
			buf.WriteString(copyEscape(c))
		}
		buf.WriteByte('\t')
		fmt.Fprint(&buf, batch)
		buf.WriteByte('\n')
	}
	return &buf, nil
}

// keyCells joins the key cells of a row; cell order follows tb.cols after company: daily/rekap/sch keys differ.
func (tb attTable) keyCells(r []string, _ int) string {
	switch tb.name {
	case tblDaily.name: // date, nik, division
		return strings.Join([]string{r[0], r[1], r[2]}, "\x1f")
	case tblSch.name: // yymm, nik, schedule, division
		return strings.Join(r[:4], "\x1f")
	default: // yymm, nik, division (cells: yymm, nik, division, nama, ...)
		return strings.Join(r[:3], "\x1f")
	}
}

func writeAttStage(ctx context.Context, db *sql.DB, q AttSyncQuery, tb attTable, div string, p attPull, rep *AttSyncReport) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer closeQuiet(conn)
	return conn.Raw(func(dc any) error {
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
				log.Warn().Err(rbErr).Msg("att sync: rollback failed")
			}
		}()
		if err := lockAndCheckPeriod(ctx, tx, q, div); err != nil {
			return err
		}
		if q.Dry {
			if err := tx.QueryRow(ctx, `INSERT INTO payroll_stg.pyr_stg_sync_batch (psb_company_id, psb_source, psb_yymm, psb_division)
VALUES ($1,$2,$3,$4) RETURNING psb_id`, q.Company, tb.source, q.YYMM, div).Scan(&rep.BatchID); err != nil {
				return err
			}
		}
		if err := applyAttStage(ctx, tx, q, tb, div, p, rep); err != nil {
			return err
		}
		if q.Dry {
			return nil // rolled back by the deferred Rollback
		}
		return tx.Commit(ctx)
	})
}

func applyAttStage(ctx context.Context, tx pgx.Tx, q AttSyncQuery, tb attTable, div string, p attPull, rep *AttSyncReport) error {
	buf, err := copyData(tb, q.Company, rep.BatchID, p)
	if err != nil {
		return err
	}
	scope, scopeArgs := tb.scope(q)
	args := append([]any{q.Company, div}, scopeArgs...)
	if err := guardEmptySource(ctx, tx, tb, len(p.rows), scope, args); err != nil {
		return err
	}
	const tmp = "tmp_att_sync"
	if _, err := tx.Exec(ctx, "CREATE TEMP TABLE "+tmp+" (LIKE payroll_stg."+tb.name+" INCLUDING DEFAULTS) ON COMMIT DROP"); err != nil {
		return err
	}
	cols := strings.Join(tb.cols, ", ")
	if _, err := tx.Conn().PgConn().CopyFrom(ctx, buf, "COPY "+tmp+" ("+cols+") FROM STDIN"); err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	conflict := " ON CONFLICT (" + strings.Join(tb.pk, ", ") + ") DO NOTHING"
	if !q.InsertOnly {
		var set []string
		for _, c := range tb.cols {
			if !inCols(tb.pk, c) {
				set = append(set, c+" = EXCLUDED."+c)
			}
		}
		conflict = " ON CONFLICT (" + strings.Join(tb.pk, ", ") + ") DO UPDATE SET " + strings.Join(set, ", ") + " WHERE " + tb.changed
	}
	ct, err := tx.Exec(ctx, "INSERT INTO payroll_stg."+tb.name+" AS t ("+cols+") SELECT "+cols+" FROM "+tmp+conflict)
	if err != nil {
		return fmt.Errorf("upsert: %w", err)
	}
	rep.Upserted = int(ct.RowsAffected())
	if !q.InsertOnly {
		var on []string
		for _, m := range tb.match {
			on = append(on, "x."+m+" = s."+m)
		}
		ct, err = tx.Exec(ctx, "DELETE FROM payroll_stg."+tb.name+" s WHERE s."+tb.company+"=$1 AND s."+tb.division+"=$2 AND "+
			strings.ReplaceAll(scope, tb.scopeCol(), "s."+tb.scopeCol())+
			" AND NOT EXISTS (SELECT 1 FROM "+tmp+" x WHERE "+strings.Join(on, " AND ")+" AND x."+tb.company+" = s."+tb.company+" AND x."+tb.division+" = s."+tb.division+")", args...)
		if err != nil {
			return fmt.Errorf("delete: %w", err)
		}
		rep.Deleted = int(ct.RowsAffected())
	}
	if err := tx.QueryRow(ctx, "SELECT coalesce(md5(string_agg("+tb.sumExpr+", '' ORDER BY "+tb.sumOrder+")), '') FROM payroll_stg."+tb.name+
		" WHERE "+tb.company+"=$1 AND "+tb.division+"=$2 AND "+scope, args...).Scan(&rep.Checksum); err != nil {
		return fmt.Errorf("checksum: %w", err)
	}
	return finishBatch(ctx, tx, rep)
}

func finishBatch(ctx context.Context, tx pgx.Tx, rep *AttSyncReport) error {
	note := ""
	if rep.Rejected > 0 {
		ns := append([]string(nil), rep.RejectedNIKs...)
		sort.Strings(ns)
		note = fmt.Sprintf("V6: %d baris ditolak (NIK: %s)", rep.Rejected, strings.Join(ns, ","))
	}
	_, err := tx.Exec(ctx, `UPDATE payroll_stg.pyr_stg_sync_batch SET psb_rows_in=$2, psb_rows_upserted=$3, psb_rows_deleted=$4,
psb_checksum=$5, psb_status='OK', psb_error=NULLIF($6,''), psb_finished_at=now() WHERE psb_id=$1`,
		rep.BatchID, rep.RowsIn, rep.Upserted, rep.Deleted, rep.Checksum, note)
	return err
}

func (tb attTable) scopeCol() string {
	if tb.yymm != "" {
		return tb.yymm
	}
	return tb.date
}

// scope is the WHERE fragment (params from $3) limiting staging to this run's period or work-date window.
func (tb attTable) scope(q AttSyncQuery) (string, []any) {
	if tb.yymm != "" {
		return tb.yymm + " = $3", []any{q.YYMM}
	}
	s, e := DailyWindow(q.YYMM)
	return tb.date + " BETWEEN $3 AND $4", []any{s, e}
}

// guardEmptySource: an empty HRIS answer must not wipe a populated staging (not in 05§4; safety).
func guardEmptySource(ctx context.Context, tx pgx.Tx, tb attTable, rows int, scope string, args []any) error {
	if rows > 0 {
		return nil
	}
	var n int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM payroll_stg."+tb.name+" WHERE "+tb.company+"=$1 AND "+tb.division+"=$2 AND "+scope, args...).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("sumber HRIS kosong padahal staging berisi %d baris: tidak dihapus", n)
	}
	return nil
}

func lockAndCheckPeriod(ctx context.Context, tx pgx.Tx, q AttSyncQuery, div string) error {
	// One sync per (company, yymm, division) at a time, then re-check CLOSED/lock with FOR SHARE row locks:
	// a concurrent approve/close (which updates those rows) waits for this commit, so it cannot slip in
	// between the check and the write; one that committed earlier is seen here.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		fmt.Sprintf("att-sync:%d:%d:%s", q.Company, q.YYMM, div)); err != nil {
		return fmt.Errorf("advisory lock: %w", err)
	}
	if err := attPeriodOpen(ctx, func(c context.Context, s string, a ...any) rowScanner { return tx.QueryRow(c, s, a...) }, q, div, true); err != nil {
		return err
	}
	return nil
}

// Staging company-id columns repeated in the sync table definitions.
const (
	colPsarCompany = "psar_company_id"
	colPsadCompany = "psad_company_id"
	colPssrCompany = "pssr_company_id"

	colPsarDivision = "psar_division"
	colPsadDivision = "psad_division"
	colPssrDivision = "pssr_division"

	colPsarYymm     = "psar_yymm"
	colPssrYymm     = "pssr_yymm"
	colPsadWorkDate = "psad_work_date"
	colPssrSchedule = "pssr_schedule"

	colPsarNIK = "psar_nik"
	colPsadNIK = "psad_nik"
	colPssrNIK = "pssr_nik"
)
