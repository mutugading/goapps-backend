package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/mutugading/goapps-backend/services/payroll/internal/cliutil"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/etl"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/recon"
)

// Exit codes of recon validate: 0 every hard rule holds, 1 a rule failed (unmapped legacy lines, plaintext PII
// column, or with -dump a DB-vs-dump difference), 2 usage/run error. The JSON is aggregate only (D-72 (3)): no NIK, name, or per-person amount.
func validateCmd(args []string, stdout, stderr io.Writer) int {
	fail := func(err error) int {
		_, _ = fmt.Fprintln(stderr, "validate:", err) //nolint:errcheck // nothing to do if stderr fails
		return exitError
	}
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	company := fs.Int("company", 1, "pyr_company.pco_company_id")
	from := fs.Int("yymm-from", 0, "periode awal yymm (wajib)")
	to := fs.Int("yymm-to", 0, "periode akhir yymm (wajib)")
	out := fs.String("out", "", "berkas laporan JSON agregat (kosong = stdout)")
	dump := fs.String("dump", "", "folder PSV dump (MGT_PAYROLL_MAIN/DETAIL.psv): bandingkan agregat DB dengan dump (T032b)")
	dsn := cliutil.SecretString(fs, "dsn", "DSN login payroll_recon (read-only, anggota payroll_ro)", "PAYROLL_RECON_DSN", "PAYROLL_RO_DSN")
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	if *from == 0 || *to == 0 || *dsn == "" {
		return fail(fmt.Errorf("-yymm-from, -yymm-to dan -dsn (atau PAYROLL_RECON_DSN) wajib"))
	}
	raw, err := sql.Open("pgx", *dsn)
	if err != nil {
		return fail(err)
	}
	defer closeDB(raw)
	rep, err := recon.Validate(context.Background(), raw, recon.Query{CompanyID: int16(*company), //nolint:gosec // flag range checked by FK
		YYMMFrom: *from, YYMMTo: *to})
	if err != nil {
		return fail(err)
	}
	if *dump != "" {
		if err := recon.CompareDump(&rep, etl.PSVDir{Dir: *dump}); err != nil {
			return fail(err)
		}
	}
	if err := writeValidate(rep, *out, stdout); err != nil {
		return fail(err)
	}
	for _, f := range rep.Failures {
		_, _ = fmt.Fprintln(stderr, "GAGAL:", f) //nolint:errcheck // informational
	}
	if c := rep.DumpCompare; c != nil {
		_, _ = fmt.Fprintf(stderr, "dump periode=%d cocok=%d selisih=%d skip_d35_1=%d sintetis_d35_2=%d\n", //nolint:errcheck // informational
			c.DumpPeriods, c.MatchedPeriods, len(c.Diffs), len(c.SkippedD351), len(c.SyntheticD352))
	}
	_, _ = fmt.Fprintf(stderr, "validate company=%d %d..%d periode=%d unmapped=%d plaintext_pii=%d ok=%t\n", //nolint:errcheck // informational
		rep.CompanyID, rep.YYMMFrom, rep.YYMMTo, len(rep.Periods), rep.UnmappedLegacyLines, len(rep.PlaintextPII), rep.OK())
	if !rep.OK() {
		return exitNoParity
	}
	return exitParity
}

func writeValidate(rep recon.Report, path string, stdout io.Writer) error {
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	b = append(b, '\n')
	if path == "" {
		_, err = stdout.Write(b)
		return err
	}
	return os.WriteFile(path, b, 0o600) //nolint:gosec // path is an operator flag
}
