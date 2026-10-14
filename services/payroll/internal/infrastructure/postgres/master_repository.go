package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	domain "github.com/mutugading/goapps-backend/services/payroll/internal/domain/master"
)

// MasterRepository implements master.Repository on pyr_currency, pyr_company,
// pyr_division_ref and pyr_employee. All methods use the caller's ScopedTx, so
// RLS (D-11/D-14) applies. PII is encrypted with pgcrypto using a key passed as
// a bind parameter (D-17); it never appears in SQL text or logs.
type MasterRepository struct{}

var _ domain.Repository = MasterRepository{}

// NewMasterRepository creates a MasterRepository.
func NewMasterRepository() MasterRepository { return MasterRepository{} }

const (
	pgUniqueViolation = "23505"
	pgFKViolation     = "23503"
)

// mapErr translates PostgreSQL constraint errors into domain sentinels.
func mapErr(err error) error {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		switch pe.Code {
		case pgUniqueViolation:
			return fmt.Errorf("%w: %s", domain.ErrDuplicate, pe.ConstraintName)
		case pgFKViolation:
			return fmt.Errorf("%w: %s", domain.ErrReference, pe.ConstraintName)
		}
	}
	return err
}

// ── Currency ─────────────────────────────────────────────────────────────────

const currencyCols = `pcy_code, pcy_minor_unit, pcy_takehome_round, pcy_version`

func scanCurrency(r interface{ Scan(...any) error }) (domain.Currency, error) {
	var c domain.Currency
	err := r.Scan(&c.Code, &c.MinorUnit, &c.TakehomeRound, &c.Version)
	return c, err
}

// CreateCurrency inserts a currency.
func (MasterRepository) CreateCurrency(ctx context.Context, tx *sql.Tx, c domain.Currency) (domain.Currency, error) {
	row := tx.QueryRowContext(ctx, `INSERT INTO payroll.pyr_currency (pcy_code, pcy_minor_unit, pcy_takehome_round)
		VALUES ($1,$2,$3) RETURNING `+currencyCols, c.Code, c.MinorUnit, c.TakehomeRound)
	out, err := scanCurrency(row)
	return out, mapErr(err)
}

// GetCurrency reads one currency.
func (MasterRepository) GetCurrency(ctx context.Context, tx *sql.Tx, code string) (domain.Currency, error) {
	out, err := scanCurrency(tx.QueryRowContext(ctx, `SELECT `+currencyCols+` FROM payroll.pyr_currency WHERE pcy_code=$1`, code))
	if errors.Is(err, sql.ErrNoRows) {
		return out, domain.ErrNotFound
	}
	return out, err
}

// ListCurrencies lists all currencies by code.
func (MasterRepository) ListCurrencies(ctx context.Context, tx *sql.Tx) ([]domain.Currency, error) {
	rows, err := tx.QueryContext(ctx, `SELECT `+currencyCols+` FROM payroll.pyr_currency ORDER BY pcy_code`)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	var out []domain.Currency
	for rows.Next() {
		c, err := scanCurrency(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// UpdateCurrency updates a currency when its version equals ifMatch.
func (r MasterRepository) UpdateCurrency(ctx context.Context, tx *sql.Tx, c domain.Currency, ifMatch int) (domain.Currency, error) {
	out, err := scanCurrency(tx.QueryRowContext(ctx, `UPDATE payroll.pyr_currency
		SET pcy_minor_unit=$2, pcy_takehome_round=$3, pcy_version=pcy_version+1
		WHERE pcy_code=$1 AND pcy_version=$4 RETURNING `+currencyCols, c.Code, c.MinorUnit, c.TakehomeRound, ifMatch))
	if errors.Is(err, sql.ErrNoRows) {
		return out, r.versionOrNotFound(ctx, tx, `SELECT 1 FROM payroll.pyr_currency WHERE pcy_code=$1`, c.Code)
	}
	return out, err
}

// versionOrNotFound distinguishes a stale If-Match from a missing/invisible row.
func (MasterRepository) versionOrNotFound(ctx context.Context, tx *sql.Tx, existsSQL string, args ...any) error {
	var one int
	err := tx.QueryRowContext(ctx, existsSQL, args...).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}
	if err != nil {
		return err
	}
	return domain.ErrVersionMismatch
}

// ── Company ──────────────────────────────────────────────────────────────────

// companyCols expects $1 = reveal (bool), $2 = key (text).
const companyCols = `pco_company_id, pco_code, pco_name,
  CASE WHEN pco_npwp_enc IS NULL THEN '' WHEN $1::boolean THEN payroll.pgp_sym_decrypt(pco_npwp_enc, $2::text) ELSE '***' END,
  pco_base_currency, pco_active, pco_version`

func scanCompany(r interface{ Scan(...any) error }) (domain.Company, error) {
	var c domain.Company
	err := r.Scan(&c.ID, &c.Code, &c.Name, &c.NPWP, &c.BaseCurrency, &c.Active, &c.Version)
	return c, err
}

// CreateCompany inserts a company.
func (MasterRepository) CreateCompany(ctx context.Context, tx *sql.Tx, c domain.Company, npwp string, p domain.PII) (domain.Company, error) {
	row := tx.QueryRowContext(ctx, `INSERT INTO payroll.pyr_company (pco_code, pco_name, pco_npwp_enc, pco_base_currency)
		VALUES ($3,$4, CASE WHEN $5::text='' THEN NULL ELSE payroll.pgp_sym_encrypt($5::text,$2::text) END, $6)
		RETURNING `+companyCols, p.Reveal, p.EncKey, c.Code, c.Name, npwp, c.BaseCurrency)
	out, err := scanCompany(row)
	return out, mapErr(err)
}

// GetCompany reads one company.
func (MasterRepository) GetCompany(ctx context.Context, tx *sql.Tx, id int16, p domain.PII) (domain.Company, error) {
	out, err := scanCompany(tx.QueryRowContext(ctx, `SELECT `+companyCols+` FROM payroll.pyr_company WHERE pco_company_id=$3`,
		p.Reveal, p.EncKey, id))
	if errors.Is(err, sql.ErrNoRows) {
		return out, domain.ErrNotFound
	}
	return out, err
}

// ListCompanies lists companies by id.
func (MasterRepository) ListCompanies(ctx context.Context, tx *sql.Tx, p domain.PII) ([]domain.Company, error) {
	rows, err := tx.QueryContext(ctx, `SELECT `+companyCols+` FROM payroll.pyr_company ORDER BY pco_company_id`, p.Reveal, p.EncKey)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	var out []domain.Company
	for rows.Next() {
		c, err := scanCompany(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// UpdateCompany updates a company when its version equals ifMatch. An empty npwp keeps the stored value.
func (r MasterRepository) UpdateCompany(ctx context.Context, tx *sql.Tx, c domain.Company, npwp string, ifMatch int, p domain.PII) (domain.Company, error) {
	out, err := scanCompany(tx.QueryRowContext(ctx, `UPDATE payroll.pyr_company SET pco_name=$3, pco_active=$4,
		pco_npwp_enc = CASE WHEN $5::text='' THEN pco_npwp_enc ELSE payroll.pgp_sym_encrypt($5::text,$2::text) END,
		pco_version=pco_version+1
		WHERE pco_company_id=$6 AND pco_version=$7 RETURNING `+companyCols,
		p.Reveal, p.EncKey, c.Name, c.Active, npwp, c.ID, ifMatch))
	if errors.Is(err, sql.ErrNoRows) {
		return out, r.versionOrNotFound(ctx, tx, `SELECT 1 FROM payroll.pyr_company WHERE pco_company_id=$1`, c.ID)
	}
	return out, err
}

// ── Division ─────────────────────────────────────────────────────────────────

const divisionCols = `pdr_company_id, pdr_code, COALESCE(pdr_name,''), pdr_version`

func scanDivision(r interface{ Scan(...any) error }) (domain.Division, error) {
	var d domain.Division
	err := r.Scan(&d.CompanyID, &d.Code, &d.Name, &d.Version)
	return d, err
}

// CreateDivision inserts a division.
func (MasterRepository) CreateDivision(ctx context.Context, tx *sql.Tx, d domain.Division) (domain.Division, error) {
	out, err := scanDivision(tx.QueryRowContext(ctx, `INSERT INTO payroll.pyr_division_ref (pdr_company_id, pdr_code, pdr_name)
		VALUES ($1,$2,NULLIF($3,'')) RETURNING `+divisionCols, d.CompanyID, d.Code, d.Name))
	return out, mapErr(err)
}

// GetDivision reads one division.
func (MasterRepository) GetDivision(ctx context.Context, tx *sql.Tx, companyID int16, code string) (domain.Division, error) {
	out, err := scanDivision(tx.QueryRowContext(ctx, `SELECT `+divisionCols+` FROM payroll.pyr_division_ref
		WHERE pdr_company_id=$1 AND pdr_code=$2`, companyID, code))
	if errors.Is(err, sql.ErrNoRows) {
		return out, domain.ErrNotFound
	}
	return out, err
}

// ListDivisions lists the divisions of a company.
func (MasterRepository) ListDivisions(ctx context.Context, tx *sql.Tx, companyID int16) ([]domain.Division, error) {
	rows, err := tx.QueryContext(ctx, `SELECT `+divisionCols+` FROM payroll.pyr_division_ref WHERE pdr_company_id=$1 ORDER BY pdr_code`, companyID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	var out []domain.Division
	for rows.Next() {
		d, err := scanDivision(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// UpdateDivision updates a division when its version equals ifMatch.
func (r MasterRepository) UpdateDivision(ctx context.Context, tx *sql.Tx, d domain.Division, ifMatch int) (domain.Division, error) {
	out, err := scanDivision(tx.QueryRowContext(ctx, `UPDATE payroll.pyr_division_ref SET pdr_name=NULLIF($3,''), pdr_version=pdr_version+1
		WHERE pdr_company_id=$1 AND pdr_code=$2 AND pdr_version=$4 RETURNING `+divisionCols, d.CompanyID, d.Code, d.Name, ifMatch))
	if errors.Is(err, sql.ErrNoRows) {
		return out, r.versionOrNotFound(ctx, tx, `SELECT 1 FROM payroll.pyr_division_ref WHERE pdr_company_id=$1 AND pdr_code=$2`, d.CompanyID, d.Code)
	}
	return out, err
}

func lockRow(ctx context.Context, tx *sql.Tx, q string, args ...any) error {
	var one int
	err := tx.QueryRowContext(ctx, q, args...).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}

// LockCurrency locks one currency row until the transaction ends.
func (MasterRepository) LockCurrency(ctx context.Context, tx *sql.Tx, code string) error {
	return lockRow(ctx, tx, `SELECT 1 FROM payroll.pyr_currency WHERE pcy_code=$1 FOR UPDATE`, code)
}

// LockCompany locks one company row until the transaction ends.
func (MasterRepository) LockCompany(ctx context.Context, tx *sql.Tx, id int16) error {
	return lockRow(ctx, tx, `SELECT 1 FROM payroll.pyr_company WHERE pco_company_id=$1 FOR UPDATE`, id)
}

// LockDivision locks one division row until the transaction ends.
func (MasterRepository) LockDivision(ctx context.Context, tx *sql.Tx, companyID int16, code string) error {
	return lockRow(ctx, tx, `SELECT 1 FROM payroll.pyr_division_ref WHERE pdr_company_id=$1 AND pdr_code=$2 FOR UPDATE`, companyID, code)
}
