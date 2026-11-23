package gladi

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/postgres"
)

// taxFromRules overrides TAX_V2_FROM of the loaded rule set (0 = absent) without touching the DB.
type taxFromRules struct {
	engine.RuleSet
	from int64
}

func (r taxFromRules) Constant(k string) (decimal.Decimal, bool) {
	if k == engine.ConstTaxV2From {
		return decimal.NewFromInt(r.from), r.from != 0
	}
	return r.RuleSet.Constant(k)
}

// dryTax dry-runs yymm (nothing persisted) with TAX_V2_FROM forced to from; returns P0/P1/P5 per NIK.
func dryTax(t *testing.T, e env, div string, yymm int, from int64) lines {
	t.Helper()
	out := lines{}
	q := run.Query{CompanyID: company, Division: div, YYMM: yymm}
	ctx := scoped()
	err := postgres.NewDBFromSQL(e.app).ScopedTxOpts(ctx, run.SnapshotTx, func(tx *sql.Tx) error {
		r, err := rulesFrom(ctx, tx, q, from)
		if err != nil {
			return err
		}
		loaded, err := run.NewLoader().Load(ctx, tx, q, r)
		if err != nil {
			return err
		}
		res, err := run.NewRunner().DryRun(ctx, loaded)
		if err != nil {
			return err
		}
		for _, r := range res.Results {
			m := map[string]*decimal.Decimal{}
			for _, l := range r.Lines {
				if pphCodes[l.Key.String()] {
					m[l.Key.String()] = l.Amount
				}
			}
			out[r.NIK] = m
		}
		return nil
	})
	if err != nil {
		t.Fatalf("dry %d %s (TAX_V2_FROM %d): %v", yymm, div, from, err)
	}
	return out
}

func rulesFrom(ctx context.Context, tx *sql.Tx, q run.Query, from int64) (run.Rules, error) {
	rr, err := postgres.NewRuleSetLoader().Load(ctx, tx, postgres.RuleQuery{RuleSetID: ruleSetID, CompanyID: q.CompanyID, Division: q.Division, YYMM: q.YYMM})
	if err != nil {
		return run.Rules{}, err
	}
	return run.Rules{Set: taxFromRules{RuleSet: rr.Rules, from: from}, Tax: rr.Tax, Subsidy: rr.Subsidy}, nil
}

// TestTaxV2Impact202612 (T033m, D-80): simulation of 202612 PPh with tax v1 (D-29 parity) vs tax v2 on payroll_d64.
// Gated like the D-64 rehearsal (GLADI_D64_*); read-only (dry run). Report in $GLADI_D64_OUT/taxv2 (default /tmp/t033m).
func TestTaxV2Impact202612(t *testing.T) {
	e := openEnv(t)
	out := os.Getenv("GLADI_D64_OUT")
	if out == "" {
		out = "/tmp/t033m"
	}
	rep := newReport()
	for _, div := range divisions {
		v1 := dryTax(t, e, div, 202612, 0)
		v2 := dryTax(t, e, div, 202612, 202612)
		niks := make([]string, 0, len(v1))
		for n := range v1 {
			niks = append(niks, n)
		}
		sort.Strings(niks)
		sum := map[string][2]decimal.Decimal{}
		changed := map[string]int{}
		type diff struct {
			nik   string
			delta decimal.Decimal
		}
		var p1Diffs []diff
		for _, n := range niks {
			for _, c := range []string{"P001", "P005", "P000"} {
				a, b := v1[n][c], v2[n][c]
				s := sum[c]
				if a != nil {
					s[0] = s[0].Add(*a)
				}
				if b != nil {
					s[1] = s[1].Add(*b)
				}
				sum[c] = s
				if !eqAmt(a, b) {
					changed[c]++
					rep.detailf("%s %s %s: v1 %s → v2 %s", div, n, c, fmtAmt(a, true), fmtAmt(b, true))
					if c == "P001" && a != nil && b != nil {
						p1Diffs = append(p1Diffs, diff{n, b.Sub(*a)})
					}
				}
			}
		}
		for _, c := range []string{"P001", "P005", "P000"} {
			s := sum[c]
			rep.addf("202612 %s %s: Σ v1 %s, Σ v2 %s, selisih %s, NIK berubah %d dari %d", div, c, s[0].StringFixed(0), s[1].StringFixed(0),
				s[1].Sub(s[0]).StringFixed(0), changed[c], len(niks))
		}
		sort.Slice(p1Diffs, func(i, j int) bool { return p1Diffs[i].delta.LessThan(p1Diffs[j].delta) })
		var top []string
		for i := 0; i < len(p1Diffs) && i < 5; i++ {
			top = append(top, fmt.Sprintf("%s %s", p1Diffs[i].nik, p1Diffs[i].delta.StringFixed(0)))
		}
		rep.addf("202612 %s P001 turun terbesar: %s", div, strings.Join(top, ", "))
	}
	rep.write(t, out)
}
