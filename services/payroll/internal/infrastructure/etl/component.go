package etl

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
)

// Options controls one master-component load.
type Options struct {
	CompanyID      int16
	ValidFrom      int // yymm for pc_valid_from of legacy components
	ExpectMaster   int // 0 = tidak dicek; produksi 133 (08§2)
	ExpectTemplate int // 0 = tidak dicek; produksi 762 (D-23)
}

// TableReport is one line of the load report (08§6: row count + checksum per table).
type TableReport struct {
	Table    string
	SrcRows  int
	DstRows  int
	Checksum string
}

// OK reports whether destination row count equals the source.
func (r TableReport) OK() bool { return r.SrcRows == r.DstRows }

// LegacySource labels attachment rows that come from legacy ETL (sisa D-21).
const LegacySource = "TEMPLATE"

// CompCode builds the component code: T001, P032, T096 (08§2).
func CompCode(legacyType string, id int) (string, error) {
	switch strings.ToLower(legacyType) {
	case "tunjangan":
		return fmt.Sprintf("T%03d", id), nil
	case "potongan":
		return fmt.Sprintf("P%03d", id), nil
	}
	return "", fmt.Errorf("tipe komponen legacy tidak dikenal: %q", legacyType)
}

// taxClass derives pc_tax_class from the legacy flags (12§2): tax=2 → taxable,
// paid by company → TAX_PAID_COMPANY, otherwise NONE.
func taxClass(tax, paidByCompany string) string {
	switch {
	case strings.EqualFold(paidByCompany, "Y"):
		return "TAX_PAID_COMPANY"
	case tax == "2":
		return "TAXABLE"
	}
	return "NONE"
}

func compType(legacy string) string {
	if strings.EqualFold(legacy, "Potongan") {
		return "Potongan"
	}
	return "Tunjangan"
}

type masterCols struct{ typ, id, name, sts, tax, serial, fix, paid, subsidy int }

func (c *masterCols) bind(t *Table) error {
	var err error
	for _, p := range []struct {
		dst  *int
		name string
	}{{&c.typ, "MPCM_TYPE"}, {&c.id, "MPCM_ID"}, {&c.name, "MPCM_NAME"}, {&c.sts, "MPCM_STS"}, {&c.tax, "MPCM_TAX"},
		{&c.serial, "MPCM_SERIAL"}, {&c.fix, "MPCM_FIX_INCOME"}, {&c.paid, "MPCM_TAX_PAID_COMPANY"}, {&c.subsidy, "MPCM_TAX_SUBSIDY"}} {
		if *p.dst, err = t.Col(p.name); err != nil {
			return err
		}
	}
	return nil
}

type masterRow struct {
	code, typ, name, sts, taxClass string
	id                             int
	subsidy                        bool
	fix, serial                    any
}

// parse validates one COMPNT_MASTER row (line = 1-based file line for messages).
func (c *masterCols) parse(table string, line int, r []string) (masterRow, error) {
	id, err := strconv.Atoi(r[c.id])
	if err != nil {
		return masterRow{}, fmt.Errorf("%s baris %d: MPCM_ID %q bukan angka", table, line, r[c.id])
	}
	code, err := CompCode(r[c.typ], id)
	if err != nil {
		return masterRow{}, fmt.Errorf("%s baris %d: %w", table, line, err)
	}
	m := masterRow{code: code, id: id, typ: compType(r[c.typ]), name: r[c.name], sts: "F",
		taxClass: taxClass(r[c.tax], r[c.paid]), subsidy: strings.EqualFold(r[c.subsidy], "Y"), fix: nullable(r[c.fix])}
	if strings.EqualFold(r[c.sts], "D") {
		m.sts = "D"
	}
	if r[c.serial] != "" {
		if m.serial, err = strconv.Atoi(r[c.serial]); err != nil {
			return masterRow{}, fmt.Errorf("%s baris %d: MPCM_SERIAL %q", table, line, r[c.serial])
		}
	}
	return m, nil
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// LoadMaster fills pyr_component (+ legacy_map for every division of the company) from
// COMPNT_MASTER, one sysid per (type,id). Re-runs update only fields the T008c guard
// does not protect (name, serial, default_sts, fix_income, subsidy flag).
func LoadMaster(ctx context.Context, tx *sql.Tx, src *Table, o Options) (TableReport, error) {
	rep := TableReport{Table: TableComponentMaster, SrcRows: len(src.Rows), Checksum: src.Checksum()}
	var c masterCols
	if err := c.bind(src); err != nil {
		return rep, err
	}
	divs, err := divisions(ctx, tx, o.CompanyID)
	if err != nil {
		return rep, err
	}
	seen := map[string]bool{}
	for i, r := range src.Rows {
		m, err := c.parse(src.Name, i+2, r)
		if err != nil {
			return rep, err
		}
		if seen[m.code] {
			return rep, fmt.Errorf("%s baris %d: (type,id) ganda %s", src.Name, i+2, m.code)
		}
		seen[m.code] = true
		var sysid int64
		err = tx.QueryRowContext(ctx, `
INSERT INTO payroll.pyr_component (pc_company_id, pc_code, pc_type, pc_name, pc_default_sts, pc_tax_class,
  pc_subsidy_eligible, pc_fix_income, pc_serial, pc_status, pc_valid_from)
VALUES ($1,$2,$3::payroll.comp_type,$4,$5,$6,$7,$8,$9,'ACTIVE',$10)
ON CONFLICT (pc_company_id, pc_code) DO UPDATE SET pc_name=EXCLUDED.pc_name, pc_default_sts=EXCLUDED.pc_default_sts,
  pc_subsidy_eligible=EXCLUDED.pc_subsidy_eligible, pc_fix_income=EXCLUDED.pc_fix_income, pc_serial=EXCLUDED.pc_serial
RETURNING pc_sysid`,
			o.CompanyID, m.code, m.typ, m.name, m.sts, m.taxClass, m.subsidy, m.fix, m.serial, o.ValidFrom).Scan(&sysid)
		if err != nil {
			return rep, fmt.Errorf("%s %s: %w", src.Name, m.code, err)
		}
		for _, d := range divs {
			if _, err = tx.ExecContext(ctx, `
INSERT INTO payroll.pyr_component_legacy_map (pclm_company_id, pclm_legacy_type, pclm_legacy_id, pclm_legacy_division, pclm_sysid)
VALUES ($1,$2::payroll.comp_type,$3,$4,$5)
ON CONFLICT (pclm_company_id, pclm_legacy_type, pclm_legacy_id, pclm_legacy_division) DO UPDATE SET pclm_sysid=EXCLUDED.pclm_sysid`,
				o.CompanyID, m.typ, m.id, d, sysid); err != nil {
				return rep, fmt.Errorf("legacy_map %s/%s: %w", m.code, d, err)
			}
		}
	}
	err = tx.QueryRowContext(ctx, `SELECT count(DISTINCT pclm_sysid) FROM payroll.pyr_component_legacy_map WHERE pclm_company_id=$1`,
		o.CompanyID).Scan(&rep.DstRows)
	return rep, err
}

func divisions(ctx context.Context, tx *sql.Tx, company int16) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT pdr_code FROM payroll.pyr_division_ref WHERE pdr_company_id=$1 ORDER BY 1`, company)
	if err != nil {
		return nil, err
	}
	defer closeQuiet(rows)
	var out []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if len(out) == 0 {
		return nil, errors.New("company belum punya divisi di pyr_division_ref")
	}
	return out, rows.Err()
}

// LoadTemplate fills pyr_component_template from COMPNT_TEMPLATE, resolving each
// (type,id) to its sysid through legacy_map (D-20). Rows without a map entry abort.
func LoadTemplate(ctx context.Context, tx *sql.Tx, src *Table, o Options) (TableReport, error) {
	rep := TableReport{Table: TableComponentTemplate, SrcRows: len(src.Rows), Checksum: src.Checksum()}
	idx := map[string]int{}
	for _, n := range []string{"MPCT_TEMPLATE", "MPCT_TYPE", "MPCT_ID", "MPCT_NAME", "MPCT_STS", "MPCT_TAX", "MPCT_AMOUNT", "MPCT_TAX_PAID_COMPANY"} {
		i, err := src.Col(n)
		if err != nil {
			return rep, err
		}
		idx[n] = i
	}
	for i, r := range src.Rows {
		id, err := strconv.Atoi(r[idx["MPCT_ID"]])
		if err != nil {
			return rep, fmt.Errorf("%s baris %d: MPCT_ID %q", src.Name, i+2, r[idx["MPCT_ID"]])
		}
		typ := compType(r[idx["MPCT_TYPE"]])
		var sysid int64
		err = tx.QueryRowContext(ctx, `
SELECT min(pclm_sysid) FROM payroll.pyr_component_legacy_map
WHERE pclm_company_id=$1 AND pclm_legacy_type=$2::payroll.comp_type AND pclm_legacy_id=$3`, o.CompanyID, typ, id).Scan(&sysid)
		if err != nil {
			return rep, fmt.Errorf("%s baris %d: (%s,%d) tidak ada di legacy_map (jalankan master dulu): %w", src.Name, i+2, typ, id, err)
		}
		var tax any
		if v := r[idx["MPCT_TAX"]]; v != "" {
			if tax, err = strconv.Atoi(v); err != nil {
				return rep, fmt.Errorf("%s baris %d: MPCT_TAX %q", src.Name, i+2, v)
			}
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO payroll.pyr_component_template (pct_template, pct_component_sysid, pct_type, pct_id, pct_name, pct_sts, pct_tax,
  pct_amount, pct_tax_paid_company)
VALUES ($1,$2,$3::payroll.comp_type,$4,$5,$6,$7,$8::numeric,$9)
ON CONFLICT (pct_template, pct_component_sysid) DO UPDATE SET pct_type=EXCLUDED.pct_type, pct_id=EXCLUDED.pct_id,
  pct_name=EXCLUDED.pct_name, pct_sts=EXCLUDED.pct_sts, pct_tax=EXCLUDED.pct_tax, pct_amount=EXCLUDED.pct_amount,
  pct_tax_paid_company=EXCLUDED.pct_tax_paid_company`,
			r[idx["MPCT_TEMPLATE"]], sysid, typ, id, nullable(r[idx["MPCT_NAME"]]), nullable(r[idx["MPCT_STS"]]), tax,
			nullable(r[idx["MPCT_AMOUNT"]]), nullable(r[idx["MPCT_TAX_PAID_COMPANY"]]))
		if err != nil {
			return rep, fmt.Errorf("%s baris %d: %w", src.Name, i+2, err)
		}
	}
	err := tx.QueryRowContext(ctx, `
SELECT count(*) FROM payroll.pyr_component_template t
JOIN payroll.pyr_component c ON c.pc_sysid = t.pct_component_sysid WHERE c.pc_company_id=$1`, o.CompanyID).Scan(&rep.DstRows)
	return rep, err
}

// Run loads master, template and level rules in ONE transaction and records the report
// in pyr_migration_log. Any count mismatch (or ExpectX violation) rolls everything back.
func Run(ctx context.Context, db *sql.DB, src Source, o Options) ([]TableReport, error) {
	master, err := src.Read(TableComponentMaster)
	if err != nil {
		return nil, err
	}
	tmpl, err := src.Read(TableComponentTemplate)
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			log.Warn().Err(rbErr).Msg("etl: rollback failed")
		}
	}()

	var reps []TableReport
	mr, err := LoadMaster(ctx, tx, master, o)
	if err != nil {
		return nil, err
	}
	reps = append(reps, mr)
	tr, err := LoadTemplate(ctx, tx, tmpl, o)
	if err != nil {
		return nil, err
	}
	reps = append(reps, tr)
	n, err := LoadLevelRules(ctx, tx)
	if err != nil {
		return nil, err
	}
	reps = append(reps, TableReport{Table: "pyr_level_rule", SrcRows: n, DstRows: n, Checksum: "-"})

	if err := logAndCheck(ctx, tx, reps, o); err != nil {
		return reps, err
	}
	return reps, tx.Commit()
}

// logAndCheck rejects count mismatches and unexpected source sizes, then writes pyr_migration_log.
func logAndCheck(ctx context.Context, tx *sql.Tx, reps []TableReport, o Options) error {
	for _, r := range reps {
		if !r.OK() {
			return fmt.Errorf("%s: jumlah baris sumber %d != tujuan %d", r.Table, r.SrcRows, r.DstRows)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO payroll.pyr_migration_log (pml_company_id, pml_table, pml_src_rows, pml_dst_rows, pml_checksum, pml_ok)
VALUES ($1,$2,$3,$4,$5,true)`, o.CompanyID, r.Table, r.SrcRows, r.DstRows, r.Checksum); err != nil {
			return err
		}
	}
	if o.ExpectMaster > 0 && reps[0].SrcRows != o.ExpectMaster {
		return fmt.Errorf("master: %d baris, diharapkan %d", reps[0].SrcRows, o.ExpectMaster)
	}
	if o.ExpectTemplate > 0 && reps[1].SrcRows != o.ExpectTemplate {
		return fmt.Errorf("template: %d baris, diharapkan %d", reps[1].SrcRows, o.ExpectTemplate)
	}
	return nil
}
