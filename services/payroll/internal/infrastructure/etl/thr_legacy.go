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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"
)

// Legacy THR/Bonus history (T024b, D-56 opsi A): one CLOSED event + one ACTIVE LEGACY run per (company, division, kind,
// yymm), the source of YTD PPh December (v_pyr_thr_detail). Only MAIN/DETAIL are loaded; MGT_THR_ADDITION is B-THR.
const (
	TableThrMain   = "MGT_THR_MAIN"
	TableThrDetail = "MGT_THR_DETAIL"
)

// ThrLegacyOptions controls the THR/Bonus history load. Expect* = 0 means unchecked (14§4: ±14918 / ±62246).
type ThrLegacyOptions struct {
	CompanyID                int16
	ExpectMain, ExpectDetail int
}

type thrKey struct{ division, kind, yymm string }

type thrGroup struct {
	key   thrKey
	heads [][]string
	lines [][]string
}

// thrKind maps TM_TYPE/TD_TYPE ("THR"/"Bonus") to the pte_kind value; "" = unknown.
func thrKind(s string) string {
	switch strings.ToUpper(s) {
	case "THR":
		return "THR"
	case "BONUS":
		return "BONUS"
	}
	return ""
}

// thrBadKey validates yymm, division and kind of one source row.
func thrBadKey(r []string, ix map[string]int, yymmCol, typeCol, divCol string) string {
	if exc := badKey(r, ix, yymmCol, divCol); exc != "" {
		return exc
	}
	if thrKind(r[ix[typeCol]]) == "" {
		return fmt.Sprintf("%s %q bukan THR/Bonus", typeCol, r[ix[typeCol]])
	}
	return ""
}

// RunThrLegacy loads MGT_THR_MAIN/DETAIL in ONE transaction. An event that already has a LEGACY run is not touched (it
// is CLOSED); the source must still match what was loaded, so re-runs are idempotent. Like salary-legacy it is
// all-or-nothing per event: any unloadable row (NIK without employee, DETAIL without MAIN, bad value) holds back the
// whole event and every row is reported, so a CLOSED event never ends up half loaded.
//
//nolint:gocognit,gocyclo // linear read -> group -> load pipeline, same shape as RunSalaryLegacy
func RunThrLegacy(ctx context.Context, db *sql.DB, src Source, o ThrLegacyOptions) ([]TableReport, error) {
	mn, err := src.Read(TableThrMain)
	if err != nil {
		return nil, err
	}
	dt, err := src.Read(TableThrDetail)
	if err != nil {
		return nil, err
	}
	repM := TableReport{Table: mn.Name, SrcRows: len(mn.Rows), Checksum: mn.Checksum()}
	repD := TableReport{Table: dt.Name, SrcRows: len(dt.Rows), Checksum: dt.Checksum()}
	reps := []TableReport{repM, repD}
	if o.ExpectMain > 0 && repM.SrcRows != o.ExpectMain {
		return reps, fmt.Errorf("%s: %d baris, diharapkan %d", TableThrMain, repM.SrcRows, o.ExpectMain)
	}
	if o.ExpectDetail > 0 && repD.SrcRows != o.ExpectDetail {
		return reps, fmt.Errorf("%s: %d baris, diharapkan %d", TableThrDetail, repD.SrcRows, o.ExpectDetail)
	}
	mix, err := bindCols(mn, []string{"TM_YYMM", "TM_TYPE", "TM_NIK", "TM_DIVISION", "TM_TOT_TUNJANGAN", "TM_TOT_POTONGAN", "TM_ROUNDING"})
	if err != nil {
		return reps, err
	}
	dix, err := bindCols(dt, []string{"TD_YYMM", "TD_TYPE", "TD_NIK", "TD_NO", "TD_JENIS", "TD_AMOUNT", "TD_DIVISION"})
	if err != nil {
		return reps, err
	}
	groups := map[thrKey]*thrGroup{}
	grp := func(k thrKey) *thrGroup {
		g := groups[k]
		if g == nil {
			g = &thrGroup{key: k}
			groups[k] = g
		}
		return g
	}
	for i, r := range mn.Rows {
		if exc := thrBadKey(r, mix, "TM_YYMM", "TM_TYPE", "TM_DIVISION"); exc != "" {
			repM.Exceptions = append(repM.Exceptions, fmt.Sprintf("%s baris %d: %s", mn.Name, i+2, exc))
			continue
		}
		g := grp(thrKey{r[mix["TM_DIVISION"]], thrKind(r[mix["TM_TYPE"]]), r[mix["TM_YYMM"]]})
		g.heads = append(g.heads, r)
	}
	for i, r := range dt.Rows {
		if exc := thrBadKey(r, dix, "TD_YYMM", "TD_TYPE", "TD_DIVISION"); exc != "" {
			repD.Exceptions = append(repD.Exceptions, fmt.Sprintf("%s baris %d: %s", dt.Name, i+2, exc))
			continue
		}
		g := grp(thrKey{r[dix["TD_DIVISION"]], thrKind(r[dix["TD_TYPE"]]), r[dix["TD_YYMM"]]})
		g.lines = append(g.lines, r)
	}
	keys := make([]thrKey, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.yymm != b.yymm {
			return a.yymm < b.yymm
		}
		if a.division != b.division {
			return a.division < b.division
		}
		return a.kind < b.kind
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
		l := &thrLoader{tx: tx, o: o, mn: mn, dt: dt, repM: &repM, repD: &repD, emps: map[string]*thrEmps{}}
		for _, k := range keys {
			if err := l.loadGroup(ctx, groups[k]); err != nil {
				return fmt.Errorf("%s/%s/%s: %w", k.yymm, k.division, k.kind, err)
			}
		}
		reps = []TableReport{repM, repD}
		for _, r := range reps {
			if !r.OK() {
				return fmt.Errorf("%s: jumlah baris sumber %d != tujuan %d + exception %d", r.Table, r.SrcRows, r.DstRows, len(r.Exceptions))
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

// thrEmps resolves a NIK to pe_id inside one (company, division): current NIK first, then NIK history.
type thrEmps struct {
	cur  map[string]int64
	hist map[string]map[int64]bool
}

func (e *thrEmps) resolve(nik string) (int64, string) {
	if id, ok := e.cur[nik]; ok {
		return id, ""
	}
	h := e.hist[nik]
	switch len(h) {
	case 0:
		return 0, "NIK tidak ada di pyr_employee"
	case 1:
		for id := range h {
			return id, ""
		}
	}
	return 0, "NIK cocok ke lebih dari satu karyawan (nik_hist)"
}

type thrLoader struct {
	tx         pgx.Tx
	o          ThrLegacyOptions
	mn, dt     *Table
	repM, repD *TableReport
	emps       map[string]*thrEmps
}

func (l *thrLoader) employees(ctx context.Context, division string) (*thrEmps, error) {
	if e := l.emps[division]; e != nil {
		return e, nil
	}
	e := &thrEmps{cur: map[string]int64{}, hist: map[string]map[int64]bool{}}
	rows, err := l.tx.Query(ctx, `SELECT pe_nik, pe_id FROM payroll.pyr_employee WHERE pe_company_id=$1 AND pe_division=$2::payroll.division`, l.o.CompanyID, division)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var nik string
		var id int64
		if err := rows.Scan(&nik, &id); err != nil {
			rows.Close()
			return nil, err
		}
		e.cur[nik] = id
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows, err = l.tx.Query(ctx, `SELECT h.penh_nik, h.penh_employee_id FROM payroll.pyr_employee_nik_hist h
JOIN payroll.pyr_employee p ON p.pe_id = h.penh_employee_id WHERE p.pe_company_id=$1 AND p.pe_division=$2::payroll.division`, l.o.CompanyID, division)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var nik string
		var id int64
		if err := rows.Scan(&nik, &id); err != nil {
			return nil, err
		}
		if e.hist[nik] == nil {
			e.hist[nik] = map[int64]bool{}
		}
		e.hist[nik][id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	l.emps[division] = e
	return e, nil
}

// thrPlan is the validated content of one event.
type thrPlan struct {
	heads, lines   bytes.Buffer
	nHead, nLine   int
	totT, totP     decimalSum
	eventDate      string
	desc           string
	warnings       []string
	unmapped, coll int
}

func (l *thrLoader) hold(g *thrGroup, why string) {
	for _, r := range g.heads {
		l.repM.Exceptions = append(l.repM.Exceptions, fmt.Sprintf("%s %s/%s/%s nik %s: %s", l.mn.Name, g.key.yymm, g.key.division, g.key.kind, r[mustCol(l.mn, "TM_NIK")], why))
	}
	for _, r := range g.lines {
		l.repD.Exceptions = append(l.repD.Exceptions, fmt.Sprintf("%s %s/%s/%s nik %s: %s", l.dt.Name, g.key.yymm, g.key.division, g.key.kind, r[mustCol(l.dt, "TD_NIK")], why))
	}
}

//nolint:gocognit,gocyclo // one event = one transaction step; kept linear so the all-or-nothing rules read top to bottom
func (l *thrLoader) loadGroup(ctx context.Context, g *thrGroup) error {
	k, o := g.key, l.o
	emps, err := l.employees(ctx, k.division)
	if err != nil {
		return err
	}
	sysids, err := legacySysids(ctx, l.tx, o.CompanyID, k.division)
	if err != nil {
		return err
	}
	mc := func(r []string, n string) string { return r[mustCol(l.mn, n)] }
	dc := func(r []string, n string) string { return r[mustCol(l.dt, n)] }

	// ---- validate: collect every problem so the report names all of them, then hold the event back.
	var probs []string
	headID := map[string]int64{}
	for _, r := range g.heads {
		nik := mc(r, "TM_NIK")
		if _, dup := headID[nik]; dup {
			probs = append(probs, "MAIN ganda untuk NIK "+nik)
			continue
		}
		id, why := emps.resolve(nik)
		if why != "" {
			probs = append(probs, fmt.Sprintf("MAIN nik %s: %s", nik, why))
			continue
		}
		headID[nik] = id
	}
	for _, r := range g.lines {
		nik := dc(r, "TD_NIK")
		if _, ok := headID[nik]; ok {
			continue
		}
		if _, why := emps.resolve(nik); why != "" {
			probs = append(probs, fmt.Sprintf("DETAIL nik %s: %s", nik, why))
		} else {
			probs = append(probs, fmt.Sprintf("DETAIL nik %s tanpa MAIN", nik))
		}
	}

	p := &thrPlan{}
	if len(probs) == 0 {
		if err := l.plan(g, p, headID, sysids); err != nil {
			probs = append(probs, err.Error())
		}
	}
	if len(probs) > 0 {
		sort.Strings(probs)
		l.hold(g, fmt.Sprintf("event ditahan, %d masalah (pertama: %s)", len(probs), probs[0]))
		return nil
	}

	// ---- already loaded? The event is CLOSED: leave it, but the source must still match.
	var eventID int64
	err = l.tx.QueryRow(ctx, `SELECT pte_id FROM payroll.pyr_thr_event WHERE pte_company_id=$1 AND pte_division=$2::payroll.division AND pte_kind=$3 AND pte_yymm=$4::int`,
		o.CompanyID, k.division, k.kind, k.yymm).Scan(&eventID)
	if err == nil {
		return l.checkLoaded(ctx, g, p, eventID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	// ---- insert
	if err := l.tx.QueryRow(ctx, `INSERT INTO payroll.pyr_thr_event (pte_company_id, pte_division, pte_kind, pte_yymm, pte_event_date, pte_description, pte_decision, pte_status, created_by, closed_by, closed_at)
VALUES ($1,$2::payroll.division,$3,$4::int,$5::date,$6,'GIVEN','CLOSED','etl','etl',now()) RETURNING pte_id`,
		o.CompanyID, k.division, k.kind, k.yymm, p.eventDate, p.desc).Scan(&eventID); err != nil {
		return fmt.Errorf("event: %w", err)
	}
	meta, err := json.Marshal(map[string]any{"lines": p.nLine, "unmapped_lines": p.unmapped, "sysid_collisions": p.coll, "notes": p.warnings})
	if err != nil {
		return err
	}
	var runID int64
	if err := l.tx.QueryRow(ctx, `INSERT INTO payroll.pyr_thr_run (ptr_event_id, ptr_kind, ptr_status, ptr_emp_count, ptr_tot_tunjangan, ptr_tot_potongan, ptr_warnings, ptr_started_by, ptr_finished_at)
VALUES ($1,'LEGACY','ACTIVE',$2,$3,$4,$5::jsonb,'etl',now()) RETURNING ptr_id`,
		eventID, p.nHead, p.totT.String(), p.totP.String(), string(meta)).Scan(&runID); err != nil {
		return fmt.Errorf("run: %w", err)
	}
	id := strconv.FormatInt(runID, 10)
	if err := stageInsert(ctx, l.tx, &p.heads, id, "pyr_tmp_thr_head", thrHeadCols, thrHeadTypes, `INSERT INTO payroll.pyr_thr_head (`+thrHeadCols+`)
SELECT `+thrHeadCols+` FROM pyr_tmp_thr_head`); err != nil {
		return fmt.Errorf("head: %w", err)
	}
	if err := stageInsert(ctx, l.tx, &p.lines, id, "pyr_tmp_thr_detail", thrDetailCols, thrDetailTypes, `INSERT INTO payroll.pyr_thr_detail (`+thrDetailCols+`)
SELECT ptd_run_id, ptd_employee_id, ptd_nik, ptd_component_sysid, ptd_type::payroll.comp_type, ptd_id, ptd_description, ptd_amount, ptd_tax, ptd_meta
FROM pyr_tmp_thr_detail`); err != nil {
		return fmt.Errorf("detail: %w", err)
	}
	if _, err := l.tx.Exec(ctx, `UPDATE payroll.pyr_thr_event SET pte_active_run_id=$2 WHERE pte_id=$1`, eventID, runID); err != nil {
		return fmt.Errorf("event aktif: %w", err)
	}
	if _, err := l.tx.Exec(ctx, `INSERT INTO payroll.pyr_audit_log (pal_actor, pal_action, pal_entity, pal_entity_key, pal_division, pal_after, pal_company_id)
VALUES ('etl','ETL_THR_LEGACY','pyr_thr_event',$1,$2,jsonb_build_object('run_id',$3::bigint,'kind','LEGACY','heads',$4::int,'lines',$5::int,
  'tot_tunjangan',$6::text,'tot_potongan',$7::text),$8)`,
		k.yymm+"/"+k.division+"/"+k.kind, k.division, runID, p.nHead, p.nLine, p.totT.String(), p.totP.String(), o.CompanyID); err != nil {
		return fmt.Errorf("audit: %w", err)
	}
	l.repM.DstRows += p.nHead
	l.repD.DstRows += p.nLine
	return nil
}

// checkLoaded handles an event that already exists: it is CLOSED and untouchable, but the source must still match.
func (l *thrLoader) checkLoaded(ctx context.Context, g *thrGroup, p *thrPlan, eventID int64) error {
	var nHead int
	var warn []byte
	err := l.tx.QueryRow(ctx, `SELECT ptr_emp_count, ptr_warnings FROM payroll.pyr_thr_run WHERE ptr_event_id=$1 AND ptr_kind='LEGACY' AND ptr_status='ACTIVE'`, eventID).Scan(&nHead, &warn)
	if errors.Is(err, pgx.ErrNoRows) {
		l.hold(g, "event sudah ada tetapi bukan hasil ETL legacy; tidak diubah")
		return nil
	}
	if err != nil {
		return err
	}
	var w struct {
		Lines int `json:"lines"`
	}
	if err := json.Unmarshal(warn, &w); err != nil {
		return err
	}
	if nHead != p.nHead || w.Lines != p.nLine {
		return fmt.Errorf("sudah dimuat (%d head, %d detail) tetapi sumber kini %d/%d; event CLOSED tidak boleh diubah", nHead, w.Lines, p.nHead, p.nLine)
	}
	l.repM.DstRows += nHead
	l.repD.DstRows += w.Lines
	return nil
}

// plan builds the COPY buffers of a validated event.
func (l *thrLoader) plan(g *thrGroup, p *thrPlan, headID map[string]int64, sysids map[sysKey]int64) error {
	for i, r := range g.heads {
		if err := l.planHead(i, r, p, headID); err != nil {
			return err
		}
	}
	if p.eventDate == "" {
		_, to, err := periodRange(g.key.yymm)
		if err != nil {
			return err
		}
		p.eventDate = to
		p.warnings = append(p.warnings, "TM_DATE kosong: tanggal event = akhir periode")
	}
	if p.desc == "" {
		p.desc = g.key.kind + " " + g.key.yymm
	}
	seenSys := map[string]bool{}
	for _, r := range g.lines {
		if err := l.planLine(r, p, headID, sysids, seenSys); err != nil {
			return err
		}
	}
	return nil
}

// planHead validates one MAIN row and appends it to the head COPY buffer.
func (l *thrLoader) planHead(i int, r []string, p *thrPlan, headID map[string]int64) error {
	mc := func(n string) string { return r[mustCol(l.mn, n)] }
	nik := mc("TM_NIK")
	join, err := parseDate(mc("TM_DATE_JOIN"))
	if err != nil {
		return fmt.Errorf("MAIN nik %s TM_DATE_JOIN: %w", nik, err)
	}
	if i == 0 { // event description/date = first MAIN row (14§4)
		if p.eventDate, err = parseDate(mc("TM_DATE")); err != nil {
			return fmt.Errorf("MAIN nik %s TM_DATE: %w", nik, err)
		}
		p.desc = mc("TM_DESCRIPTION")
	}
	tun, pot, rnd := zero0(mc("TM_TOT_TUNJANGAN")), zero0(mc("TM_TOT_POTONGAN")), zero0(mc("TM_ROUNDING"))
	if err := thrCheckMoney(nik, tun, pot, rnd); err != nil {
		return err
	}
	if tot := mc("TM_TOT_THR"); tot != "" {
		net := decimal.RequireFromString(tun).Sub(decimal.RequireFromString(pot)).Sub(decimal.RequireFromString(rnd))
		if t, err := decimal.NewFromString(tot); err == nil && !t.Equal(net) {
			p.warnings = append(p.warnings, fmt.Sprintf("nik %s: TM_TOT_THR %s != tunjangan-potongan-rounding %s", nik, t.String(), net.String()))
		}
	}
	copyRow(&p.heads, "@RUN@", strconv.FormatInt(headID[nik], 10), nik, mc("TM_NAMA"), mc("TM_DEPT"), mc("TM_EMP_TYPE"), join,
		mc("TM_TAX_CODE"), mc("TM_GAPOK"), tun, pot, rnd)
	p.totT.add(tun)
	p.totP.add(pot)
	p.nHead++
	return nil
}

// thrLineKind validates a DETAIL row's type, id and amount.
func (l *thrLoader) thrLineKind(r []string) (nik, typ string, id int, err error) {
	dc := func(n string) string { return r[mustCol(l.dt, n)] }
	nik = dc("TD_NIK")
	typ = strings.ToLower(dc("TD_JENIS"))
	id, aerr := strconv.Atoi(dc("TD_NO"))
	switch {
	case typ != typeTunjangan && typ != typePotongan:
		return nik, typ, id, fmt.Errorf("DETAIL nik %s TD_JENIS %q", nik, dc("TD_JENIS"))
	case aerr != nil:
		return nik, typ, id, fmt.Errorf("DETAIL nik %s TD_NO %q", nik, dc("TD_NO"))
	case dc("TD_AMOUNT") == "":
		return nik, typ, id, fmt.Errorf("DETAIL nik %s %s %d: TD_AMOUNT kosong", nik, typ, id)
	}
	if _, derr := decimal.NewFromString(dc("TD_AMOUNT")); derr != nil {
		return nik, typ, id, fmt.Errorf("DETAIL nik %s TD_AMOUNT %q", nik, dc("TD_AMOUNT"))
	}
	return nik, typ, id, nil
}

// planLine validates one DETAIL row, resolves its master component (D-56) and appends it to the line COPY buffer.
func (l *thrLoader) planLine(r []string, p *thrPlan, headID map[string]int64, sysids map[sysKey]int64, seenSys map[string]bool) error {
	dc := func(n string) string { return r[mustCol(l.dt, n)] }
	nik, typ, id, err := l.thrLineKind(r)
	if err != nil {
		return err
	}
	typName, tax := nameTunjangan, "t"
	if typ == typePotongan {
		typName, tax = namePotongan, "f"
	}
	sys, mapped := sysids[sysKey{typ, id}]
	meta := map[string]any{"legacy": true}
	sysCell := ""
	if mapped {
		sk := nik + "/" + strconv.FormatInt(sys, 10)
		if seenSys[sk] { // uix_ptd_comp: two legacy ids of one NIK share a master component
			mapped = false
			p.coll++
			meta["sysid_collision"] = sys
		} else {
			seenSys[sk] = true
			sysCell = strconv.FormatInt(sys, 10)
		}
	}
	if !mapped { // D-56: no master counterpart until B-THR
		meta["legacy_unmapped"] = true
		p.unmapped++
	}
	mj, merr := json.Marshal(meta)
	if merr != nil {
		return merr
	}
	copyRow(&p.lines, "@RUN@", strconv.FormatInt(headID[nik], 10), nik, sysCell, typName, strconv.Itoa(id), dc("TD_URAIAN"), dc("TD_AMOUNT"), tax, string(mj))
	p.nLine++
	return nil
}

func thrCheckMoney(nik string, cells ...string) error {
	for _, c := range cells {
		if _, err := decimal.NewFromString(c); err != nil {
			return fmt.Errorf("MAIN nik %s nominal %q bukan angka", nik, c)
		}
	}
	return nil
}

const (
	thrHeadCols = `pth_run_id, pth_employee_id, pth_nik, pth_nama, pth_dept, pth_emp_type, pth_date_join, pth_tax_code,
  pth_gapok, pth_tot_tunjangan, pth_tot_potongan, pth_rounding`
	thrHeadTypes = `bigint, bigint, text, text, text, text, date, text, numeric, numeric, numeric, numeric`

	thrDetailCols  = `ptd_run_id, ptd_employee_id, ptd_nik, ptd_component_sysid, ptd_type, ptd_id, ptd_description, ptd_amount, ptd_tax, ptd_meta`
	thrDetailTypes = `bigint, bigint, text, bigint, text, int, text, numeric, boolean, jsonb`
)
