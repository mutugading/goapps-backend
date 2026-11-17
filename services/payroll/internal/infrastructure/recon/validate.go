// Package recon holds the read-only migration validator (D-72, 08§6 Postgres side): aggregates of the LEGACY data
// loaded by the ETL, to be compared with the Oracle dump. The report carries no NIK, name, or per-person amount.
package recon

import (
	"context"
	"database/sql"
	"fmt"
	"sort"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/scope"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/postgres"
)

// Query selects the company and the period range (inclusive, yymm) of the validation.
type Query struct {
	CompanyID int16
	YYMMFrom  int
	YYMMTo    int
}

// PeriodAgg is the Σ of one LEGACY ACTIVE run (one per company/yymm/division). Amounts are exact decimals.
type PeriodAgg struct {
	YYMM     int    `json:"yymm"`
	Division string `json:"division"`
	// DetailT / DetailP: Σ psd_amount (08§6 "Σ PD_AMOUNT per yymm/div/type"); lines: row count of the detail.
	DetailT     decimal.Decimal `json:"detail_t"`
	DetailP     decimal.Decimal `json:"detail_p"`
	DetailLines int64           `json:"detail_lines"`
	// Head: Σ PM_TOT_TUNJANGAN, PM_TOT_POTONGAN and net (08§6); HeadRows = number of MAIN rows.
	HeadTunjangan decimal.Decimal `json:"head_tot_tunjangan"`
	HeadPotongan  decimal.Decimal `json:"head_tot_potongan"`
	HeadNet       decimal.Decimal `json:"head_net_pay"`
	HeadRows      int64           `json:"head_rows"`
}

// LevelCount is the number of active (pe_sts_payroll = 2) employees of one level in a division.
type LevelCount struct {
	Division string `json:"division"`
	Level    string `json:"level"` // "" = employee without level
	Active   int64  `json:"active"`
}

// UnmappedComponent is a legacy (type,id,division) of the detail lines without a pyr_component_legacy_map row.
type UnmappedComponent struct {
	Type     string `json:"type"` // T | P
	ID       int    `json:"id"`
	Division string `json:"division"`
	Lines    int64  `json:"lines"`
	// Aliased marks a D-36 alias id (100/101/200/201): in the unmapped list it means the alias target T70/T71/P70/P71 is
	// missing from the map; in AliasedD36 it means the line is covered through the alias.
	Aliased bool `json:"aliased_d36"`
}

// BirthDateGap counts employees without pe_birth_date per division (08§6, derived from PE_PASSWORD by the ETL).
type BirthDateGap struct {
	Division      string `json:"division"`
	Employees     int64  `json:"employees"`
	Missing       int64  `json:"missing"`
	MissingActive int64  `json:"missing_active"`
}

// PlainPIIColumn is a PII-looking column whose type is not bytea (ciphertext), 08§6 "Enkripsi PII".
type PlainPIIColumn struct {
	Schema   string `json:"schema"`
	Table    string `json:"table"`
	Column   string `json:"column"`
	DataType string `json:"data_type"`
}

// Totals are the Σ over all periods of the range, convenient for a first comparison with the dump.
type Totals struct {
	DetailT       decimal.Decimal `json:"detail_t"`
	DetailP       decimal.Decimal `json:"detail_p"`
	HeadTunjangan decimal.Decimal `json:"head_tot_tunjangan"`
	HeadPotongan  decimal.Decimal `json:"head_tot_potongan"`
	HeadNet       decimal.Decimal `json:"head_net_pay"`
}

// Report is the JSON written by `recon validate`.
type Report struct {
	CompanyID           int16               `json:"company_id"`
	YYMMFrom            int                 `json:"yymm_from"`
	YYMMTo              int                 `json:"yymm_to"`
	Divisions           []string            `json:"divisions"`
	Periods             []PeriodAgg         `json:"periods"`
	Totals              Totals              `json:"totals"`
	ActiveByLevel       []LevelCount        `json:"active_by_level"`
	UnmappedLegacyLines int64               `json:"unmapped_legacy_lines"` // must be 0
	UnmappedComponents  []UnmappedComponent `json:"unmapped_components"`
	AliasedD36          []UnmappedComponent `json:"aliased_d36_components"` // covered via D-36 alias (informational)
	BirthDate           []BirthDateGap      `json:"birth_date"`
	PlaintextPII        []PlainPIIColumn    `json:"plaintext_pii_columns"` // must be empty
	Failures            []string            `json:"failures"`              // violated "must" rules; empty = OK
}

// OK reports whether every hard rule holds (unmapped = 0, no plaintext PII column).
func (r Report) OK() bool { return len(r.Failures) == 0 }

// raw is what the SQL layer returns; Build turns it into the report without I/O (unit-testable).
type raw struct {
	divisions []string
	detail    []detailRow
	head      []headRow
	levels    []LevelCount
	unmapped  []UnmappedComponent
	aliased   []UnmappedComponent
	birth     []BirthDateGap
	pii       []PlainPIIColumn
}

type detailRow struct {
	yymm     int
	division string
	typ      string // T | P
	sum      decimal.Decimal
	lines    int64
}

type headRow struct {
	yymm           int
	division       string
	tunj, pot, net decimal.Decimal
	rows           int64
}

type key struct {
	yymm     int
	division string
}

// build merges the raw rows into the report: one PeriodAgg per (yymm, division), grand totals, failure list.
func build(q Query, r raw) Report {
	rep := Report{CompanyID: q.CompanyID, YYMMFrom: q.YYMMFrom, YYMMTo: q.YYMMTo, Divisions: r.divisions,
		ActiveByLevel: r.levels, UnmappedComponents: r.unmapped, AliasedD36: r.aliased, BirthDate: r.birth, PlaintextPII: r.pii}
	idx := map[key]*PeriodAgg{}
	get := func(y int, d string) *PeriodAgg {
		k := key{y, d}
		if p, ok := idx[k]; ok {
			return p
		}
		p := &PeriodAgg{YYMM: y, Division: d}
		idx[k] = p
		return p
	}
	for _, d := range r.detail {
		p := get(d.yymm, d.division)
		if d.typ == "T" {
			p.DetailT = p.DetailT.Add(d.sum)
		} else {
			p.DetailP = p.DetailP.Add(d.sum)
		}
		p.DetailLines += d.lines
	}
	for _, h := range r.head {
		p := get(h.yymm, h.division)
		p.HeadTunjangan = p.HeadTunjangan.Add(h.tunj)
		p.HeadPotongan = p.HeadPotongan.Add(h.pot)
		p.HeadNet = p.HeadNet.Add(h.net)
		p.HeadRows += h.rows
	}
	rep.Periods = make([]PeriodAgg, 0, len(idx))
	for _, p := range idx {
		rep.Periods = append(rep.Periods, *p)
		rep.Totals.DetailT = rep.Totals.DetailT.Add(p.DetailT)
		rep.Totals.DetailP = rep.Totals.DetailP.Add(p.DetailP)
		rep.Totals.HeadTunjangan = rep.Totals.HeadTunjangan.Add(p.HeadTunjangan)
		rep.Totals.HeadPotongan = rep.Totals.HeadPotongan.Add(p.HeadPotongan)
		rep.Totals.HeadNet = rep.Totals.HeadNet.Add(p.HeadNet)
	}
	sort.Slice(rep.Periods, func(i, j int) bool {
		a, b := rep.Periods[i], rep.Periods[j]
		return a.YYMM < b.YYMM || (a.YYMM == b.YYMM && a.Division < b.Division)
	})
	for _, u := range r.unmapped {
		rep.UnmappedLegacyLines += u.Lines
	}
	rep.Failures = []string{}
	if rep.UnmappedLegacyLines > 0 {
		rep.Failures = append(rep.Failures, fmt.Sprintf("%d baris detail LEGACY tanpa pyr_component_legacy_map (%d komponen)",
			rep.UnmappedLegacyLines, len(r.unmapped)))
	}
	if len(r.pii) > 0 {
		rep.Failures = append(rep.Failures, fmt.Sprintf("%d kolom PII bukan ciphertext (bytea)", len(r.pii)))
	}
	return rep
}

// Validate reads the aggregates as the read-only role (payroll_ro) inside one REPEATABLE READ READ ONLY transaction
// scoped to every division of the company, so the figures come from a single snapshot.
func Validate(ctx context.Context, db *sql.DB, q Query) (Report, error) {
	if q.CompanyID <= 0 || q.YYMMFrom <= 0 || q.YYMMTo < q.YYMMFrom {
		return Report{}, fmt.Errorf("validate: company dan rentang yymm tidak valid")
	}
	divs, err := divisions(ctx, db, q.CompanyID)
	if err != nil {
		return Report{}, err
	}
	scopes := make([]scope.Scope, 0, len(divs))
	for _, d := range divs {
		sc, err := scope.New(q.CompanyID, d)
		if err != nil {
			return Report{}, fmt.Errorf("divisi %q: %w", d, err)
		}
		scopes = append(scopes, sc)
	}
	ctx = scope.WithIdentity(ctx, scope.Identity{UserID: "recon-validate", Scopes: scopes})
	r := raw{divisions: divs}
	opts := &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead}
	err = postgres.NewDBFromSQL(db).ScopedTxOpts(ctx, opts, func(tx *sql.Tx) error { return load(ctx, tx, q, &r) })
	if err != nil {
		return Report{}, err
	}
	return build(q, r), nil
}

func divisions(ctx context.Context, db *sql.DB, company int16) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT pdr_code FROM payroll.pyr_division_ref WHERE pdr_company_id = $1 ORDER BY pdr_code`, company)
	if err != nil {
		return nil, fmt.Errorf("divisi company: %w", err)
	}
	defer closeRows(rows)
	out := []string{}
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func closeRows(r *sql.Rows) { _ = r.Close() } //nolint:errcheck // read-only cursor, error surfaces via Err()
