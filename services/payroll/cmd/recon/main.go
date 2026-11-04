// Package main is the payroll reconciliation entry point (D-01, D-41). Subcommands: extract-golden, diff.
// It reads the LEGACY run in Postgres (role payroll_ro); there is no Oracle connection.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib" // driver

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/recon"
	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/scope"
	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/postgres"
)

const usage = `usage:
  recon extract-golden -yymm 202608 [-per-formula 20] [-company 1] [-divisions SOLO,JAKARTA] [-rule-set 1] [-out tests/golden/out] -dsn <payroll_ro dsn>
  recon diff -yymm 202608 -division SOLO [-company 1] [-target dry|run:<id>] [-rule-set 1] [-out report.json] -dsn <payroll_ro dsn>`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "extract-golden":
		if err := extractGolden(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "extract-golden:", err)
			os.Exit(1)
		}
	case "diff":
		os.Exit(diffCmd(os.Args[2:], os.Stdout, os.Stderr))
	default:
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
}

// openRO opens the read-only connection and returns a context carrying the recon identity for scopes.
func openRO(dsn string, scopes []scope.Scope) (*sql.DB, context.Context, error) {
	raw, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, nil, err
	}
	return raw, scope.WithIdentity(context.Background(), scope.Identity{UserID: "recon", Scopes: scopes}), nil
}

func closeDB(raw *sql.DB) {
	if err := raw.Close(); err != nil {
		fmt.Fprintln(os.Stderr, "close:", err)
	}
}

func extractGolden(args []string) error {
	fs := flag.NewFlagSet("extract-golden", flag.ExitOnError)
	yymm := fs.Int("yymm", 0, "periode yymm (wajib)")
	per := fs.Int("per-formula", 20, "jumlah kasus per formula")
	company := fs.Int("company", 1, "pyr_company.pco_company_id")
	divs := fs.String("divisions", "SOLO,JAKARTA", "divisi, dipisah koma")
	ruleSet := fs.Int("rule-set", 1, "rule set untuk memuat input (v1 = 1)")
	out := fs.String("out", "tests/golden/out", "folder keluaran (gitignored; berisi gaji nyata)")
	dsn := fs.String("dsn", os.Getenv("PAYROLL_RO_DSN"), "DSN role payroll_ro (read-only)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *yymm == 0 || *dsn == "" {
		return fmt.Errorf("-yymm dan -dsn (atau PAYROLL_RO_DSN) wajib")
	}
	scopes, names := make([]scope.Scope, 0, 2), make([]string, 0, 2)
	for _, d := range strings.Split(*divs, ",") {
		d = strings.TrimSpace(d)
		sc, err := scope.New(int16(*company), d) //nolint:gosec // flag range checked by FK
		if err != nil {
			return fmt.Errorf("divisi %q: %w", d, err)
		}
		scopes, names = append(scopes, sc), append(names, d)
	}
	raw, ctx, err := openRO(*dsn, scopes)
	if err != nil {
		return err
	}
	defer closeDB(raw)
	src := recon.PGSource{DB: postgres.NewDBFromSQL(raw), Rules: ruleFunc, FormulaOf: formulaOf}
	rep, err := recon.ExtractGolden(ctx, src, recon.Options{CompanyID: int16(*company), //nolint:gosec // as above
		Divisions: names, YYMM: *yymm, RuleSetID: *ruleSet, PerFormula: *per, Out: *out})
	if err != nil {
		return err
	}
	fmt.Printf("kandidat=%d berkas=%d out=%s\n", rep.Candidates, len(rep.Files), *out)
	for f, n := range rep.PerFormula {
		fmt.Printf("  %-24s %d\n", f, n)
	}
	return nil
}

// ruleFunc wires the rule set loader like the server does.
func ruleFunc(ctx context.Context, tx *sql.Tx, q run.Query, ruleSetID int) (run.Rules, error) {
	rr, err := postgres.NewRuleSetLoader().Load(ctx, tx, postgres.RuleQuery{RuleSetID: ruleSetID, CompanyID: q.CompanyID,
		Division: q.Division, YYMM: q.YYMM})
	if postgres.IsRuleError(err) {
		return run.Rules{}, fmt.Errorf("%w: %w", run.ErrRuleIncomplete, err)
	}
	if err != nil {
		return run.Rules{}, err
	}
	return run.Rules{Set: rr.Rules, Tax: rr.Tax, Subsidy: rr.Subsidy}, nil
}

// formulaOf names the registry formula of a component in the loaded rule set (09§3: cases per formula).
func formulaOf(rs engine.RuleSet) recon.FormulaFunc {
	r, ok := rs.(*postgres.RuleSet)
	if !ok {
		return func(engine.CompKey) string { return "" }
	}
	return func(k engine.CompKey) string {
		if c, ok := r.Component(k); ok {
			return c.Formula
		}
		return ""
	}
}
