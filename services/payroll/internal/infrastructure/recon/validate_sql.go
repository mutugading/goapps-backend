package recon

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/shopspring/decimal"
)

// Every query is bound to the ACTIVE LEGACY run of a period (the run the ETL loads, 08§2); RLS (app.scopes) still
// limits the rows to the divisions of the company. Sums are cast to text and parsed as decimal: no float.
const (
	detailSQL = `SELECT r.pr_yymm, r.pr_division::text, CASE d.psd_type WHEN 'Tunjangan' THEN 'T' ELSE 'P' END,
  COALESCE(sum(d.psd_amount), 0)::text, count(*)
FROM payroll.pyr_run r JOIN payroll.pyr_salary_detail d ON d.psd_run_id = r.pr_id
WHERE r.pr_company_id = $1 AND r.pr_kind = 'LEGACY' AND r.pr_status = 'ACTIVE' AND r.pr_yymm BETWEEN $2 AND $3
GROUP BY 1, 2, 3 ORDER BY 1, 2, 3`

	headSQL = `SELECT r.pr_yymm, r.pr_division::text, COALESCE(sum(h.psh_tot_tunjangan), 0)::text,
  COALESCE(sum(h.psh_tot_potongan), 0)::text, COALESCE(sum(h.psh_net_pay), 0)::text, count(*)
FROM payroll.pyr_run r JOIN payroll.pyr_salary_head h ON h.psh_run_id = r.pr_id
WHERE r.pr_company_id = $1 AND r.pr_kind = 'LEGACY' AND r.pr_status = 'ACTIVE' AND r.pr_yymm BETWEEN $2 AND $3
GROUP BY 1, 2 ORDER BY 1, 2`

	levelSQL = `SELECT pe_division::text, COALESCE(pe_level, ''), count(*) FROM payroll.pyr_employee
WHERE pe_company_id = $1 AND pe_sts_payroll = 2 GROUP BY 1, 2 ORDER BY 1, 2`

	// D-36: SOLO used ids 100/200 (T) and 101/201 (P) that COMPNT_MASTER never had; the ETL resolves their sysid through
	// the alias to T70/T71/P70/P71 (etl.legacyAlias), so a line counts as mapped when its own id or its alias target
	// has a legacy_map row. Both queries share the alias table; keep it in sync with etl/salary_legacy.go.
	aliasCTE = `WITH alias(typ, id, to_id) AS (VALUES ('Tunjangan'::payroll.comp_type, 100, 70), ('Tunjangan', 200, 71),
  ('Potongan', 101, 70), ('Potongan', 201, 71)),
lines AS (
  SELECT r.pr_company_id AS co, r.pr_division::text AS div, d.psd_type AS typ, d.psd_id AS id, COALESCE(a.to_id, d.psd_id) AS map_id,
    (a.to_id IS NOT NULL) AS aliased
  FROM payroll.pyr_run r JOIN payroll.pyr_salary_detail d ON d.psd_run_id = r.pr_id
  LEFT JOIN alias a ON a.typ = d.psd_type AND a.id = d.psd_id
  WHERE r.pr_company_id = $1 AND r.pr_kind = 'LEGACY' AND r.pr_status = 'ACTIVE' AND r.pr_yymm BETWEEN $2 AND $3
)`

	unmappedSQL = aliasCTE + `
SELECT CASE l.typ WHEN 'Tunjangan' THEN 'T' ELSE 'P' END, l.id, l.div, count(*), l.aliased
FROM lines l
WHERE NOT EXISTS (SELECT 1 FROM payroll.pyr_component_legacy_map m WHERE m.pclm_company_id = l.co
    AND m.pclm_legacy_type = l.typ AND m.pclm_legacy_id = l.map_id AND m.pclm_legacy_division = l.div)
GROUP BY 1, 2, 3, 5 ORDER BY 3, 1, 2`

	aliasedSQL = aliasCTE + `
SELECT CASE l.typ WHEN 'Tunjangan' THEN 'T' ELSE 'P' END, l.id, l.div, count(*), true
FROM lines l
WHERE l.aliased AND EXISTS (SELECT 1 FROM payroll.pyr_component_legacy_map m WHERE m.pclm_company_id = l.co
    AND m.pclm_legacy_type = l.typ AND m.pclm_legacy_id = l.map_id AND m.pclm_legacy_division = l.div)
GROUP BY 1, 2, 3 ORDER BY 3, 1, 2`

	birthSQL = `SELECT pe_division::text, count(*), count(*) FILTER (WHERE pe_birth_date IS NULL),
  count(*) FILTER (WHERE pe_birth_date IS NULL AND pe_sts_payroll = 2)
FROM payroll.pyr_employee WHERE pe_company_id = $1 GROUP BY 1 ORDER BY 1`

	// PII-looking columns (NPWP, KTP, bank account number) that are not bytea ciphertext. Descriptive names
	// (…_name) and the HMAC (…_hash) are not PII values. Staging schema included.
	piiSQL = `SELECT table_schema, table_name, column_name, data_type FROM information_schema.columns
WHERE table_schema IN ('payroll', 'payroll_stg')
  AND column_name ~* '(npwp|ktp|rekening|no_rek|bank_acnt|bank_acc|acnt_no|account_no|account_number)'
  AND column_name !~* '(_name|_hash)$' AND data_type <> 'bytea'
ORDER BY 1, 2, 3`
)

func load(ctx context.Context, tx *sql.Tx, q Query, r *raw) error {
	args := []any{q.CompanyID, q.YYMMFrom, q.YYMMTo}
	var err error
	if r.detail, err = queryAll(ctx, tx, detailSQL, args, func(s scanner) (detailRow, error) {
		var d detailRow
		var sum string
		if err := s.Scan(&d.yymm, &d.division, &d.typ, &sum, &d.lines); err != nil {
			return d, err
		}
		d.sum, err = decimal.NewFromString(sum)
		return d, err
	}); err != nil {
		return fmt.Errorf("detail: %w", err)
	}
	if r.head, err = queryAll(ctx, tx, headSQL, args, func(s scanner) (headRow, error) {
		var h headRow
		var a, b, c string
		if err := s.Scan(&h.yymm, &h.division, &a, &b, &c, &h.rows); err != nil {
			return h, err
		}
		for i, p := range []*decimal.Decimal{&h.tunj, &h.pot, &h.net} {
			if *p, err = decimal.NewFromString([]string{a, b, c}[i]); err != nil {
				return h, err
			}
		}
		return h, nil
	}); err != nil {
		return fmt.Errorf("head: %w", err)
	}
	if r.levels, err = queryAll(ctx, tx, levelSQL, args[:1], func(s scanner) (LevelCount, error) {
		var l LevelCount
		return l, s.Scan(&l.Division, &l.Level, &l.Active)
	}); err != nil {
		return fmt.Errorf("level: %w", err)
	}
	scanComp := func(s scanner) (UnmappedComponent, error) {
		var u UnmappedComponent
		return u, s.Scan(&u.Type, &u.ID, &u.Division, &u.Lines, &u.Aliased)
	}
	if r.unmapped, err = queryAll(ctx, tx, unmappedSQL, args, scanComp); err != nil {
		return fmt.Errorf("unmapped: %w", err)
	}
	if r.aliased, err = queryAll(ctx, tx, aliasedSQL, args, scanComp); err != nil {
		return fmt.Errorf("aliased: %w", err)
	}
	if r.birth, err = queryAll(ctx, tx, birthSQL, args[:1], func(s scanner) (BirthDateGap, error) {
		var b BirthDateGap
		return b, s.Scan(&b.Division, &b.Employees, &b.Missing, &b.MissingActive)
	}); err != nil {
		return fmt.Errorf("birth_date: %w", err)
	}
	if r.pii, err = queryAll(ctx, tx, piiSQL, nil, func(s scanner) (PlainPIIColumn, error) {
		var p PlainPIIColumn
		return p, s.Scan(&p.Schema, &p.Table, &p.Column, &p.DataType)
	}); err != nil {
		return fmt.Errorf("pii: %w", err)
	}
	return nil
}

type scanner interface{ Scan(dest ...any) error }

func queryAll[T any](ctx context.Context, tx *sql.Tx, q string, args []any, fn func(scanner) (T, error)) ([]T, error) {
	rows, err := tx.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	out := []T{}
	for rows.Next() {
		v, err := fn(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
