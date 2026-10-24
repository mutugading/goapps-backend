package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/component"
)

// ComponentRepository implements component.Repository on pyr_component and
// pyr_component_template. Both are RLS-scoped per company (D-14, D-20: the template
// follows its component), so reads and writes only touch the caller's scope.
type ComponentRepository struct{ MasterRepository }

var _ component.Repository = ComponentRepository{}

// NewComponentRepository creates a ComponentRepository.
func NewComponentRepository() ComponentRepository { return ComponentRepository{} }

const componentCols = `pc_sysid, pc_company_id, pc_code, CASE pc_type WHEN 'Tunjangan' THEN 'T' ELSE 'P' END, pc_name,
  COALESCE(array_to_string(pc_scope_divisions, ','), ''), COALESCE(pc_currency,''), pc_default_sts, pc_tax_class,
  pc_subsidy_eligible, COALESCE(pc_fix_income,''), pc_include_in_estimate, array_to_string(pc_tags, ','),
  COALESCE(pc_serial,0), COALESCE(pc_base_component_sysid,0), pc_status, pc_valid_from, COALESCE(pc_valid_to,0), pc_version`

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

func scanComponent(r interface{ Scan(...any) error }) (component.Component, error) {
	var c component.Component
	var divs, tags string
	err := r.Scan(&c.Sysid, &c.CompanyID, &c.Code, &c.Type, &c.Name, &divs, &c.Currency, &c.DefaultSts, &c.TaxClass,
		&c.SubsidyEligible, &c.FixIncome, &c.IncludeInEstimate, &tags, &c.Serial, &c.BaseSysid, &c.Status,
		&c.ValidFrom, &c.ValidTo, &c.Version)
	c.ScopeDivisions, c.Tags = splitCSV(divs), splitCSV(tags)
	return c, err
}

// Create inserts a component.
func (r ComponentRepository) Create(ctx context.Context, tx *sql.Tx, c component.Component) (component.Component, error) {
	out, err := scanComponent(tx.QueryRowContext(ctx, `INSERT INTO payroll.pyr_component
	  (pc_company_id, pc_code, pc_type, pc_name, pc_scope_divisions, pc_currency, pc_default_sts, pc_tax_class,
	   pc_subsidy_eligible, pc_fix_income, pc_include_in_estimate, pc_tags, pc_serial, pc_base_component_sysid,
	   pc_status, pc_valid_from, pc_valid_to)
	VALUES ($1,$2, CASE $3::text WHEN 'T' THEN 'Tunjangan' ELSE 'Potongan' END::payroll.comp_type, $4,
	  string_to_array(NULLIF($5::text,''), ',')::varchar(30)[], NULLIF($6,''), $7, $8, $9, NULLIF($10,''), $11,
	  COALESCE(string_to_array(NULLIF($12::text,''), ','), '{}')::varchar(30)[], NULLIF($13,0), NULLIF($14::bigint,0),
	  $15, $16, NULLIF($17,0))
	RETURNING `+componentCols,
		c.CompanyID, c.Code, c.Type, c.Name, strings.Join(c.ScopeDivisions, ","), c.Currency, c.DefaultSts, c.TaxClass,
		c.SubsidyEligible, c.FixIncome, c.IncludeInEstimate, strings.Join(c.Tags, ","), c.Serial, c.BaseSysid,
		c.Status, c.ValidFrom, c.ValidTo))
	return out, mapErr(err)
}

// Get reads one component (invisible rows are ErrNotFound).
func (ComponentRepository) Get(ctx context.Context, tx *sql.Tx, sysid int64) (component.Component, error) {
	out, err := scanComponent(tx.QueryRowContext(ctx, `SELECT `+componentCols+` FROM payroll.pyr_component WHERE pc_sysid=$1`, sysid))
	if errors.Is(err, sql.ErrNoRows) {
		return out, component.ErrNotFound
	}
	return out, err
}

// Lock takes a row lock for the audit before-image.
func (ComponentRepository) Lock(ctx context.Context, tx *sql.Tx, sysid int64) error {
	var one int
	err := tx.QueryRowContext(ctx, `SELECT 1 FROM payroll.pyr_component WHERE pc_sysid=$1 FOR UPDATE`, sysid).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return component.ErrNotFound
	}
	return err
}

// List lists components by sysid with a keyset cursor.
func (ComponentRepository) List(ctx context.Context, tx *sql.Tx, f component.Filter) ([]component.Component, error) {
	rows, err := tx.QueryContext(ctx, `SELECT `+componentCols+` FROM payroll.pyr_component
	  WHERE pc_sysid > $1
	    AND ($2::text = '{}' OR pc_company_id = ANY($2::smallint[]))
	    AND ($3 = '' OR pc_type = CASE $3::text WHEN 'T' THEN 'Tunjangan' ELSE 'Potongan' END::payroll.comp_type)
	    AND ($4 = '' OR pc_status = $4)
	    AND ($5 = '' OR pc_code ILIKE '%'||$5||'%' OR pc_name ILIKE '%'||$5||'%')
	  ORDER BY pc_sysid LIMIT $6`,
		f.AfterSysid, pq(f.CompanyIDs), f.Type, f.Status, likeEscape(f.Q), f.Limit)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	var out []component.Component
	for rows.Next() {
		c, err := scanComponent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// likeEscape neutralizes LIKE wildcards in a user search term.
func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// Update changes the writable fields when the version equals ifMatch.
func (r ComponentRepository) Update(ctx context.Context, tx *sql.Tx, c component.Component, ifMatch int) (component.Component, error) {
	out, err := scanComponent(tx.QueryRowContext(ctx, `UPDATE payroll.pyr_component SET
	  pc_name=$2, pc_scope_divisions=string_to_array(NULLIF($3::text,''), ',')::varchar(30)[], pc_currency=NULLIF($4,''),
	  pc_default_sts=$5, pc_tax_class=$6, pc_subsidy_eligible=$7, pc_fix_income=NULLIF($8,''), pc_include_in_estimate=$9,
	  pc_tags=COALESCE(string_to_array(NULLIF($10::text,''), ','), '{}')::varchar(30)[], pc_serial=NULLIF($11,0),
	  pc_base_component_sysid=NULLIF($12::bigint,0), pc_valid_from=$13, pc_valid_to=NULLIF($14,0), pc_version=pc_version+1
	WHERE pc_sysid=$1 AND pc_version=$15 RETURNING `+componentCols,
		c.Sysid, c.Name, strings.Join(c.ScopeDivisions, ","), c.Currency, c.DefaultSts, c.TaxClass, c.SubsidyEligible,
		c.FixIncome, c.IncludeInEstimate, strings.Join(c.Tags, ","), c.Serial, c.BaseSysid, c.ValidFrom, c.ValidTo, ifMatch))
	if errors.Is(err, sql.ErrNoRows) {
		return out, r.versionOrNotFound(ctx, tx, `SELECT 1 FROM payroll.pyr_component WHERE pc_sysid=$1`, c.Sysid)
	}
	return out, mapErr(err)
}

// SetStatus changes the status under If-Match.
func (r ComponentRepository) SetStatus(ctx context.Context, tx *sql.Tx, sysid int64, status string, ifMatch int) (component.Component, error) {
	out, err := scanComponent(tx.QueryRowContext(ctx, `UPDATE payroll.pyr_component SET pc_status=$2, pc_version=pc_version+1
	  WHERE pc_sysid=$1 AND pc_version=$3 RETURNING `+componentCols, sysid, status, ifMatch))
	if errors.Is(err, sql.ErrNoRows) {
		return out, r.versionOrNotFound(ctx, tx, `SELECT 1 FROM payroll.pyr_component WHERE pc_sysid=$1`, sysid)
	}
	return out, err
}

// MissingDivisions returns the requested divisions absent from pyr_division_ref.
func (ComponentRepository) MissingDivisions(ctx context.Context, tx *sql.Tx, companyID int16, divisions []string) ([]string, error) {
	if len(divisions) == 0 {
		return nil, nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT d FROM unnest(string_to_array($2::text, ',')) AS d
	  WHERE NOT EXISTS (SELECT 1 FROM payroll.pyr_division_ref r WHERE r.pdr_company_id=$1 AND r.pdr_code=d)`,
		companyID, strings.Join(divisions, ","))
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	var out []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// GetTemplate lists the rows of a level whose component the caller can see.
func (ComponentRepository) GetTemplate(ctx context.Context, tx *sql.Tx, level string) ([]component.TemplateItem, error) {
	rows, err := tx.QueryContext(ctx, `SELECT t.pct_component_sysid, COALESCE(t.pct_sts,''), COALESCE(t.pct_tax,0),
	    COALESCE(trim_scale(t.pct_amount)::text,''), COALESCE(t.pct_tax_paid_company,''), c.pc_code, c.pc_name
	  FROM payroll.pyr_component_template t
	  JOIN payroll.pyr_component c ON c.pc_sysid = t.pct_component_sysid
	  WHERE t.pct_template=$1 ORDER BY c.pc_serial NULLS LAST, c.pc_sysid`, level)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	var out []component.TemplateItem
	for rows.Next() {
		var it component.TemplateItem
		if err := rows.Scan(&it.Sysid, &it.Sts, &it.Tax, &it.Amount, &it.TaxPaidCompany, &it.Code, &it.Name); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// ReplaceTemplate deletes the visible rows of a level and inserts items. Rows of
// components in other companies stay untouched (D-20).
func (ComponentRepository) ReplaceTemplate(ctx context.Context, tx *sql.Tx, level string, items []component.TemplateItem) error {
	// Serialize concurrent PUTs of one level; the lock is released at commit.
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('pyr_component_template:' || $1))`, level); err != nil {
		return err
	}
	// RLS (p_parent) limits the DELETE to rows of components in the caller's scope (D-20).
	if _, err := tx.ExecContext(ctx, `DELETE FROM payroll.pyr_component_template WHERE pct_template=$1`, level); err != nil {
		return err
	}
	for _, it := range items {
		if _, err := tx.ExecContext(ctx, `INSERT INTO payroll.pyr_component_template
		  (pct_template, pct_component_sysid, pct_sts, pct_tax, pct_amount, pct_tax_paid_company)
		  VALUES ($1,$2,NULLIF($3,''),$4,NULLIF($5,'')::numeric,NULLIF($6,''))`,
			level, it.Sysid, it.Sts, it.Tax, it.Amount, it.TaxPaidCompany); err != nil {
			return mapErr(err)
		}
	}
	return nil
}
