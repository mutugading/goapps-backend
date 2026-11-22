// Package master holds the payroll master entities (currency, company,
// division, employee) and their repository contract (07§1, 04§4/§14/§18).
package master

import (
	"context"
	"database/sql"
	"time"
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
	PrevEmployeeID                                               int64
	HrisSysID                                                    string
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
	// ListCompanies returns only the given company ids, so PII outside the scope is never decrypted.
	ListCompanies(ctx context.Context, tx *sql.Tx, ids []int16, p PII) ([]Company, error)
	UpdateCompany(ctx context.Context, tx *sql.Tx, c Company, npwp string, ifMatch int, p PII) (Company, error)

	CreateDivision(ctx context.Context, tx *sql.Tx, d Division) (Division, error)
	GetDivision(ctx context.Context, tx *sql.Tx, companyID int16, code string) (Division, error)
	ListDivisions(ctx context.Context, tx *sql.Tx, companyID int16) ([]Division, error)
	UpdateDivision(ctx context.Context, tx *sql.Tx, d Division, ifMatch int) (Division, error)

	CreateEmployee(ctx context.Context, tx *sql.Tx, e Employee, npwp, ktp, bankAcnt, actor string, p PII) (Employee, error)
	GetEmployeeByID(ctx context.Context, tx *sql.Tx, id int64, p PII) (Employee, error)
	// ResolveEmployeeIDs maps a current or historical NIK to distinct employee ids (13§3.1).
	// division "" = any division in scope.
	ResolveEmployeeIDs(ctx context.Context, tx *sql.Tx, nik string, companyID int16, division string) ([]int64, error)
	ListEmployees(ctx context.Context, tx *sql.Tx, f EmployeeFilter, p PII) ([]Employee, error)
	UpdateEmployee(ctx context.Context, tx *sql.Tx, id int64, e Employee, npwp, ktp, bankAcnt, actor string, ifMatch int, p PII) (Employee, error)
	// TaxStatusKnown reports whether code is an EMPTAX code (pe_tax_id) of an ACTIVE rule set.
	TaxStatusKnown(ctx context.Context, tx *sql.Tx, code string) (bool, error)
	// CorrectTaxStatus records taxID as the status on validFrom (YYYY-MM-DD) until the next recorded change, and sets
	// pe_tax_id when no change is recorded after validFrom (D-82 (5)/(6), T033n2). It returns the history before
	// the correction (for the audit log) and ErrTaxYearFinal when an affected tax year is already final.
	CorrectTaxStatus(ctx context.Context, tx *sql.Tx, id int64, taxID, validFrom, reason string) ([]TaxHistory, error)
	// ListTaxHistory returns the tax-status history of one employee, newest first, for display (D-82 (1), T033n4).
	ListTaxHistory(ctx context.Context, tx *sql.Tx, id int64) ([]TaxHistoryEntry, error)

	// Lock* take a row lock (SELECT ... FOR UPDATE) so the "before" image read for
	// the audit log cannot differ from the row the UPDATE then changes.
	LockCurrency(ctx context.Context, tx *sql.Tx, code string) error
	LockCompany(ctx context.Context, tx *sql.Tx, id int16) error
	LockDivision(ctx context.Context, tx *sql.Tx, companyID int16, code string) error
	LockEmployee(ctx context.Context, tx *sql.Tx, id int64) error
}

// TaxHistory is one pyr_employee_tax_hist row (D-82): the tax status valid from a date.
type TaxHistory struct {
	ValidFrom string // YYYY-MM-DD
	TaxID     string
	Source    string
}

// TaxHistoryEntry is a pyr_employee_tax_hist row with its provenance, for the read RPC (T033n4).
type TaxHistoryEntry struct {
	ValidFrom string // YYYY-MM-DD
	TaxID     string
	Source    string // BACKFILL | INSERT | UPDATE | CORRECT
	Reason    string // CORRECT rows only
	CreatedBy string
	CreatedAt time.Time
}
