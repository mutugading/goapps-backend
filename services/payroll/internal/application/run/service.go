package run

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/audit"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/scope"
	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
)

// T017: commit a run (03§3 steps 1, 2, 5, 6). Run 1 NIK (pr_parent_run_id) and pyr_loan_txn are
// deferred (B-UI3, B-LN1).

// Period statuses (payroll.period_status).
const (
	PeriodOpen     = "OPEN"
	PeriodDraft    = "DRAFT"
	PeriodApproved = "APPROVED"
	PeriodClosed   = "CLOSED"

	auditActionCommit = "RUN_COMMIT"
	auditEntityRun    = "pyr_run"
)

// ErrRuleIncomplete marks a rule set that cannot be used for the run (RULE_INCOMPLETE).
var ErrRuleIncomplete = errors.New("run: rule set incomplete")

// TxRunner runs fn in a scoped transaction (postgres.DB.ScopedTx).
type TxRunner interface {
	ScopedTx(ctx context.Context, fn func(tx *sql.Tx) error) error
}

// RulesFunc loads the rules of a run inside the scoped transaction (postgres.RuleSetLoader, T016a).
// It returns ErrRuleIncomplete (wrapped) when the rule set itself is unusable; any other error is internal.
type RulesFunc func(ctx context.Context, tx *sql.Tx, q Query, ruleSetID int) (Rules, error)

// Service is the run application service.
type Service struct {
	db     TxRunner
	rules  RulesFunc
	audit  audit.Writer
	loader Loader
	runner Runner
}

// NewService creates a Service with the default loader and runner.
func NewService(db TxRunner, rules RulesFunc, w audit.Writer) *Service {
	return NewServiceWith(db, rules, w, NewRunner())
}

// NewServiceWith creates a Service with an explicit runner (tests inject the calculation).
func NewServiceWith(db TxRunner, rules RulesFunc, w audit.Writer, r Runner) *Service {
	return &Service{db: db, rules: rules, audit: w, loader: NewLoader(), runner: r}
}

// CommitCommand is a full-division run (mode commit, 07§4).
type CommitCommand struct {
	CompanyID int16
	Division  string
	YYMM      int
	RuleSetID int
	PayDate   time.Time // zero = end of the period (the 25th)
	IP        string
}

// CommitResult is the persisted run.
type CommitResult struct {
	RunID        int64
	SupersededID int64 // 0 = no previous ACTIVE run
	Summary      Summary
	InputHash    string
	Warnings     []Issue
	Duration     time.Duration
}

// Commit calculates the division and persists the result in one transaction: the previous ACTIVE run
// becomes SUPERSEDED, head and detail of the new run are inserted, the period points to it and is set
// to DRAFT, and an audit row is written. A second run with unchanged input yields the same input_hash
// and the same lines (the run id differs). APPROVED/CLOSED periods are rejected here and, for the result
// tables, again by fn_guard_closed (04§9).
func (s *Service) Commit(ctx context.Context, cmd CommitCommand) (out CommitResult, err error) {
	start := time.Now()
	if err = InScope(ctx, cmd.CompanyID, cmd.Division); err != nil {
		return out, err
	}
	period, err := validateRun(cmd.YYMM, cmd.RuleSetID)
	if err != nil {
		return out, err
	}
	payDate := cmd.PayDate
	if payDate.IsZero() {
		payDate = period.End
	}
	q := Query{CompanyID: cmd.CompanyID, Division: cmd.Division, YYMM: cmd.YYMM}

	err = s.db.ScopedTx(ctx, func(tx *sql.Tx) error {
		if e := Lock(ctx, tx, q); e != nil {
			return e
		}
		if e := ensurePeriodOpen(ctx, tx, q, period.Start, period.End); e != nil {
			return e
		}
		rules, e := s.rules(ctx, tx, q, cmd.RuleSetID)
		if e != nil {
			return ruleErr(e)
		}
		loaded, e := s.loader.Load(ctx, tx, q, rules)
		if e != nil {
			return loadErr(e)
		}
		res, e := s.runner.DryRun(ctx, loaded)
		if e != nil {
			return e
		}
		if e := checkDry(ctx, tx, q, &res); e != nil {
			return e
		}
		actor, _ := scope.FromContext(ctx) //nolint:errcheck // checked by InScope
		p := persistRun{q: q, ruleSetID: cmd.RuleSetID, snapshotID: loaded.SnapshotID, payDate: payDate,
			actor: actor.UserID, res: res, inputs: loaded.Inputs}
		ids, e := p.save(ctx, tx)
		if e != nil {
			return e
		}
		out = CommitResult{RunID: ids.run, SupersededID: ids.superseded, Summary: res.Summary,
			InputHash: res.InputHash, Warnings: res.Warnings}
		return s.audit.Write(ctx, tx, audit.Entry{CompanyID: cmd.CompanyID, Division: cmd.Division,
			Action: auditActionCommit, Entity: auditEntityRun, EntityKey: strconv.FormatInt(ids.run, 10),
			After: map[string]any{"run_id": ids.run, "superseded_run_id": ids.superseded, "yymm": cmd.YYMM,
				"rule_set_id": cmd.RuleSetID, "snapshot_id": loaded.SnapshotID, "input_hash": res.InputHash,
				"emp_count": res.Summary.EmpCount, "line_count": res.Summary.LineCount,
				"tot_tunjangan": res.Summary.TotalT.StringFixed(2), "tot_potongan": res.Summary.TotalP.StringFixed(2),
				"warnings": len(res.Warnings)},
			IP: cmd.IP})
	})
	if err != nil {
		return CommitResult{}, err
	}
	out.Duration = time.Since(start)
	return out, nil
}

// Lock takes the transaction advisory lock of (company, yymm, division) without waiting (03§3 step 1):
// a second run of the same period fails fast with RUN_IN_PROGRESS instead of queueing.
func Lock(ctx context.Context, tx *sql.Tx, q Query) error {
	var ok bool
	key := fmt.Sprintf("%d:%d:%s", q.CompanyID, q.YYMM, q.Division)
	if err := tx.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock(hashtext('payroll.run'), hashtext($1))`, key).
		Scan(&ok); err != nil {
		return fmt.Errorf("run lock: %w", err)
	}
	if !ok {
		return apperr.New(apperr.CodeRunInProgress, "run periode ini sedang berjalan",
			map[string]string{"yymm": strconv.Itoa(q.YYMM), "division": q.Division})
	}
	return nil
}

// ensurePeriodOpen creates the period OPEN when missing, locks its row and rejects APPROVED/CLOSED.
func ensurePeriodOpen(ctx context.Context, tx *sql.Tx, q Query, from, to time.Time) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO payroll.pyr_period (pp_company_id, pp_yymm, pp_division, pp_date_from, pp_date_to)
VALUES ($1, $2, $3, $4, $5) ON CONFLICT DO NOTHING`, q.CompanyID, q.YYMM, q.Division, from, to); err != nil {
		return fmt.Errorf("create period: %w", err)
	}
	var st string
	if err := tx.QueryRowContext(ctx, `SELECT pp_status::text FROM payroll.pyr_period
WHERE pp_company_id = $1 AND pp_yymm = $2 AND pp_division = $3 FOR UPDATE`, q.CompanyID, q.YYMM, q.Division).Scan(&st); err != nil {
		return fmt.Errorf("lock period: %w", err)
	}
	switch st {
	case PeriodClosed:
		return apperr.PeriodClosed(q.YYMM, q.Division)
	case PeriodApproved:
		return apperr.PeriodApproved(q.YYMM, q.Division)
	default:
		return nil
	}
}

// InScope requires company:division in the caller scope (RLS would only hide the rows).
func InScope(ctx context.Context, companyID int16, division string) error {
	id, ok := scope.FromContext(ctx)
	if !ok {
		return scope.ErrNoIdentity
	}
	for _, sc := range id.Scopes {
		if sc.CompanyID() == companyID && sc.Division() == division {
			return nil
		}
	}
	return apperr.DivisionForbidden(companyID, division)
}

// loadErr maps the loader errors to the 422 codes of 07§4.
func loadErr(err error) error {
	var ae *apperr.Error
	switch {
	case errors.As(err, &ae), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, ErrMissingAttdDays):
		return apperr.New(apperr.CodeMissingAttdDays, err.Error(), nil)
	case errors.Is(err, ErrMissingAttendance), errors.Is(err, ErrNoSnapshot):
		return apperr.New(apperr.CodeMissingAttendance, err.Error(), nil)
	case errors.Is(err, ErrSubsidyBasisPeriods):
		return apperr.New(apperr.CodeRuleIncomplete, err.Error(), nil)
	default:
		return fmt.Errorf("load run input: %w", err)
	}
}

// ruleErr: an unusable rule set is RULE_INCOMPLETE (07§4); database and other errors stay internal.
func ruleErr(err error) error {
	var ae *apperr.Error
	switch {
	case errors.As(err, &ae):
		return err
	case errors.Is(err, ErrRuleIncomplete):
		return apperr.New(apperr.CodeRuleIncomplete, err.Error(), nil)
	default:
		return fmt.Errorf("load rules: %w", err)
	}
}

// checkDry rejects a run without calculated employees and adds the stale-lock warning (D-39).
func checkDry(ctx context.Context, tx *sql.Tx, q Query, res *DryRunResult) error {
	if res.Summary.EmpCount == 0 {
		return apperr.New(apperr.CodeBusinessRule, "tidak ada karyawan yang terhitung; run tidak disimpan", nil)
	}
	return addLockStaleWarning(ctx, tx, q, res)
}

// InputHash reloads the input of a full-division run with ruleSetID inside tx and returns the input_hash a Commit
// would store now (same loader, same NIK order, same hash as DryRun). Period approve/close compare it with the
// ACTIVE run's pr_input_hash to refuse a stale result (D-54). Loader and rule errors map as in Commit.
func (s *Service) InputHash(ctx context.Context, tx *sql.Tx, q Query, ruleSetID int) (string, error) {
	q.NIKs, q.SnapshotID = nil, 0
	rules, err := s.rules(ctx, tx, q, ruleSetID)
	if err != nil {
		return "", ruleErr(err)
	}
	loaded, err := s.loader.Load(ctx, tx, q, rules)
	if err != nil {
		return "", loadErr(err)
	}
	inputs := append([]engine.Input(nil), loaded.Inputs...)
	sort.SliceStable(inputs, func(i, j int) bool { return inputs[i].Emp.NIK < inputs[j].Emp.NIK })
	return hashInputs(inputs), nil
}
