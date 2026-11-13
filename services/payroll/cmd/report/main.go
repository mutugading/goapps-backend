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
  report recon-komponen -yymm 202701 -division SOLO [-company 1] [-allow-open] [-actor cli-report] -out x.xlsx -dsn <payroll_app dsn>
  report slip -yymm 202701 -division SOLO [-nik N] [-nopassword] [-company 1] [-allow-open] [-actor cli-report] -out dir -dsn <payroll_app dsn>`

func main() {
	cmds := map[string]func(context.Context, []string) error{"recon-komponen": reconKomponen, "slip": slip}
	var fn func(context.Context, []string) error
	if len(os.Args) >= 2 {
		fn = cmds[os.Args[1]]
	}
	if fn == nil {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	if err := fn(context.Background(), os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, os.Args[1]+":", err)
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

// periodName renders 202701 as "Januari 2027".
func periodName(yymm int) string {
	months := []string{"Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}
	m := yymm % 100
	if m < 1 || m > 12 {
		return fmt.Sprint(yymm)
	}
	return fmt.Sprintf("%s %d", months[m-1], 2000+yymm/100)
}

func slip(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("slip", flag.ContinueOnError)
	yymm := fs.Int("yymm", 0, "periode yymm (wajib)")
	division := fs.String("division", "", "divisi (wajib)")
	company := fs.Int("company", 1, "pyr_company.pco_company_id")
	nik := fs.String("nik", "", "satu NIK saja (default: semua di divisi)")
	noPassword := fs.Bool("nopassword", false, "cetak finance: PDF tanpa password (tercatat di audit)")
	allowOpen := fs.Bool("allow-open", false, "izinkan periode belum APPROVED (gladi; tercatat di audit)")
	actor := fs.String("actor", "cli-report", "identitas aktor untuk audit")
	out := fs.String("out", "", "folder keluaran (wajib; berisi slip nyata, jangan di-commit)")
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
	if err := os.MkdirAll(*out, 0o700); err != nil {
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
	res, err := svc.ExportSlips(ctx, report.Query{CompanyID: int16(*company), YYMM: *yymm, //nolint:gosec // as above
		Division: *division, AllowOpen: *allowOpen},
		report.SlipOptions{NIK: *nik, NoPassword: *noPassword, OutDir: *out, PeriodName: periodName(*yymm)})
	if err != nil {
		return err
	}
	fmt.Printf("slip ditulis=%d dilewati_tanpa_tanggal_lahir=%d out=%s\n", len(res.Written), len(res.Skipped), *out)
	for _, n := range res.Skipped {
		fmt.Printf("DILEWATI %s (pe_birth_date kosong; tindak lanjut HR)\n", n)
	}
	return nil
}
