// Package master holds the payroll master entities (currency, company,
// division, employee) and their repository contract (07§1, 04§4/§14/§18).
package master

import (
	"context"
	"database/sql"
)

// MaskedValue replaces PII in responses and logs for callers without permission (D-17).
const MaskedValue = "***"

// Currency is pyr_currency.
type Currency struct {
	Code          string
	MinorUnit     int
	TakehomeRound int
	Version       int
}

// Company is pyr_company. NPWP is MaskedValue, "" (absent) or plaintext when revealed.
type Company struct {
	ID           int16
	Code         string
	Name         string
	NPWP         string `json:"npwp"`
	BaseCurrency string
	Active       bool
	Version      int
}

// Division is pyr_division_ref.
type Division struct {
	CompanyID int16
	Code      string
	Name      string
	Version   int
}

// Employee is pyr_employee. PII fields follow the same masking rule as Company.NPWP.
// Date fields are "YYYY-MM-DD" or empty.
type Employee struct {
	ID        int64
	NIK       string
	CompanyID int16
	Division  string

	Nama, Email                                                  string
	DeptID, Dept, SectionID, Section, DesignationID, Designation string
	Grade, Level, TaxID                                          string
	StsEmp, StsPayroll, StsBonus                                 int
	DateJoin, ResignDate, BirthDate, BirthPlace                  string
	Sex, MarriedSts, Religion                                    string
	NPWP                                                         string `json:"npwp"`
	KTP                                                          string `json:"ktp"`
	Bank, BankBranch, BankAcntName                               string
	BankAcnt                                                     string `json:"bank_acct"`
	OrionSubAcnt, IDMesin, CutiTahunan                           string
	PayCurrency, EmploymentStatus, CostGroup                     string
	HrisSysID, PrevEmployeeID                                    int64
	PlacementReason                                              string

	Version   int
	CreatedBy string
	CreatedAt string
	UpdatedBy string
	UpdatedAt string
}

// EmployeeFilter selects ListEmployees rows (RLS still applies on top).
type EmployeeFilter struct {
	CompanyID  int16
	Division   string
	StsPayroll int
	Q          string
	AfterID    int64
	Limit      int
}

// PII carries the plaintext secrets needed to write encrypted columns and the
// keys that protect them (D-17). Keys come from config, never from the DB.
type PII struct {
	EncKey  string
	HMACKey []byte
	Reveal  bool // decrypt PII in returned rows
}

// Repository persists master data. Every method runs inside the caller's
// ScopedTx so RLS (app.scopes) applies.
type Repository interface {
	CreateCurrency(ctx context.Context, tx *sql.Tx, c Currency) (Currency, error)
	ListCurrencies(ctx context.Context, tx *sql.Tx) ([]Currency, error)
	UpdateCurrency(ctx context.Context, tx *sql.Tx, c Currency, ifMatch int) (Currency, error)
	GetCurrency(ctx context.Context, tx *sql.Tx, code string) (Currency, error)

	CreateCompany(ctx context.Context, tx *sql.Tx, c Company, npwp string, p PII) (Company, error)
	GetCompany(ctx context.Context, tx *sql.Tx, id int16, p PII) (Company, error)
	ListCompanies(ctx context.Context, tx *sql.Tx, p PII) ([]Company, error)
	UpdateCompany(ctx context.Context, tx *sql.Tx, c Company, npwp string, ifMatch int, p PII) (Company, error)

	CreateDivision(ctx context.Context, tx *sql.Tx, d Division) (Division, error)
	GetDivision(ctx context.Context, tx *sql.Tx, companyID int16, code string) (Division, error)
	ListDivisions(ctx context.Context, tx *sql.Tx, companyID int16) ([]Division, error)
	UpdateDivision(ctx context.Context, tx *sql.Tx, d Division, ifMatch int) (Division, error)

	CreateEmployee(ctx context.Context, tx *sql.Tx, e Employee, npwp, ktp, bankAcnt, actor string, p PII) (Employee, error)
	GetEmployeeByID(ctx context.Context, tx *sql.Tx, id int64, p PII) (Employee, error)
	// ResolveEmployeeIDs maps a current or historical NIK to distinct employee ids (13§3.1).
	ResolveEmployeeIDs(ctx context.Context, tx *sql.Tx, nik string, companyID int16) ([]int64, error)
	ListEmployees(ctx context.Context, tx *sql.Tx, f EmployeeFilter, p PII) ([]Employee, error)
	UpdateEmployee(ctx context.Context, tx *sql.Tx, id int64, e Employee, npwp, ktp, bankAcnt, actor string, ifMatch int, p PII) (Employee, error)
}
