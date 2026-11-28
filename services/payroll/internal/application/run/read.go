package run

import (
	"context"
	"time"
)

// T022b: read models of the run and period API (07§4–5, D-51). Amounts stay decimal strings as stored
// (numeric::text); masking without payroll.salary.amount.view is done by the delivery layer.

// Info is the summary of one run (pyr_run), or of a dry run that was never persisted (ID 0, status DRY).
type Info struct {
	ID         int64
	YYMM       int
	Division   string
	PayDate    string // YYYY-MM-DD
	Status     string
	EmpCount   int
	LineCount  int
	TotT, TotP string
	RuleSetID  int
	SnapshotID int64
	NIKFilter  []string
	Warnings   []Issue
	StartedBy  string
	StartedAt  time.Time
	FinishedAt time.Time // zero while running
	DurationMs int64
}

// LineInfo is one component line (pyr_salary_detail). Empty amount strings mean a legacy NULL.
type LineInfo struct {
	NIK, CompType    string // CompType: T or P
	CompID           int
	Description      string
	Amount, Base     string
	Qty, Rate        string
	Formula, Remarks string
}

// EmployeeTotal is the per-employee total of a dry run.
type EmployeeTotal struct {
	NIK, Name            string
	TotT, TotP, TakeHome string
}

// PeriodInfo is one pyr_period row.
type PeriodInfo struct {
	YYMM               int
	Division           string
	DateFrom, DateTo   string
	Status             string
	ActiveRunID        int64
	ApprovedBy         string
	ApprovedAt         string
	ClosedBy, ClosedAt string
	ReopenReason       string
}

// Reader reads runs, lines and periods as payroll_app inside the caller scope (RLS). A row outside
// the scope is invisible, so by-id reads of another division report apperr.CodeNotFound.
type Reader interface {
	GetRun(ctx context.Context, id int64) (Info, error)
	ListRuns(ctx context.Context, yymm int, division string) ([]Info, error)
	ListLines(ctx context.Context, runID int64, nik string, page, size int) ([]LineInfo, int64, error)
	ListPeriods(ctx context.Context, division string) ([]PeriodInfo, error)
	GetPeriod(ctx context.Context, companyID int16, yymm int, division string) (PeriodInfo, error)
}
