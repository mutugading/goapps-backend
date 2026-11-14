package etl

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"
)

// Legacy component ids whose DETAIL rows carry the attendance-failure signal (D-33, 06§3).
const (
	legacyPremiHadir = 40 // T40 PREMI_HADIR: NULL amount or no row (OPEN period, D-46) = premi gagal (D10)
	legacyBeras      = 74 // T74 BERAS: tipe D = A − A/BD × BF
	berasDays        = 25 // BD (pyr_rule_constant BERAS_DAYS)
)

// AttLegacyFailReport counts what the reversal of T40/T74 produced (T009e3, D-33).
type AttLegacyFailReport struct {
	PremiDetail, BerasDetail int            // DETAIL rows T40 / T74 read (known yymm/division)
	PremiSet, PremiFailed    int            // pasr_premi_fail written / of which = 1
	PremiNoRow               int            // of PremiFailed: T40 attached (frz=2) but no DETAIL row, OPEN periods only (D-46)
	PremiNoRowBy             map[string]int // PremiNoRow per "yymm/division": a CLOSED period listed by mistake shows up here
	BerasSet                 int            // pasr_beras_fail written
	BerasResidue             []string       // tipe D whose BF is not an integer (or A missing): column left NULL
	NoSnapshotRow            []string       // T40/T74 rows without a REKAP snapshot row: nothing written
	Updated                  int            // snapshot rows touched
}

type failKey struct{ yymm, division, nik string }

type failVal struct {
	premi, beras *int
	noRow        bool // premi = 1 because DETAIL has no T40 row (D-46)
}

// AttLegacyFailOptions selects the periods (yymm) that were still OPEN when the spool was taken. Only there is the
// "T40 attached but no DETAIL row ⇒ PF=1" rule applied (D-46): COMPNT_EMP is the state at dump time (D-44), so on
// CLOSED periods it would invent failures. Empty = D-33 only.
type AttLegacyFailOptions struct {
	OpenPeriods []string
}

// berasFail reverses A − A/BD×BF = v (type D) into BF. ok=false: not an integer or A unusable (residue).
func berasFail(a, v decimal.Decimal) (int, bool) {
	if a.Sign() <= 0 {
		return 0, false
	}
	bf := a.Sub(v).Mul(decimal.NewFromInt(berasDays)).DivRound(a, 9)
	r := bf.Round(0)
	if bf.Sub(r).Abs().GreaterThan(decimal.New(1, -6)) || r.Sign() < 0 || r.GreaterThan(decimal.NewFromInt(berasDays)) {
		return 0, false
	}
	return int(r.IntPart()), true
}

// failCollector accumulates the reversed values per (yymm, division, nik) while DETAIL is scanned.
type failCollector struct {
	rep     *AttLegacyFailReport
	amounts map[failKey]string // T74 attachment amount (A) per nik/division
	premiOn map[failKey]bool   // T40 attached with frz=2 per nik/division (D-46)
	vals    map[failKey]*failVal
	order   []failKey
}

func (c *failCollector) get(k failKey) *failVal {
	if v := c.vals[k]; v != nil {
		return v
	}
	v := &failVal{}
	c.vals[k] = v
	c.order = append(c.order, k)
	return v
}

func (c *failCollector) loadAmounts(ce *Table) error {
	ci, err := bindCols(ce, []string{colMpceNIK, colMpceType, colMpceID, colMpceAmount, "MPCE_DIVISION"})
	if err != nil {
		return err
	}
	frzIdx := -1
	if c.premiOn != nil {
		if frzIdx, err = ce.Col("MPCE_FRZ_FLAG"); err != nil {
			return err
		}
	}
	for _, r := range ce.Rows {
		id, aerr := strconv.Atoi(r[ci[colMpceID]])
		if aerr != nil || !strings.EqualFold(r[ci[colMpceType]], "Tunjangan") {
			continue
		}
		k := failKey{"", r[ci["MPCE_DIVISION"]], r[ci[colMpceNIK]]}
		switch {
		case id == legacyBeras:
			c.amounts[k] = r[ci[colMpceAmount]]
		case id == legacyPremiHadir && frzIdx >= 0:
			// empty = 2, as LoadEmployeeComponents; several T40 rows for one nik: any frz=2 row counts.
			if frz := strings.TrimSpace(r[frzIdx]); frz == "2" || frz == "" {
				c.premiOn[k] = true
			}
		}
	}
	return nil
}

// markMissingPremi applies D-46 after DETAIL is scanned: a MAIN nik of an OPEN period with T40 attached (frz=2)
// and no DETAIL T40 row gets PF=1. A DETAIL row (NULL or not) keeps the D-33 value.
func (c *failCollector) markMissingPremi(mn *Table, open map[string]bool) error {
	mi, err := bindCols(mn, []string{"PM_YYMM", "PM_NIK", "PM_DIVISION"})
	if err != nil {
		return err
	}
	for _, r := range mn.Rows {
		k := failKey{r[mi["PM_YYMM"]], r[mi["PM_DIVISION"]], r[mi["PM_NIK"]]}
		if !open[k.yymm] || !c.premiOn[failKey{"", k.division, k.nik}] {
			continue
		}
		if v := c.vals[k]; v != nil && v.premi != nil {
			continue // DETAIL row present (or MAIN duplicated)
		}
		pf := 1
		v := c.get(k)
		v.premi, v.noRow = &pf, true
	}
	return nil
}

func (c *failCollector) scanDetail(dt *Table) error {
	di, err := bindCols(dt, []string{colPdYymm, "PD_NIK", "PD_NO", "PD_TYPE", "PD_AMOUNT", "PD_STATUS", "PD_DIVISION"})
	if err != nil {
		return err
	}
	for _, r := range dt.Rows {
		id, aerr := strconv.Atoi(r[di["PD_NO"]])
		if strings.ToLower(r[di["PD_TYPE"]]) != typeTunjangan || aerr != nil || badYYMM(r, di, []string{colPdYymm}) != "" {
			continue
		}
		k := failKey{r[di[colPdYymm]], r[di["PD_DIVISION"]], r[di["PD_NIK"]]}
		amt := strings.TrimSpace(r[di["PD_AMOUNT"]])
		switch id {
		case legacyPremiHadir:
			c.premi(k, amt)
		case legacyBeras:
			c.beras(k, amt, r[di["PD_STATUS"]])
		}
	}
	return nil
}

func (c *failCollector) premi(k failKey, amt string) {
	c.rep.PremiDetail++
	if v := c.get(k); v.premi == nil {
		pf := 0
		if amt == "" {
			pf = 1
		}
		v.premi = &pf
	}
}

func (c *failCollector) beras(k failKey, amt, sts string) {
	c.rep.BerasDetail++
	v := c.get(k)
	if v.beras != nil {
		return
	}
	where := fmt.Sprintf("%s/%s nik %s", k.yymm, k.division, k.nik)
	if sts == "F" {
		bf := 0
		v.beras = &bf
		return
	}
	a, aerr := decimal.NewFromString(c.amounts[failKey{"", k.division, k.nik}])
	x, xerr := decimal.NewFromString(amt)
	if aerr != nil || xerr != nil {
		c.rep.BerasResidue = append(c.rep.BerasResidue, where+": A atau nilai DETAIL kosong")
		return
	}
	bf, ok := berasFail(a, x)
	if !ok {
		c.rep.BerasResidue = append(c.rep.BerasResidue, fmt.Sprintf("%s: A=%s nilai=%s ⇒ BF bukan integer", where, a, x))
		return
	}
	v.beras = &bf
}

const updateFailSQL = `
UPDATE payroll.pyr_att_snapshot_row r SET pasr_premi_fail = coalesce($4::int, r.pasr_premi_fail), pasr_beras_fail = coalesce($5::int, r.pasr_beras_fail)
FROM payroll.pyr_att_snapshot s
WHERE r.pasr_snapshot_id = s.pas_snapshot_id AND s.pas_source_batch_ids = '{}' AND s.pas_yymm = $1::int AND s.pas_division = $2::payroll.division
  AND r.pasr_kind = 'REKAP' AND r.pasr_nik = $3
  AND NOT EXISTS (SELECT 1 FROM payroll.pyr_run x WHERE x.pr_company_id = s.pas_company_id AND x.pr_yymm = s.pas_yymm
                  AND x.pr_division = s.pas_division AND x.pr_status = 'ACTIVE' AND x.pr_kind = 'ENGINE')`

// lockRuns takes the run advisory lock (same key as run.Lock, D-39) of every period touched, in sorted order
// (no deadlock between ETLs), before the first UPDATE: the NOT EXISTS in updateFailSQL sees the statement snapshot,
// so a Commit that finishes while an UPDATE waits on a row lock would be missed (T017f/T017g, D-40).
func (c *failCollector) lockRuns(ctx context.Context, tx *sql.Tx) error {
	seen := map[[2]string]bool{}
	var periods [][2]string
	for _, k := range c.order {
		p := [2]string{k.yymm, k.division}
		if !seen[p] {
			seen[p] = true
			periods = append(periods, p)
		}
	}
	sort.Slice(periods, func(i, j int) bool {
		if periods[i][0] != periods[j][0] {
			return periods[i][0] < periods[j][0]
		}
		return periods[i][1] < periods[j][1]
	})
	for _, p := range periods {
		if _, err := tx.ExecContext(ctx, `
SELECT pg_advisory_xact_lock(hashtext('payroll.run'), hashtext(k))
FROM (SELECT DISTINCT pas_company_id::text || ':' || pas_yymm::text || ':' || pas_division::text AS k
      FROM payroll.pyr_att_snapshot WHERE pas_source_batch_ids = '{}' AND pas_yymm = $1::int AND pas_division = $2::payroll.division
      ORDER BY 1) q`, p[0], p[1]); err != nil {
			return fmt.Errorf("run lock %s/%s: %w", p[0], p[1], err)
		}
	}
	return nil
}

func (c *failCollector) write(ctx context.Context, tx *sql.Tx) error {
	if err := c.lockRuns(ctx, tx); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, updateFailSQL)
	if err != nil {
		return err
	}
	defer closeQuiet(stmt)
	for _, k := range c.order {
		v := c.vals[k]
		if v.premi == nil && v.beras == nil {
			continue // residue only: column stays NULL
		}
		res, err := stmt.ExecContext(ctx, k.yymm, k.division, k.nik, nullInt(v.premi), nullInt(v.beras))
		if err != nil {
			return fmt.Errorf("%s/%s nik %s: %w", k.yymm, k.division, k.nik, err)
		}
		n, _ := res.RowsAffected() //nolint:errcheck // pgx always reports
		if n == 0 {
			c.rep.NoSnapshotRow = append(c.rep.NoSnapshotRow, fmt.Sprintf("%s/%s nik %s", k.yymm, k.division, k.nik))
			continue
		}
		c.rep.Updated += int(n)
		c.count(k, v)
	}
	return nil
}

// count adds one written snapshot row to the report.
func (c *failCollector) count(k failKey, v *failVal) {
	if v.beras != nil {
		c.rep.BerasSet++
	}
	if v.premi == nil {
		return
	}
	c.rep.PremiSet++
	c.rep.PremiFailed += *v.premi
	if !v.noRow {
		return
	}
	c.rep.PremiNoRow++
	if c.rep.PremiNoRowBy == nil {
		c.rep.PremiNoRowBy = map[string]int{}
	}
	c.rep.PremiNoRowBy[k.yymm+"/"+k.division]++
}

// openSet validates the OPEN periods (yymm domain) and returns them as a set (empty when none).
func openSet(periods []string) (map[string]bool, error) {
	set := make(map[string]bool, len(periods))
	idx := map[string]int{"OPEN_YYMM": 0}
	for _, p := range periods {
		p = strings.TrimSpace(p)
		if e := badYYMM([]string{p}, idx, []string{"OPEN_YYMM"}); e != "" {
			return nil, fmt.Errorf("periode OPEN: %s", e)
		}
		set[p] = true
	}
	return set, nil
}

// RunAttLegacyFail fills pasr_premi_fail/pasr_beras_fail on the LEGACY REKAP rows from DETAIL T40/T74 (D-33), plus
// PF=1 for T40 without a DETAIL row in o.OpenPeriods (D-46, needs MAIN). One transaction, idempotent (same input =
// same values). Needs att-legacy (T009d) loaded; DETAIL/COMPNT_EMP/MAIN are read from the PSV spool, so
// payroll_etl never needs SELECT on salary tables.
func RunAttLegacyFail(ctx context.Context, db *sql.DB, src Source, o AttLegacyFailOptions) (AttLegacyFailReport, error) {
	var rep AttLegacyFailReport
	open, err := openSet(o.OpenPeriods)
	if err != nil {
		return rep, err
	}
	dt, err := src.Read(TableSalaryDetail)
	if err != nil {
		return rep, err
	}
	ce, err := src.Read(TableEmployeeComponent)
	if err != nil {
		return rep, err
	}
	c := &failCollector{rep: &rep, amounts: map[failKey]string{}, vals: map[failKey]*failVal{}}
	if len(open) > 0 {
		c.premiOn = map[failKey]bool{}
	}
	if err := c.loadAmounts(ce); err != nil {
		return rep, err
	}
	if err := c.scanDetail(dt); err != nil {
		return rep, err
	}
	if len(open) > 0 {
		mn, err := src.Read(TableSalaryMain)
		if err != nil {
			return rep, err
		}
		if err := c.markMissingPremi(mn, open); err != nil {
			return rep, err
		}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return rep, err
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			log.Warn().Err(rbErr).Msg("etl: rollback failed")
		}
	}()
	if err := c.write(ctx, tx); err != nil {
		return rep, err
	}
	return rep, tx.Commit()
}

func nullInt(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}
