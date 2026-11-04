package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/recon"
	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/scope"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/postgres"
)

// Exit codes of recon diff: 0 parity, 1 parity failed (FORMULA/ROUNDING > 0 or Σ take_home ≠ 0), 2 usage/run error.
const (
	exitParity   = 0
	exitNoParity = 1
	exitError    = 2
)

func diffCmd(args []string, stdout, stderr io.Writer) int {
	fail := func(err error) int {
		_, _ = fmt.Fprintln(stderr, "diff:", err) //nolint:errcheck // nothing to do if stderr fails
		return exitError
	}
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	fs.SetOutput(stderr)
	yymm := fs.Int("yymm", 0, "periode yymm (wajib)")
	div := fs.String("division", "", "divisi (wajib)")
	company := fs.Int("company", 1, "pyr_company.pco_company_id")
	target := fs.String("target", recon.TargetDry, "dry | run:<id> (run ENGINE periode yang sama)")
	ruleSet := fs.Int("rule-set", 1, "rule set untuk dry-run (v1 = 1)")
	out := fs.String("out", "", "berkas laporan JSON (kosong = stdout); memuat NIK & gaji nyata, jangan dikomit")
	dsn := fs.String("dsn", os.Getenv("PAYROLL_RO_DSN"), "DSN role payroll_ro (read-only)")
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	rep, err := runDiff(*yymm, *div, *company, *target, *ruleSet, *dsn)
	if err != nil {
		return fail(err)
	}
	if err := writeReport(rep, *out, stdout); err != nil {
		return fail(err)
	}
	printSummary(stderr, rep)
	if !rep.Summary.Parity {
		return exitNoParity
	}
	return exitParity
}

func runDiff(yymm int, div string, company int, target string, ruleSet int, dsn string) (recon.DiffReport, error) {
	if yymm == 0 || div == "" || dsn == "" {
		return recon.DiffReport{}, fmt.Errorf("-yymm, -division dan -dsn (atau PAYROLL_RO_DSN) wajib")
	}
	if _, err := recon.ParseTarget(target); err != nil {
		return recon.DiffReport{}, err
	}
	sc, err := scope.New(int16(company), div) //nolint:gosec // flag range checked by FK
	if err != nil {
		return recon.DiffReport{}, fmt.Errorf("divisi %q: %w", div, err)
	}
	raw, ctx, err := openRO(dsn, []scope.Scope{sc})
	if err != nil {
		return recon.DiffReport{}, err
	}
	defer closeDB(raw)
	src := recon.DiffSource{DB: postgres.NewDBFromSQL(raw), Rules: ruleFunc, Runner: run.NewRunner()}
	in, err := src.Load(ctx, recon.DiffQuery{CompanyID: int16(company), Division: div, YYMM: yymm, //nolint:gosec // as above
		RuleSetID: ruleSet, Target: target})
	if err != nil {
		return recon.DiffReport{}, err
	}
	return recon.Diff(in), nil
}

func writeReport(rep recon.DiffReport, path string, stdout io.Writer) error {
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	b = append(b, '\n')
	if path == "" {
		_, err = stdout.Write(b)
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func printSummary(w io.Writer, rep recon.DiffReport) {
	s := rep.Summary
	var b strings.Builder
	fmt.Fprintf(&b, "recon %s %d target=%s legacy_run=%d nik legacy=%d target=%d\n",
		rep.Division, rep.YYMM, rep.Target, rep.LegacyRunID, s.NIKLegacy, s.NIKTarget)
	for _, c := range recon.Categories {
		fmt.Fprintf(&b, "  %-13s %d\n", c, s.Categories[c])
	}
	fmt.Fprintf(&b, "  head_mismatch %d · Σ take_home legacy=%s target=%s Δ=%s · warnings %d\n",
		s.HeadMismatch, s.TakeHomeLegacy, s.TakeHomeTarget, s.TakeHomeDelta, len(rep.Warnings))
	fmt.Fprintf(&b, "  parity=%t\n", s.Parity)
	_, _ = io.WriteString(w, b.String()) //nolint:errcheck // summary is informational
}
