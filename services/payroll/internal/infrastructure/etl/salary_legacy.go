package etl

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"
)

// Legacy salary tables (T009e, 08§2): one LEGACY run per (yymm, division), the source of YTD / subsidy basis (T016c).
const (
	TableSalaryMain   = "MGT_PAYROLL_MAIN"
	TableSalaryDetail = "MGT_PAYROLL_DETAIL"

	legacyRuleSetZero = 0

	typeTunjangan = "tunjangan"
	typePotongan  = "potongan"
)

// SalaryLegacyOptions controls the historical salary load. Expect* = 0 means unchecked (08§2: 91916 / 2274418).
type SalaryLegacyOptions struct {
	CompanyID                int16
	ExpectMain, ExpectDetail int
}

// legacySkipped reports D-35.1 out-of-scope periods: JAKARTA before 202601 is trial data, SOLO 202607 has DETAIL without MAIN.
func legacySkipped(k snapKey) bool {
	return (k.division == "JAKARTA" && k.yymm < "202601") || (k.division == "SOLO" && k.yymm == "202607")
}

// noSnapshotOK reports D-35.3 periods that never had an attendance recap (201712-202003): loaded without a snapshot.
func noSnapshotOK(yymm string) bool { return yymm >= "201712" && yymm <= "202003" }

type salaryGroup struct {
	key   snapKey
	heads [][]string
	lines [][]string
}

var lemburCols = [][2]string{
	{"lembur1", "PM_JAM_LEMBUR1"}, {"lembur2", "PM_JAM_LEMBUR2"}, {"nas1", "PM_JAM_LEMBURNAS1"}, {"nas2", "PM_JAM_LEMBURNAS2"},
	{"nas3", "PM_JAM_LEMBURNAS3"}, {"off1", "PM_JAM_LEMBUROFF1"}, {"off2", "PM_JAM_LEMBUROFF2"}, {"off3", "PM_JAM_LEMBUROFF3"},
	{"nasp1", "PM_JAM_LEMBURNASP1"}, {"nasp2", "PM_JAM_LEMBURNASP2"}, {"nasp3", "PM_JAM_LEMBURNASP3"},
}

// copyText escapes one cell for COPY text format ("" = NULL).
func copyText(s string) string {
	if s == "" {
		return `\N`
	}
	r := strings.NewReplacer(`\`, `\\`, "\t", `\t`, "\n", `\n`, "\r", `\r`)
	return r.Replace(s)
}

func copyRow(buf *bytes.Buffer, cells ...string) {
	for i, c := range cells {
		if i > 0 {
			buf.WriteByte('\t')
		}
		buf.WriteString(copyText(c))
	}
	buf.WriteByte('\n')
}

// zero0 turns an empty NOT NULL amount into "0".
func zero0(s string) string {
	if s == "" {
		return "0"
	}
	return s
}

// periodRange returns the payroll cut-off window of a yymm: 26th of the previous month .. 25th (04§8).
func periodRange(yymm string) (from, to string, err error) {
	t, err := time.Parse("200601", yymm)
	if err != nil {
		return "", "", err
	}
	return t.AddDate(0, -1, 25).Format("2006-01-02"), t.AddDate(0, 0, 24).Format("2006-01-02"), nil
}

func lemburJSON(c func(string) string) (string, error) {
	m := map[string]any{}
	for _, lc := range lemburCols {
		if v := c(lc[1]); v != "" {
			m[lc[0]] = jsonValue(v)
		}
	}
	if len(m) == 0 {
		return "", nil
	}
	b, err := json.Marshal(m)
	return string(b), err
}

// RunSalaryLegacy loads MGT_PAYROLL_MAIN/DETAIL into one ACTIVE LEGACY run per (yymm, division) in ONE transaction.
// Periods are created OPEN, filled, then CLOSED (fn_guard_closed rejects inserts into a CLOSED period). A period that
// already has a LEGACY run is skipped (it is CLOSED and the guard forbids touching it), so re-runs are idempotent.
//
//nolint:gocognit,gocyclo // linear read -> group -> load pipeline; splitting would only scatter the report bookkeeping
func RunSalaryLegacy(ctx context.Context, db *sql.DB, src Source, o SalaryLegacyOptions) ([]TableReport, error) {
	mn, err := src.Read(TableSalaryMain)
	if err != nil {
		return nil, err
	}
	dt, err := src.Read(TableSalaryDetail)
	if err != nil {
		return nil, err
	}
	repM := TableReport{Table: mn.Name, SrcRows: len(mn.Rows), Checksum: mn.Checksum()}
	repD := TableReport{Table: dt.Name, SrcRows: len(dt.Rows), Checksum: dt.Checksum()}
	reps := []TableReport{repM, repD}
	if o.ExpectMain > 0 && repM.SrcRows != o.ExpectMain {
		return reps, fmt.Errorf("%s: %d baris, diharapkan %d", TableSalaryMain, repM.SrcRows, o.ExpectMain)
	}
	if o.ExpectDetail > 0 && repD.SrcRows != o.ExpectDetail {
		return reps, fmt.Errorf("%s: %d baris, diharapkan %d", TableSalaryDetail, repD.SrcRows, o.ExpectDetail)
	}
	mix, err := bindCols(mn, []string{"PM_YYMM", "PM_NIK", "PM_DIVISION"})
	if err != nil {
		return reps, err
	}
	dix, err := bindCols(dt, []string{"PD_YYMM", "PD_NIK", "PD_TYPE", "PD_NO", "PD_DIVISION"})
	if err != nil {
		return reps, err
	}
	groups := map[snapKey]*salaryGroup{}
	grp := func(k snapKey) *salaryGroup {
		g := groups[k]
		if g == nil {
			g = &salaryGroup{key: k}
			groups[k] = g
		}
		return g
	}
	for i, r := range mn.Rows {
		if exc := badKey(r, mix, "PM_YYMM", "PM_DIVISION"); exc != "" {
			repM.Exceptions = append(repM.Exceptions, fmt.Sprintf("%s baris %d: %s", mn.Name, i+2, exc))
			continue
		}
		k := snapKey{r[mix["PM_YYMM"]], r[mix["PM_DIVISION"]]}
		if legacySkipped(k) {
			repM.SkippedRows++
			skipNote(&repM, mn.Name, k)
			continue
		}
		g := grp(k)
		g.heads = append(g.heads, r)
	}
	for i, r := range dt.Rows {
		if exc := badKey(r, dix, "PD_YYMM", "PD_DIVISION"); exc != "" {
			repD.Exceptions = append(repD.Exceptions, fmt.Sprintf("%s baris %d: %s", dt.Name, i+2, exc))
			continue
		}
		k := snapKey{r[dix["PD_YYMM"]], r[dix["PD_DIVISION"]]}
		if legacySkipped(k) {
			repD.SkippedRows++
			skipNote(&repD, dt.Name, k)
			continue
		}
		g := grp(k)
		g.lines = append(g.lines, r)
	}
	keys := make([]snapKey, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].yymm != keys[j].yymm {
			return keys[i].yymm < keys[j].yymm
		}
		return keys[i].division < keys[j].division
	})

	conn, err := db.Conn(ctx)
	if err != nil {
		return reps, err
	}
	defer closeQuiet(conn)
	err = conn.Raw(func(dc any) error {
		sc, ok := dc.(*stdlib.Conn)
		if !ok {
			return fmt.Errorf("driver bukan pgx")
		}
		tx, err := sc.Conn().Begin(ctx)
		if err != nil {
			return err
		}
		defer func() {
			if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
				log.Warn().Err(rbErr).Msg("etl: rollback failed")
			}
		}()
		for _, k := range keys {
			if err := loadSalaryGroup(ctx, tx, o, mn, dt, &lazyEmp{src: src}, groups[k], &repM, &repD); err != nil {
				return fmt.Errorf("%s/%s: %w", k.yymm, k.division, err)
			}
		}
		reps = []TableReport{repM, repD}
		for _, r := range reps {
			if !r.OK() {
				return fmt.Errorf("%s: jumlah baris sumber %d != tujuan %d", r.Table, r.SrcRows, r.DstRows)
			}
			if _, err := tx.Exec(ctx, `INSERT INTO payroll.pyr_migration_log (pml_company_id, pml_table, pml_src_rows, pml_dst_rows, pml_checksum, pml_ok)
VALUES ($1,$2,$3,$4,$5,true)`, o.CompanyID, r.Table, r.SrcRows, r.DstRows, r.Checksum); err != nil {
				return err
			}
		}
		return tx.Commit(ctx)
	})
	return []TableReport{repM, repD}, err
}

// skipNote adds one SKIP line per period (not per row) to the report.
func skipNote(r *TableReport, table string, k snapKey) {
	note := fmt.Sprintf("SKIP %s %s/%s: di luar cakupan migrasi (D-35.1)", table, k.yymm, k.division)
	for _, n := range r.Skipped {
		if n == note {
			return
		}
	}
	r.Skipped = append(r.Skipped, note)
}

func badKey(r []string, ix map[string]int, yymmCol, divCol string) string {
	if exc := badYYMM(r, ix, []string{yymmCol}); exc != "" {
		return exc
	}
	if d := r[ix[divCol]]; d != "SOLO" && d != "JAKARTA" {
		return "division tidak dikenal: " + strconv.Quote(d)
	}
	return ""
}

//nolint:gocognit,gocyclo // one period = one transaction step; kept linear so the all-or-nothing rules read top to bottom
func loadSalaryGroup(ctx context.Context, tx pgx.Tx, o SalaryLegacyOptions, mn, dt *Table, emp *lazyEmp, g *salaryGroup, repM, repD *TableReport) error {
	var runID int64
	err := tx.QueryRow(ctx, `SELECT pr_id FROM payroll.pyr_run WHERE pr_company_id=$1 AND pr_yymm=$2 AND pr_division=$3 AND pr_kind='LEGACY' AND pr_status='ACTIVE'`,
		o.CompanyID, g.key.yymm, g.key.division).Scan(&runID)
	if err == nil { // already loaded and CLOSED: nothing to change, but the source must still match what was loaded
		var nHead, nLine int
		if err := tx.QueryRow(ctx, `SELECT pr_emp_count, pr_line_count FROM payroll.pyr_run WHERE pr_id=$1`, runID).Scan(&nHead, &nLine); err != nil {
			return err
		}
		orphans := len(orphanNiks(g))
		if nHead != len(g.heads)+orphans || nLine != len(g.lines) {
			return fmt.Errorf("sudah dimuat (%d head, %d detail) tetapi sumber kini %d/%d; periode CLOSED tidak boleh diubah", nHead, nLine, len(g.heads)+orphans, len(g.lines))
		}
		repM.DstRows += nHead - orphans
		repD.DstRows += nLine
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	rejectAll := func(why string) {
		for _, r := range g.heads {
			repM.Exceptions = append(repM.Exceptions, fmt.Sprintf("%s %s/%s nik %s: %s", mn.Name, g.key.yymm, g.key.division, r[1], why))
		}
		for _, r := range g.lines {
			repD.Exceptions = append(repD.Exceptions, fmt.Sprintf("%s %s/%s nik %s: %s", dt.Name, g.key.yymm, g.key.division, r[1], why))
		}
	}
	var snap *int64
	err = tx.QueryRow(ctx, `SELECT pas_snapshot_id FROM payroll.pyr_att_snapshot WHERE pas_company_id=$1 AND pas_yymm=$2 AND pas_division=$3 AND pas_source_batch_ids='{}'`,
		o.CompanyID, g.key.yymm, g.key.division).Scan(&snap)
	switch {
	case errors.Is(err, pgx.ErrNoRows) && noSnapshotOK(g.key.yymm):
		snap = nil // D-35.3: no attendance recap existed before 202004
	case errors.Is(err, pgx.ErrNoRows):
		rejectAll("tanpa snapshot LEGACY (jalankan att-legacy / T009d dulu)")
		return nil
	case err != nil:
		return err
	}
	sysids, err := legacySysids(ctx, tx, o.CompanyID, g.key.division)
	if err != nil {
		return err
	}
	from, to, err := periodRange(g.key.yymm)
	if err != nil {
		return err
	}

	// All-or-nothing per period: a CLOSED period cannot take late rows, so any unloadable row holds back the whole group
	// (every row reported) until the cause (missing legacy map, orphan NIK) is fixed and the load is re-run.
	problems := make([]string, len(g.lines)) // "" = line is fine
	nProblems := 0
	for i, r := range g.lines {
		c := func(n string) string { return r[mustCol(dt, n)] }
		typ := strings.ToLower(c("PD_TYPE"))
		id, perr := strconv.Atoi(c("PD_NO"))
		switch {
		case typ != typeTunjangan && typ != typePotongan:
			problems[i] = fmt.Sprintf("PD_TYPE %q", c("PD_TYPE"))
		case perr != nil:
			problems[i] = fmt.Sprintf("PD_NO %q", c("PD_NO"))
		default:
			if _, ok := sysids[sysKey{typ, id}]; !ok {
				problems[i] = fmt.Sprintf("tanpa peta sysid (%s %d)", c("PD_TYPE"), id)
			}
		}
		if problems[i] != "" {
			nProblems++
		}
	}
	if nProblems > 0 { // every row gets exactly one exception so the report still balances
		for _, r := range g.heads {
			repM.Exceptions = append(repM.Exceptions, fmt.Sprintf("%s %s/%s nik %s: periode ditahan, %d baris DETAIL bermasalah", mn.Name, g.key.yymm, g.key.division, r[1], nProblems))
		}
		for i, r := range g.lines {
			why := problems[i]
			if why == "" {
				why = "periode ditahan karena baris DETAIL lain bermasalah"
			}
			repD.Exceptions = append(repD.Exceptions, fmt.Sprintf("%s %s/%s nik %s: %s", dt.Name, g.key.yymm, g.key.division, r[1], why))
		}
		return nil
	}

	mc := func(r []string) func(string) string { return func(n string) string { return r[mustCol(mn, n)] } }
	var heads, lines bytes.Buffer
	var payDate string
	var totT, totP decimalSum
	nHead, nLine := 0, 0
	for _, r := range g.heads {
		c := mc(r)
		pay, err := parseDate(c("PM_DATE"))
		if err != nil {
			return fmt.Errorf("nik %s PM_DATE: %w", r[1], err)
		}
		join, err := parseDate(c("PM_DATE_JOIN"))
		if err != nil {
			return fmt.Errorf("nik %s PM_DATE_JOIN: %w", r[1], err)
		}
		if pay > payDate {
			payDate = pay
		}
		lem, err := lemburJSON(c)
		if err != nil {
			return err
		}
		copyRow(&heads, "@RUN@", c("PM_NIK"), c("PM_YYMM"), c("PM_DIVISION"), c("PM_NAMA"), c("PM_DEPT"), c("PM_DEPT_CODE"), c("PM_SECTION"),
			c("PM_GRADE"), c("PM_JABATAN"), c("PM_EMP_TYPE"), c("PM_PEMBAYARAN"), join, pay, c("PM_TAX_CODE"),
			c("PM_BASIC_PAY"), c("PM_TUNJJABATAN"), c("PM_TUNJRUMAH"), c("PM_TUNJKESEJAHTERAAN"), c("PM_TUNJTRANSPORT"), c("PM_RATE_PERJAM"),
			zero0(c("PM_TOT_TUNJANGAN")), zero0(c("PM_TOT_POTONGAN")), zero0(c("PM_ROUNDING")), c("PM_DAY_PAY"), lem, c("PM_REMARKS"),
			strconv.Itoa(int(o.CompanyID)))
		totT.add(c("PM_TOT_TUNJANGAN"))
		totP.add(c("PM_TOT_POTONGAN"))
		nHead++
	}
	// D-35.2: DETAIL without MAIN → synthetic MAIN (sum of DETAIL per type, ROUND half-up), attributes from EMP/COMPNT_EMP.
	nSynth := 0
	for _, nik := range orphanNiks(g) {
		var t, p decimal.Decimal
		for _, r := range g.lines {
			if r[1] != nik {
				continue
			}
			cell := r[mustCol(dt, "PD_AMOUNT")]
			if cell == "" {
				continue // NULL amount counts as 0 (same as a legacy NULL)
			}
			a, err := decimal.NewFromString(cell)
			if err != nil {
				return fmt.Errorf("nik %s PD_AMOUNT %q: %w", nik, cell, err)
			}
			if strings.EqualFold(r[mustCol(dt, "PD_TYPE")], typePotongan) {
				p = p.Add(a)
			} else {
				t = t.Add(a)
			}
		}
		t, p = t.Round(2), p.Round(2)
		at, err := emp.attrs(nik, g.key.division)
		if err != nil {
			return err
		}
		if at["nama"] == "" {
			repM.Warnings = append(repM.Warnings, fmt.Sprintf("%s %s/%s nik %s: atribut karyawan tidak ditemukan di EMP (nama kosong)", mn.Name, g.key.yymm, g.key.division, nik))
		}
		copyRow(&heads, "@RUN@", nik, g.key.yymm, g.key.division, at["nama"], at["dept"], "", at["section"], at["grade"], at["jabatan"], at["emp_type"], "",
			at["join"], "", at["tax"], at["basic"], "", "", "", "", "", t.StringFixed(2), p.StringFixed(2), "0", "", "", "", strconv.Itoa(int(o.CompanyID)))
		totT.add(t.StringFixed(2))
		totP.add(p.StringFixed(2))
		nHead++
		nSynth++
		repM.Warnings = append(repM.Warnings, fmt.Sprintf("%s %s/%s nik %s: MAIN sintetis dari DETAIL (D-35.2), tunjangan %s potongan %s",
			mn.Name, g.key.yymm, g.key.division, nik, t.StringFixed(2), p.StringFixed(2)))
	}
	for _, r := range g.lines {
		c := func(n string) string { return r[mustCol(dt, n)] }
		typ := strings.ToLower(c("PD_TYPE"))
		id, _ := strconv.Atoi(c("PD_NO")) //nolint:errcheck // validated above
		sys := sysids[sysKey{typ, id}]
		typName := "Tunjangan"
		if typ == typePotongan {
			typName = "Potongan"
		}
		copyRow(&lines, "@RUN@", c("PD_NIK"), typName, strconv.Itoa(id), c("PD_DESCRIPTION"), c("PD_AMOUNT"), c("PD_STATUS"), c("PD_TAX"),
			c("PD_TAX_PAID_COMPANY"), c("PD_BASIS_AMOUNT"), "LEGACY", c("PD_REMARKS"), strconv.FormatInt(sys, 10))
		nLine++
	}
	if payDate == "" {
		payDate = to
	}

	var created int
	if err := tx.QueryRow(ctx, `WITH ins AS (INSERT INTO payroll.pyr_period (pp_company_id, pp_yymm, pp_division, pp_date_from, pp_date_to, pp_status)
VALUES ($1,$2,$3,$4,$5,'OPEN') ON CONFLICT DO NOTHING RETURNING 1) SELECT count(*) FROM ins`,
		o.CompanyID, g.key.yymm, g.key.division, from, to).Scan(&created); err != nil {
		return err
	}
	if created == 0 { // period exists but has no LEGACY run: it belongs to the engine (or a half-made one); never touch it
		rejectAll("periode sudah ada tetapi bukan hasil ETL legacy; tidak diubah")
		return nil
	}
	if err := tx.QueryRow(ctx, `INSERT INTO payroll.pyr_run (pr_company_id, pr_yymm, pr_division, pr_pay_date, pr_rule_set_id, pr_snapshot_id, pr_status, pr_kind,
  pr_emp_count, pr_line_count, pr_tot_tunjangan, pr_tot_potongan, pr_started_by, pr_finished_at)
VALUES ($1,$2,$3,$4,$5,$6,'ACTIVE','LEGACY',$7,$8,$9,$10,'etl',now()) RETURNING pr_id`,
		o.CompanyID, g.key.yymm, g.key.division, payDate, legacyRuleSetZero, snap, nHead, nLine, totT.String(), totP.String()).Scan(&runID); err != nil {
		return err
	}
	id := strconv.FormatInt(runID, 10)
	// COPY is not allowed on RLS tables: stream into TEMP text tables, then INSERT..SELECT (policy + guard still apply).
	if err := stageInsert(ctx, tx, &heads, id, "pyr_tmp_head", headCols, headTypes, `INSERT INTO payroll.pyr_salary_head (`+headCols+`)
SELECT `+headSelect+` FROM pyr_tmp_head`); err != nil {
		return fmt.Errorf("head: %w", err)
	}
	if err := stageInsert(ctx, tx, &lines, id, "pyr_tmp_detail", detailCols, detailTypes, `INSERT INTO payroll.pyr_salary_detail (`+detailCols+`)
SELECT `+detailSelect+` FROM pyr_tmp_detail`); err != nil {
		return fmt.Errorf("detail: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE payroll.pyr_period SET pp_active_run_id=$4, pp_status='CLOSED', closed_by='etl', closed_at=now()
WHERE pp_company_id=$1 AND pp_yymm=$2 AND pp_division=$3`, o.CompanyID, g.key.yymm, g.key.division, runID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO payroll.pyr_audit_log (pal_actor, pal_action, pal_entity, pal_entity_key, pal_division, pal_after, pal_company_id)
VALUES ('etl','ETL_PERIOD_CLOSE','pyr_period',$1,$2,jsonb_build_object('run_id',$3::bigint,'kind','LEGACY','heads',$4::int,'lines',$5::int,
  'tot_tunjangan',$6::text,'tot_potongan',$7::text),$8)`,
		g.key.yymm+"/"+g.key.division, g.key.division, runID, nHead, nLine, totT.String(), totP.String(), o.CompanyID); err != nil {
		return fmt.Errorf("audit: %w", err)
	}
	repM.DstRows += nHead - nSynth
	repD.DstRows += nLine
	return nil
}

// orphanNiks lists NIKs that have DETAIL but no MAIN in the group (sorted, so the load is deterministic).
func orphanNiks(g *salaryGroup) []string {
	has := map[string]bool{}
	for _, r := range g.heads {
		has[r[1]] = true
	}
	seen := map[string]bool{}
	var out []string
	for _, r := range g.lines {
		if !has[r[1]] && !seen[r[1]] {
			seen[r[1]] = true
			out = append(out, r[1])
		}
	}
	sort.Strings(out)
	return out
}

// lazyEmp reads MGT_PAYROLL_EMP / COMPNT_EMP only when a synthetic MAIN is needed; absent files mean empty attributes.
type lazyEmp struct {
	src     Source
	loaded  bool
	emp, ce *Table
}

func (l *lazyEmp) load() {
	if l.loaded {
		return
	}
	l.loaded = true
	l.emp, _ = l.src.Read(TableEmployee)         //nolint:errcheck // optional: attributes stay empty
	l.ce, _ = l.src.Read(TableEmployeeComponent) //nolint:errcheck // optional
}

// attrs returns the MAIN attributes for (nik, division); keys are empty when unknown.
func (l *lazyEmp) attrs(nik, division string) (map[string]string, error) {
	l.load()
	out := map[string]string{}
	if err := l.empAttrs(nik, division, out); err != nil {
		return nil, err
	}
	l.basicPay(nik, division, out)
	return out, nil
}

// basicPay sets out["basic"] to the Gaji Pokok (Tunjangan 1) amount of the employee's COMPNT_EMP, if any.
func (l *lazyEmp) basicPay(nik, division string, out map[string]string) {
	if l.ce == nil {
		return
	}
	ix, err := bindCols(l.ce, []string{"MPCE_NIK", "MPCE_TYPE", "MPCE_ID", "MPCE_AMOUNT"})
	if err != nil {
		return
	}
	ni, ti, ii, ai := ix["MPCE_NIK"], ix["MPCE_TYPE"], ix["MPCE_ID"], ix["MPCE_AMOUNT"]
	di, derr := l.ce.Col("MPCE_DIVISION")
	if derr != nil {
		di = -1
	}
	for _, r := range l.ce.Rows {
		if r[ni] == nik && strings.EqualFold(r[ti], typeTunjangan) && r[ii] == "1" && (di < 0 || r[di] == division) {
			out["basic"] = r[ai]
			return
		}
	}
}

// empAttrs fills out from the MGT_PAYROLL_EMP row of (nik, division), if any.
func (l *lazyEmp) empAttrs(nik, division string, out map[string]string) error {
	if l.emp == nil {
		return nil
	}
	col := func(n string) int {
		i, err := l.emp.Col(n)
		if err != nil {
			return -1
		}
		return i
	}
	ni, di := col("PE_NIK"), col("PE_DIVISION")
	if ni < 0 {
		return nil
	}
	for _, r := range l.emp.Rows {
		if r[ni] != nik || (di >= 0 && r[di] != division) {
			continue
		}
		get := func(n string) string {
			if i := col(n); i >= 0 {
				return r[i]
			}
			return ""
		}
		out["nama"], out["dept"], out["section"], out["grade"] = get("PE_NAMA"), get("PE_DEPT"), get("PE_SECTION"), get("PE_GRADE")
		out["jabatan"], out["emp_type"], out["tax"] = get("PE_DESIGNATION"), get("PE_STS_EMP"), get("PE_TAX_ID")
		if j := get("PE_DATE_JOIN"); j != "" {
			d, err := parseDate(j)
			if err != nil {
				return fmt.Errorf("nik %s PE_DATE_JOIN: %w", nik, err)
			}
			out["join"] = d
		}
		return nil
	}
	return nil
}

const (
	headCols = `psh_run_id, psh_nik, psh_yymm, psh_division, psh_nama, psh_dept, psh_dept_code, psh_section,
  psh_grade, psh_jabatan, psh_emp_type, psh_pembayaran, psh_date_join, psh_pay_date, psh_tax_code,
  psh_basic_pay, psh_tunj_jabatan, psh_tunj_rumah, psh_tunj_kesejahteraan, psh_tunj_transport, psh_rate_perjam,
  psh_tot_tunjangan, psh_tot_potongan, psh_rounding, psh_day_pay, psh_jam_lembur, psh_remarks, psh_company_id`
	headTypes = `bigint, text, int, text, text, text, text, text, text, text, text, text, date, date, text,
  numeric, numeric, numeric, numeric, numeric, numeric, numeric, numeric, numeric, numeric, jsonb, text, smallint`
	headSelect = `psh_run_id, psh_nik, psh_yymm, psh_division, psh_nama, psh_dept, psh_dept_code, psh_section,
  psh_grade, psh_jabatan, psh_emp_type, psh_pembayaran, psh_date_join, psh_pay_date, psh_tax_code,
  psh_basic_pay, psh_tunj_jabatan, psh_tunj_rumah, psh_tunj_kesejahteraan, psh_tunj_transport, psh_rate_perjam,
  psh_tot_tunjangan, psh_tot_potongan, psh_rounding, psh_day_pay, psh_jam_lembur, psh_remarks, psh_company_id`

	detailCols = `psd_run_id, psd_nik, psd_type, psd_id, psd_description, psd_amount, psd_sts, psd_tax,
  psd_tax_paid_company, psd_basis_amount, psd_formula, psd_remarks, psd_component_sysid`
	detailTypes  = `bigint, text, text, int, text, numeric, text, smallint, text, numeric, text, text, bigint`
	detailSelect = `psd_run_id, psd_nik, psd_type::payroll.comp_type, psd_id, psd_description, psd_amount, psd_sts, psd_tax,
  psd_tax_paid_company, psd_basis_amount, psd_formula, psd_remarks, psd_component_sysid`
)

// stageInsert loads buf (placeholder @RUN@ = run id) into a temp table typed by cols/types, runs the INSERT..SELECT,
// then drops the temp table so the next group starts clean.
func stageInsert(ctx context.Context, tx pgx.Tx, buf *bytes.Buffer, runID, tmp, cols, types, insert string) error {
	names := strings.Fields(strings.ReplaceAll(cols, ",", " "))
	typs := strings.Split(strings.ReplaceAll(types, "\n", ""), ",")
	defs := make([]string, len(names))
	for i, n := range names {
		defs[i] = n + " " + strings.TrimSpace(typs[i])
	}
	if _, err := tx.Exec(ctx, "CREATE TEMP TABLE "+tmp+" ("+strings.Join(defs, ", ")+") ON COMMIT DROP"); err != nil {
		return err
	}
	data := bytes.ReplaceAll(buf.Bytes(), []byte("@RUN@"), []byte(runID))
	if _, err := tx.Conn().PgConn().CopyFrom(ctx, bytes.NewReader(data), "COPY "+tmp+" ("+cols+") FROM STDIN"); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, insert); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, "DROP TABLE "+tmp)
	return err
}

type sysKey struct {
	typ string
	id  int
}

// legacyAlias maps DETAIL component ids that SOLO used in 201712-202003 but never existed in COMPNT_MASTER onto the
// same-named master components (D-36 option a). PD_NO stays as in the source; only the sysid lookup is aliased.
var legacyAlias = map[sysKey]sysKey{
	{typeTunjangan, 100}: {typeTunjangan, 70}, // Additional Earning
	{typeTunjangan, 200}: {typeTunjangan, 71}, // Other Earning
	{typePotongan, 101}:  {typePotongan, 70},  // Additional Deduction
	{typePotongan, 201}:  {typePotongan, 71},  // Other Deduction
}

func legacySysids(ctx context.Context, tx pgx.Tx, company int16, division string) (map[sysKey]int64, error) {
	rows, err := tx.Query(ctx, `SELECT lower(pclm_legacy_type::text), pclm_legacy_id, pclm_sysid FROM payroll.pyr_component_legacy_map
WHERE pclm_company_id=$1 AND pclm_legacy_division=$2`, company, division)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[sysKey]int64{}
	for rows.Next() {
		var k sysKey
		var s int64
		if err := rows.Scan(&k.typ, &k.id, &s); err != nil {
			return nil, err
		}
		m[k] = s
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for from, to := range legacyAlias {
		if _, ok := m[from]; ok {
			continue
		}
		if s, ok := m[to]; ok {
			m[from] = s
		}
	}
	return m, nil
}

func mustCol(t *Table, name string) int {
	i, err := t.Col(name)
	if err != nil {
		return len(t.Cols) // out of range → caller sees a panic in tests, never silent
	}
	return i
}

// decimalSum adds money cells exactly (never float); empty cells count as 0, a malformed cell is ignored here
// because the same cell is validated by the numeric(18,2) column on COPY.
type decimalSum struct{ v decimal.Decimal }

func (d *decimalSum) add(s string) {
	if s == "" {
		return
	}
	if x, err := decimal.NewFromString(s); err == nil {
		d.v = d.v.Add(x)
	}
}

func (d *decimalSum) String() string { return d.v.StringFixed(2) }
