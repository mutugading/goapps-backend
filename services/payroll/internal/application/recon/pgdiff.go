package recon

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
)

// Diff source errors.
var (
	ErrBadTarget    = errors.New("recon: target must be dry or run:<id>")
	ErrTargetRun    = errors.New("recon: target run is not an ENGINE run of the period")
	ErrDiffNotReady = errors.New("recon: diff source is not configured")
)

// TargetDry is the target that recalculates with Runner.DryRun (nothing is written).
const TargetDry = "dry"

// ParseTarget accepts "dry" or "run:<id>" and returns the run id (0 = dry).
func ParseTarget(s string) (int64, error) {
	if s == TargetDry {
		return 0, nil
	}
	v, ok := strings.CutPrefix(s, "run:")
	if !ok {
		return 0, fmt.Errorf("%w: %q", ErrBadTarget, s)
	}
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%w: %q", ErrBadTarget, s)
	}
	return id, nil
}

// DiffQuery selects one division and the target to compare with its LEGACY run.
type DiffQuery struct {
	CompanyID int16
	Division  string
	YYMM      int
	RuleSetID int
	Target    string // dry | run:<id>
}

// DiffSource reads both sides of a recon from Postgres inside one scoped transaction (D-41). It only reads:
// the dry target is calculated in memory and the CLI connects as payroll_ro.
type DiffSource struct {
	DB     run.TxRunner
	Rules  run.RulesFunc
	Runner run.Runner
}

// Load builds the DiffInput of q.
func (s DiffSource) Load(ctx context.Context, q DiffQuery) (DiffInput, error) {
	if s.DB == nil || s.Rules == nil {
		return DiffInput{}, ErrDiffNotReady
	}
	targetRun, err := ParseTarget(q.Target)
	if err != nil {
		return DiffInput{}, err
	}
	rq := run.Query{CompanyID: q.CompanyID, Division: q.Division, YYMM: q.YYMM}
	out := DiffInput{Meta: DiffMeta{CompanyID: q.CompanyID, Division: q.Division, YYMM: q.YYMM, Target: q.Target,
		RuleSetID: q.RuleSetID}}
	err = s.DB.ScopedTx(ctx, func(tx *sql.Tx) error {
		legacySnap, err := s.loadLegacy(ctx, tx, rq, &out)
		if err != nil {
			return err
		}
		if targetRun == 0 && legacySnap != 0 {
			rq.SnapshotID = legacySnap // dry input = the recap legacy calculated with (09§4 step 2)
		}
		rules, err := s.Rules(ctx, tx, rq, q.RuleSetID)
		if err != nil {
			return fmt.Errorf("load rules: %w", err)
		}
		loaded, err := run.NewLoader().Load(ctx, tx, rq, rules)
		if err != nil {
			return fmt.Errorf("load inputs: %w", err)
		}
		out.Components = componentsByNIK(loaded.Inputs)
		if out.Drift, err = loadDrift(ctx, tx, rq, out.Meta.LegacyRunID, out.Meta.LegacySnapshotID); err != nil {
			return err
		}
		targetSnap, err := s.loadTarget(ctx, tx, rq, targetRun, loaded, &out)
		if err != nil {
			return err
		}
		out.Meta.TargetSnapshotID = targetSnap
		out.AttChanged, err = attChanged(ctx, tx, legacySnap, targetSnap)
		return err
	})
	return out, err
}

// loadTarget fills out.Target from a dry run (snapshot of loaded) or from an ENGINE run, and returns its snapshot.
func (s DiffSource) loadTarget(ctx context.Context, tx *sql.Tx, q run.Query, targetRun int64, loaded run.Result,
	out *DiffInput) (int64, error) {
	if targetRun == 0 {
		res, err := s.Runner.DryRun(ctx, loaded)
		if err != nil {
			return 0, fmt.Errorf("dry run: %w", err)
		}
		out.Target, out.Warnings = TargetFromResults(res.Results), res.Warnings
		return loaded.SnapshotID, nil
	}
	snap, err := engineRunSnapshot(ctx, tx, q, targetRun)
	if err != nil {
		return 0, err
	}
	out.Target, err = runSide(ctx, tx, targetRun, false)
	return snap, err
}

func (s DiffSource) loadLegacy(ctx context.Context, tx *sql.Tx, q run.Query, out *DiffInput) (int64, error) {
	runID, err := legacyRun(ctx, tx, q)
	if err != nil {
		return 0, err
	}
	var snap sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT pr_snapshot_id FROM payroll.pyr_run WHERE pr_id = $1`, runID).Scan(&snap); err != nil {
		return 0, fmt.Errorf("legacy snapshot: %w", err)
	}
	out.Meta.LegacyRunID, out.Meta.LegacySnapshotID = runID, snap.Int64
	out.Legacy, err = runSide(ctx, tx, runID, true)
	return snap.Int64, err
}

// engineRunSnapshot checks that id is an ENGINE run of the period and returns its snapshot.
func engineRunSnapshot(ctx context.Context, tx *sql.Tx, q run.Query, id int64) (int64, error) {
	var snap sql.NullInt64
	err := tx.QueryRowContext(ctx, `SELECT pr_snapshot_id FROM payroll.pyr_run
WHERE pr_id = $1 AND pr_company_id = $2 AND pr_yymm = $3 AND pr_division = $4 AND pr_kind = 'ENGINE'`,
		id, q.CompanyID, q.YYMM, q.Division).Scan(&snap)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("%w: run %d", ErrTargetRun, id)
	}
	if err != nil {
		return 0, fmt.Errorf("find target run: %w", err)
	}
	return snap.Int64, nil
}

// runSide reads DETAIL and MAIN of a run. withInputs also reads the basis/status legacy calculated with.
func runSide(ctx context.Context, tx *sql.Tx, runID int64, withInputs bool) (map[string]DiffEmp, error) {
	rows, err := tx.QueryContext(ctx, `SELECT psd_nik, psd_type::text, psd_id, psd_amount::text, psd_basis_amount::text,
  COALESCE(psd_sts, '') FROM payroll.pyr_salary_detail WHERE psd_run_id = $1 ORDER BY psd_nik, psd_type, psd_id`, runID)
	if err != nil {
		return nil, fmt.Errorf("load detail run %d: %w", runID, err)
	}
	defer closeRows(rows)
	out := map[string]DiffEmp{}
	for rows.Next() {
		var nik, typ, sts string
		var id int
		var amt, basis sql.NullString
		if err := rows.Scan(&nik, &typ, &id, &amt, &basis, &sts); err != nil {
			return nil, fmt.Errorf("scan detail: %w", err)
		}
		l := DiffLine{Key: engine.CompKey{Type: compType(typ), ID: id}}
		if l.Amount, err = decPtr(amt); err != nil {
			return nil, err
		}
		if withInputs {
			if l.Basis, err = decPtr(basis); err != nil {
				return nil, err
			}
			l.Sts = sts
		}
		e := out[nik]
		e.Lines = append(e.Lines, l)
		out[nik] = e
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, heads(ctx, tx, runID, out)
}

func heads(ctx context.Context, tx *sql.Tx, runID int64, out map[string]DiffEmp) error {
	rows, err := tx.QueryContext(ctx, `SELECT psh_nik, psh_tot_tunjangan::text, psh_tot_potongan::text, psh_net_pay::text
FROM payroll.pyr_salary_head WHERE psh_run_id = $1`, runID)
	if err != nil {
		return fmt.Errorf("load head run %d: %w", runID, err)
	}
	defer closeRows(rows)
	for rows.Next() {
		var nik, t, p, net string
		if err := rows.Scan(&nik, &t, &p, &net); err != nil {
			return fmt.Errorf("scan head: %w", err)
		}
		var h HeadTotals
		var err error
		if h.TotTunjangan, err = decimal.NewFromString(t); err != nil {
			return fmt.Errorf("head %s: %w", nik, err)
		}
		if h.TotPotongan, err = decimal.NewFromString(p); err != nil {
			return fmt.Errorf("head %s: %w", nik, err)
		}
		if h.TakeHome, err = decimal.NewFromString(net); err != nil {
			return fmt.Errorf("head %s: %w", nik, err)
		}
		e := out[nik]
		e.Head = &h
		out[nik] = e
	}
	return rows.Err()
}

// attChanged lists NIKs whose recap rows (REKAP/ADD/SCH, fail counters) differ between the legacy snapshot and
// the snapshot the target used. Same snapshot, or a LEGACY run without one (D-35.3), means no difference.
func attChanged(ctx context.Context, tx *sql.Tx, legacy, target int64) (map[string]bool, error) {
	out := map[string]bool{}
	if legacy == 0 || target == 0 || legacy == target {
		return out, nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT COALESCE(a.pasr_nik, b.pasr_nik)
FROM (SELECT * FROM payroll.pyr_att_snapshot_row WHERE pasr_snapshot_id = $1) a
FULL JOIN (SELECT * FROM payroll.pyr_att_snapshot_row WHERE pasr_snapshot_id = $2) b
  ON a.pasr_kind = b.pasr_kind AND a.pasr_nik = b.pasr_nik
WHERE a.pasr_data IS DISTINCT FROM b.pasr_data OR a.pasr_premi_fail IS DISTINCT FROM b.pasr_premi_fail
   OR a.pasr_beras_fail IS DISTINCT FROM b.pasr_beras_fail`, legacy, target)
	if err != nil {
		return nil, fmt.Errorf("compare snapshots: %w", err)
	}
	defer closeRows(rows)
	for rows.Next() {
		var nik string
		if err := rows.Scan(&nik); err != nil {
			return nil, fmt.Errorf("scan snapshot diff: %w", err)
		}
		out[nik] = true
	}
	return out, rows.Err()
}

func componentsByNIK(inputs []engine.Input) map[string][]engine.EmpComponent {
	out := make(map[string][]engine.EmpComponent, len(inputs))
	for _, in := range inputs {
		out[in.Emp.NIK] = in.Components
	}
	return out
}

func compType(s string) engine.CompType {
	if s == "Potongan" || s == "P" {
		return engine.CompP
	}
	return engine.CompT
}

func decPtr(s sql.NullString) (*decimal.Decimal, error) {
	if !s.Valid {
		return nil, nil //nolint:nilnil // NULL amount (D10) is a valid nil
	}
	d, err := decimal.NewFromString(s.String)
	if err != nil {
		return nil, fmt.Errorf("recon: amount %q: %w", s.String, err)
	}
	return &d, nil
}

// SyntheticMainRemark is the psh_remarks marker the ETL writes on the synthetic MAIN (etl.SyntheticMainRemark; same
// value, the layers cannot import each other and tests/etl compares them).
const SyntheticMainRemark = "[ETL:SYNTHETIC_MAIN D-35.2]"

// loadDrift reads the attributes of the D-47 exclusions for the LEGACY run (5 queries).
func loadDrift(ctx context.Context, tx *sql.Tx, q run.Query, legacyRun, legacySnap int64) (DriftInput, error) {
	var d DriftInput
	var err error
	// The ETL marks the synthetic MAIN of D-35.2 explicitly in psh_remarks (T009e10); LEGACY runs loaded before that
	// must be reloaded with `sync salary-legacy` to carry the marker.
	if d.SyntheticMain, err = nikSet(ctx, tx, `SELECT psh_nik FROM payroll.pyr_salary_head WHERE psh_run_id = $1
  AND psh_remarks = $2`, legacyRun, SyntheticMainRemark); err != nil {
		return d, fmt.Errorf("synthetic main: %w", err)
	}
	// Legacy calculated (MAIN) a NIK the current master no longer marks active, or no longer has.
	const inactive = `SELECT h.psh_nik FROM payroll.pyr_salary_head h
LEFT JOIN payroll.pyr_employee e ON e.pe_company_id = $2 AND e.pe_division = $3 AND e.pe_nik = h.psh_nik
WHERE h.psh_run_id = $1 AND `
	if d.Inactive, err = nikSet(ctx, tx, inactive+`e.pe_nik IS NOT NULL AND e.pe_sts_payroll IS DISTINCT FROM 2`,
		legacyRun, q.CompanyID, q.Division); err != nil {
		return d, fmt.Errorf("inactive master: %w", err)
	}
	if d.Missing, err = nikSet(ctx, tx, inactive+`e.pe_nik IS NULL`, legacyRun, q.CompanyID, q.Division); err != nil {
		return d, fmt.Errorf("missing master: %w", err)
	}
	// D-50: active master, not resigned, joined after the period end, no REKAP row in the legacy snapshot.
	// Without a legacy snapshot the "no recap" condition cannot be proven, so nothing is excluded.
	if legacySnap != 0 {
		if d.LateJoin, err = nikSet(ctx, tx, `SELECT e.pe_nik FROM payroll.pyr_employee e
JOIN payroll.pyr_period p ON p.pp_company_id = e.pe_company_id AND p.pp_division = e.pe_division AND p.pp_yymm = $3
WHERE e.pe_company_id = $1 AND e.pe_division = $2 AND e.pe_sts_payroll = 2 AND e.pe_date_join > p.pp_date_to
  AND (e.pe_resign_date IS NULL OR e.pe_resign_date > p.pp_date_to)
  AND NOT EXISTS (SELECT 1 FROM payroll.pyr_att_snapshot_row r WHERE r.pasr_snapshot_id = $4
    AND r.pasr_kind = 'REKAP' AND r.pasr_nik = e.pe_nik)`,
			q.CompanyID, q.Division, q.YYMM, legacySnap); err != nil {
			return d, fmt.Errorf("late join: %w", err)
		}
	}
	d.Frozen, err = frozenComponents(ctx, tx, q)
	return d, err
}

func nikSet(ctx context.Context, tx *sql.Tx, query string, args ...any) (map[string]bool, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	out := map[string]bool{}
	for rows.Next() {
		var nik string
		if err := rows.Scan(&nik); err != nil {
			return nil, err
		}
		out[nik] = true
	}
	return out, rows.Err()
}

// frozenComponents lists the master components with frz_flag <> 2 inside the validity window of the period.
func frozenComponents(ctx context.Context, tx *sql.Tx, q run.Query) (map[string]map[engine.CompKey]bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT e.pec_nik, c.pc_type::text, c.pc_code
FROM payroll.pyr_employee_component e JOIN payroll.pyr_component c ON c.pc_sysid = e.pec_component_sysid
WHERE e.pec_company_id = $1 AND e.pec_division = $2 AND e.pec_frz_flag IS DISTINCT FROM 2
  AND (e.pec_valid_from IS NULL OR e.pec_valid_from <= $3) AND (e.pec_valid_to IS NULL OR e.pec_valid_to >= $3)`,
		q.CompanyID, q.Division, q.YYMM)
	if err != nil {
		return nil, fmt.Errorf("frozen components: %w", err)
	}
	defer closeRows(rows)
	out := map[string]map[engine.CompKey]bool{}
	for rows.Next() {
		var nik, typ, code string
		if err := rows.Scan(&nik, &typ, &code); err != nil {
			return nil, fmt.Errorf("scan frozen component: %w", err)
		}
		k, err := compKeyOf(typ, code)
		if err != nil {
			return nil, err
		}
		if out[nik] == nil {
			out[nik] = map[engine.CompKey]bool{}
		}
		out[nik][k] = true
	}
	return out, rows.Err()
}

// compKeyOf maps (pc_type, pc_code 'T015') to the legacy key.
func compKeyOf(typ, code string) (engine.CompKey, error) {
	n, err := strconv.Atoi(strings.TrimLeft(code, "TPtp"))
	if err != nil || len(code) < 2 {
		return engine.CompKey{}, fmt.Errorf("recon: component code %q", code)
	}
	return engine.CompKey{Type: compType(typ), ID: n}, nil
}
