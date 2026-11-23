package recon

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
)

// T030c: diff of one stored run against another run of the same period (07§4 GET /runs/{id}/diff). It reuses the
// recon categorization (Diff) and readers (runSide, LoadAttChanged, DiffSource.loadTx); nothing is recalculated
// except the master input DiffSource reads for against=legacy, exactly like `cmd/recon diff -target run:<id>`.

// Against values of the run diff.
const (
	AgainstActive = "active" // the ACTIVE run of the period (pp_active_run_id)
	AgainstLegacy = "legacy" // the ACTIVE LEGACY run of the period: full recon (D-41, D-47/D-50 exclusions)
)

// Run diff errors.
var (
	ErrBadAgainst    = errors.New("recon: against must be active, legacy or run:<id>")
	ErrRunNotFound   = errors.New("recon: run not found")
	ErrNoActiveRun   = errors.New("recon: period has no ACTIVE run")
	ErrAgainstPeriod = errors.New("recon: against run is not a run of the same period")
)

// RunDiffQuery selects the run and what to compare it with.
type RunDiffQuery struct {
	RunID   int64
	Against string // active | legacy | run:<id>
}

// RunDiffSource loads a RunDiffQuery as the caller (payroll_app, RLS scope). Rules is only used for against=legacy.
type RunDiffSource struct {
	DB    run.TxRunner
	Rules run.RulesFunc
}

// runRef is the period identity of a stored run.
type runRef struct {
	id       int64
	company  int16
	yymm     int
	division string
	kind     string
	ruleSet  int
	snapshot int64
	activeID int64
}

// ParseAgainst validates the against value and returns the run id of "run:<id>" (0 for active/legacy).
func ParseAgainst(s string) (int64, error) {
	if s == AgainstActive || s == AgainstLegacy {
		return 0, nil
	}
	v, ok := strings.CutPrefix(s, "run:")
	if !ok {
		return 0, fmt.Errorf("%w: %q", ErrBadAgainst, s)
	}
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%w: %q", ErrBadAgainst, s)
	}
	return id, nil
}

// Load returns the report: Target = q.RunID, Legacy side = the against run (Δ = run − against).
// A run outside the caller scope is invisible (ErrRunNotFound, like GetRun); the division of the found run is
// checked against the caller scope before anything else is read.
func (s RunDiffSource) Load(ctx context.Context, q RunDiffQuery) (DiffReport, error) {
	againstID, err := ParseAgainst(q.Against)
	if err != nil {
		return DiffReport{}, err
	}
	if s.DB == nil {
		return DiffReport{}, ErrDiffNotReady
	}
	var rep DiffReport
	err = s.DB.ScopedTxOpts(ctx, run.SnapshotTx, func(tx *sql.Tx) error {
		in, err := s.loadTx(ctx, tx, q.RunID, q.Against, againstID)
		if err != nil {
			return err
		}
		rep = Diff(in)
		return nil
	})
	return rep, err
}

// loadTx builds the DiffInput inside the scoped transaction.
func (s RunDiffSource) loadTx(ctx context.Context, tx *sql.Tx, runID int64, against string, againstID int64) (DiffInput, error) {
	self, err := findRun(ctx, tx, runID)
	if err != nil {
		return DiffInput{}, err
	}
	if err := run.InScope(ctx, self.company, self.division); err != nil {
		return DiffInput{}, err
	}
	switch against {
	case AgainstLegacy:
		// A LEGACY run has no rule set (rule_set 0), so the engine recompute would fail with an unclear
		// "PPh TER table missing"; against=legacy is only for ENGINE runs (T030f).
		if self.kind != "ENGINE" {
			return DiffInput{}, fmt.Errorf("%w: run %d", ErrTargetRun, self.id)
		}
		if s.Rules == nil {
			return DiffInput{}, ErrDiffNotReady
		}
		return DiffSource{Rules: s.Rules}.loadTx(ctx, tx, DiffQuery{CompanyID: self.company, Division: self.division,
			YYMM: self.yymm, RuleSetID: self.ruleSet, Target: "run:" + strconv.FormatInt(self.id, 10)})
	case AgainstActive:
		if self.activeID == 0 {
			return DiffInput{}, fmt.Errorf("%w: %s %d", ErrNoActiveRun, self.division, self.yymm)
		}
		againstID = self.activeID
	}
	other, err := findRun(ctx, tx, againstID)
	if err != nil {
		return DiffInput{}, err
	}
	if other.company != self.company || other.yymm != self.yymm || other.division != self.division {
		return DiffInput{}, fmt.Errorf("%w: run %d", ErrAgainstPeriod, other.id)
	}
	return runPairInput(ctx, tx, self, other)
}

// findRun reads the period identity of a run; RLS hides runs outside the caller scope.
func findRun(ctx context.Context, tx *sql.Tx, id int64) (runRef, error) {
	r := runRef{id: id}
	var snap, active sql.NullInt64
	err := tx.QueryRowContext(ctx, `SELECT r.pr_company_id, r.pr_yymm, r.pr_division, r.pr_kind, r.pr_rule_set_id, r.pr_snapshot_id,
  p.pp_active_run_id
FROM payroll.pyr_run r
LEFT JOIN payroll.pyr_period p ON p.pp_company_id = r.pr_company_id AND p.pp_yymm = r.pr_yymm AND p.pp_division = r.pr_division
WHERE r.pr_id = $1`, id).Scan(&r.company, &r.yymm, &r.division, &r.kind, &r.ruleSet, &snap, &active)
	if errors.Is(err, sql.ErrNoRows) {
		return r, fmt.Errorf("%w: %d", ErrRunNotFound, id)
	}
	if err != nil {
		return r, fmt.Errorf("find run %d: %w", id, err)
	}
	r.snapshot, r.activeID = snap.Int64, active.Int64
	return r, nil
}

// runPairInput compares two stored runs: the against run is the legacy side (with its basis/status), the attendance
// of the two snapshots gives INPUT_ATT, and a line whose basis/status differs between the runs gives INPUT_MASTER.
// There are no D-47 exclusions: both sides use the same master history.
func runPairInput(ctx context.Context, tx *sql.Tx, self, other runRef) (DiffInput, error) {
	in := DiffInput{Meta: DiffMeta{CompanyID: self.company, Division: self.division, YYMM: self.yymm,
		Target: "run:" + strconv.FormatInt(self.id, 10), RuleSetID: self.ruleSet, LegacyRunID: other.id,
		LegacySnapshotID: other.snapshot, TargetSnapshotID: self.snapshot}}
	var err error
	if in.Legacy, err = runSide(ctx, tx, other.id, true); err != nil {
		return in, err
	}
	target, err := runSide(ctx, tx, self.id, true)
	if err != nil {
		return in, err
	}
	in.Components = make(map[string][]engine.EmpComponent, len(target))
	in.Target = make(map[string]DiffEmp, len(target))
	for nik, e := range target {
		in.Components[nik] = componentsOfLines(e.Lines)
		in.Target[nik] = DiffEmp{Lines: stripInputs(e.Lines), Head: e.Head}
	}
	in.AttChanged, err = LoadAttChanged(ctx, tx, other.snapshot, self.snapshot)
	return in, err
}

// componentsOfLines turns the basis/status of a run's lines into the input masterChanged compares with.
func componentsOfLines(ls []DiffLine) []engine.EmpComponent {
	out := make([]engine.EmpComponent, 0, len(ls))
	for _, l := range ls {
		if l.Basis == nil && l.Sts == "" {
			continue
		}
		c := engine.EmpComponent{Key: l.Key, Sts: l.Sts}
		if l.Basis != nil {
			c.Amount = *l.Basis
		}
		out = append(out, c)
	}
	return out
}

func stripInputs(ls []DiffLine) []DiffLine {
	out := make([]DiffLine, len(ls))
	for i, l := range ls {
		out[i] = DiffLine{Key: l.Key, Amount: l.Amount}
	}
	return out
}
