package etl

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"
)

// Legacy component ids whose DETAIL rows carry the attendance-failure signal (D-33, 06§3).
const (
	legacyPremiHadir = 40 // T40 PREMI_HADIR: NULL amount = premi gagal (D10)
	legacyBeras      = 74 // T74 BERAS: tipe D = A − A/BD × BF
	berasDays        = 25 // BD (pyr_rule_constant BERAS_DAYS)
)

// AttLegacyFailReport counts what the reversal of T40/T74 produced (T009e3, D-33).
type AttLegacyFailReport struct {
	PremiDetail, BerasDetail int      // DETAIL rows T40 / T74 read (known yymm/division)
	PremiSet, PremiFailed    int      // pasr_premi_fail written / of which = 1
	BerasSet                 int      // pasr_beras_fail written
	BerasResidue             []string // tipe D whose BF is not an integer (or A missing): column left NULL
	NoSnapshotRow            []string // T40/T74 rows without a REKAP snapshot row: nothing written
	Updated                  int      // snapshot rows touched
}

type failKey struct{ yymm, division, nik string }

type failVal struct {
	premi, beras *int
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
	ci, err := bindCols(ce, []string{"MPCE_NIK", "MPCE_TYPE", "MPCE_ID", "MPCE_AMOUNT", "MPCE_DIVISION"})
	if err != nil {
		return err
	}
	for _, r := range ce.Rows {
		id, aerr := strconv.Atoi(r[ci["MPCE_ID"]])
		if aerr == nil && id == legacyBeras && strings.EqualFold(r[ci["MPCE_TYPE"]], "Tunjangan") {
			c.amounts[failKey{"", r[ci["MPCE_DIVISION"]], r[ci["MPCE_NIK"]]}] = r[ci["MPCE_AMOUNT"]]
		}
	}
	return nil
}

func (c *failCollector) scanDetail(dt *Table) error {
	di, err := bindCols(dt, []string{"PD_YYMM", "PD_NIK", "PD_NO", "PD_TYPE", "PD_AMOUNT", "PD_STATUS", "PD_DIVISION"})
	if err != nil {
		return err
	}
	for _, r := range dt.Rows {
		id, aerr := strconv.Atoi(r[di["PD_NO"]])
		if strings.ToLower(r[di["PD_TYPE"]]) != typeTunjangan || aerr != nil || badYYMM(r, di, []string{"PD_YYMM"}) != "" {
			continue
		}
		k := failKey{r[di["PD_YYMM"]], r[di["PD_DIVISION"]], r[di["PD_NIK"]]}
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
  AND r.pasr_kind = 'REKAP' AND r.pasr_nik = $3`

func (c *failCollector) write(ctx context.Context, tx *sql.Tx) error {
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
		if v.premi != nil {
			c.rep.PremiSet++
			c.rep.PremiFailed += *v.premi
		}
		if v.beras != nil {
			c.rep.BerasSet++
		}
	}
	return nil
}

// RunAttLegacyFail fills pasr_premi_fail/pasr_beras_fail on the LEGACY REKAP rows from DETAIL T40/T74 (D-33).
// One transaction, idempotent (same input = same values). Needs att-legacy (T009d) loaded; DETAIL/COMPNT_EMP are read
// from the PSV spool, so payroll_etl never needs SELECT on salary tables.
func RunAttLegacyFail(ctx context.Context, db *sql.DB, src Source) (AttLegacyFailReport, error) {
	var rep AttLegacyFailReport
	dt, err := src.Read(TableSalaryDetail)
	if err != nil {
		return rep, err
	}
	ce, err := src.Read(TableEmployeeComponent)
	if err != nil {
		return rep, err
	}
	c := &failCollector{rep: &rep, amounts: map[failKey]string{}, vals: map[failKey]*failVal{}}
	if err := c.loadAmounts(ce); err != nil {
		return rep, err
	}
	if err := c.scanDetail(dt); err != nil {
		return rep, err
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
