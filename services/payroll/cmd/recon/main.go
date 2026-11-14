// Package main is the payroll reconciliation entry point (D-01, D-41). Subcommands: extract-golden, diff.
// It reads the LEGACY run in Postgres (role payroll_ro); there is no Oracle connection.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
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
  recon extract-golden -yymm 202608 [-per-formula 20] [-company 1] [-divisions SOLO,JAKARTA] [-rule-set 1] [-out tests/golden/out] [-rules-out rules.json] -dsn <payroll_ro dsn>
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
	rulesOut := fs.String("rules-out", "", "berkas GOLDEN_RULES (rule set + tabel pajak periode itu) untuk TestGoldenReal")
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
	for _, e := range recon.Exclusions {
		fmt.Printf("  dikecualikan %-17s %d (D-47/D-50)\n", e, rep.Excluded[e])
	}
	if *rulesOut != "" {
		if err := writeRules(ctx, raw, *rulesOut, int16(*company), names, *yymm, *ruleSet); err != nil { //nolint:gosec // as above
			return err
		}
		fmt.Printf("rules=%s\n", *rulesOut)
	}
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

// writeRules writes the GOLDEN_RULES document: constants and tax tables of the period. TestGoldenReal applies one
// rule document to every case, so the divisions must resolve to identical rules or the export fails.
func writeRules(ctx context.Context, raw *sql.DB, path string, company int16, divs []string, yymm, ruleSet int) error {
	var first []byte
	for _, d := range divs {
		var doc []byte
		err := postgres.NewDBFromSQL(raw).ScopedTx(ctx, func(tx *sql.Tx) error {
			rr, err := ruleFunc(ctx, tx, run.Query{CompanyID: company, Division: d, YYMM: yymm}, ruleSet)
			if err != nil {
				return err
			}
			rs, ok := rr.Set.(*postgres.RuleSet)
			if !ok {
				return fmt.Errorf("rule set bukan *postgres.RuleSet")
			}
			consts := map[string]string{}
			for k, v := range rs.Constants() {
				consts[k] = v.String()
			}
			doc, err = json.MarshalIndent(map[string]any{
				"rules": map[string]any{"legacy": rs.LegacyCompat(), "constants": consts},
				"tax":   rr.Tax,
			}, "", "  ")
			return err
		})
		if err != nil {
			return fmt.Errorf("rules %s: %w", d, err)
		}
		if first != nil && string(first) != string(doc) {
			return fmt.Errorf("rules divisi %s berbeda dari %s: ekspor per divisi terpisah", d, divs[0])
		}
		first = doc
	}
	return os.WriteFile(path, append(first, '\n'), 0o600)
}
