package etl

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/postgres"
)

// EmployeeOptions controls one employee + attachment load (T009b, 08§2, 13§3).
type EmployeeOptions struct {
	CompanyID   int16
	Divisions   []string // default SOLO, JAKARTA (08§2: hanya dua divisi ini)
	EncKey      string   // PAYROLL_PII_KEY (D-17): bind parameter only
	HMACKey     string   // PAYROLL_PII_HMAC_KEY
	ExpectEmp   int      // 0 = tidak dicek; produksi 2.860 sebelum filter divisi
	ExpectCompE int      // 0 = tidak dicek; produksi 117.827
}

var defaultDivisions = []string{"SOLO", "JAKARTA"}

// parseDate accepts ISO (2006-01-02, optionally with time) and Oracle DD-MON-YY(YY); "" stays "" (NULL).
func parseDate(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	if len(s) > 10 && (s[10] == ' ' || s[10] == 'T') {
		s = s[:10]
	}
	for _, layout := range []string{"2006-01-02", "02-Jan-2006", "02-Jan-06"} {
		if t, err := time.Parse(layout, titleMon(s)); err == nil {
			return t.Format("2006-01-02"), nil
		}
	}
	return "", errors.New("tanggal tidak valid")
}

// titleMon turns "05-JAN-24" into "05-Jan-24" so time.Parse accepts Oracle's upper-case month.
func titleMon(s string) string {
	if len(s) >= 6 && s[2] == '-' && s[5] == '-' {
		return s[:3] + s[3:4] + strings.ToLower(s[4:6]) + s[6:]
	}
	return s
}

// birthFromPassword parses legacy PE_PASSWORD DDMMYYYY (08§2); ok=false when not a valid date.
func birthFromPassword(pw string) (string, bool) {
	if len(pw) != 8 {
		return "", false
	}
	t, err := time.Parse("02012006", pw)
	if err != nil {
		return "", false
	}
	return t.Format("2006-01-02"), true
}

// SubTable returns the rows whose column col equals one of vals (case-insensitive) as a new Table.
func (t *Table) SubTable(col string, vals ...string) (*Table, error) {
	ci, err := t.Col(col)
	if err != nil {
		return nil, err
	}
	out := &Table{Name: t.Name, Cols: t.Cols}
	for _, r := range t.Rows {
		for _, v := range vals {
			if strings.EqualFold(r[ci], v) {
				out.Rows = append(out.Rows, r)
				break
			}
		}
	}
	return out, nil
}

type empKey struct{ nik, div string }

// LoadEmployees fills pyr_employee + pyr_employee_nik_hist (PAYROLL_NIK) for one division's rows.
// PII is encrypted in SQL (pgcrypto) with keys passed as bind parameters (D-17); PE_PASSWORD, photo
// links and address are not migrated. hris maps NIK → HMEMD_SYS_ID (nil = unmatched warning for all).
func LoadEmployees(ctx context.Context, tx *sql.Tx, src *Table, division string, hris map[string]string, o EmployeeOptions) (TableReport, map[empKey]bool, error) {
	rep := TableReport{Table: fmt.Sprintf("%s[%s]", TableEmployee, division), SrcRows: len(src.Rows), Checksum: src.Checksum()}
	loaded := map[empKey]bool{}
	idx, err := bindCols(src, employeeCols)
	if err != nil {
		return rep, nil, err
	}
	pwIdx := -1 // PE_PASSWORD is read only to derive a missing birth date, never stored
	if i, perr := src.Col("PE_PASSWORD"); perr == nil {
		pwIdx = i
	}
	stmt, err := tx.PrepareContext(ctx, upsertEmployeeSQL)
	if err != nil {
		return rep, nil, err
	}
	defer closeQuiet(stmt)

	for i, r := range src.Rows {
		c := func(n string) string { return r[idx[n]] }
		nik := c("PE_NIK")
		where := fmt.Sprintf("%s/%s (baris %d)", nik, division, i+2)
		if nik == "" || c("PE_NAMA") == "" {
			rep.Exceptions = append(rep.Exceptions, where+": NIK/nama kosong")
			continue
		}
		dates, warns, err := employeeDates(c, r, pwIdx)
		if err != nil {
			return rep, nil, fmt.Errorf("%s: %w", where, err)
		}
		for _, w := range warns {
			rep.Warnings = append(rep.Warnings, where+": "+w)
		}
		sys, w := hrisSysID(hris, nik)
		if w != "" {
			rep.Warnings = append(rep.Warnings, where+": "+w)
		}
		var id int64
		if err = stmt.QueryRowContext(ctx, employeeArgs(o, division, c, dates, sys)...).Scan(&id); err != nil {
			return rep, nil, fmt.Errorf("%s: %w", where, scrub(err))
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO payroll.pyr_employee_nik_hist (penh_employee_id, penh_nik, penh_valid_from, penh_source)
VALUES ($1,$2,COALESCE($3::date,'2000-01-01'),'PAYROLL_NIK') ON CONFLICT DO NOTHING`, id, nik, nullable(dates[0])); err != nil {
			return rep, nil, fmt.Errorf("%s: nik_hist: %w", where, err)
		}
		loaded[empKey{nik, division}] = true
	}
	err = tx.QueryRowContext(ctx, `SELECT count(*) FROM payroll.pyr_employee WHERE pe_company_id=$1 AND pe_division=$2::payroll.division`,
		o.CompanyID, division).Scan(&rep.DstRows)
	return rep, loaded, err
}

// hrisSysID resolves HMEMD_SYS_ID for a NIK; a warning (13§3.5) when HRIS was provided but has no match.
func hrisSysID(hris map[string]string, nik string) (any, string) {
	if hris == nil {
		return nil, ""
	}
	if v := hris[nik]; v != "" {
		return v, ""
	}
	return nil, "tidak ada HMEMD_SYS_ID di HRIS (13§3.5)"
}

var employeeCols = []string{"PE_NIK", "PE_NAMA", "PE_EMAIL_ADDR", "PE_DEPT_ID", "PE_DEPT", "PE_SECTION_ID", "PE_SECTION",
	"PE_DATE_JOIN", "PE_SEX", "PE_MARRIED_STS", "PE_BIRTH_DATE", "PE_BIRTH_PLACE", "PE_RELIGION", "PE_GRADE", "PE_TAX_ID",
	"PE_STS_EMP", "PE_DESIGNATION_ID", "PE_DESIGNATION", "PE_BANK", "PE_BANK_ACNT", "PE_BANK_ACNT_NAME", "PE_BANK_BRANCH",
	"PE_RESIGN_DATE", "PE_LEVEL", "PE_NPWP", "PE_STS_PAYROLL", "PE_STS_BONUS", "PE_KTP", "PE_CUTI_TAHUNAN", "PE_ID_MESIN", "PE_SUB_ACNT"}

// bindCols maps each required column name to its index or fails naming the missing one.
func bindCols(t *Table, names []string) (map[string]int, error) {
	idx := make(map[string]int, len(names))
	for _, n := range names {
		i, err := t.Col(n)
		if err != nil {
			return nil, err
		}
		idx[n] = i
	}
	return idx, nil
}

// employeeDates returns [join, resign, birth] (""=NULL). A missing birth date falls back to
// PE_PASSWORD DDMMYYYY (08§2); failures become warnings for HR, not errors.
func employeeDates(c func(string) string, r []string, pwIdx int) ([3]string, []string, error) {
	var d [3]string
	var warns []string
	var err error
	for j, n := range []string{"PE_DATE_JOIN", "PE_RESIGN_DATE", "PE_BIRTH_DATE"} {
		if d[j], err = parseDate(c(n)); err != nil {
			return d, nil, fmt.Errorf("%s: %w", n, err)
		}
	}
	if d[2] == "" && pwIdx >= 0 {
		if b, ok := birthFromPassword(r[pwIdx]); ok {
			d[2] = b
		} else if r[pwIdx] != "" {
			warns = append(warns, "tanggal lahir kosong dan PE_PASSWORD bukan DDMMYYYY valid")
		}
	}
	if d[2] == "" {
		warns = append(warns, "tanggal lahir kosong")
	}
	return d, warns, nil
}

// employeeArgs builds the bind list of upsertEmployeeSQL ($1 = encryption key, never logged).
func employeeArgs(o EmployeeOptions, division string, c func(string) string, dates [3]string, sys any) []any {
	sub := c("PE_SUB_ACNT")
	if sub == "" {
		sub = c("PE_NIK")
	}
	npwp := c("PE_NPWP")
	var hash any
	if h := postgres.NPWPHash([]byte(o.HMACKey), npwp); h != nil {
		hash = h
	}
	stsPayroll := c("PE_STS_PAYROLL")
	if stsPayroll == "" {
		stsPayroll = "2"
	}
	return []any{o.EncKey,
		o.CompanyID, division, c("PE_NIK"), c("PE_NAMA"), nullable(c("PE_EMAIL_ADDR")), nullable(c("PE_DEPT_ID")), nullable(c("PE_DEPT")),
		nullable(c("PE_SECTION_ID")), nullable(c("PE_SECTION")), nullable(c("PE_DESIGNATION_ID")), nullable(c("PE_DESIGNATION")),
		nullable(c("PE_GRADE")), nullable(c("PE_LEVEL")), nullable(c("PE_TAX_ID")), nullable(c("PE_STS_EMP")), stsPayroll,
		nullable(c("PE_STS_BONUS")), nullable(dates[0]), nullable(dates[1]), nullable(dates[2]), nullable(c("PE_BIRTH_PLACE")), nullable(c("PE_SEX")),
		nullable(c("PE_MARRIED_STS")), nullable(c("PE_RELIGION")), nullable(npwp), nullable(c("PE_KTP")), nullable(c("PE_BANK_ACNT")),
		hash, nullable(c("PE_BANK")), nullable(c("PE_BANK_BRANCH")), nullable(c("PE_BANK_ACNT_NAME")), sub, nullable(c("PE_ID_MESIN")),
		nullable(c("PE_CUTI_TAHUNAN")), sys}
}

const upsertEmployeeSQL = `
INSERT INTO payroll.pyr_employee (pe_company_id, pe_division, pe_nik, pe_nama, pe_email, pe_dept_id, pe_dept, pe_section_id, pe_section,
  pe_designation_id, pe_designation, pe_grade, pe_level, pe_tax_id, pe_sts_emp, pe_sts_payroll, pe_sts_bonus, pe_date_join, pe_resign_date,
  pe_birth_date, pe_birth_place, pe_sex, pe_married_sts, pe_religion, pe_npwp_enc, pe_ktp_enc, pe_bank_acnt_enc, pe_npwp_hash, pe_bank,
  pe_bank_branch, pe_bank_acnt_name, pe_orion_sub_acnt, pe_id_mesin, pe_cuti_tahunan, pe_hris_sys_id, created_by)
VALUES ($2,$3::payroll.division,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16::smallint,$17::smallint,$18::smallint,$19::date,$20::date,$21::date,$22,$23,$24,$25,
  CASE WHEN $26::text IS NULL THEN NULL ELSE payroll.pgp_sym_encrypt($26::text,$1::text) END,
  CASE WHEN $27::text IS NULL THEN NULL ELSE payroll.pgp_sym_encrypt($27::text,$1::text) END,
  CASE WHEN $28::text IS NULL THEN NULL ELSE payroll.pgp_sym_encrypt($28::text,$1::text) END,
  $29,$30,$31,$32,$33,$34,$35::numeric,$36::varchar,'ETL')
ON CONFLICT (pe_company_id, pe_nik, pe_division) DO UPDATE SET
  pe_nama=EXCLUDED.pe_nama, pe_email=EXCLUDED.pe_email, pe_dept_id=EXCLUDED.pe_dept_id, pe_dept=EXCLUDED.pe_dept,
  pe_section_id=EXCLUDED.pe_section_id, pe_section=EXCLUDED.pe_section, pe_designation_id=EXCLUDED.pe_designation_id,
  pe_designation=EXCLUDED.pe_designation, pe_grade=EXCLUDED.pe_grade, pe_level=EXCLUDED.pe_level, pe_tax_id=EXCLUDED.pe_tax_id,
  pe_sts_emp=EXCLUDED.pe_sts_emp, pe_sts_payroll=EXCLUDED.pe_sts_payroll, pe_sts_bonus=EXCLUDED.pe_sts_bonus,
  pe_date_join=EXCLUDED.pe_date_join, pe_resign_date=EXCLUDED.pe_resign_date, pe_birth_date=EXCLUDED.pe_birth_date,
  pe_birth_place=EXCLUDED.pe_birth_place, pe_sex=EXCLUDED.pe_sex, pe_married_sts=EXCLUDED.pe_married_sts,
  pe_religion=EXCLUDED.pe_religion, pe_npwp_enc=EXCLUDED.pe_npwp_enc, pe_ktp_enc=EXCLUDED.pe_ktp_enc,
  pe_bank_acnt_enc=EXCLUDED.pe_bank_acnt_enc, pe_npwp_hash=EXCLUDED.pe_npwp_hash, pe_bank=EXCLUDED.pe_bank,
  pe_bank_branch=EXCLUDED.pe_bank_branch, pe_bank_acnt_name=EXCLUDED.pe_bank_acnt_name,
  pe_id_mesin=EXCLUDED.pe_id_mesin, pe_cuti_tahunan=EXCLUDED.pe_cuti_tahunan,
  pe_hris_sys_id=COALESCE(EXCLUDED.pe_hris_sys_id, payroll.pyr_employee.pe_hris_sys_id), updated_by='ETL', updated_at=now()
RETURNING pe_id`

// scrub drops driver detail that could echo a PII value (D-17); the SQLSTATE and message stay.
func scrub(err error) error {
	var s interface{ SQLState() string }
	if errors.As(err, &s) {
		return fmt.Errorf("sqlstate %s", s.SQLState())
	}
	return err
}

type tmplKey struct {
	level string
	sysid int64
}
type tmplVal struct{ sts, tax, amount, paid string }

func norm(s string) string {
	if s == "" {
		return ""
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return strconv.FormatFloat(f, 'f', 6, 64)
	}
	return s
}

// LoadEmployeeComponents fills pyr_employee_component for one division via legacy_map (D-20).
// A row is labeled TEMPLATE when it equals the level template (D-22) and MANUAL otherwise.
// Rows whose employee was not loaded or whose (type,id) has no sysid map are reported as exceptions.
func LoadEmployeeComponents(ctx context.Context, tx *sql.Tx, src *Table, division string, emps map[empKey]bool, o EmployeeOptions) (TableReport, error) {
	rep := TableReport{Table: fmt.Sprintf("%s[%s]", TableEmployeeComponent, division), SrcRows: len(src.Rows), Checksum: src.Checksum()}
	idx, err := bindCols(src, []string{"MPCE_NIK", "MPCE_TYPE", "MPCE_ID", "MPCE_NAME", "MPCE_STS", "MPCE_TAX", "MPCE_AMOUNT",
		"MPCE_FRZ_FLAG", "MPCE_LEVEL", "MPCE_TAX_PAID_COMPANY"})
	if err != nil {
		return rep, err
	}
	sysids, err := loadSysids(ctx, tx, o.CompanyID, division)
	if err != nil {
		return rep, err
	}
	tmpl, err := loadTemplates(ctx, tx, o.CompanyID)
	if err != nil {
		return rep, err
	}
	stmt, err := tx.PrepareContext(ctx, upsertAttachmentSQL)
	if err != nil {
		return rep, err
	}
	defer closeQuiet(stmt)

	for i, r := range src.Rows {
		c := func(n string) string { return r[idx[n]] }
		id, err := strconv.Atoi(c("MPCE_ID"))
		if err != nil {
			return rep, fmt.Errorf("%s baris %d: MPCE_ID %q", src.Name, i+2, c("MPCE_ID"))
		}
		typ := compType(c("MPCE_TYPE"))
		where := fmt.Sprintf("%s/%s %s/%d (baris %d)", c("MPCE_NIK"), division, typ, id, i+2)
		if !emps[empKey{c("MPCE_NIK"), division}] {
			rep.Exceptions = append(rep.Exceptions, where+": karyawan tidak ada di pyr_employee")
			continue
		}
		sysid, ok := sysids[fmt.Sprintf("%s/%d", typ, id)]
		if !ok {
			rep.Exceptions = append(rep.Exceptions, where+": komponen tanpa peta sysid")
			continue
		}
		frz := c("MPCE_FRZ_FLAG")
		if frz == "" {
			frz = "2"
		}
		if _, err = stmt.ExecContext(ctx, o.CompanyID, c("MPCE_NIK"), division, sysid, typ, id, nullable(c("MPCE_NAME")),
			nullable(c("MPCE_STS")), nullable(c("MPCE_TAX")), nullable(c("MPCE_AMOUNT")), frz, nullable(c("MPCE_TAX_PAID_COMPANY")),
			attachmentSource(tmpl, sysid, c)); err != nil {
			return rep, fmt.Errorf("%s: %w", where, err)
		}
	}
	err = tx.QueryRowContext(ctx, `SELECT count(*) FROM payroll.pyr_employee_component WHERE pec_company_id=$1 AND pec_division=$2::payroll.division`,
		o.CompanyID, division).Scan(&rep.DstRows)
	return rep, err
}

const upsertAttachmentSQL = `
INSERT INTO payroll.pyr_employee_component (pec_company_id, pec_nik, pec_division, pec_component_sysid, pec_type, pec_id, pec_name,
  pec_sts, pec_tax, pec_amount, pec_frz_flag, pec_tax_paid_company, pec_source)
VALUES ($1,$2,$3::payroll.division,$4,$5::payroll.comp_type,$6,$7,$8,$9::smallint,$10::numeric,$11::smallint,$12,$13)
ON CONFLICT (pec_company_id, pec_nik, pec_division, pec_component_sysid) DO UPDATE SET pec_name=EXCLUDED.pec_name,
  pec_sts=EXCLUDED.pec_sts, pec_tax=EXCLUDED.pec_tax, pec_amount=EXCLUDED.pec_amount, pec_frz_flag=EXCLUDED.pec_frz_flag,
  pec_tax_paid_company=EXCLUDED.pec_tax_paid_company, pec_source=EXCLUDED.pec_source, updated_by='ETL', updated_at=now()`

// attachmentSource labels a row TEMPLATE when it equals its level template, else MANUAL (D-22).
func attachmentSource(tmpl map[tmplKey]tmplVal, sysid int64, c func(string) string) string {
	t, ok := tmpl[tmplKey{c("MPCE_LEVEL"), sysid}]
	if ok && t.sts == c("MPCE_STS") && t.tax == c("MPCE_TAX") && t.amount == norm(c("MPCE_AMOUNT")) && t.paid == c("MPCE_TAX_PAID_COMPANY") {
		return LegacySource
	}
	return "MANUAL"
}

// loadSysids returns "type/id" → sysid for one legacy division.
func loadSysids(ctx context.Context, tx *sql.Tx, company int16, division string) (map[string]int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT pclm_legacy_type::text, pclm_legacy_id, pclm_sysid FROM payroll.pyr_component_legacy_map
WHERE pclm_company_id=$1 AND pclm_legacy_division=$2`, company, division)
	if err != nil {
		return nil, err
	}
	defer closeQuiet(rows)
	out := map[string]int64{}
	for rows.Next() {
		var t string
		var id int
		var s int64
		if err = rows.Scan(&t, &id, &s); err != nil {
			return nil, err
		}
		out[fmt.Sprintf("%s/%d", t, id)] = s
	}
	return out, rows.Err()
}

// loadTemplates returns the level templates of a company keyed by (level, sysid), amounts normalised.
func loadTemplates(ctx context.Context, tx *sql.Tx, company int16) (map[tmplKey]tmplVal, error) {
	rows, err := tx.QueryContext(ctx, `SELECT t.pct_template, t.pct_component_sysid, coalesce(t.pct_sts,''), coalesce(t.pct_tax::text,''),
coalesce(t.pct_amount::text,''), coalesce(t.pct_tax_paid_company,'') FROM payroll.pyr_component_template t
JOIN payroll.pyr_component c ON c.pc_sysid=t.pct_component_sysid WHERE c.pc_company_id=$1`, company)
	if err != nil {
		return nil, err
	}
	defer closeQuiet(rows)
	out := map[tmplKey]tmplVal{}
	for rows.Next() {
		var k tmplKey
		var v tmplVal
		if err = rows.Scan(&k.level, &k.sysid, &v.sts, &v.tax, &v.amount, &v.paid); err != nil {
			return nil, err
		}
		v.amount = norm(v.amount)
		out[k] = v
	}
	return out, rows.Err()
}

// RunEmployees loads employees then attachments for every division in ONE transaction and logs per-division
// counts + checksums to pyr_migration_log. Source rows that cannot be loaded are listed in the report's
// Exceptions (08§6); master/template must be loaded first.
func RunEmployees(ctx context.Context, db *sql.DB, src Source, o EmployeeOptions) ([]TableReport, error) {
	if o.EncKey == "" || o.HMACKey == "" {
		return nil, errors.New("kunci PII (PAYROLL_PII_KEY, PAYROLL_PII_HMAC_KEY) wajib (D-17)")
	}
	if len(o.Divisions) == 0 {
		o.Divisions = defaultDivisions
	}
	emp, err := src.Read(TableEmployee)
	if err != nil {
		return nil, err
	}
	ce, err := src.Read(TableEmployeeComponent)
	if err != nil {
		return nil, err
	}
	hris, err := readHRIS(src)
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
	reps, err := loadDivisions(ctx, tx, emp, ce, hris, o)
	if err != nil {
		return reps, err
	}
	if err := logEmployees(ctx, tx, reps, o, len(emp.Rows), len(ce.Rows)); err != nil {
		return reps, err
	}
	return reps, tx.Commit()
}

// readHRIS reads the optional HM_EMP_DATA spool; absent = nil map (pe_hris_sys_id stays empty).
func readHRIS(src Source) (map[string]string, error) {
	h, err := src.Read(TableHRISEmployee)
	if err != nil {
		log.Warn().Err(err).Msg("etl: HM_EMP_DATA tidak tersedia, pe_hris_sys_id dikosongkan")
		return nil, nil //nolint:nilerr,nilnil // optional source; nil map = "not provided"
	}
	return hrisMap(h)
}

func loadDivisions(ctx context.Context, tx *sql.Tx, emp, ce *Table, hris map[string]string, o EmployeeOptions) ([]TableReport, error) {
	var reps []TableReport
	loaded := map[string]map[empKey]bool{}
	for _, d := range o.Divisions {
		sub, err := emp.SubTable("PE_DIVISION", d)
		if err != nil {
			return reps, err
		}
		r, keys, err := LoadEmployees(ctx, tx, sub, d, hris, o)
		if err != nil {
			return reps, err
		}
		reps = append(reps, r)
		loaded[d] = keys
	}
	for _, d := range o.Divisions {
		sub, err := ce.SubTable("MPCE_DIVISION", d)
		if err != nil {
			return reps, err
		}
		r, err := LoadEmployeeComponents(ctx, tx, sub, d, loaded[d], o)
		if err != nil {
			return reps, err
		}
		reps = append(reps, r)
	}
	return reps, nil
}

func logEmployees(ctx context.Context, tx *sql.Tx, reps []TableReport, o EmployeeOptions, empAll, ceAll int) error {
	for _, r := range reps {
		if !r.OK() {
			return fmt.Errorf("%s: sumber %d != tujuan %d + exception %d", r.Table, r.SrcRows, r.DstRows, len(r.Exceptions))
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO payroll.pyr_migration_log (pml_company_id, pml_table, pml_src_rows, pml_dst_rows, pml_checksum, pml_ok)
VALUES ($1,$2,$3,$4,$5,true)`, o.CompanyID, r.Table, r.SrcRows, r.DstRows, r.Checksum); err != nil {
			return err
		}
	}
	if o.ExpectEmp > 0 && empAll != o.ExpectEmp {
		return fmt.Errorf("MGT_PAYROLL_EMP: %d baris, diharapkan %d", empAll, o.ExpectEmp)
	}
	if o.ExpectCompE > 0 && ceAll != o.ExpectCompE {
		return fmt.Errorf("MGT_PAYROLL_COMPNT_EMP: %d baris, diharapkan %d", ceAll, o.ExpectCompE)
	}
	return nil
}

func hrisMap(t *Table) (map[string]string, error) {
	ni, err := t.Col("HMEMD_NIK")
	if err != nil {
		return nil, err
	}
	si, err := t.Col("HMEMD_SYS_ID")
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, len(t.Rows))
	for _, r := range t.Rows {
		m[r[ni]] = r[si]
	}
	return m, nil
}
