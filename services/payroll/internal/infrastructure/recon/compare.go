package recon

import (
	"fmt"
	"sort"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/etl"
)

// Field names of a Diff: the figures compared per (yymm, division). Detail Σ is per type (T/P).
const (
	FieldDetailT     = "detail_t"
	FieldDetailP     = "detail_p"
	FieldDetailLines = "detail_lines"
	FieldHeadTunj    = "head_tot_tunjangan"
	FieldHeadPot     = "head_tot_potongan"
	FieldHeadNet     = "head_net_pay"
	FieldHeadRows    = "head_rows"
)

// Diff is one figure that differs between the DB and the dump. A period present on one side only yields a diff for
// every non-zero figure of that side (the other side counts as 0). Values are decimal strings (counts are integers).
type Diff struct {
	YYMM     int    `json:"yymm"`
	Division string `json:"division"`
	Field    string `json:"field"`
	DB       string `json:"db"`
	Dump     string `json:"dump"`
	Delta    string `json:"delta"` // dump - db
	// Where tells which side holds the period when only one does: "db_only" | "dump_only" | "" (both).
	Where string `json:"where,omitempty"`
}

// SkippedPeriod is a D-35.1 period of the dump left out of the migration; reported on its own, not a difference.
type SkippedPeriod struct {
	YYMM       int    `json:"yymm"`
	Division   string `json:"division"`
	MainRows   int64  `json:"main_rows"`
	DetailRows int64  `json:"detail_rows"`
	DetailT    string `json:"detail_t"`
	DetailP    string `json:"detail_p"`
}

// SyntheticPeriod counts the D-35.2 synthetic MAIN rows the dump implies for a period (informational).
type SyntheticPeriod struct {
	YYMM     int    `json:"yymm"`
	Division string `json:"division"`
	Rows     int64  `json:"rows"`
}

// Comparison is the DB-vs-dump result written under "dump_compare". Aggregates only (D-72 (3)).
type Comparison struct {
	DumpPeriods     int               `json:"dump_periods"`
	DBPeriods       int               `json:"db_periods"`
	MatchedPeriods  int               `json:"matched_periods"` // periods on both sides with no diff
	Diffs           []Diff            `json:"diffs"`           // must be empty
	SkippedD351     []SkippedPeriod   `json:"skipped_d35_1"`   // out of scope by D-35.1, not a diff
	SyntheticD352   []SyntheticPeriod `json:"synthetic_main_d35_2"`
	RejectedRowsSrc int64             `json:"rejected_dump_rows"` // dump rows with an invalid yymm/division key (loader exceptions)
}

// OK reports whether the DB equals the dump (no diff).
func (c Comparison) OK() bool { return len(c.Diffs) == 0 }

type figure struct {
	field string
	db    decimal.Decimal
	dump  decimal.Decimal
}

// Compare diffs the DB report with the dump aggregate, per (yymm, division) and per figure. Both inputs must cover the
// same yymm range (the dump is cut to it by etl.AggregateSalaryLegacy). Pure function: no I/O.
func Compare(rep Report, dump etl.LegacyDump) Comparison {
	c := Comparison{DumpPeriods: len(dump.Periods), DBPeriods: len(rep.Periods), Diffs: []Diff{},
		SkippedD351: []SkippedPeriod{}, SyntheticD352: []SyntheticPeriod{}, RejectedRowsSrc: dump.Rejected}
	dbIdx := map[key]PeriodAgg{}
	for _, p := range rep.Periods {
		dbIdx[key{p.YYMM, p.Division}] = p
	}
	dumpIdx := map[key]etl.LegacyAgg{}
	for _, a := range dump.Periods {
		dumpIdx[key{a.YYMM, a.Division}] = a
		if a.Synthetic > 0 {
			c.SyntheticD352 = append(c.SyntheticD352, SyntheticPeriod{YYMM: a.YYMM, Division: a.Division, Rows: a.Synthetic})
		}
	}
	for _, s := range dump.Skipped {
		c.SkippedD351 = append(c.SkippedD351, SkippedPeriod{YYMM: s.YYMM, Division: s.Division, MainRows: s.MainRows,
			DetailRows: s.DetailRows, DetailT: s.DetailT.StringFixed(2), DetailP: s.DetailP.StringFixed(2)})
	}
	for _, k := range unionKeys(dbIdx, dumpIdx) {
		db, inDB := dbIdx[k]
		du, inDump := dumpIdx[k]
		n := len(c.Diffs)
		c.Diffs = append(c.Diffs, periodDiffs(k, db, du, whereOnly(inDB, inDump))...)
		if inDB && inDump && len(c.Diffs) == n {
			c.MatchedPeriods++
		}
	}
	return c
}

// whereOnly names the side holding a period when only one side does ("" = both).
func whereOnly(inDB, inDump bool) string {
	switch {
	case inDB && !inDump:
		return "db_only"
	case !inDB && inDump:
		return "dump_only"
	}
	return ""
}

func unionKeys(a map[key]PeriodAgg, b map[key]etl.LegacyAgg) []key {
	seen := map[key]bool{}
	keys := make([]key, 0, len(a)+len(b))
	for k := range a {
		seen[k] = true
		keys = append(keys, k)
	}
	for k := range b {
		if !seen[k] {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].yymm < keys[j].yymm || (keys[i].yymm == keys[j].yymm && keys[i].division < keys[j].division)
	})
	return keys
}

func periodDiffs(k key, db PeriodAgg, du etl.LegacyAgg, where string) []Diff {
	var out []Diff
	for _, f := range figures(db, du) {
		if f.db.Equal(f.dump) {
			continue
		}
		out = append(out, Diff{YYMM: k.yymm, Division: k.division, Field: f.field, DB: fmtFigure(f.field, f.db),
			Dump: fmtFigure(f.field, f.dump), Delta: fmtFigure(f.field, f.dump.Sub(f.db)), Where: where})
	}
	return out
}

func figures(db PeriodAgg, du etl.LegacyAgg) []figure {
	return []figure{
		{FieldDetailT, db.DetailT, du.DetailT},
		{FieldDetailP, db.DetailP, du.DetailP},
		{FieldDetailLines, decimal.NewFromInt(db.DetailLines), decimal.NewFromInt(du.DetailLines)},
		{FieldHeadTunj, db.HeadTunjangan, du.HeadTunj},
		{FieldHeadPot, db.HeadPotongan, du.HeadPot},
		{FieldHeadNet, db.HeadNet, du.HeadNet},
		{FieldHeadRows, decimal.NewFromInt(db.HeadRows), decimal.NewFromInt(du.HeadRows)},
	}
}

func fmtFigure(field string, d decimal.Decimal) string {
	if field == FieldDetailLines || field == FieldHeadRows {
		return d.String()
	}
	return d.StringFixed(2)
}

// CompareDump reads the dump with the loader's own parser/filters and diffs it against rep, then records the result in
// rep.DumpCompare and, when it differs, a failure line (so rep.OK() turns false and the command exits 1).
func CompareDump(rep *Report, src etl.Source) error {
	d, err := etl.AggregateSalaryLegacy(src, rep.YYMMFrom, rep.YYMMTo)
	if err != nil {
		return fmt.Errorf("dump: %w", err)
	}
	c := Compare(*rep, d)
	rep.DumpCompare = &c
	if !c.OK() {
		rep.Failures = append(rep.Failures, fmt.Sprintf("%d selisih agregat DB vs dump (lihat dump_compare.diffs)", len(c.Diffs)))
	}
	return nil
}
