package postgres

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	domain "github.com/mutugading/goapps-backend/services/payroll/internal/domain/master"
)

// dec returns the SQL that decrypts (or masks) a bytea PII column; $1 = reveal, $2 = key.
func dec(col string) string {
	return fmt.Sprintf(`CASE WHEN %[1]s IS NULL THEN '' WHEN $1::boolean THEN payroll.pgp_sym_decrypt(%[1]s, $2::text) ELSE '***' END`, col)
}

// employeeCols lists the columns scanned by scanEmployee ($1 = reveal, $2 = key).
var employeeCols = `pe_id, pe_nik, pe_company_id, pe_division, pe_nama, COALESCE(pe_email,''),
  COALESCE(pe_dept_id,''), COALESCE(pe_dept,''), COALESCE(pe_section_id,''), COALESCE(pe_section,''),
  COALESCE(pe_designation_id,''), COALESCE(pe_designation,''), COALESCE(pe_grade,''), COALESCE(pe_level,''), COALESCE(pe_tax_id,''),
  COALESCE(pe_sts_emp,0), pe_sts_payroll, COALESCE(pe_sts_bonus,0),
  COALESCE(to_char(pe_date_join,'YYYY-MM-DD'),''), COALESCE(to_char(pe_resign_date,'YYYY-MM-DD'),''),
  COALESCE(to_char(pe_birth_date,'YYYY-MM-DD'),''), COALESCE(pe_birth_place,''),
  COALESCE(pe_sex,''), COALESCE(pe_married_sts,''), COALESCE(pe_religion,''),
  ` + dec("pe_npwp_enc") + `, ` + dec("pe_ktp_enc") + `,
  COALESCE(pe_bank,''), COALESCE(pe_bank_branch,''), ` + dec("pe_bank_acnt_enc") + `, COALESCE(pe_bank_acnt_name,''),
  pe_orion_sub_acnt, COALESCE(pe_id_mesin,''), COALESCE(pe_cuti_tahunan::text,''),
  pe_pay_currency, COALESCE(pe_employment_status,''), COALESCE(pe_cost_group,''),
  COALESCE(pe_hris_sys_id,0)::bigint, COALESCE(pe_prev_employee_id,0), COALESCE(pe_placement_reason,''),
  pe_version, COALESCE(created_by,''), to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SSOF'),
  COALESCE(updated_by,''), COALESCE(to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SSOF'),'')`

func scanEmployee(r interface{ Scan(...any) error }) (domain.Employee, error) {
	var e domain.Employee
	err := r.Scan(&e.ID, &e.NIK, &e.CompanyID, &e.Division, &e.Nama, &e.Email,
		&e.DeptID, &e.Dept, &e.SectionID, &e.Section, &e.DesignationID, &e.Designation, &e.Grade, &e.Level, &e.TaxID,
		&e.StsEmp, &e.StsPayroll, &e.StsBonus,
		&e.DateJoin, &e.ResignDate, &e.BirthDate, &e.BirthPlace,
		&e.Sex, &e.MarriedSts, &e.Religion,
		&e.NPWP, &e.KTP,
		&e.Bank, &e.BankBranch, &e.BankAcnt, &e.BankAcntName,
		&e.OrionSubAcnt, &e.IDMesin, &e.CutiTahunan,
		&e.PayCurrency, &e.EmploymentStatus, &e.CostGroup,
		&e.HrisSysID, &e.PrevEmployeeID, &e.PlacementReason,
		&e.Version, &e.CreatedBy, &e.CreatedAt, &e.UpdatedBy, &e.UpdatedAt)
	return e, err
}

// NPWPHash returns HMAC-SHA256 over the digits of npwp (13§3.3); nil when empty.
func NPWPHash(key []byte, npwp string) []byte {
	var digits strings.Builder
	for _, r := range npwp {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	if digits.Len() == 0 {
		return nil
	}
	m := hmac.New(sha256.New, key)
	m.Write([]byte(digits.String()))
	return m.Sum(nil)
}

// CreateEmployee inserts an employee and its current-NIK history row.
func (r MasterRepository) CreateEmployee(ctx context.Context, tx *sql.Tx, e domain.Employee, npwp, ktp, bankAcnt, actor string, p domain.PII) (domain.Employee, error) {
	const q = `INSERT INTO payroll.pyr_employee (
	  pe_company_id, pe_division, pe_nik, pe_nama, pe_email, pe_dept_id, pe_dept, pe_section_id, pe_section,
	  pe_designation_id, pe_designation, pe_grade, pe_level, pe_tax_id, pe_sts_emp, pe_sts_payroll, pe_sts_bonus,
	  pe_date_join, pe_resign_date, pe_birth_date, pe_birth_place, pe_sex, pe_married_sts, pe_religion,
	  pe_npwp_enc, pe_ktp_enc, pe_bank_acnt_enc, pe_npwp_hash, pe_bank, pe_bank_branch, pe_bank_acnt_name,
	  pe_orion_sub_acnt, pe_id_mesin, pe_cuti_tahunan, pe_pay_currency, pe_employment_status, pe_cost_group,
	  pe_hris_sys_id, created_by)
	VALUES ($2,$3,$4,$5,NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),NULLIF($10,''),
	  NULLIF($11,''),NULLIF($12,''),NULLIF($13,''),NULLIF($14,''),NULLIF($15,''),NULLIF($16,0),$17,NULLIF($18,0),
	  NULLIF($19,'')::date,NULLIF($20,'')::date,NULLIF($21,'')::date,NULLIF($22,''),NULLIF($23,''),NULLIF($24,''),NULLIF($25,''),
	  CASE WHEN $26::text='' THEN NULL ELSE payroll.pgp_sym_encrypt($26::text,$1::text) END,
	  CASE WHEN $27::text='' THEN NULL ELSE payroll.pgp_sym_encrypt($27::text,$1::text) END,
	  CASE WHEN $28::text='' THEN NULL ELSE payroll.pgp_sym_encrypt($28::text,$1::text) END,
	  $29, NULLIF($30,''),NULLIF($31,''),NULLIF($32,''),
	  $33, NULLIF($34,''), NULLIF($35,'')::numeric, $36, NULLIF($37,''), NULLIF($38,''),
	  NULLIF($39,0), $40)
	RETURNING pe_id`
	var id int64
	err := tx.QueryRowContext(ctx, q, p.EncKey,
		e.CompanyID, e.Division, e.NIK, e.Nama, e.Email, e.DeptID, e.Dept, e.SectionID, e.Section,
		e.DesignationID, e.Designation, e.Grade, e.Level, e.TaxID, e.StsEmp, e.StsPayroll, e.StsBonus,
		e.DateJoin, e.ResignDate, e.BirthDate, e.BirthPlace, e.Sex, e.MarriedSts, e.Religion,
		npwp, ktp, bankAcnt, NPWPHash(p.HMACKey, npwp), e.Bank, e.BankBranch, e.BankAcntName,
		e.OrionSubAcnt, e.IDMesin, e.CutiTahunan, e.PayCurrency, e.EmploymentStatus, e.CostGroup,
		e.HrisSysID, actor).Scan(&id)
	if err != nil {
		return domain.Employee{}, mapErr(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO payroll.pyr_employee_nik_hist (penh_employee_id, penh_nik, penh_valid_from, penh_source)
		VALUES ($1,$2,COALESCE(NULLIF($3,'')::date, CURRENT_DATE),'PAYROLL_NIK')`, id, e.NIK, e.DateJoin); err != nil {
		return domain.Employee{}, mapErr(err)
	}
	return r.GetEmployeeByID(ctx, tx, id, p)
}

// GetEmployeeByID reads one employee.
func (MasterRepository) GetEmployeeByID(ctx context.Context, tx *sql.Tx, id int64, p domain.PII) (domain.Employee, error) {
	out, err := scanEmployee(tx.QueryRowContext(ctx, `SELECT `+employeeCols+` FROM payroll.pyr_employee WHERE pe_id=$3`, p.Reveal, p.EncKey, id))
	if errors.Is(err, sql.ErrNoRows) {
		return out, domain.ErrNotFound
	}
	return out, err
}

// ResolveEmployeeIDs maps a current or historical NIK to distinct employee ids.
func (MasterRepository) ResolveEmployeeIDs(ctx context.Context, tx *sql.Tx, nik string, companyID int16) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT e.pe_id FROM payroll.pyr_employee e
		WHERE ($2::smallint = 0 OR e.pe_company_id = $2)
		  AND (e.pe_nik = $1 OR EXISTS (SELECT 1 FROM payroll.pyr_employee_nik_hist h WHERE h.penh_employee_id = e.pe_id AND h.penh_nik = $1))
		ORDER BY 1 LIMIT 2`, nik, companyID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// listEmployeesSQL is built only from the package-level column list; all values are bind parameters.
var listEmployeesSQL = `SELECT ` + employeeCols + ` FROM payroll.pyr_employee
		WHERE pe_id > $3
		  AND ($4::smallint = 0 OR pe_company_id = $4)
		  AND ($5::text = '' OR pe_division = $5)
		  AND ($6::int = 0 OR pe_sts_payroll = $6)
		  AND ($7::text = '' OR pe_nik = $7 OR pe_nama ILIKE '%' || $7 || '%'
		       OR EXISTS (SELECT 1 FROM payroll.pyr_employee_nik_hist h WHERE h.penh_employee_id = pe_id AND h.penh_nik = $7))
		ORDER BY pe_id LIMIT $8` //nolint:gosec // no user input in the SQL text

// ListEmployees lists employees ordered by id (keyset on AfterID).
func (MasterRepository) ListEmployees(ctx context.Context, tx *sql.Tx, f domain.EmployeeFilter, p domain.PII) ([]domain.Employee, error) {
	rows, err := tx.QueryContext(ctx, listEmployeesSQL,
		p.Reveal, p.EncKey, f.AfterID, f.CompanyID, f.Division, f.StsPayroll, f.Q, f.Limit)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	var out []domain.Employee
	for rows.Next() {
		e, err := scanEmployee(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// UpdateEmployee updates an employee when its version equals ifMatch. Empty PII
// arguments keep the stored ciphertext; NIK, company and division are immutable here.
func (r MasterRepository) UpdateEmployee(ctx context.Context, tx *sql.Tx, id int64, e domain.Employee, npwp, ktp, bankAcnt, actor string, ifMatch int, p domain.PII) (domain.Employee, error) {
	const q = `UPDATE payroll.pyr_employee SET
	  pe_nama=$2, pe_email=NULLIF($3,''), pe_dept_id=NULLIF($4,''), pe_dept=NULLIF($5,''), pe_section_id=NULLIF($6,''), pe_section=NULLIF($7,''),
	  pe_designation_id=NULLIF($8,''), pe_designation=NULLIF($9,''), pe_grade=NULLIF($10,''), pe_level=NULLIF($11,''), pe_tax_id=NULLIF($12,''),
	  pe_sts_emp=NULLIF($13,0), pe_sts_payroll=$14, pe_sts_bonus=NULLIF($15,0),
	  pe_date_join=NULLIF($16,'')::date, pe_resign_date=NULLIF($17,'')::date, pe_birth_date=NULLIF($18,'')::date, pe_birth_place=NULLIF($19,''),
	  pe_sex=NULLIF($20,''), pe_married_sts=NULLIF($21,''), pe_religion=NULLIF($22,''),
	  pe_npwp_enc = CASE WHEN $23::text='' THEN pe_npwp_enc ELSE payroll.pgp_sym_encrypt($23::text,$1::text) END,
	  pe_npwp_hash = CASE WHEN $23::text='' THEN pe_npwp_hash ELSE $26 END,
	  pe_ktp_enc = CASE WHEN $24::text='' THEN pe_ktp_enc ELSE payroll.pgp_sym_encrypt($24::text,$1::text) END,
	  pe_bank_acnt_enc = CASE WHEN $25::text='' THEN pe_bank_acnt_enc ELSE payroll.pgp_sym_encrypt($25::text,$1::text) END,
	  pe_bank=NULLIF($27,''), pe_bank_branch=NULLIF($28,''), pe_bank_acnt_name=NULLIF($29,''),
	  pe_orion_sub_acnt=$30, pe_id_mesin=NULLIF($31,''), pe_cuti_tahunan=NULLIF($32,'')::numeric,
	  pe_pay_currency=$33, pe_employment_status=NULLIF($34,''), pe_cost_group=NULLIF($35,''),
	  pe_hris_sys_id=NULLIF($36,0), updated_by=$37, updated_at=now(), pe_version=pe_version+1
	WHERE pe_id=$38 AND pe_version=$39 RETURNING pe_id`
	var got int64
	err := tx.QueryRowContext(ctx, q, p.EncKey,
		e.Nama, e.Email, e.DeptID, e.Dept, e.SectionID, e.Section,
		e.DesignationID, e.Designation, e.Grade, e.Level, e.TaxID,
		e.StsEmp, e.StsPayroll, e.StsBonus,
		e.DateJoin, e.ResignDate, e.BirthDate, e.BirthPlace,
		e.Sex, e.MarriedSts, e.Religion,
		npwp, ktp, bankAcnt, NPWPHash(p.HMACKey, npwp),
		e.Bank, e.BankBranch, e.BankAcntName,
		e.OrionSubAcnt, e.IDMesin, e.CutiTahunan,
		e.PayCurrency, e.EmploymentStatus, e.CostGroup,
		e.HrisSysID, actor, id, ifMatch).Scan(&got)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Employee{}, r.versionOrNotFound(ctx, tx, `SELECT 1 FROM payroll.pyr_employee WHERE pe_id=$1`, id)
	}
	if err != nil {
		return domain.Employee{}, mapErr(err)
	}
	return r.GetEmployeeByID(ctx, tx, id, p)
}
