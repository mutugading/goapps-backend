package etl

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/scope"
	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/etl"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/postgres"
)

// overlay reads IM_VS_STATIC_VALUE from the full legacy dump (159 rows, spec/_analysis/ddl2.txt)
// and every other input table from testdata.
type overlay struct{}

func (overlay) Read(table string) (*etl.Table, error) {
	if table == etl.TableStaticValue {
		return etl.PSVDir{Dir: "testdata_ruleset"}.Read(table)
	}
	return etl.PSVDir{Dir: "testdata"}.Read(table)
}

// T016a: the rule set loader reads, as payroll_app with a scope, what the T009c ETL wrote.
// Runs after the other ETL tests (file order): it replaces the 3-row static value fixture.
func TestRuleSetLoader_FromETL(t *testing.T) {
	ctx := context.Background()
	// Components first (idempotent): the subsidy rule needs a target sysid when run alone with -run.
	if _, err := etl.Run(ctx, etlDB, etl.PSVDir{Dir: "testdata"}, etl.Options{CompanyID: 1, ValidFrom: 202001}); err != nil {
		t.Fatal(err)
	}
	if _, err := etl.RunInputs(ctx, etlDB, overlay{}, etl.InputOptions{CompanyID: 1}); err != nil {
		t.Fatal(err)
	}
	if n := scalar(t, `SELECT count(*) FROM payroll.pyr_static_value WHERE psv_rule_set_id = 1`); n != "159" {
		t.Fatalf("static values: %s, want 159", n)
	}
	seedSubsidy(t)
	raw, err := sql.Open("pgx", appDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	app := postgres.NewDBFromSQL(raw)
	both := asScopes(t, "SOLO", "JAKARTA")

	load := func(ctx context.Context, q postgres.RuleQuery) (postgres.RunRules, error) {
		var out postgres.RunRules
		err := app.ScopedTx(ctx, func(tx *sql.Tx) error {
			var lerr error
			out, lerr = postgres.NewRuleSetLoader().Load(ctx, tx, q)
			return lerr
		})
		return out, err
	}

	t.Run("SOLO non-December", func(t *testing.T) {
		rr, err := load(both, postgres.RuleQuery{RuleSetID: 1, CompanyID: 1, Division: "SOLO", YYMM: 202609})
		if err != nil {
			t.Fatal(err)
		}
		checkConst(t, rr.Rules, "DAYS_MONTH", "30")
		checkConst(t, rr.Rules, engine.ConstKesMin, "2592154") // division row wins
		if !rr.Rules.LegacyCompat() || rr.Rules.ID() != 1 {
			t.Fatalf("rule set: id %d legacy %v", rr.Rules.ID(), rr.Rules.LegacyCompat())
		}
		checkTax(t, rr.Tax)
		if len(rr.SubsidyRules) != 1 || rr.SubsidyRules[0].Name != "SOLO 2026" || !rr.SubsidyRules[0].Limit.Equal(dec("10000000")) ||
			rr.SubsidyRules[0].BasisPeriod != 202601 || rr.SubsidyRules[0].Divisions[0] != "SOLO" {
			t.Fatalf("subsidy rules (DRAFT and other periods left out): %+v", rr.SubsidyRules)
		}
		if o := rr.Subsidy("S1").Override; o == nil || o.Action != engine.OverrideExclude {
			t.Fatalf("approved override S1: %+v", o)
		}
		if o := rr.Subsidy("S2").Override; o != nil {
			t.Fatalf("unapproved override S2 must be ignored: %+v", o)
		}
	})
	t.Run("JAKARTA constants, limit NULL fails (D-31)", func(t *testing.T) {
		rr, err := load(both, postgres.RuleQuery{RuleSetID: 1, CompanyID: 1, Division: "JAKARTA", YYMM: 202512})
		if err != nil {
			t.Fatal(err)
		}
		checkConst(t, rr.Rules, engine.ConstKesMin, "5729876")
		if len(rr.Tax.PTKP) != 9 || len(rr.Tax.Progressive) != 5 {
			t.Fatalf("December tables: PTKP %d layers %d", len(rr.Tax.PTKP), len(rr.Tax.Progressive))
		}
		_, err = load(both, postgres.RuleQuery{RuleSetID: 1, CompanyID: 1, Division: "JAKARTA", YYMM: 202609})
		if !errors.Is(err, postgres.ErrSubsidyNoLimit) {
			t.Fatalf("got %v, want ErrSubsidyNoLimit", err)
		}
	})
	t.Run("division outside scope", func(t *testing.T) {
		_, err := load(asScopes(t, "SOLO"), postgres.RuleQuery{RuleSetID: 1, CompanyID: 1, Division: "JAKARTA", YYMM: 202609})
		if !errors.Is(err, postgres.ErrRuleOutOfScope) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("empty tax table fails before calculation", func(t *testing.T) {
		_, err := load(both, postgres.RuleQuery{RuleSetID: 2, CompanyID: 1, Division: "SOLO", YYMM: 202609})
		if !errors.Is(err, engine.ErrNoTaxTable) {
			t.Fatalf("got %v, want ErrNoTaxTable", err)
		}
	})
	t.Run("missing and DRAFT rule set", func(t *testing.T) {
		if _, err := load(both, postgres.RuleQuery{RuleSetID: 9, CompanyID: 1, Division: "SOLO", YYMM: 202609}); !errors.Is(err, postgres.ErrRuleSetNotFound) {
			t.Fatalf("got %v", err)
		}
		if _, err := load(both, postgres.RuleQuery{RuleSetID: 3, CompanyID: 1, Division: "SOLO", YYMM: 202609}); !errors.Is(err, postgres.ErrRuleSetStatus) {
			t.Fatalf("got %v", err)
		}
	})
}

func seedSubsidy(t *testing.T) {
	t.Helper()
	for _, q := range []string{
		`INSERT INTO payroll.pyr_rule_set (prs_id, prs_name, prs_effective_from, prs_status) VALUES
		   (2, 'tanpa tabel pajak', 202601, 'ACTIVE'), (3, 'draft', 202601, 'DRAFT') ON CONFLICT DO NOTHING`,
		`INSERT INTO payroll.pyr_subsidy_rule (psr_company_id, psr_name, psr_valid_from, psr_valid_to, psr_divisions, psr_basis_kind,
		   psr_basis_period_yymm, psr_limit_amount, psr_subsidy_kind, psr_target_component_sysid, psr_status, created_by, approved_by)
		 SELECT 1, v.name, v.vf, v.vt, v.divs, 'COMPONENTS_PERIOD', 202601, v.lim, 'FULL_PPH', c.pc_sysid, v.sts, 'maker', 'checker'
		 FROM (VALUES ('SOLO 2026', 202601, 202612, '{SOLO}'::varchar[], 10000000::numeric, 'ACTIVE'),
		              ('SOLO draft', 202601, NULL, '{SOLO}', NULL, 'DRAFT'),
		              ('SOLO 2025', 202501, 202512, '{SOLO}', 10000000, 'ACTIVE'),
		              ('JKT tanpa limit', 202601, NULL, '{JAKARTA}', NULL, 'ACTIVE')) v(name, vf, vt, divs, lim, sts)
		 CROSS JOIN LATERAL (SELECT min(pc_sysid) pc_sysid FROM payroll.pyr_component WHERE pc_company_id = 1) c`,
		`INSERT INTO payroll.pyr_subsidy_override (pso_yymm, pso_company_id, pso_division, pso_nik, pso_action, pso_reason, created_by, approved_by) VALUES
		   (202609, 1, 'SOLO', 'S1', 'EXCLUDE', 'warisan legacy', 'maker', 'checker'),
		   (202609, 1, 'SOLO', 'S2', 'INCLUDE', 'belum disetujui', 'maker', NULL)`,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
}

func asScopes(t *testing.T, divs ...string) context.Context {
	t.Helper()
	var ss []scope.Scope
	for _, d := range divs {
		s, err := scope.New(1, d)
		if err != nil {
			t.Fatal(err)
		}
		ss = append(ss, s)
	}
	return scope.WithIdentity(context.Background(), scope.Identity{UserID: "loader-test", Scopes: ss})
}

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func checkConst(t *testing.T, r engine.RuleSet, k, want string) {
	t.Helper()
	if v, ok := r.Constant(k); !ok || !v.Equal(dec(want)) {
		t.Fatalf("constant %s = %s (%v), want %s", k, v, ok, want)
	}
}

// checkTax compares with the legacy rows quoted in D-28/D-29 and the engine fixtures.
func checkTax(t *testing.T, tt engine.TaxTables) {
	t.Helper()
	wantCat := map[string]string{"K0": "A", "K1": "B", "K2": "B", "K3": "C", "TK": "A", "TK1": "A", "TK2": "B", "TK3": "B"}
	if len(tt.Category) != len(wantCat) {
		t.Fatalf("category: %v", tt.Category)
	}
	for k, v := range wantCat {
		if tt.Category[k] != v {
			t.Fatalf("category %s = %q, want %s", k, tt.Category[k], v)
		}
	}
	if len(tt.TER["A"]) != 44 || len(tt.TER["B"]) != 40 || len(tt.TER["C"]) != 41 {
		t.Fatalf("TER sizes A %d B %d C %d", len(tt.TER["A"]), len(tt.TER["B"]), len(tt.TER["C"]))
	}
	// D-28: B 6.500.000 | 6.500.001 is a bracket boundary (1-rupiah gap between rows).
	brackets := []struct{ cat, gross, rate string }{
		{"A", "5400000", "0"}, {"A", "5400001", "0.25"}, {"A", "10000000", "2"},
		{"B", "6500000", "0.25"}, {"B", "6500001", "0.5"}, {"C", "6950000", "0.25"}, {"C", "1419000001", "34"},
	}
	for _, b := range brackets {
		if got, ok := terRate(tt, b.cat, dec(b.gross)); !ok || !got.Equal(dec(b.rate)) {
			t.Fatalf("TER %s %s = %s (%v), want %s", b.cat, b.gross, got, ok, b.rate)
		}
	}
	// D-29: PTKP monthly, BJ 5 % max 500.000, five layers up to 9 M.
	for k, v := range map[string]string{"TK": "4500000", "TK1": "4875000", "K": "4875000", "K0": "4875000", "K3": "6000000"} {
		if !tt.PTKP[k].Equal(dec(v)) {
			t.Fatalf("PTKP %s = %s, want %s", k, tt.PTKP[k], v)
		}
	}
	if len(tt.PTKP) != 9 || !tt.JobCost.Rate.Equal(dec("5")) || !tt.JobCost.MaxMonthly.Equal(dec("500000")) {
		t.Fatalf("PTKP %d rows, job cost %+v", len(tt.PTKP), tt.JobCost)
	}
	layers := map[string][2]string{ // Max → Min, Rate
		"60000000": {"1", "5"}, "250000000": {"60000001", "15"}, "500000000": {"250000001", "25"},
		"5000000000": {"500000001", "30"}, "9000000000": {"5000000001", "35"},
	}
	if len(tt.Progressive) != len(layers) {
		t.Fatalf("progressive: %d layers, want 5", len(tt.Progressive))
	}
	for _, l := range tt.Progressive {
		w, ok := layers[l.Max.String()]
		if !ok || w[0] != l.Min.String() || w[1] != l.Rate.String() {
			t.Fatalf("layer %+v", l)
		}
	}
	// Every TER category: rows sorted by Min start at 0, are contiguous with a 1-rupiah step
	// (D-28) and end at 1.000.000.000.000.
	for cat, bs := range tt.TER {
		s := append([]engine.TERBracket(nil), bs...)
		sort.Slice(s, func(i, j int) bool { return s[i].Min.LessThan(s[j].Min) })
		if !s[0].Min.IsZero() || !s[len(s)-1].Max.Equal(dec("1000000000000")) {
			t.Fatalf("TER %s range %s..%s", cat, s[0].Min, s[len(s)-1].Max)
		}
		for i := 1; i < len(s); i++ {
			if !s[i].Min.Equal(s[i-1].Max.Add(decimal.NewFromInt(1))) || s[i].Rate.LessThan(s[i-1].Rate) {
				t.Fatalf("TER %s gap/order at %+v after %+v", cat, s[i], s[i-1])
			}
		}
	}
}

func terRate(tt engine.TaxTables, cat string, g decimal.Decimal) (decimal.Decimal, bool) {
	for _, b := range tt.TER[cat] {
		if g.GreaterThanOrEqual(b.Min) && g.LessThanOrEqual(b.Max) {
			return b.Rate, true
		}
	}
	return decimal.Zero, false
}
