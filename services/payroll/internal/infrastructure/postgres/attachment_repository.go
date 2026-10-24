package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/component"
)

// AttachmentRepository implements component.AttachmentRepository on
// pyr_employee_component (RLS p_scope per company/division) and pyr_level_rule.
type AttachmentRepository struct{ MasterRepository }

var _ component.AttachmentRepository = AttachmentRepository{}

// NewAttachmentRepository creates an AttachmentRepository.
func NewAttachmentRepository() AttachmentRepository { return AttachmentRepository{} }

// List returns the attachments of one employee ordered by component serial.
func (AttachmentRepository) List(ctx context.Context, tx *sql.Tx, k component.EmployeeKey) ([]component.Attachment, error) {
	rows, err := tx.QueryContext(ctx, `SELECT e.pec_component_sysid, c.pc_code, c.pc_name, COALESCE(e.pec_sts,''), COALESCE(e.pec_tax,0),
	    COALESCE(trim_scale(e.pec_amount)::text,''), COALESCE(e.pec_tax_paid_company,''), e.pec_frz_flag, e.pec_source,
	    COALESCE(e.pec_valid_from,0), COALESCE(e.pec_valid_to,0)
	  FROM payroll.pyr_employee_component e
	  JOIN payroll.pyr_component c ON c.pc_sysid = e.pec_component_sysid
	  WHERE e.pec_company_id=$1 AND e.pec_nik=$2 AND e.pec_division=$3
	  ORDER BY c.pc_serial NULLS LAST, c.pc_sysid`, k.CompanyID, k.NIK, k.Division)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	var out []component.Attachment
	for rows.Next() {
		var a component.Attachment
		if err := rows.Scan(&a.Sysid, &a.Code, &a.Name, &a.Sts, &a.Tax, &a.Amount, &a.TaxPaidCompany, &a.Frz, &a.Source,
			&a.ValidFrom, &a.ValidTo); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// TemplateFor returns the level template restricted to the employee's company and division.
func (AttachmentRepository) TemplateFor(ctx context.Context, tx *sql.Tx, level string, companyID int16, division string) ([]component.Attachment, error) {
	rows, err := tx.QueryContext(ctx, `SELECT t.pct_component_sysid, c.pc_code, c.pc_name, COALESCE(t.pct_sts, c.pc_default_sts),
	    COALESCE(t.pct_tax,0), COALESCE(trim_scale(t.pct_amount)::text,''), COALESCE(t.pct_tax_paid_company,'')
	  FROM payroll.pyr_component_template t
	  JOIN payroll.pyr_component c ON c.pc_sysid = t.pct_component_sysid
	  WHERE t.pct_template=$1 AND c.pc_company_id=$2 AND c.pc_status='ACTIVE'
	    AND (c.pc_scope_divisions IS NULL OR cardinality(c.pc_scope_divisions)=0 OR $3 = ANY(c.pc_scope_divisions))
	  ORDER BY c.pc_sysid`, level, companyID, division)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	var out []component.Attachment
	for rows.Next() {
		a := component.Attachment{Frz: 2, Source: component.SourceTemplate}
		if err := rows.Scan(&a.Sysid, &a.Code, &a.Name, &a.Sts, &a.Tax, &a.Amount, &a.TaxPaidCompany); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// LevelRule reads pyr_level_rule; nil when the level has no row.
func (AttachmentRepository) LevelRule(ctx context.Context, tx *sql.Tx, level string) (*component.LevelRule, error) {
	var r component.LevelRule
	err := tx.QueryRowContext(ctx, `SELECT plr_level, plr_grp, trim_scale(plr_otinwork_rate)::text, plr_otinwork_sts, plr_shiftadj_sts,
	    plr_tunjshift_sts, trim_scale(plr_tunjshift_rate)::text FROM payroll.pyr_level_rule WHERE plr_level=$1`, level).
		Scan(&r.Level, &r.Group, &r.OtinworkRate, &r.OtinworkSts, &r.ShiftadjSts, &r.TunjshiftSts, &r.TunjshiftRate)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil //nolint:nilnil // absent rule is a normal state
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// CatalogByCode maps code to the ACTIVE component of the company that applies to the division.
func (AttachmentRepository) CatalogByCode(ctx context.Context, tx *sql.Tx, companyID int16, division string, codes []string) (map[string]component.CatalogEntry, error) {
	rows, err := tx.QueryContext(ctx, `SELECT pc_sysid, pc_code, pc_name, pc_default_sts FROM payroll.pyr_component
	  WHERE pc_company_id=$1 AND pc_code = ANY(string_to_array($2::text, ',')) AND pc_status='ACTIVE'
	    AND (pc_scope_divisions IS NULL OR cardinality(pc_scope_divisions)=0 OR $3 = ANY(pc_scope_divisions))`,
		companyID, strings.Join(codes, ","), division)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	out := map[string]component.CatalogEntry{}
	for rows.Next() {
		var c component.CatalogEntry
		if err := rows.Scan(&c.Sysid, &c.Code, &c.Name, &c.DefaultSts); err != nil {
			return nil, err
		}
		out[c.Code] = c
	}
	return out, rows.Err()
}

// Upsert inserts or replaces one attachment.
func (AttachmentRepository) Upsert(ctx context.Context, tx *sql.Tx, k component.EmployeeKey, a component.Attachment, actor string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO payroll.pyr_employee_component
	  (pec_company_id, pec_nik, pec_division, pec_component_sysid, pec_name, pec_sts, pec_tax, pec_amount,
	   pec_tax_paid_company, pec_frz_flag, pec_source, pec_valid_from, pec_valid_to, updated_by, updated_at)
	  VALUES ($1,$2,$3,$4,$5,NULLIF($6,''),$7,NULLIF($8,'')::numeric,NULLIF($9,''),$10,$11,NULLIF($12,0),NULLIF($13,0),$14,now())
	  ON CONFLICT (pec_company_id, pec_nik, pec_division, pec_component_sysid) DO UPDATE SET
	    pec_name=EXCLUDED.pec_name, pec_sts=EXCLUDED.pec_sts, pec_tax=EXCLUDED.pec_tax, pec_amount=EXCLUDED.pec_amount,
	    pec_tax_paid_company=EXCLUDED.pec_tax_paid_company, pec_frz_flag=EXCLUDED.pec_frz_flag, pec_source=EXCLUDED.pec_source,
	    pec_valid_from=EXCLUDED.pec_valid_from, pec_valid_to=EXCLUDED.pec_valid_to,
	    updated_by=EXCLUDED.updated_by, updated_at=now()`,
		k.CompanyID, k.NIK, k.Division, a.Sysid, a.Name, a.Sts, a.Tax, a.Amount, a.TaxPaidCompany, a.Frz, a.Source,
		a.ValidFrom, a.ValidTo, actor)
	return mapErr(err)
}

// Delete detaches one component.
func (AttachmentRepository) Delete(ctx context.Context, tx *sql.Tx, k component.EmployeeKey, sysid int64) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM payroll.pyr_employee_component
	  WHERE pec_company_id=$1 AND pec_nik=$2 AND pec_division=$3 AND pec_component_sysid=$4`, k.CompanyID, k.NIK, k.Division, sysid)
	return err
}

// Snapshot copies the current attachment row to pyr_employee_component_hist.
func (AttachmentRepository) Snapshot(ctx context.Context, tx *sql.Tx, k component.EmployeeKey, sysid int64, yymm int) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO payroll.pyr_employee_component_hist
	  (pec_company_id, pec_nik, pec_division, pec_component_sysid, pec_name, pec_sts, pec_tax, pec_amount,
	   pec_frz_flag, pec_tax_paid_company, pec_source, pec_valid_from, pec_valid_to, updated_by, updated_at, pec_snapshot_yymm)
	  SELECT pec_company_id, pec_nik, pec_division, pec_component_sysid, pec_name, pec_sts, pec_tax, pec_amount,
	   pec_frz_flag, pec_tax_paid_company, pec_source, pec_valid_from, pec_valid_to, updated_by, updated_at, $5
	  FROM payroll.pyr_employee_component
	  WHERE pec_company_id=$1 AND pec_nik=$2 AND pec_division=$3 AND pec_component_sysid=$4`, k.CompanyID, k.NIK, k.Division, sysid, yymm)
	return mapErr(err)
}

// SetEmployeeLevel changes pe_level when the employee version equals ifMatch.
func (r AttachmentRepository) SetEmployeeLevel(ctx context.Context, tx *sql.Tx, employeeID int64, level, actor string, ifMatch int) error {
	var got int64
	err := tx.QueryRowContext(ctx, `UPDATE payroll.pyr_employee SET pe_level=$2, updated_by=$3, updated_at=now(), pe_version=pe_version+1
	  WHERE pe_id=$1 AND pe_version=$4 RETURNING pe_id`, employeeID, level, actor, ifMatch).Scan(&got)
	if errors.Is(err, sql.ErrNoRows) {
		return r.versionOrNotFound(ctx, tx, `SELECT 1 FROM payroll.pyr_employee WHERE pe_id=$1`, employeeID)
	}
	return mapErr(err)
}

// CatalogBySysid maps sysid to the ACTIVE component of the company that applies to the division.
func (AttachmentRepository) CatalogBySysid(ctx context.Context, tx *sql.Tx, companyID int16, division string, sysids []int64) (map[int64]component.CatalogEntry, error) {
	ids := make([]string, len(sysids))
	for i, id := range sysids {
		ids[i] = strconv.FormatInt(id, 10)
	}
	rows, err := tx.QueryContext(ctx, `SELECT pc_sysid, pc_code, pc_name, pc_default_sts FROM payroll.pyr_component
	  WHERE pc_company_id=$1 AND pc_sysid = ANY(string_to_array($2::text, ',')::bigint[]) AND pc_status='ACTIVE'
	    AND (pc_scope_divisions IS NULL OR cardinality(pc_scope_divisions)=0 OR $3 = ANY(pc_scope_divisions))`,
		companyID, strings.Join(ids, ","), division)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	out := map[int64]component.CatalogEntry{}
	for rows.Next() {
		var c component.CatalogEntry
		if err := rows.Scan(&c.Sysid, &c.Code, &c.Name, &c.DefaultSts); err != nil {
			return nil, err
		}
		out[c.Sysid] = c
	}
	return out, rows.Err()
}

// PeriodClosed reports whether the period of the employee's company/division is CLOSED.
func (AttachmentRepository) PeriodClosed(ctx context.Context, tx *sql.Tx, k component.EmployeeKey, yymm int) (bool, error) {
	var closed bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM payroll.pyr_period
	  WHERE pp_company_id=$1 AND pp_yymm=$2 AND pp_division=$3 AND pp_status='CLOSED')`, k.CompanyID, yymm, k.Division).Scan(&closed)
	return closed, err
}
