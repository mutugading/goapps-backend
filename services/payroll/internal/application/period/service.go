// Package period implements the payroll period transitions of 07§5 (T017b): approve, close and
// reopen, plus the service guard that refuses input mutations of an APPROVED/CLOSED period (07§2, 04§9).
// Permissions (payroll.period.approve/.close/.reopen, Supervisor) are checked by the delivery layer
// (B-API-RUN); the reopen notification follows with it.
package period

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/audit"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/scope"
)

// Audit actions.
const (
	ActionApprove = "PERIOD_APPROVE"
	ActionClose   = "PERIOD_CLOSE"
	ActionReopen  = "PERIOD_REOPEN"

	auditEntity     = "pyr_period"
	minReasonLength = 10
)

// InputHasher recomputes the input_hash of a full-division run inside tx (run.Service.InputHash).
type InputHasher interface {
	InputHash(ctx context.Context, tx *sql.Tx, q run.Query, ruleSetID int) (string, error)
}

// Service is the period application service.
type Service struct {
	db     run.TxRunner
	audit  audit.Writer
	hasher InputHasher
}

// NewService creates a Service. hasher checks on approve/close that the input did not change after the run (D-54).
func NewService(db run.TxRunner, w audit.Writer, hasher InputHasher) *Service {
	return &Service{db: db, audit: w, hasher: hasher}
}

// blockingCodes are per-NIK run warnings whose employee has no result (not paid): approve is refused (D-54).
// NO_WORKDAYS stays a warning: an employee without workdays may legitimately get nothing (D-27).
var blockingCodes = map[string]bool{run.CodeCalcError: true, run.CodeLoanDouble: true, run.CodeIncomplete: true}

const maxBlockedNIKs = 20

// Command identifies one period. Reason is required by Reopen only.
type Command struct {
	CompanyID int16
	Division  string
	YYMM      int
	Reason    string
	IP        string
}

// state is the locked period row.
type state struct {
	status     string
	runID      sql.NullInt64
	runStatus  sql.NullString
	runKind    sql.NullString
	hasLegacy  bool
	snapshotID sql.NullInt64
	ruleSetID  sql.NullInt64
	inputHash  sql.NullString
	warnings   sql.NullString
}

// Approve moves DRAFT → APPROVED. The period needs an ACTIVE run without blocking warnings whose input is
// unchanged since the commit (D-54); the snapshot of that run is locked so a later sync is refused (05§6).
func (s *Service) Approve(ctx context.Context, cmd Command) error {
	return s.transition(ctx, cmd, ActionApprove, func(tx *sql.Tx, st state, actor string) (string, error) {
		if st.status != run.PeriodDraft {
			return "", wrongStatus(cmd, st.status, "approve", run.PeriodDraft)
		}
		if !st.runID.Valid || st.runStatus.String != "ACTIVE" {
			return "", apperr.New(apperr.CodeBusinessRule, "periode belum punya run ACTIVE", details(cmd))
		}
		if err := blockingWarnings(cmd, st); err != nil {
			return "", err
		}
		if err := s.checkInput(ctx, tx, cmd, st); err != nil {
			return "", err
		}
		if st.snapshotID.Valid {
			if _, err := tx.ExecContext(ctx, `UPDATE payroll.pyr_att_snapshot SET pas_locked = true WHERE pas_snapshot_id = $1`,
				st.snapshotID.Int64); err != nil {
				return "", fmt.Errorf("lock snapshot: %w", err)
			}
		}
		return run.PeriodApproved, update(ctx, tx, cmd, sqlApprove, actor)
	})
}

// Close moves APPROVED → CLOSED after the same input check as approve (D-54). From then on fn_guard_closed
// rejects every change of the period's result and input rows (04§9, 000038).
func (s *Service) Close(ctx context.Context, cmd Command) error {
	return s.transition(ctx, cmd, ActionClose, func(tx *sql.Tx, st state, actor string) (string, error) {
		if st.status != run.PeriodApproved {
			return "", wrongStatus(cmd, st.status, "close", run.PeriodApproved)
		}
		if err := s.checkInput(ctx, tx, cmd, st); err != nil {
			return "", err
		}
		return run.PeriodClosed, update(ctx, tx, cmd, sqlClose, actor)
	})
}

// Reopen moves APPROVED/CLOSED → DRAFT with a reason of at least 10 characters. The ACTIVE run stays,
// the approval/close stamps are cleared and the snapshot lock of the run is released (05§6).
func (s *Service) Reopen(ctx context.Context, cmd Command) error {
	cmd.Reason = strings.TrimSpace(cmd.Reason)
	if utf8.RuneCountInString(cmd.Reason) < minReasonLength {
		return apperr.New(apperr.CodeValidation, "alasan reopen minimal 10 karakter", map[string]string{"field": "reason"})
	}
	return s.transition(ctx, cmd, ActionReopen, func(tx *sql.Tx, st state, _ string) (string, error) {
		if st.status != run.PeriodApproved && st.status != run.PeriodClosed {
			return "", wrongStatus(cmd, st.status, "reopen", run.PeriodApproved+"/"+run.PeriodClosed)
		}
		// A LEGACY run is Oracle history loaded by the ETL (no snapshot, no engine input): reopening would let
		// a re-run supersede it with engine output, so the period stays closed (D-40). Any LEGACY run of the
		// period counts, also when pp_active_run_id is NULL or points elsewhere.
		if st.runKind.String == "LEGACY" || st.hasLegacy {
			return "", apperr.New(apperr.CodeBusinessRule, "periode hasil migrasi legacy tidak bisa dibuka kembali", details(cmd))
		}
		if st.snapshotID.Valid {
			if _, err := tx.ExecContext(ctx, `UPDATE payroll.pyr_att_snapshot SET pas_locked = false WHERE pas_snapshot_id = $1`,
				st.snapshotID.Int64); err != nil {
				return "", fmt.Errorf("unlock snapshot: %w", err)
			}
		}
		return run.PeriodDraft, update(ctx, tx, cmd, sqlReopen, cmd.Reason)
	})
}

// Guard refuses a mutation of period input (addition, attachment) when the period is APPROVED or
// CLOSED (07§2). A missing period is OPEN. Call it inside the scoped transaction of the mutation;
// fn_guard_closed is the second layer for CLOSED.
func Guard(ctx context.Context, tx *sql.Tx, companyID int16, yymm int, division string) error {
	var st string
	err := tx.QueryRowContext(ctx, `SELECT pp_status::text FROM payroll.pyr_period
WHERE pp_company_id = $1 AND pp_yymm = $2 AND pp_division = $3 FOR SHARE`, companyID, yymm, division).Scan(&st)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil
	case err != nil:
		return fmt.Errorf("period guard: %w", err)
	case st == run.PeriodClosed:
		return apperr.PeriodClosed(yymm, division)
	case st == run.PeriodApproved:
		return apperr.PeriodApproved(yymm, division)
	default:
		return nil
	}
}

// step validates the locked state, applies the change and returns the new status.
type step func(tx *sql.Tx, st state, actor string) (string, error)

// transition runs one change under the run advisory lock (no run may commit meanwhile) and the period
// row lock, then writes the audit row in the same transaction.
func (s *Service) transition(ctx context.Context, cmd Command, action string, fn step) error {
	if err := run.InScope(ctx, cmd.CompanyID, cmd.Division); err != nil {
		return err
	}
	actor, _ := scope.FromContext(ctx) //nolint:errcheck // checked by InScope
	return s.db.ScopedTx(ctx, func(tx *sql.Tx) error {
		if err := run.Lock(ctx, tx, run.Query{CompanyID: cmd.CompanyID, Division: cmd.Division, YYMM: cmd.YYMM}); err != nil {
			return err
		}
		st, err := lockPeriod(ctx, tx, cmd)
		if err != nil {
			return err
		}
		next, err := fn(tx, st, actor.UserID)
		if err != nil {
			return err
		}
		after := map[string]any{"status": next, "active_run_id": st.runID.Int64}
		if cmd.Reason != "" {
			after["reason"] = cmd.Reason
		}
		return s.audit.Write(ctx, tx, audit.Entry{CompanyID: cmd.CompanyID, Division: cmd.Division, Action: action,
			Entity: auditEntity, EntityKey: strconv.Itoa(cmd.YYMM) + ":" + cmd.Division,
			Before: map[string]any{"status": st.status}, After: after, IP: cmd.IP})
	})
}

// lockPeriod reads the period with its ACTIVE run FOR UPDATE.
func lockPeriod(ctx context.Context, tx *sql.Tx, cmd Command) (state, error) {
	var st state
	err := tx.QueryRowContext(ctx, `SELECT p.pp_status::text, p.pp_active_run_id, r.pr_status::text, r.pr_snapshot_id, r.pr_kind,
  r.pr_rule_set_id, r.pr_input_hash, r.pr_warnings::text,
  EXISTS (SELECT 1 FROM payroll.pyr_run l WHERE l.pr_company_id = p.pp_company_id AND l.pr_yymm = p.pp_yymm
    AND l.pr_division = p.pp_division AND l.pr_kind = 'LEGACY')
FROM payroll.pyr_period p LEFT JOIN payroll.pyr_run r ON r.pr_id = p.pp_active_run_id
WHERE p.pp_company_id = $1 AND p.pp_yymm = $2 AND p.pp_division = $3 FOR UPDATE OF p`,
		cmd.CompanyID, cmd.YYMM, cmd.Division).Scan(&st.status, &st.runID, &st.runStatus, &st.snapshotID, &st.runKind,
		&st.ruleSetID, &st.inputHash, &st.warnings, &st.hasLegacy)
	if errors.Is(err, sql.ErrNoRows) {
		return st, apperr.New(apperr.CodeNotFound, "periode tidak ditemukan", details(cmd))
	}
	if err != nil {
		return st, fmt.Errorf("lock period: %w", err)
	}
	return st, nil
}

// Period updates: $1-$3 = key, $4 = actor or reopen reason. The status predicate repeats the check made
// on the locked row (D-39), so exactly one row must change.
const (
	sqlApprove = `UPDATE payroll.pyr_period SET pp_status = 'APPROVED', approved_by = $4, approved_at = now(), pp_reopen_reason = NULL
WHERE pp_company_id = $1 AND pp_yymm = $2 AND pp_division = $3 AND pp_status = 'DRAFT'`
	sqlClose = `UPDATE payroll.pyr_period SET pp_status = 'CLOSED', closed_by = $4, closed_at = now()
WHERE pp_company_id = $1 AND pp_yymm = $2 AND pp_division = $3 AND pp_status = 'APPROVED'`
	sqlReopen = `UPDATE payroll.pyr_period SET pp_status = 'DRAFT', approved_by = NULL, approved_at = NULL,
  closed_by = NULL, closed_at = NULL, pp_reopen_reason = $4
WHERE pp_company_id = $1 AND pp_yymm = $2 AND pp_division = $3 AND pp_status IN ('APPROVED', 'CLOSED')`
)

// blockingWarnings refuses approve when the ACTIVE run left employees without a result (D-54).
func blockingWarnings(cmd Command, st state) error {
	if !st.warnings.Valid {
		return nil
	}
	var ws []run.Issue
	if err := json.Unmarshal([]byte(st.warnings.String), &ws); err != nil {
		return fmt.Errorf("parse run warnings: %w", err)
	}
	var niks []string
	seen := map[string]bool{}
	for _, w := range ws {
		if blockingCodes[w.Code] && !seen[w.NIK] {
			seen[w.NIK] = true
			niks = append(niks, w.NIK)
		}
	}
	if len(niks) == 0 {
		return nil
	}
	d := details(cmd)
	d["count"] = strconv.Itoa(len(niks))
	if len(niks) > maxBlockedNIKs {
		niks = niks[:maxBlockedNIKs]
	}
	d["niks"] = strings.Join(niks, ",")
	return apperr.New(apperr.CodeBusinessRule,
		fmt.Sprintf("%d karyawan tanpa hasil (CALC_ERROR/LOAN_DOUBLE/INCOMPLETE_RESULT); perbaiki lalu jalankan ulang run", len(seen)), d)
}

// checkInput refuses approve/close when the period input changed after the ACTIVE ENGINE run was committed: the
// result would be stale (D-54). A LEGACY run has no engine input and is not checked.
func (s *Service) checkInput(ctx context.Context, tx *sql.Tx, cmd Command, st state) error {
	if st.runKind.String != "ENGINE" || !st.runID.Valid {
		return nil
	}
	if s.hasher == nil {
		return errors.New("period: input hasher not configured")
	}
	if !st.inputHash.Valid || !st.ruleSetID.Valid {
		return fmt.Errorf("period %d %s: ACTIVE ENGINE run %d has no input hash or rule set", cmd.YYMM, cmd.Division, st.runID.Int64)
	}
	h, err := s.hasher.InputHash(ctx, tx, run.Query{CompanyID: cmd.CompanyID, Division: cmd.Division, YYMM: cmd.YYMM},
		int(st.ruleSetID.Int64))
	if err != nil {
		return err
	}
	if h != st.inputHash.String {
		return apperr.New(apperr.CodeConflict, "input periode berubah sesudah run; jalankan ulang run", details(cmd))
	}
	return nil
}

// update runs one of the period statements with arg as $4.
func update(ctx context.Context, tx *sql.Tx, cmd Command, stmt, arg string) error {
	res, err := tx.ExecContext(ctx, stmt, cmd.CompanyID, cmd.YYMM, cmd.Division, arg)
	if err != nil {
		return fmt.Errorf("update period: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update period: %w", err)
	}
	if n != 1 {
		return fmt.Errorf("update period: %d rows changed, want 1", n)
	}
	return nil
}

// wrongStatus is the 409 for a transition that does not start from the required status.
func wrongStatus(cmd Command, current, op, want string) error {
	switch current {
	case run.PeriodClosed:
		return apperr.PeriodClosed(cmd.YYMM, cmd.Division)
	case run.PeriodApproved:
		return apperr.PeriodApproved(cmd.YYMM, cmd.Division)
	default:
		d := details(cmd)
		d["status"] = current
		return apperr.New(apperr.CodeConflict, fmt.Sprintf("%s hanya dari status %s", op, want), d)
	}
}

func details(cmd Command) map[string]string {
	return map[string]string{"yymm": strconv.Itoa(cmd.YYMM), "division": cmd.Division}
}
