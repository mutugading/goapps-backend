package etl

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
)

// LegacyAgg is the aggregate of one (yymm, division) of the MAIN/DETAIL dump as RunSalaryLegacy would load it
// (T032b, D-72): amounts are rounded to 2 decimals per cell exactly like the numeric(18,2) columns, then summed.
type LegacyAgg struct {
	YYMM     int
	Division string
	// DetailT / DetailP: Σ PD_AMOUNT per type; DetailLines: DETAIL rows of the period.
	DetailT, DetailP decimal.Decimal
	DetailLines      int64
	// HeadTunj / HeadPot / HeadNet: Σ of the MAIN rows (net = tunjangan - potongan - rounding, as psh_net_pay);
	// HeadRows counts real and synthetic MAIN rows, Synthetic only the D-35.2 ones.
	HeadTunj, HeadPot, HeadNet decimal.Decimal
	HeadRows, Synthetic        int64
}

// LegacySkip is a period left out of the migration by D-35.1: reported on its own, never compared with the DB.
type LegacySkip struct {
	YYMM             int
	Division         string
	MainRows         int64
	DetailRows       int64
	DetailT, DetailP decimal.Decimal
}

// LegacyDump is the result of AggregateSalaryLegacy.
type LegacyDump struct {
	Periods []LegacyAgg
	Skipped []LegacySkip
	// Rejected counts rows with an invalid yymm/division key (the loader reports them as exceptions, nothing is loaded).
	Rejected int64
}

// AggregateSalaryLegacy reads MGT_PAYROLL_MAIN/DETAIL from src and aggregates what RunSalaryLegacy would load, for
// yymmFrom..yymmTo (inclusive). It reuses the loader's key check, the D-35.1 skip and the D-35.2 synthetic MAIN
// (orphan NIKs), so the figures describe the same population as the DB. Read-only; no NIK leaves this function.
//
//nolint:gocognit,gocyclo // mirrors the loader's read -> group -> total pipeline, kept linear on purpose
func AggregateSalaryLegacy(src Source, yymmFrom, yymmTo int) (LegacyDump, error) {
	var out LegacyDump
	mn, err := src.Read(TableSalaryMain)
	if err != nil {
		return out, err
	}
	dt, err := src.Read(TableSalaryDetail)
	if err != nil {
		return out, err
	}
	mix, err := bindCols(mn, []string{colPmYymm, colPmNIK, colPmDivision, "PM_TOT_TUNJANGAN", "PM_TOT_POTONGAN", "PM_ROUNDING"})
	if err != nil {
		return out, err
	}
	dix, err := bindCols(dt, []string{colPdYymm, colPdNIK, colPdType, "PD_AMOUNT", colPdDivision})
	if err != nil {
		return out, err
	}
	inRange := func(k snapKey) bool {
		n, _ := strconv.Atoi(k.yymm) //nolint:errcheck // key validated by badKey
		return n >= yymmFrom && n <= yymmTo
	}
	groups := map[snapKey]*salaryGroup{}
	skips := map[snapKey]*LegacySkip{}
	grp := func(k snapKey) *salaryGroup {
		if groups[k] == nil {
			groups[k] = &salaryGroup{key: k}
		}
		return groups[k]
	}
	skip := func(k snapKey) *LegacySkip {
		if skips[k] == nil {
			n, _ := strconv.Atoi(k.yymm) //nolint:errcheck // key validated by badKey
			skips[k] = &LegacySkip{YYMM: n, Division: k.division}
		}
		return skips[k]
	}
	for _, r := range mn.Rows {
		if badKey(r, mix, colPmYymm, colPmDivision) != "" {
			out.Rejected++
			continue
		}
		k := snapKey{r[mix[colPmYymm]], r[mix[colPmDivision]]}
		switch {
		case !inRange(k):
		case legacySkipped(k):
			skip(k).MainRows++
		default:
			g := grp(k)
			g.heads = append(g.heads, r)
		}
	}
	for _, r := range dt.Rows {
		if badKey(r, dix, colPdYymm, colPdDivision) != "" {
			out.Rejected++
			continue
		}
		k := snapKey{r[dix[colPdYymm]], r[dix[colPdDivision]]}
		switch {
		case !inRange(k):
		case legacySkipped(k):
			s := skip(k)
			s.DetailRows++
			t, p, err := lineAmount(r, dix)
			if err != nil {
				return out, fmt.Errorf("%s %s/%s: %w", dt.Name, k.yymm, k.division, err)
			}
			s.DetailT, s.DetailP = s.DetailT.Add(t), s.DetailP.Add(p)
		default:
			g := grp(k)
			g.lines = append(g.lines, r)
		}
	}
	for k, g := range groups {
		a, err := aggregateGroup(g, mix, dix)
		if err != nil {
			return out, fmt.Errorf("%s/%s: %w", k.yymm, k.division, err)
		}
		out.Periods = append(out.Periods, a)
	}
	sort.Slice(out.Periods, func(i, j int) bool {
		a, b := out.Periods[i], out.Periods[j]
		return a.YYMM < b.YYMM || (a.YYMM == b.YYMM && a.Division < b.Division)
	})
	for _, s := range skips {
		out.Skipped = append(out.Skipped, *s)
	}
	sort.Slice(out.Skipped, func(i, j int) bool {
		a, b := out.Skipped[i], out.Skipped[j]
		return a.YYMM < b.YYMM || (a.YYMM == b.YYMM && a.Division < b.Division)
	})
	return out, nil
}

func aggregateGroup(g *salaryGroup, mix, dix map[string]int) (LegacyAgg, error) {
	yymm, _ := strconv.Atoi(g.key.yymm) //nolint:errcheck // key validated by badKey
	a := LegacyAgg{YYMM: yymm, Division: g.key.division, DetailLines: int64(len(g.lines))}
	for _, r := range g.lines {
		t, p, err := lineAmount(r, dix)
		if err != nil {
			return a, err
		}
		a.DetailT, a.DetailP = a.DetailT.Add(t), a.DetailP.Add(p)
	}
	for _, r := range g.heads {
		tunj, err := cellMoney(r[mix["PM_TOT_TUNJANGAN"]])
		if err != nil {
			return a, fmt.Errorf("PM_TOT_TUNJANGAN: %w", err)
		}
		pot, err := cellMoney(r[mix["PM_TOT_POTONGAN"]])
		if err != nil {
			return a, fmt.Errorf("PM_TOT_POTONGAN: %w", err)
		}
		rnd, err := cellMoney(r[mix["PM_ROUNDING"]])
		if err != nil {
			return a, fmt.Errorf("PM_ROUNDING: %w", err)
		}
		a.HeadTunj, a.HeadPot, a.HeadNet = a.HeadTunj.Add(tunj), a.HeadPot.Add(pot), a.HeadNet.Add(tunj.Sub(pot).Sub(rnd))
		a.HeadRows++
	}
	// D-35.2: one synthetic MAIN per NIK with DETAIL but no MAIN: Σ DETAIL per type rounded once (not per line), rounding 0.
	for _, nik := range orphanNiks(g) {
		var t, p decimal.Decimal
		for _, r := range g.lines {
			if r[1] != nik {
				continue
			}
			ta, pa, err := rawLineAmount(r, dix)
			if err != nil {
				return a, err
			}
			t, p = t.Add(ta), p.Add(pa)
		}
		t, p = t.Round(2), p.Round(2)
		a.HeadTunj, a.HeadPot, a.HeadNet = a.HeadTunj.Add(t), a.HeadPot.Add(p), a.HeadNet.Add(t.Sub(p))
		a.HeadRows++
		a.Synthetic++
	}
	return a, nil
}

// cellMoney parses a money cell like the numeric(18,2) column stores it: empty = 0, rounded half away from zero.
func cellMoney(s string) (decimal.Decimal, error) {
	if s == "" {
		return decimal.Zero, nil
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero, fmt.Errorf("nominal %q: %w", s, err)
	}
	return d.Round(2), nil
}

// rawLineAmount returns the unrounded PD_AMOUNT in the (tunjangan, potongan) slot of the line's type; the loader
// treats every non-potongan type as tunjangan in the synthetic MAIN.
func rawLineAmount(r []string, dix map[string]int) (t, p decimal.Decimal, err error) {
	cell := r[dix["PD_AMOUNT"]]
	if cell == "" {
		return t, p, nil
	}
	a, err := decimal.NewFromString(cell)
	if err != nil {
		return t, p, fmt.Errorf("PD_AMOUNT %q: %w", cell, err)
	}
	if strings.EqualFold(r[dix[colPdType]], typePotongan) {
		return t, a, nil
	}
	return a, p, nil
}

// lineAmount is rawLineAmount rounded to 2 decimals (what psd_amount holds).
func lineAmount(r []string, dix map[string]int) (t, p decimal.Decimal, err error) {
	t, p, err = rawLineAmount(r, dix)
	return t.Round(2), p.Round(2), err
}
