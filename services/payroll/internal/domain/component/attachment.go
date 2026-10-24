package component

import (
	"context"
	"database/sql"
)

// Attachment sources (12§2.2 rule 4): level-change only touches TEMPLATE rows.
const (
	SourceTemplate = "TEMPLATE"
	SourceManual   = "MANUAL"
)

// Level-change actions.
const (
	ActionAdd    = "ADD"
	ActionRemove = "REMOVE"
	ActionChange = "CHANGE"
)

// Attachment is one pyr_employee_component row: a component attached to an
// employee. Frz 2 = active. ValidFrom/ValidTo 0 = open.
type Attachment struct {
	Sysid          int64
	Code, Name     string
	Sts            string
	Tax            int
	Amount         string
	TaxPaidCompany string
	Frz            int
	Source         string
	ValidFrom      int
	ValidTo        int
}

// Change is one line of a level-change diff.
type Change struct {
	Action        string
	Sysid         int64
	Code, Name    string
	Before, After *Attachment
}

// LevelRule is pyr_level_rule (02§5.1): per-level overrides applied on top of the
// level template. The *Sts columns are freeze flags (2 = active, 1 = frozen) (D-21).
type LevelRule struct {
	Level         string
	Group         string
	OtinworkRate  string
	OtinworkSts   int
	ShiftadjSts   int
	TunjshiftSts  int
	TunjshiftRate string
}

// CatalogEntry is an ACTIVE component an employee may be attached to.
type CatalogEntry struct {
	Sysid      int64
	Code, Name string
	DefaultSts string
}

// EmployeeKey identifies the attachment set of one employee.
type EmployeeKey struct {
	CompanyID int16
	NIK       string
	Division  string
}

// AttachmentRepository persists employee attachments inside the caller's ScopedTx.
type AttachmentRepository interface {
	List(ctx context.Context, tx *sql.Tx, k EmployeeKey) ([]Attachment, error)
	// TemplateFor returns the template rows of a level restricted to ACTIVE components of
	// the company that apply to the division (D-20), as TEMPLATE attachments.
	TemplateFor(ctx context.Context, tx *sql.Tx, level string, companyID int16, division string) ([]Attachment, error)
	// LevelRule returns nil when the level has no rule row.
	LevelRule(ctx context.Context, tx *sql.Tx, level string) (*LevelRule, error)
	// CatalogByCode maps component code to the ACTIVE component of the company/division.
	CatalogByCode(ctx context.Context, tx *sql.Tx, companyID int16, division string, codes []string) (map[string]CatalogEntry, error)
	// CatalogBySysid maps sysid to the ACTIVE component of the company that applies to the division.
	CatalogBySysid(ctx context.Context, tx *sql.Tx, companyID int16, division string, sysids []int64) (map[int64]CatalogEntry, error)
	// PeriodClosed reports whether the (company, yymm, division) period is CLOSED (04§9).
	PeriodClosed(ctx context.Context, tx *sql.Tx, k EmployeeKey, yymm int) (bool, error)
	Upsert(ctx context.Context, tx *sql.Tx, k EmployeeKey, a Attachment, actor string) error
	Delete(ctx context.Context, tx *sql.Tx, k EmployeeKey, sysid int64) error
	// Snapshot copies the current row to the history table at yymm.
	Snapshot(ctx context.Context, tx *sql.Tx, k EmployeeKey, sysid int64, yymm int) error
	// SetEmployeeLevel changes pe_level under If-Match and bumps the employee version.
	SetEmployeeLevel(ctx context.Context, tx *sql.Tx, employeeID int64, level, actor string, ifMatch int) error
}
