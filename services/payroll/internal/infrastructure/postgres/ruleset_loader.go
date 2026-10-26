package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
)

// Rule set loader errors.
var (
	ErrRuleSetNotFound  = errors.New("rule set not found")
	ErrRuleSetStatus    = errors.New("rule set is not ACTIVE or RETIRED")
	ErrBadRuleQuery     = errors.New("invalid rule query")
	ErrRuleOutOfScope   = errors.New("company/division outside the caller scope")
	ErrSubsidyNoLimit   = errors.New("subsidy rule has no limit (psr_limit_amount NULL, D-31)")
	ErrSubsidyNoAmount  = errors.New("subsidy override AMOUNT has no amount")
	ErrStaticValueParse = errors.New("pyr_static_value is not a number")
	ErrStaticValueDup   = errors.New("pyr_static_value row duplicated")
	ErrRuleCompType     = errors.New("unknown pyr_rule_component type")
)

// RuleQuery selects the rules of one run: rule set, company, division and period.
type RuleQuery struct {
	RuleSetID int
	CompanyID int16
	Division  string
	YYMM      int
}

// ComponentRule is one pyr_rule_component row: the registry formula of a component (03§2.2).
type ComponentRule struct {
	Formula   string
	Params    engine.Params
	RemarkTpl string
}

// RuleSet is the immutable engine.RuleSet of one run, resolved for one company and division:
// a division row of pyr_rule_constant / pyr_rule_component wins over the NULL (all) row.
type RuleSet struct {
	id         int
	legacy     bool
	constants  map[string]decimal.Decimal
	components map[engine.CompKey]ComponentRule
}

var _ engine.RuleSet = (*RuleSet)(nil)

// ID implements engine.RuleSet.
func (r *RuleSet) ID() int { return r.id }

// LegacyCompat implements engine.RuleSet.
func (r *RuleSet) LegacyCompat() bool { return r.legacy }

// Constant implements engine.RuleSet.
func (r *RuleSet) Constant(key string) (decimal.Decimal, bool) {
	v, ok := r.constants[key]
	return v, ok
}

// Component returns the formula configured for k, if any. Params is a copy.
func (r *RuleSet) Component(k engine.CompKey) (ComponentRule, bool) {
	c, ok := r.components[k]
	if !ok {
		return c, false
	}
	p := make(engine.Params, len(c.Params))
	for key, v := range c.Params {
		p[key] = v
	}
	c.Params = p
	return c, true
}

// RunRules is everything the loader reads once per run (not per NIK). Tax and the subsidy rules
// are shared by every Input of the run and must be treated as read-only.
type RunRules struct {
	Rules        *RuleSet
	Tax          engine.TaxTables
	SubsidyRules []engine.SubsidyRule
	overrides    map[string]engine.SubsidyOverride
}

// Subsidy returns the P0 input of nik with the run's rules and the employee's approved override.
// The basis fields (Basis, PrevBasis, Locked) are per employee and filled by the input loader.
func (r RunRules) Subsidy(nik string) engine.Subsidy {
	s := engine.Subsidy{Rules: r.SubsidyRules}
	if o, ok := r.overrides[nik]; ok {
		s.Override = &o
	}
	return s
}

// RuleSetLoader reads the rule set, tax tables and subsidy rules of a run (T016a).
type RuleSetLoader struct{}

// NewRuleSetLoader creates a RuleSetLoader.
func NewRuleSetLoader() RuleSetLoader { return RuleSetLoader{} }

// Load reads the rules of q inside tx, which must carry the caller scope (DB.ScopedTx). A division
// outside the scope fails instead of loading without its RLS-hidden division rows.
func (RuleSetLoader) Load(ctx context.Context, tx *sql.Tx, q RuleQuery) (RunRules, error) {
	if m := q.YYMM % 100; q.YYMM < 190001 || m < 1 || m > 12 || q.Division == "" {
		return RunRules{}, fmt.Errorf("%w: %+v", ErrBadRuleQuery, q)
	}
	var in bool
	if err := tx.QueryRowContext(ctx, `SELECT payroll.fn_in_scope($1, $2)`, q.CompanyID, q.Division).Scan(&in); err != nil {
		return RunRules{}, fmt.Errorf("check rule scope: %w", err)
	}
	if !in {
		return RunRules{}, fmt.Errorf("%w: %d:%s", ErrRuleOutOfScope, q.CompanyID, q.Division)
	}
	rs, err := loadRuleSet(ctx, tx, q)
	if err != nil {
		return RunRules{}, err
	}
	tax, err := loadTaxTables(ctx, tx, q.RuleSetID, q.YYMM%100 == 12)
	if err != nil {
		return RunRules{}, err
	}
	rules, err := loadSubsidyRules(ctx, tx, q)
	if err != nil {
		return RunRules{}, err
	}
	ovr, err := loadSubsidyOverrides(ctx, tx, q)
	if err != nil {
		return RunRules{}, err
	}
	return RunRules{Rules: rs, Tax: tax, SubsidyRules: rules, overrides: ovr}, nil
}

func loadRuleSet(ctx context.Context, tx *sql.Tx, q RuleQuery) (*RuleSet, error) {
	rs := &RuleSet{id: q.RuleSetID, constants: map[string]decimal.Decimal{}, components: map[engine.CompKey]ComponentRule{}}
	var status string
	err := tx.QueryRowContext(ctx, `SELECT prs_legacy_compat, prs_status FROM payroll.pyr_rule_set WHERE prs_id = $1`,
		q.RuleSetID).Scan(&rs.legacy, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %d", ErrRuleSetNotFound, q.RuleSetID)
	}
	if err != nil {
		return nil, fmt.Errorf("read rule set %d: %w", q.RuleSetID, err)
	}
	if status != "ACTIVE" && status != "RETIRED" { // RETIRED: reprocessing an old period uses its own rule set
		return nil, fmt.Errorf("%w: %d is %s", ErrRuleSetStatus, q.RuleSetID, status)
	}
	if err := loadConstants(ctx, tx, q, rs.constants); err != nil {
		return nil, err
	}
	if err := loadComponentRules(ctx, tx, q, rs.components); err != nil {
		return nil, err
	}
	return rs, nil
}

// loadConstants orders NULL-division rows first so the division row overwrites them.
func loadConstants(ctx context.Context, tx *sql.Tx, q RuleQuery, out map[string]decimal.Decimal) error {
	rows, err := tx.QueryContext(ctx, `
SELECT prc_key, prc_value FROM payroll.pyr_rule_constant
WHERE prc_rule_set_id = $1 AND prc_company_id = $2 AND (prc_division IS NULL OR prc_division = $3)
ORDER BY prc_division NULLS FIRST, prc_key`, q.RuleSetID, q.CompanyID, q.Division)
	if err != nil {
		return fmt.Errorf("read rule constants: %w", err)
	}
	defer closeRows(rows)
	for rows.Next() {
		var (
			key string
			v   decimal.Decimal
		)
		if err := rows.Scan(&key, &v); err != nil {
			return fmt.Errorf("scan rule constant: %w", err)
		}
		out[key] = v
	}
	return rows.Err()
}

func loadComponentRules(ctx context.Context, tx *sql.Tx, q RuleQuery, out map[engine.CompKey]ComponentRule) error {
	rows, err := tx.QueryContext(ctx, `
SELECT prcp_type::text, prcp_id, prcp_formula, prcp_params, COALESCE(prcp_remark_tpl, '')
FROM payroll.pyr_rule_component
WHERE prcp_rule_set_id = $1 AND prcp_company_id = $2 AND (prcp_division IS NULL OR prcp_division = $3)
ORDER BY prcp_division NULLS FIRST, prcp_type, prcp_id`, q.RuleSetID, q.CompanyID, q.Division)
	if err != nil {
		return fmt.Errorf("read rule components: %w", err)
	}
	defer closeRows(rows)
	for rows.Next() {
		var (
			typ    string
			id     int
			c      ComponentRule
			params []byte
		)
		if err := rows.Scan(&typ, &id, &c.Formula, &params, &c.RemarkTpl); err != nil {
			return fmt.Errorf("scan rule component: %w", err)
		}
		k := engine.CompKey{ID: id}
		switch typ {
		case "Tunjangan":
			k.Type = engine.CompT
		case "Potongan":
			k.Type = engine.CompP
		default:
			return fmt.Errorf("%w: %q", ErrRuleCompType, typ)
		}
		if c.Params, err = decodeParams(params); err != nil {
			return fmt.Errorf("rule component %s params: %w", k, err)
		}
		out[k] = c
	}
	return rows.Err()
}

// decodeParams turns the jsonb object into engine.Params: strings unquoted, other values as JSON text.
func decodeParams(b []byte) (engine.Params, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	p := make(engine.Params, len(raw))
	for k, v := range raw {
		var s string
		if err := json.Unmarshal(v, &s); err == nil {
			p[k] = s
			continue
		}
		p[k] = string(v)
	}
	return p, nil
}

func loadSubsidyRules(ctx context.Context, tx *sql.Tx, q RuleQuery) ([]engine.SubsidyRule, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT psr_id, psr_name, psr_priority, psr_valid_from, COALESCE(psr_valid_to, 0),
       COALESCE(array_to_string(psr_divisions, ','), ''), COALESCE(array_to_string(psr_depts, ','), ''),
       COALESCE(psr_basis_period_yymm, 0),
       psr_limit_amount, psr_subsidy_kind
FROM payroll.pyr_subsidy_rule
WHERE psr_company_id = $1 AND psr_status = 'ACTIVE' AND psr_valid_from <= $2
  AND (psr_valid_to IS NULL OR psr_valid_to >= $2) AND (psr_divisions IS NULL OR $3 = ANY (psr_divisions))
ORDER BY psr_priority, psr_id`, q.CompanyID, q.YYMM, q.Division)
	if err != nil {
		return nil, fmt.Errorf("read subsidy rules: %w", err)
	}
	defer closeRows(rows)
	var out []engine.SubsidyRule
	for rows.Next() {
		var (
			r           engine.SubsidyRule
			divs, depts string
			limit       decimal.NullDecimal
		)
		if err := rows.Scan(&r.ID, &r.Name, &r.Priority, &r.ValidFrom, &r.ValidTo, &divs, &depts,
			&r.BasisPeriod, &limit, &r.Kind); err != nil {
			return nil, fmt.Errorf("scan subsidy rule: %w", err)
		}
		if !limit.Valid { // legacy limit NULL gives P0 = +P1; the engine reads 0 as a 0 limit (D-31)
			return nil, fmt.Errorf("%w: rule %d %s", ErrSubsidyNoLimit, r.ID, r.Name)
		}
		r.Limit, r.Divisions, r.Depts = limit.Decimal, splitCSV(divs), splitCSV(depts)
		out = append(out, r)
	}
	return out, rows.Err()
}

// loadSubsidyOverrides reads the approved overrides of the period, keyed by NIK.
func loadSubsidyOverrides(ctx context.Context, tx *sql.Tx, q RuleQuery) (map[string]engine.SubsidyOverride, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT pso_nik, pso_action, pso_amount, pso_reason FROM payroll.pyr_subsidy_override
WHERE pso_company_id = $1 AND pso_yymm = $2 AND pso_division = $3 AND approved_by IS NOT NULL
ORDER BY pso_nik`,
		q.CompanyID, q.YYMM, q.Division)
	if err != nil {
		return nil, fmt.Errorf("read subsidy overrides: %w", err)
	}
	defer closeRows(rows)
	out := map[string]engine.SubsidyOverride{}
	for rows.Next() {
		var (
			nik string
			o   engine.SubsidyOverride
			amt decimal.NullDecimal
		)
		if err := rows.Scan(&nik, &o.Action, &amt, &o.Reason); err != nil {
			return nil, fmt.Errorf("scan subsidy override: %w", err)
		}
		if o.Action == engine.OverrideAmount && !amt.Valid {
			return nil, fmt.Errorf("%w: %s", ErrSubsidyNoAmount, nik)
		}
		o.Amount = amt.Decimal
		out[nik] = o
	}
	return out, rows.Err()
}
