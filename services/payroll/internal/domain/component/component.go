// Package component holds the sysid component master and the per-level
// component template (T008, 12§2, 04§15).
package component

import (
	"context"
	"database/sql"

	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/master"
)

// Sentinel errors are shared with the master repositories.
var (
	ErrNotFound        = master.ErrNotFound
	ErrVersionMismatch = master.ErrVersionMismatch
	ErrDuplicate       = master.ErrDuplicate
	ErrReference       = master.ErrReference
)

// Component statuses.
const (
	StatusDraft   = "DRAFT"
	StatusActive  = "ACTIVE"
	StatusRetired = "RETIRED"
)

// Component is pyr_component. Type is "T" or "P"; ValidTo 0 means open-ended.
type Component struct {
	Sysid             int64
	CompanyID         int16
	Code              string
	Type              string
	Name              string
	ScopeDivisions    []string
	Currency          string
	DefaultSts        string
	TaxClass          string
	SubsidyEligible   bool
	FixIncome         string
	IncludeInEstimate bool
	Tags              []string
	Serial            int
	BaseSysid         int64
	Status            string
	ValidFrom         int
	ValidTo           int
	Version           int
}

// Filter selects ListComponents rows (RLS still applies on top).
type Filter struct {
	CompanyIDs []int16
	Type       string
	Status     string
	Q          string
	AfterSysid int64
	Limit      int
}

// TemplateItem is one pyr_component_template row of a level. Code and Name are
// read-only echoes of the component.
type TemplateItem struct {
	Sysid          int64
	Sts            string
	Tax            int
	Amount         string
	TaxPaidCompany string
	Code, Name     string
}

// Usage tells whether a component is already used: attachments in any division, or
// salary detail of a CLOSED period (T008c).
type Usage struct {
	Attachments int64
	ClosedRun   bool
}

// Used reports whether the component is referenced by data that must stay stable.
func (u Usage) Used() bool { return u.Attachments > 0 || u.ClosedRun }

// Repository persists components and templates inside the caller's ScopedTx.
type Repository interface {
	Create(ctx context.Context, tx *sql.Tx, c Component) (Component, error)
	Get(ctx context.Context, tx *sql.Tx, sysid int64) (Component, error)
	List(ctx context.Context, tx *sql.Tx, f Filter) ([]Component, error)
	Update(ctx context.Context, tx *sql.Tx, c Component, ifMatch int) (Component, error)
	// SetStatus changes the status under If-Match and bumps the version.
	SetStatus(ctx context.Context, tx *sql.Tx, sysid int64, status string, ifMatch int) (Component, error)
	Lock(ctx context.Context, tx *sql.Tx, sysid int64) error
	// InUse counts usage across all divisions of the company (not limited by the caller's scope).
	InUse(ctx context.Context, tx *sql.Tx, sysid int64) (Usage, error)
	// MissingDivisions returns the divisions that do not exist in the company.
	MissingDivisions(ctx context.Context, tx *sql.Tx, companyID int16, divisions []string) ([]string, error)

	// GetTemplate lists the template rows of a level whose component is visible (D-20).
	GetTemplate(ctx context.Context, tx *sql.Tx, level string) ([]TemplateItem, error)
	// ReplaceTemplate swaps the visible rows of a level for items (D-20).
	ReplaceTemplate(ctx context.Context, tx *sql.Tx, level string, items []TemplateItem) error
}
