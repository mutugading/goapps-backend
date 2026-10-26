// Package main is the payroll sync/ETL entry point (D-01, D-22). Subcommands: migrate, employees, inputs, att-legacy, salary-legacy, att-legacy-fail.
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
	if len(os.Args) < 2 || (os.Args[1] != "migrate" && os.Args[1] != "employees" && os.Args[1] != "inputs" && os.Args[1] != "att-legacy" && os.Args[1] != "salary-legacy" && os.Args[1] != "att-legacy-fail") {
		fmt.Fprintln(os.Stderr, "usage: sync migrate -dir <psv dir> -dsn <payroll_etl dsn> [-company 1] [-valid-from 202001] [-expect-master 133 -expect-template 762]")
		fmt.Fprintln(os.Stderr, "       sync employees -dir <psv dir> -dsn <payroll_etl dsn> [-company 1] [-expect-emp 2860 -expect-compe 117827]  (kunci: PAYROLL_PII_KEY, PAYROLL_PII_HMAC_KEY)")
		fmt.Fprintln(os.Stderr, "       sync att-legacy -dir <psv dir> -dsn <payroll_etl dsn> [-company 1] [-expect-rekap 68925 -expect-add 870 -expect-sch 3996]")
		fmt.Fprintln(os.Stderr, "       sync att-legacy-fail -dir <psv dir> -dsn <payroll_etl dsn>  (setelah salary-legacy; isi premi_fail/beras_fail dari DETAIL T40/T74, D-33)")
		fmt.Fprintln(os.Stderr, "       sync inputs -dir <psv dir> -dsn <payroll_etl dsn> [-company 1] [-expect-addition 28464 -expect-abnormal 28550]")
		fmt.Fprintln(os.Stderr, "       sync salary-legacy -dir <psv dir> -dsn <payroll_etl dsn> [-company 1] [-expect-main 91916 -expect-detail 2274418] [-allow-exceptions]")
		os.Exit(2)
	}
	run := migrate
	switch os.Args[1] {
	case "employees":
		run = employees
	case "inputs":
		run = inputs
	case "att-legacy":
		run = attLegacy
	case "salary-legacy":
		run = salaryLegacy
	case "att-legacy-fail":
		run = attLegacyFail
	}
	if err := run(os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, os.Args[1]+":", err)
		os.Exit(1)
	}
}

func employees(args []string) error {
	fs := flag.NewFlagSet("employees", flag.ExitOnError)
	dir := fs.String("dir", "", "folder .psv hasil spool sqlplus (08§1)")
	dsn := fs.String("dsn", os.Getenv("PAYROLL_ETL_DSN"), "DSN role payroll_etl")
	company := fs.Int("company", 1, "pyr_company.pco_company_id")
	expE := fs.Int("expect-emp", 0, "jumlah baris MGT_PAYROLL_EMP yang diharapkan (2860)")
	expC := fs.Int("expect-compe", 0, "jumlah baris MGT_PAYROLL_COMPNT_EMP yang diharapkan (117827)")
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
	reps, err := etl.RunEmployees(context.Background(), db, etl.PSVDir{Dir: *dir}, etl.EmployeeOptions{
		CompanyID: int16(*company), //nolint:gosec // flag range checked by FK
		EncKey:    os.Getenv("PAYROLL_PII_KEY"), HMACKey: os.Getenv("PAYROLL_PII_HMAC_KEY"), ExpectEmp: *expE, ExpectCompE: *expC,
	})
	for _, r := range reps {
		fmt.Printf("%-40s src=%-6d dst=%-6d exc=%-4d warn=%-4d sha256=%s\n", r.Table, r.SrcRows, r.DstRows, len(r.Exceptions), len(r.Warnings), r.Checksum)
		for _, e := range r.Exceptions {
			fmt.Println("  EXCEPTION", e)
		}
		for _, w := range r.Warnings {
			fmt.Println("  WARN", w)
		}
	}
	return err
}

func migrate(args []string) error {
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	dir := fs.String("dir", "", "folder .psv hasil spool sqlplus (08§1)")
	dsn := fs.String("dsn", os.Getenv("PAYROLL_ETL_DSN"), "DSN role payroll_etl")
	company := fs.Int("company", 1, "pyr_company.pco_company_id")
	validFrom := fs.Int("valid-from", 202001, "pc_valid_from komponen legacy (yymm)")
	expM := fs.Int("expect-master", 0, "jumlah baris master yang diharapkan (133 di produksi)")
	expT := fs.Int("expect-template", 0, "jumlah baris template yang diharapkan (762 di produksi, D-23)")
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

func inputs(args []string) error {
	fs := flag.NewFlagSet("inputs", flag.ExitOnError)
	dir := fs.String("dir", "", "folder .psv hasil spool sqlplus (08§1)")
	dsn := fs.String("dsn", os.Getenv("PAYROLL_ETL_DSN"), "DSN role payroll_etl")
	company := fs.Int("company", 1, "pyr_company.pco_company_id")
	expA := fs.Int("expect-addition", 0, "jumlah baris MGT_PAYROLL_ADDITION yang diharapkan (28464)")
	expP := fs.Int("expect-abnormal", 0, "jumlah baris MGT_PAYROLL_ABNORMAL_PREMI yang diharapkan (28550)")
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
	reps, err := etl.RunInputs(context.Background(), db, etl.PSVDir{Dir: *dir}, etl.InputOptions{
		CompanyID: int16(*company), ExpectAddition: *expA, ExpectAbnormal: *expP, //nolint:gosec // flag range checked by FK
	})
	for _, r := range reps {
		fmt.Printf("%-32s src=%-6d dst=%-6d exc=%-4d sha256=%s\n", r.Table, r.SrcRows, r.DstRows, len(r.Exceptions), r.Checksum)
		for _, e := range r.Exceptions {
			fmt.Println("  EXCEPTION", e)
		}
	}
	return err
}

func attLegacy(args []string) error {
	fs := flag.NewFlagSet("att-legacy", flag.ExitOnError)
	dir := fs.String("dir", "", "folder .psv hasil spool sqlplus (08§1)")
	dsn := fs.String("dsn", os.Getenv("PAYROLL_ETL_DSN"), "DSN role payroll_etl")
	company := fs.Int("company", 1, "pyr_company.pco_company_id")
	expR := fs.Int("expect-rekap", 0, "jumlah baris MGT_PAYROLL_ATT_REKAP yang diharapkan (68925)")
	expA := fs.Int("expect-add", 0, "jumlah baris MGT_PAYROLL_ATT_REKAP_ADD yang diharapkan (870)")
	expS := fs.Int("expect-sch", 0, "jumlah baris MGT_PAYROLL_SCH_REKAP yang diharapkan (3996)")
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
	reps, err := etl.RunAttLegacy(context.Background(), db, etl.PSVDir{Dir: *dir}, etl.AttLegacyOptions{
		CompanyID: int16(*company), ExpectRekap: *expR, ExpectAdd: *expA, ExpectS: *expS, //nolint:gosec // flag range checked by FK
	})
	for _, r := range reps {
		fmt.Printf("%-32s src=%-6d dst=%-6d exc=%-4d sha256=%s\n", r.Table, r.SrcRows, r.DstRows, len(r.Exceptions), r.Checksum)
		for _, e := range r.Exceptions {
			fmt.Println("  EXCEPTION", e)
		}
	}
	return err
}

func salaryLegacy(args []string) error {
	fs := flag.NewFlagSet("salary-legacy", flag.ExitOnError)
	dir := fs.String("dir", "", "folder .psv hasil spool sqlplus (08§1)")
	dsn := fs.String("dsn", os.Getenv("PAYROLL_ETL_DSN"), "DSN role payroll_etl")
	company := fs.Int("company", 1, "pyr_company.pco_company_id")
	expM := fs.Int("expect-main", 0, "jumlah baris MGT_PAYROLL_MAIN yang diharapkan (91916)")
	expD := fs.Int("expect-detail", 0, "jumlah baris MGT_PAYROLL_DETAIL yang diharapkan (2274418)")
	allowExc := fs.Bool("allow-exceptions", false, "exit 0 walau ada exception (default: exit non-nol bila ada exception)")
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
	reps, err := etl.RunSalaryLegacy(context.Background(), db, etl.PSVDir{Dir: *dir}, etl.SalaryLegacyOptions{
		CompanyID: int16(*company), ExpectMain: *expM, ExpectDetail: *expD, //nolint:gosec // flag range checked by FK
	})
	nExc := 0
	for _, r := range reps {
		fmt.Printf("%-32s src=%-8d dst=%-8d exc=%-4d sha256=%s\n", r.Table, r.SrcRows, r.DstRows, len(r.Exceptions), r.Checksum)
		for _, e := range r.Skipped {
			fmt.Println(" ", e)
		}
		for _, w := range r.Warnings {
			fmt.Println("  WARNING", w)
		}
		for _, e := range r.Exceptions {
			fmt.Println("  EXCEPTION", e)
		}
		nExc += len(r.Exceptions)
	}
	if err == nil && nExc > 0 && !*allowExc {
		return fmt.Errorf("%d exception (periode ditahan/baris ditolak); tinjau lalu jalankan ulang, atau -allow-exceptions", nExc)
	}
	return err
}

func attLegacyFail(args []string) error {
	fs := flag.NewFlagSet("att-legacy-fail", flag.ExitOnError)
	dir := fs.String("dir", "", "folder .psv hasil spool sqlplus (08§1): MGT_PAYROLL_DETAIL + MGT_PAYROLL_COMPNT_EMP")
	dsn := fs.String("dsn", os.Getenv("PAYROLL_ETL_DSN"), "DSN role payroll_etl")
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
	r, err := etl.RunAttLegacyFail(context.Background(), db, etl.PSVDir{Dir: *dir})
	fmt.Printf("T40 detail=%d premi_fail set=%d (gagal=%d) | T74 detail=%d beras_fail set=%d | baris snapshot diubah=%d\n",
		r.PremiDetail, r.PremiSet, r.PremiFailed, r.BerasDetail, r.BerasSet, r.Updated)
	fmt.Printf("residu BF (NULL)=%d, tanpa baris REKAP=%d\n", len(r.BerasResidue), len(r.NoSnapshotRow))
	for _, e := range r.BerasResidue {
		fmt.Println("  RESIDU", e)
	}
	for _, e := range r.NoSnapshotRow {
		fmt.Println("  TANPA-REKAP", e)
	}
	return err
}
