// Package report reads an approved/closed payroll run and builds the CLI reports of D-62 (11§1).
// Reads run in a scoped transaction as payroll_app; every export writes REPORT_EXPORT to pyr_audit_log
// (type, period, division, row count; never amounts or NIKs).
package report

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/audit"
)

// ActionExport is the audit action of every report export.
const ActionExport = "REPORT_EXPORT"

// ErrPeriodNotApproved is returned for a period that is not APPROVED/CLOSED without AllowOpen.
var ErrPeriodNotApproved = errors.New("report: periode belum APPROVED/CLOSED")

// ErrNoRun is returned when the period has no ACTIVE engine run.
var ErrNoRun = errors.New("report: tidak ada run ACTIVE untuk periode ini")

// Query selects the run to report on.
type Query struct {
	CompanyID int16
	YYMM      int
	Division  string
	// AllowOpen permits a period that is not yet APPROVED (rehearsal); the audit row records it.
	AllowOpen bool
}

// Comp identifies a component column.
type Comp struct {
	Type   string // "T" | "P"
	ID     int
	Name   string
	Serial int // pcm_serial; large when unknown so it sorts last
}

// Key is the column key, e.g. "T1".
func (c Comp) Key() string { return fmt.Sprintf("%s%d", c.Type, c.ID) }

// Emp is one head row with its non-zero component amounts.
type Emp struct {
	NIK, Nama, Dept           string
	TotTunjangan, TotPotongan decimal.Decimal
	Rounding, NetPay          decimal.Decimal
	Amounts                   map[string]decimal.Decimal // Comp.Key → amount (zero lines omitted)
}

// RunData is the loaded run: components in pcm_serial order and employees by NIK.
type RunData struct {
	RunID  int64
	Status string // period status
	Comps  []Comp
	Emps   []Emp
}

// Service builds reports.
type Service struct {
	DB    run.TxRunner
	Audit audit.Writer
}

// NewService creates a Service.
func NewService(db run.TxRunner, w audit.Writer) *Service { return &Service{DB: db, Audit: w} }

// Export loads the run, lets build produce the output from it, and writes the audit row in the same
// transaction. rows is the row count recorded in the audit entry. If build fails nothing is audited.
func (s *Service) Export(ctx context.Context, kind string, q Query, build func(RunData) (rows int, err error)) error {
	return s.DB.ScopedTxOpts(ctx, run.SnapshotTx, func(tx *sql.Tx) error {
		d, err := Load(ctx, tx, q)
		if err != nil {
			return err
		}
		rows, err := build(d)
		if err != nil {
			return err
		}
		return s.writeAudit(ctx, tx, q, kind, rows, nil)
	})
}

// writeAudit records REPORT_EXPORT (counts and flags only; never amounts or NIKs).
func (s *Service) writeAudit(ctx context.Context, tx *sql.Tx, q Query, kind string, rows int, extra map[string]any) error {
	after := map[string]any{"jenis": kind, "yymm": q.YYMM, "divisi": q.Division, "baris": rows, "allow_open": q.AllowOpen}
	for k, v := range extra {
		after[k] = v
	}
	return s.Audit.Write(ctx, tx, audit.Entry{CompanyID: q.CompanyID, Division: q.Division, Action: ActionExport,
		Entity: "report", EntityKey: kind, After: after})
}

// Load reads the ACTIVE engine run of the period inside tx.
func Load(ctx context.Context, tx *sql.Tx, q Query) (RunData, error) {
	var d RunData
	err := tx.QueryRowContext(ctx, `SELECT pp_status::text FROM payroll.pyr_period
WHERE pp_company_id = $1 AND pp_yymm = $2 AND pp_division = $3`, q.CompanyID, q.YYMM, q.Division).Scan(&d.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return d, apperr.New(apperr.CodeNotFound, fmt.Sprintf("periode %d %s tidak ada (atau di luar scope)", q.YYMM, q.Division), nil)
	}
	if err != nil {
		return d, fmt.Errorf("read period: %w", err)
	}
	if d.Status != "APPROVED" && d.Status != "CLOSED" && !q.AllowOpen {
		return d, fmt.Errorf("%w (status %s, periode %d %s); gunakan -allow-open untuk gladi", ErrPeriodNotApproved, d.Status, q.YYMM, q.Division)
	}
	err = tx.QueryRowContext(ctx, `SELECT pr_id FROM payroll.pyr_run
WHERE pr_company_id = $1 AND pr_yymm = $2 AND pr_division = $3 AND pr_status = 'ACTIVE' AND pr_kind = 'ENGINE'
  AND pr_parent_run_id IS NULL`, q.CompanyID, q.YYMM, q.Division).Scan(&d.RunID)
	if errors.Is(err, sql.ErrNoRows) {
		return d, fmt.Errorf("%w: %d %s", ErrNoRun, q.YYMM, q.Division)
	}
	if err != nil {
		return d, fmt.Errorf("find run: %w", err)
	}
	if err := loadHeads(ctx, tx, d.RunID, &d); err != nil {
		return d, err
	}
	return d, loadLines(ctx, tx, d.RunID, &d)
}

func loadHeads(ctx context.Context, tx *sql.Tx, runID int64, d *RunData) error {
	rows, err := tx.QueryContext(ctx, `SELECT psh_nik, COALESCE(psh_nama,''), COALESCE(psh_dept,''),
  psh_tot_tunjangan::text, psh_tot_potongan::text, psh_rounding::text, psh_net_pay::text
FROM payroll.pyr_salary_head WHERE psh_run_id = $1 ORDER BY psh_nik`, runID)
	if err != nil {
		return fmt.Errorf("read head: %w", err)
	}
	defer closeRows(rows)
	for rows.Next() {
		var e Emp
		var t, p, r, n string
		if err := rows.Scan(&e.NIK, &e.Nama, &e.Dept, &t, &p, &r, &n); err != nil {
			return fmt.Errorf("scan head: %w", err)
		}
		for _, f := range []struct {
			dst *decimal.Decimal
			s   string
		}{{&e.TotTunjangan, t}, {&e.TotPotongan, p}, {&e.Rounding, r}, {&e.NetPay, n}} {
			if *f.dst, err = decimal.NewFromString(f.s); err != nil {
				return fmt.Errorf("head: %w", err)
			}
		}
		e.Amounts = map[string]decimal.Decimal{}
		d.Emps = append(d.Emps, e)
	}
	return rows.Err()
}

const unknownSerial = 1 << 30

func loadLines(ctx context.Context, tx *sql.Tx, runID int64, d *RunData) error {
	rows, err := tx.QueryContext(ctx, `SELECT d.psd_nik, d.psd_type::text, d.psd_id, COALESCE(m.pcm_name, d.psd_description, ''),
  COALESCE(m.pcm_serial, $2), COALESCE(d.psd_amount::text, '0')
FROM payroll.pyr_salary_detail d
LEFT JOIN payroll.pyr_component_master m ON m.pcm_type = d.psd_type AND m.pcm_id = d.psd_id
WHERE d.psd_run_id = $1 ORDER BY d.psd_nik`, runID, unknownSerial)
	if err != nil {
		return fmt.Errorf("read detail: %w", err)
	}
	defer closeRows(rows)
	idx := make(map[string]int, len(d.Emps))
	for i, e := range d.Emps {
		idx[e.NIK] = i
	}
	seen := map[string]Comp{}
	for rows.Next() {
		var nik, typ, amt string
		var c Comp
		if err := rows.Scan(&nik, &typ, &c.ID, &c.Name, &c.Serial, &amt); err != nil {
			return fmt.Errorf("scan detail: %w", err)
		}
		c.Type = "T"
		if typ == "Potongan" || typ == "P" {
			c.Type = "P"
		}
		v, err := decimal.NewFromString(amt)
		if err != nil {
			return fmt.Errorf("detail %s: %w", c.Key(), err)
		}
		i, ok := idx[nik]
		if !ok {
			return errors.New("detail tanpa head")
		}
		if _, ok := seen[c.Key()]; !ok {
			seen[c.Key()] = c
		}
		if !v.IsZero() {
			d.Emps[i].Amounts[c.Key()] = d.Emps[i].Amounts[c.Key()].Add(v)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, c := range seen {
		d.Comps = append(d.Comps, c)
	}
	sortComps(d.Comps)
	return nil
}
