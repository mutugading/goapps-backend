// Package main is the payroll sync/ETL entry point (D-01, D-22). Subcommand: migrate.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib" // driver

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/etl"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "migrate" {
		fmt.Fprintln(os.Stderr, "usage: sync migrate -dir <psv dir> -dsn <payroll_etl dsn> [-company 1] [-valid-from 202001] [-expect-master 133 -expect-template 748]")
		os.Exit(2)
	}
	if err := migrate(os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func migrate(args []string) error {
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	dir := fs.String("dir", "", "folder .psv hasil spool sqlplus (08§1)")
	dsn := fs.String("dsn", os.Getenv("PAYROLL_ETL_DSN"), "DSN role payroll_etl")
	company := fs.Int("company", 1, "pyr_company.pco_company_id")
	validFrom := fs.Int("valid-from", 202001, "pc_valid_from komponen legacy (yymm)")
	expM := fs.Int("expect-master", 0, "jumlah baris master yang diharapkan (133 di produksi)")
	expT := fs.Int("expect-template", 0, "jumlah baris template yang diharapkan (748 di produksi)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dir == "" || *dsn == "" {
		return fmt.Errorf("-dir dan -dsn (atau PAYROLL_ETL_DSN) wajib")
	}
	db, err := sql.Open("pgx", *dsn)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			fmt.Fprintln(os.Stderr, "close:", cerr)
		}
	}()
	reps, err := etl.Run(context.Background(), db, etl.PSVDir{Dir: *dir}, etl.Options{
		CompanyID: int16(*company), ValidFrom: *validFrom, ExpectMaster: *expM, ExpectTemplate: *expT, //nolint:gosec // flag range checked by FK
	})
	for _, r := range reps {
		fmt.Printf("%-32s src=%-5d dst=%-5d sha256=%s\n", r.Table, r.SrcRows, r.DstRows, r.Checksum)
	}
	return err
}
