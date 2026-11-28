package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
)

// pyr_static_value groups and codes read by legacy FNC_MGT_PAYROLL_PPH (D-28, D-29).
const (
	vsEmpTax       = "EMPTAX"       // code = pe_tax_id, field_01 = TER category
	vsPayrollSetup = "PAYROLLSETUP" // TER brackets, PTKP, BIAYAJABATAN, TARIFxx
	codeJobCost    = "BIAYAJABATAN" // name = rate %, short_name = monthly max
	prefixTariff   = "TARIF"        // name = min, short_name = max, field_01 = rate %
)

// ptkpCodes are the PAYROLLSETUP PTKP rows of the December P1 query (name = monthly PTKP).
var ptkpCodes = map[string]bool{"TK": true, "K": true, "K0": true, "K1": true, "K2": true, "K3": true,
	"TK1": true, "TK2": true, "TK3": true}

// ptkpCompanyCodes are the codes of the company-paid P5 query: no TK1–TK3 (D-37).
var ptkpCompanyCodes = map[string]bool{"TK": true, "K": true, "K0": true, "K1": true, "K2": true, "K3": true}

// terCategories are the PAYROLLSETUP field_01 values of a TER bracket (name = min,
// short_name = max, field_02 = rate %).
var terCategories = map[string]bool{"A": true, "B": true, "C": true}

type staticValue struct {
	vs, code, name, short, f1, f2 string
}

// loadTaxTables builds engine.TaxTables from pyr_static_value of the rule set. An empty TER table
// fails with engine.ErrNoTaxTable; in December a missing annual table fails with
// engine.ErrNoAnnualTaxTable, so the run stops before any employee is calculated.
func loadTaxTables(ctx context.Context, tx *sql.Tx, ruleSetID int, december bool) (engine.TaxTables, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT psv_vs_code, psv_code, COALESCE(psv_name,''), COALESCE(psv_short_name,''),
       COALESCE(psv_field_01,''), COALESCE(psv_field_02,'')
FROM payroll.pyr_static_value
WHERE psv_rule_set_id = $1 AND psv_vs_code IN ($2, $3)
ORDER BY psv_vs_code, psv_code`, ruleSetID, vsEmpTax, vsPayrollSetup)
	if err != nil {
		return engine.TaxTables{}, fmt.Errorf("read static values: %w", err)
	}
	defer closeRows(rows)
	var svs []staticValue
	for rows.Next() {
		var v staticValue
		if err := rows.Scan(&v.vs, &v.code, &v.name, &v.short, &v.f1, &v.f2); err != nil {
			return engine.TaxTables{}, fmt.Errorf("scan static value: %w", err)
		}
		svs = append(svs, v)
	}
	if err := rows.Err(); err != nil {
		return engine.TaxTables{}, fmt.Errorf("read static values: %w", err)
	}
	t, err := buildTaxTables(svs)
	if err != nil {
		return t, err
	}
	return t, checkTaxTables(t, ruleSetID, december)
}

func buildTaxTables(svs []staticValue) (engine.TaxTables, error) {
	t := engine.TaxTables{
		Category:    map[string]string{},
		TER:         map[string][]engine.TERBracket{},
		PTKP:        map[string]decimal.Decimal{},
		PTKPCompany: map[string]decimal.Decimal{},
	}
	for _, v := range svs {
		if v.vs == vsEmpTax {
			if v.f1 != "" {
				t.Category[v.code] = v.f1
			}
			continue
		}
		if err := addPayrollSetup(&t, v); err != nil {
			return t, err
		}
	}
	return t, nil
}

// isTERCode reports a TER bracket row: field_01 is A/B/C and the code is that category followed by
// the rate (A0, A0.25, …), so a non-tax PAYROLLSETUP row with field_01 A/B/C is not taken as TER.
func isTERCode(v staticValue) bool {
	if !terCategories[v.f1] || !strings.HasPrefix(v.code, v.f1) {
		return false
	}
	_, err := decimal.NewFromString(strings.TrimPrefix(v.code, v.f1))
	return err == nil
}

func addPayrollSetup(t *engine.TaxTables, v staticValue) error {
	var err error
	switch {
	case isTERCode(v):
		var b engine.TERBracket
		if b.Min, b.Max, b.Rate, err = nums3(v, v.name, v.short, v.f2); err == nil {
			t.TER[v.f1] = append(t.TER[v.f1], b)
		}
	case ptkpCodes[v.code]:
		var p decimal.Decimal
		if p, err = num(v, v.name); err == nil {
			t.PTKP[v.code] = p
			if ptkpCompanyCodes[v.code] {
				t.PTKPCompany[v.code] = p
			}
		}
	case v.code == codeJobCost:
		if !t.JobCost.Rate.IsZero() || !t.JobCost.MaxMonthly.IsZero() {
			return fmt.Errorf("%w: %s/%s", ErrStaticValueDup, v.vs, v.code)
		}
		t.JobCost.Rate, t.JobCost.MaxMonthly, _, err = nums3(v, v.name, v.short, "0")
	case strings.HasPrefix(v.code, prefixTariff):
		var l engine.TaxLayer
		if l.Min, l.Max, l.Rate, err = nums3(v, v.name, v.short, v.f1); err != nil {
			return err
		}
		if overlaps(t.Progressive, l) {
			return fmt.Errorf("%w: %s/%s overlaps another layer", ErrStaticValueDup, v.vs, v.code)
		}
		t.Progressive = append(t.Progressive, l)
	}
	return err
}

// overlaps reports whether l shares a PKP range with an existing layer (it would be taxed twice).
func overlaps(ls []engine.TaxLayer, l engine.TaxLayer) bool {
	for _, o := range ls {
		if l.Min.LessThanOrEqual(o.Max) && o.Min.LessThanOrEqual(l.Max) {
			return true
		}
	}
	return false
}

// checkTaxTables reports an empty table as the engine error the formula would return.
func checkTaxTables(t engine.TaxTables, ruleSetID int, december bool) error {
	if len(t.Category) == 0 || len(t.TER) == 0 {
		return fmt.Errorf("%w: rule set %d", engine.ErrNoTaxTable, ruleSetID)
	}
	if december && (len(t.PTKP) == 0 || t.JobCost.Rate.IsZero() || t.JobCost.MaxMonthly.IsZero() || len(t.Progressive) == 0) {
		return fmt.Errorf("%w: rule set %d", engine.ErrNoAnnualTaxTable, ruleSetID)
	}
	return nil
}

func num(v staticValue, s string) (decimal.Decimal, error) {
	d, err := decimal.NewFromString(strings.TrimSpace(s))
	if err != nil {
		return d, fmt.Errorf("%w: %s/%s %q", ErrStaticValueParse, v.vs, v.code, s)
	}
	return d, nil
}

func nums3(v staticValue, a, b, c string) (x, y, z decimal.Decimal, err error) {
	if x, err = num(v, a); err != nil {
		return
	}
	if y, err = num(v, b); err != nil {
		return
	}
	z, err = num(v, c)
	return
}
