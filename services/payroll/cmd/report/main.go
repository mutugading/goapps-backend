// Package main is the payroll report CLI (D-62 butir 3). Run by IT on the server as payroll_app with an explicit
// scope; every export is written to pyr_audit_log. Subcommands: recon-komponen.
package main

import (
	"bytes"
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib" // driver

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/report"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/scope"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/audit"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/postgres"
)

const usage = `usage:
  report recon-komponen -yymm 202701 -division SOLO [-company 1] [-allow-open] [-actor cli-report] -out x.xlsx -dsn <payroll_app dsn>`

func main() {
	if len(os.Args) < 2 || os.Args[1] != "recon-komponen" {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	if err := reconKomponen(context.Background(), os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "recon-komponen:", err)
		os.Exit(1)
	}
}

func reconKomponen(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("recon-komponen", flag.ContinueOnError)
	yymm := fs.Int("yymm", 0, "periode yymm (wajib)")
	division := fs.String("division", "", "divisi (wajib)")
	company := fs.Int("company", 1, "pyr_company.pco_company_id")
	allowOpen := fs.Bool("allow-open", false, "izinkan periode belum APPROVED (gladi; tercatat di audit)")
	actor := fs.String("actor", "cli-report", "identitas aktor untuk audit")
	out := fs.String("out", "", "berkas xlsx keluaran (wajib; berisi gaji nyata, jangan di-commit)")
	dsn := fs.String("dsn", os.Getenv("PAYROLL_APP_DSN"), "DSN role payroll_app")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *yymm == 0 || *division == "" || *out == "" || *dsn == "" {
		return fmt.Errorf("-yymm, -division, -out, dan -dsn (atau PAYROLL_APP_DSN) wajib")
	}
	sc, err := scope.New(int16(*company), *division) //nolint:gosec // range checked by FK
	if err != nil {
		return err
	}
	raw, err := sql.Open("pgx", *dsn)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := raw.Close(); cerr != nil {
			fmt.Fprintln(os.Stderr, "close:", cerr)
		}
	}()
	ctx = scope.WithIdentity(ctx, scope.Identity{UserID: *actor, Scopes: []scope.Scope{sc}})
	svc := report.NewService(postgres.NewDBFromSQL(raw), audit.NewWriter())
	// The workbook goes to out.tmp first and is renamed only after the audit row has committed: a failed audit or
	// commit never leaves a file with NIKs and salaries behind without its REPORT_EXPORT row (D-62 butir 3).
	tmp := *out + ".tmp"
	err = svc.Export(ctx, "recon-komponen", report.Query{CompanyID: int16(*company), YYMM: *yymm, //nolint:gosec // as above
		Division: *division, AllowOpen: *allowOpen}, func(d report.RunData) (int, error) {
		var buf bytes.Buffer
		n, err := report.WriteReconKomponen(d, &buf)
		if err != nil {
			return 0, err
		}
		return n, os.WriteFile(tmp, buf.Bytes(), 0o600)
	})
	if err == nil {
		err = os.Rename(tmp, *out)
	}
	if err != nil {
		if rerr := os.Remove(tmp); rerr != nil && !os.IsNotExist(rerr) {
			fmt.Fprintln(os.Stderr, "remove tmp:", rerr)
		}
		return err
	}
	fmt.Printf("out=%s\n", *out)
	return nil
}
