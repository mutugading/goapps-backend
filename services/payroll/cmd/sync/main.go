// Package main is the payroll sync/ETL entry point (D-01, D-22). Subcommands: dummy-period, dummy-janbase, migrate, employees, inputs, att, snapshot, att-legacy, salary-legacy, thr-legacy, att-legacy-fail, gl-setup, fx-rate, cost-group.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib" // driver

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/config"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/etl"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/oracle"
)

// commands maps each subcommand to its handler.
var commands = map[string]func([]string) error{
	"migrate":         migrate,
	"employees":       employees,
	"inputs":          inputs,
	"att":             att,
	"snapshot":        snapshot,
	"att-legacy":      attLegacy,
	"salary-legacy":   salaryLegacy,
	"thr-legacy":      thrLegacy,
	"att-legacy-fail": attLegacyFail,
	"prune-excluded":  pruneExcluded,
	"gl-setup":        glSetup,
	"fx-rate":         fxRate,
	"cost-group":      costGroup,
	"dummy-period":    dummyPeriod,
	"dummy-janbase":   dummyJanBase,
}

func main() {
	if len(os.Args) < 2 || commands[os.Args[1]] == nil {
		fmt.Fprintln(os.Stderr, "usage: sync migrate -dir <psv dir> -dsn <payroll_etl dsn> [-company 1] [-valid-from 202001] [-expect-master 133 -expect-template 762]")
		fmt.Fprintln(os.Stderr, "       sync employees -dir <psv dir> -dsn <payroll_etl dsn> [-company 1] [-expect-emp 2860 -expect-compe 117827]  (kunci: PAYROLL_PII_KEY, PAYROLL_PII_HMAC_KEY)")
		fmt.Fprintln(os.Stderr, "       sync att -dsn <payroll_etl dsn> -yymm 202610 [-company 1] [-division SOLO|JAKARTA] [-source rekap|daily|add|sch|all] [-insert-only] [-dry]  (Oracle: PAYROLL_SYNC_ORACLE_HOST/PORT/SERVICE/USER/PASSWORD; D-58)")
		fmt.Fprintln(os.Stderr, "       sync snapshot -dsn <payroll_etl dsn> -yymm 202610 [-company 1] [-division SOLO|JAKARTA]  (bekukan staging → pyr_att_snapshot + PREMI/BERAS_FAIL, 05§6–7; sesudah att)")
		fmt.Fprintln(os.Stderr, "       sync att-legacy -dir <psv dir> -dsn <payroll_etl dsn> [-company 1] [-expect-rekap 68925 -expect-add 870 -expect-sch 3996]")
		fmt.Fprintln(os.Stderr, "       sync thr-legacy -dir <psv dir> -dsn <payroll_etl dsn> [-company 1] [-expect-main 14918 -expect-detail 62246] [-allow-exceptions]  (setelah employees; D-56)")
		fmt.Fprintln(os.Stderr, "       sync att-legacy-fail -dir <psv dir> -dsn <payroll_etl dsn>  (setelah salary-legacy; isi premi_fail/beras_fail dari DETAIL T40/T74, D-33)")
		fmt.Fprintln(os.Stderr, "       sync gl-setup -dir <psv dir> -dsn <payroll_etl dsn> [-company 1] [-expect-rows 1338] [-allow-exceptions]  (MGT_PAYROLL_SETUP → pyr_gl_setup + pyr_gl_group_map; sesudah migrate, 11§4.5)")
		fmt.Fprintln(os.Stderr, "       sync cost-group -dir <psv dir> -dsn <payroll_etl dsn> [-company 1] [-expect-rows 41] [-allow-exceptions]  (FM_DEPARTMENT.DEPT_FIELD_01 → pyr_employee.pe_cost_group per dept; sesudah employees, D-63/T028q)")
		fmt.Fprintln(os.Stderr, "       sync fx-rate -dir <psv dir> -dsn <payroll_etl dsn> [-expect-rows 297] [-allow-exceptions]  (FM_EXCHANGE_RATE IDR→USD BOOK → pyr_fx_rate; spool dengan SET NUMWIDTH 40, D-63)")
		fmt.Fprintln(os.Stderr, "       sync dummy-period -dsn <payroll_etl dsn> -from 202610 -to 202611 -division SOLO|JAKARTA [-company 1] [-workdays-delta N] [-purge -owner-dsn <payroll_owner/migrator dsn>]  (klon snapshot+attd_days+addition tanpa THR sebagai data uji D-64; HANYA DB salinan)")
		fmt.Fprintln(os.Stderr, "       sync dummy-janbase -dsn <payroll_etl dsn> -yymm 202701 [-company 1]  (T901 = Σ TETAP bulan sebelumnya, valid yymm saja; D-64 (4); HANYA DB salinan)")
		fmt.Fprintln(os.Stderr, "       sync inputs -dir <psv dir> -dsn <payroll_etl dsn> [-company 1] [-expect-addition 28464 -expect-abnormal 28550]")
		fmt.Fprintln(os.Stderr, "       sync salary-legacy -dir <psv dir> -dsn <payroll_etl dsn> [-company 1] [-expect-main 91916 -expect-detail 2274418] [-allow-exceptions]")
		fmt.Fprintln(os.Stderr, "       sync prune-excluded -owner-dsn <payroll_owner/migrator dsn> [-company 1] [-apply]  (hapus run LEGACY di luar cakupan D-35.1; default dry-run)")
		os.Exit(2)
	}
	run := commands[os.Args[1]]
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

func thrLegacy(args []string) error {
	fs := flag.NewFlagSet("thr-legacy", flag.ExitOnError)
	dir := fs.String("dir", "", "folder .psv hasil spool sqlplus (08§1): MGT_THR_MAIN + MGT_THR_DETAIL")
	dsn := fs.String("dsn", os.Getenv("PAYROLL_ETL_DSN"), "DSN role payroll_etl (wajib login payroll_etl: pengecualian guard CLOSED memakai session_user)")
	company := fs.Int("company", 1, "pyr_company.pco_company_id")
	expM := fs.Int("expect-main", 0, "jumlah baris MGT_THR_MAIN yang diharapkan (14918)")
	expD := fs.Int("expect-detail", 0, "jumlah baris MGT_THR_DETAIL yang diharapkan (62246)")
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
	reps, err := etl.RunThrLegacy(context.Background(), db, etl.PSVDir{Dir: *dir}, etl.ThrLegacyOptions{
		CompanyID: int16(*company), ExpectMain: *expM, ExpectDetail: *expD, //nolint:gosec // flag range checked by FK
	})
	nExc := 0
	for _, r := range reps {
		fmt.Printf("%-20s src=%-8d dst=%-8d exc=%-4d sha256=%s\n", r.Table, r.SrcRows, r.DstRows, len(r.Exceptions), r.Checksum)
		for _, e := range r.Exceptions {
			fmt.Println("  EXCEPTION", e)
		}
		nExc += len(r.Exceptions)
	}
	if err == nil && nExc > 0 && !*allowExc {
		return fmt.Errorf("%d exception (event ditahan/baris ditolak); tinjau lalu jalankan ulang, atau -allow-exceptions", nExc)
	}
	return err
}

func glSetup(args []string) error {
	fs := flag.NewFlagSet("gl-setup", flag.ExitOnError)
	dir := fs.String("dir", "", "folder .psv hasil spool sqlplus (08§1): MGT_PAYROLL_SETUP")
	dsn := fs.String("dsn", os.Getenv("PAYROLL_ETL_DSN"), "DSN role payroll_etl")
	company := fs.Int("company", 1, "pyr_company.pco_company_id")
	expRows := fs.Int("expect-rows", 0, "jumlah baris MGT_PAYROLL_SETUP yang diharapkan (1338)")
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
	r, err := etl.RunGlSetup(context.Background(), db, etl.PSVDir{Dir: *dir}, etl.GlSetupOptions{
		CompanyID: int16(*company), ExpectRows: *expRows, //nolint:gosec // flag range checked by FK
	})
	if err != nil {
		return err
	}
	fmt.Printf("%-20s src=%-8d dst=%-8d exc=%-4d sha256=%s\n", r.Table, r.SrcRows, r.DstRows, len(r.Exceptions), r.Checksum)
	for _, w := range r.Warnings {
		fmt.Println("  WARNING", w)
	}
	for _, e := range r.Exceptions {
		fmt.Println("  EXCEPTION", e)
	}
	if len(r.Exceptions) > 0 && !*allowExc {
		return fmt.Errorf("%d exception (baris ditahan; laporkan ke Finance lalu jalankan ulang, atau -allow-exceptions)", len(r.Exceptions))
	}
	return nil
}

func fxRate(args []string) error {
	fs := flag.NewFlagSet("fx-rate", flag.ExitOnError)
	dir := fs.String("dir", "", "folder .psv hasil spool sqlplus (08§1): FM_EXCHANGE_RATE")
	dsn := fs.String("dsn", os.Getenv("PAYROLL_ETL_DSN"), "DSN role payroll_etl")
	expRows := fs.Int("expect-rows", 0, "jumlah baris IDR→USD BOOK yang diharapkan")
	allowExc := fs.Bool("allow-exceptions", false, "exit 0 walau ada exception")
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
	r, err := etl.RunFxRate(context.Background(), db, etl.PSVDir{Dir: *dir}, etl.FxRateOptions{ExpectRows: *expRows})
	if err != nil {
		return err
	}
	fmt.Printf("%-20s src=%-8d dst=%-8d exc=%-4d sha256=%s\n", r.Table, r.SrcRows, r.DstRows, len(r.Exceptions), r.Checksum)
	for _, w := range r.Warnings {
		fmt.Println("  WARNING", w)
	}
	for _, e := range r.Exceptions {
		fmt.Println("  EXCEPTION", e)
	}
	if len(r.Exceptions) > 0 && !*allowExc {
		return fmt.Errorf("%d exception (baris ditahan; atau -allow-exceptions)", len(r.Exceptions))
	}
	return nil
}

func costGroup(args []string) error {
	fs := flag.NewFlagSet("cost-group", flag.ExitOnError)
	dir := fs.String("dir", "", "folder .psv hasil spool sqlplus (08§1): FM_DEPARTMENT")
	dsn := fs.String("dsn", os.Getenv("PAYROLL_ETL_DSN"), "DSN role payroll_etl")
	company := fs.Int("company", 1, "company id")
	expRows := fs.Int("expect-rows", 0, "jumlah baris FM_DEPARTMENT ber-DEPT_FIELD_01 yang diharapkan")
	allowExc := fs.Bool("allow-exceptions", false, "exit 0 walau ada exception")
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
	r, err := etl.RunCostGroup(context.Background(), db, etl.PSVDir{Dir: *dir}, etl.CostGroupOptions{CompanyID: int16(*company), ExpectRows: *expRows}) //nolint:gosec // small id
	if err != nil {
		return err
	}
	fmt.Printf("%-20s src=%-8d updated=%-6d exc=%-4d sha256=%s\n", r.Table, r.SrcRows, r.DstRows, len(r.Exceptions), r.Checksum)
	for _, w := range r.Warnings {
		fmt.Println("  WARNING", w)
	}
	for _, e := range r.Exceptions {
		fmt.Println("  EXCEPTION", e)
	}
	if len(r.Exceptions) > 0 && !*allowExc {
		return fmt.Errorf("%d exception (baris ditahan; atau -allow-exceptions)", len(r.Exceptions))
	}
	return nil
}

func attLegacyFail(args []string) error {
	fs := flag.NewFlagSet("att-legacy-fail", flag.ExitOnError)
	dir := fs.String("dir", "", "folder .psv hasil spool sqlplus (08§1): MGT_PAYROLL_DETAIL + MGT_PAYROLL_COMPNT_EMP")
	dsn := fs.String("dsn", os.Getenv("PAYROLL_ETL_DSN"), "DSN role payroll_etl")
	open := fs.String("open-periods", "", "yymm dipisah koma yang masih OPEN saat dump (D-46: T40 tanpa baris DETAIL ⇒ PF=1; butuh MGT_PAYROLL_MAIN). Kosong = hanya D-33")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dir == "" || *dsn == "" {
		return fmt.Errorf("-dir dan -dsn (atau PAYROLL_ETL_DSN) wajib")
	}
	var o etl.AttLegacyFailOptions
	if *open != "" {
		o.OpenPeriods = strings.Split(*open, ",")
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
	r, err := etl.RunAttLegacyFail(context.Background(), db, etl.PSVDir{Dir: *dir}, o)
	fmt.Printf("T40 detail=%d premi_fail set=%d (gagal=%d, tanpa baris DETAIL=%d) | T74 detail=%d beras_fail set=%d | baris snapshot diubah=%d\n",
		r.PremiDetail, r.PremiSet, r.PremiFailed, r.PremiNoRow, r.BerasDetail, r.BerasSet, r.Updated)
	fmt.Printf("residu BF (NULL)=%d, tanpa baris REKAP=%d\n", len(r.BerasResidue), len(r.NoSnapshotRow))
	periods := make([]string, 0, len(r.PremiNoRowBy))
	for p := range r.PremiNoRowBy {
		periods = append(periods, p)
	}
	sort.Strings(periods)
	for _, p := range periods { // D-46: hanya periode yang OPEN saat dump boleh muncul di sini
		fmt.Printf("  PF=1 tanpa baris DETAIL %s: %d\n", p, r.PremiNoRowBy[p])
	}
	for _, e := range r.BerasResidue {
		fmt.Println("  RESIDU", e)
	}
	for _, e := range r.NoSnapshotRow {
		fmt.Println("  TANPA-REKAP", e)
	}
	return err
}

func pruneExcluded(args []string) error {
	fs := flag.NewFlagSet("prune-excluded", flag.ExitOnError)
	dsn := fs.String("owner-dsn", os.Getenv("PAYROLL_OWNER_DSN"), "DSN pemilik tabel (payroll_migrator); payroll_etl tidak punya DELETE")
	company := fs.Int("company", 1, "pyr_company.pco_company_id")
	apply := fs.Bool("apply", false, "hapus sungguhan (default: dry-run, hanya melaporkan)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dsn == "" {
		return fmt.Errorf("-owner-dsn (atau PAYROLL_OWNER_DSN) wajib")
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
	runs, err := etl.PruneExcluded(context.Background(), db, etl.PruneOptions{CompanyID: int16(*company), Apply: *apply}) //nolint:gosec // flag range checked by FK
	for _, r := range runs {
		fmt.Printf("  run %-6d %s/%-8s head=%-5d detail=%d\n", r.RunID, r.YYMM, r.Division, r.Heads, r.Lines)
	}
	mode := "DRY-RUN (tambahkan -apply untuk menghapus)"
	if *apply {
		mode = "DIHAPUS"
	}
	if err == nil {
		fmt.Printf("%d run LEGACY di luar cakupan: %s\n", len(runs), mode)
	}
	return err
}

// oracleSyncFromEnv reads the HRIS Oracle account (D-58 (3)); the sync binary needs no JWT/PII config.
func oracleSyncFromEnv() config.OracleSyncConfig {
	port, err := strconv.Atoi(os.Getenv("PAYROLL_SYNC_ORACLE_PORT"))
	if err != nil {
		port = 1521
	}
	return config.OracleSyncConfig{Host: os.Getenv("PAYROLL_SYNC_ORACLE_HOST"), Port: port,
		Service: os.Getenv("PAYROLL_SYNC_ORACLE_SERVICE"), User: os.Getenv("PAYROLL_SYNC_ORACLE_USER"),
		Password: os.Getenv("PAYROLL_SYNC_ORACLE_PASSWORD")}
}

// att pulls attendance from HRIS Oracle into payroll_stg (05§4, D-58). Exit non-zero on any failure.
func att(args []string) error {
	fs := flag.NewFlagSet("att", flag.ExitOnError)
	dsn := fs.String("dsn", os.Getenv("PAYROLL_ETL_DSN"), "DSN role payroll_etl")
	company := fs.Int("company", 1, "pyr_company.pco_company_id")
	yymm := fs.Int("yymm", 0, "periode YYYYMM (wajib)")
	division := fs.String("division", "", "SOLO atau JAKARTA (kosong = keduanya)")
	source := fs.String("source", "all", "rekap|daily|add|sch|all")
	insertOnly := fs.Bool("insert-only", false, "05§4.1: jangan update/hapus baris staging yang sudah ada (hanya parallel-run)")
	dry := fs.Bool("dry", false, "tarik dan hitung perubahan, tanpa menulis")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dsn == "" || *yymm == 0 {
		return fmt.Errorf("-dsn (atau PAYROLL_ETL_DSN) dan -yymm wajib")
	}
	if *company < 1 || *company > math.MaxInt16 {
		return fmt.Errorf("-company %d di luar 1..%d", *company, math.MaxInt16)
	}
	client, err := oracle.NewClient(oracleSyncFromEnv())
	if err != nil {
		return err
	}
	defer func() {
		if cerr := client.Close(); cerr != nil {
			fmt.Fprintln(os.Stderr, "close oracle:", cerr)
		}
	}()
	db, err := sql.Open("pgx", *dsn)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			fmt.Fprintln(os.Stderr, "close:", cerr)
		}
	}()
	reps, err := etl.RunAttSync(context.Background(), db, oracle.NewAttSource(client), etl.AttSyncQuery{
		Company: int16(*company), YYMM: *yymm, Division: *division, Source: *source, InsertOnly: *insertOnly, Dry: *dry, //nolint:gosec // -company checked to 1..MaxInt16 above
	})
	for _, r := range reps { // counts and NIK only (I25)
		fmt.Printf("%-6s %-8s %d batch=%-5d in=%-6d upserted=%-6d deleted=%-5d rejected=%-4d %s checksum=%s\n",
			r.Source, r.Division, r.YYMM, r.BatchID, r.RowsIn, r.Upserted, r.Deleted, r.Rejected, r.Status, r.Checksum)
		if len(r.RejectedNIKs) > 0 {
			fmt.Println("  DITOLAK (V6) NIK:", strings.Join(r.RejectedNIKs, ","))
		}
	}
	return err
}

// snapshot freezes the current staging into a new attendance snapshot per division (05§6–7, D-58 (4)(5)).
func snapshot(args []string) error {
	fs := flag.NewFlagSet("snapshot", flag.ExitOnError)
	dsn := fs.String("dsn", os.Getenv("PAYROLL_ETL_DSN"), "DSN role payroll_etl")
	company := fs.Int("company", 1, "pyr_company.pco_company_id")
	yymm := fs.Int("yymm", 0, "periode YYYYMM (wajib)")
	division := fs.String("division", "", "SOLO atau JAKARTA (kosong = keduanya)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dsn == "" || *yymm == 0 {
		return fmt.Errorf("-dsn (atau PAYROLL_ETL_DSN) dan -yymm wajib")
	}
	if *company < 1 || *company > math.MaxInt16 {
		return fmt.Errorf("-company %d di luar 1..%d", *company, math.MaxInt16)
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
	reps, err := etl.RunAttSnapshot(context.Background(), db, etl.AttSnapshotQuery{
		Company: int16(*company), YYMM: *yymm, Division: *division, //nolint:gosec // -company checked to 1..MaxInt16 above
	})
	for _, r := range reps { // counts only (I25)
		if r.Unchanged {
			fmt.Printf("%-8s %d unchanged: staging & batch = snapshot %d (tidak dibuat baru) checksum=%s\n", r.Division, r.YYMM, r.SnapshotID, r.Checksum)
			continue
		}
		fmt.Printf("%-8s %d snapshot=%-6d batches=%v rekap=%-5d add=%-4d sch=%-5d premi_fail_nik=%-4d beras_fail_nik=%-4d hari=%d checksum=%s\n",
			r.Division, r.YYMM, r.SnapshotID, r.BatchIDs, r.Rekap, r.Add, r.Sch, r.PremiFailNIKs, r.BerasFailNIKs, r.PeriodDayCount, r.Checksum)
	}
	return err
}

// dummyPeriod clones a real period into a synthetic one (D-64), or purges it with -purge (owner DSN, user decision batch T029a).
func dummyPeriod(args []string) error {
	fs := flag.NewFlagSet("dummy-period", flag.ExitOnError)
	dsn := fs.String("dsn", os.Getenv("PAYROLL_ETL_DSN"), "DSN role payroll_etl (klon)")
	ownerDSN := fs.String("owner-dsn", os.Getenv("PAYROLL_OWNER_DSN"), "DSN pemilik tabel (payroll_migrator); hanya untuk -purge")
	company := fs.Int("company", 1, "pyr_company.pco_company_id")
	from := fs.Int("from", 0, "periode sumber YYYYMM (klon; mis. 202610)")
	to := fs.Int("to", 0, "periode dummy YYYYMM (wajib)")
	division := fs.String("division", "", "SOLO atau JAKARTA (wajib)")
	delta := fs.Int("workdays-delta", 0, "tambahan pyr_attd_days dari periode sumber (mis. -1 untuk bulan 30 hari)")
	purge := fs.Bool("purge", false, "hapus baris dummy periode -to (butuh -owner-dsn)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *company < 1 || *company > math.MaxInt16 {
		return fmt.Errorf("-company %d di luar 1..%d", *company, math.MaxInt16)
	}
	if *to == 0 || *division == "" {
		return fmt.Errorf("-to dan -division wajib")
	}
	var rep etl.DummyReport
	var err error
	if *purge {
		if *ownerDSN == "" {
			return fmt.Errorf("-purge butuh -owner-dsn (atau PAYROLL_OWNER_DSN): payroll_etl tidak punya DELETE")
		}
		db, oerr := sql.Open("pgx", *ownerDSN)
		if oerr != nil {
			return oerr
		}
		defer closeDB(db)
		rep, err = etl.RunDummyPurge(context.Background(), db, int16(*company), *to, *division) //nolint:gosec // -company checked to 1..MaxInt16 above
		if err == nil {
			fmt.Printf("%-8s %d purge: snapshot=%d rows=%d addition=%d attd_days=%d\n", rep.Division, rep.YYMM, rep.SnapshotID, rep.Rows, rep.Additions, rep.AttdDays)
		}
		return err
	}
	if *dsn == "" || *from == 0 {
		return fmt.Errorf("-dsn (atau PAYROLL_ETL_DSN) dan -from wajib")
	}
	db, err := sql.Open("pgx", *dsn)
	if err != nil {
		return err
	}
	defer closeDB(db)
	rep, err = etl.RunDummyPeriod(context.Background(), db, etl.DummyPeriodQuery{
		Company: int16(*company), From: *from, To: *to, Division: *division, WorkdaysDelta: *delta, //nolint:gosec // -company checked to 1..MaxInt16 above
	})
	if err == nil { // counts only (I25)
		fmt.Printf("%-8s %d dari %d snapshot=%d rows=%d addition=%d attd_days=%d unchanged=%t\n", rep.Division, rep.YYMM, *from, rep.SnapshotID, rep.Rows, rep.Additions, rep.AttdDays, rep.Unchanged)
	}
	return err
}

// dummyJanBase seeds T901 for a January from the TETAP of the previous month (D-64 (4)).
func dummyJanBase(args []string) error {
	fs := flag.NewFlagSet("dummy-janbase", flag.ExitOnError)
	dsn := fs.String("dsn", os.Getenv("PAYROLL_ETL_DSN"), "DSN role payroll_etl")
	company := fs.Int("company", 1, "pyr_company.pco_company_id")
	yymm := fs.Int("yymm", 0, "periode Januari YYYYMM (wajib; mis. 202701)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dsn == "" || *yymm == 0 {
		return fmt.Errorf("-dsn (atau PAYROLL_ETL_DSN) dan -yymm wajib")
	}
	if *company < 1 || *company > math.MaxInt16 {
		return fmt.Errorf("-company %d di luar 1..%d", *company, math.MaxInt16)
	}
	db, err := sql.Open("pgx", *dsn)
	if err != nil {
		return err
	}
	defer closeDB(db)
	rep, err := etl.RunDummyJanBase(context.Background(), db, int16(*company), *yymm) //nolint:gosec // -company checked to 1..MaxInt16 above
	if err == nil {
		fmt.Printf("T901 %d dari TETAP %d: %d baris\n", rep.YYMM, rep.Source, rep.Written)
	}
	return err
}

func closeDB(db *sql.DB) {
	if cerr := db.Close(); cerr != nil {
		fmt.Fprintln(os.Stderr, "close:", cerr)
	}
}
