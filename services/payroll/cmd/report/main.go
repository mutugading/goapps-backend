// Package main is the payroll report CLI (D-62 butir 3). Run by IT on the server as payroll_app with an explicit
// scope; every export is written to pyr_audit_log. Subcommands: recon-komponen, reco, bank, slip, gl.
package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // driver

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/report"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/scope"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/audit"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/config"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/postgres"
)

const usage = `usage:
  report recon-komponen -yymm 202701 -division SOLO [-company 1] [-allow-open] [-actor cli-report] -out x.xlsx -dsn <payroll_app dsn>
  report reco -yymm 202701 -division SOLO [-company 1] [-allow-open] [-actor cli-report] -out x.xlsx -dsn <payroll_app dsn> [-pii-key K | PAYROLL_PII_KEY]
  report bank -yymm 202701 -division SOLO -transfer-date 2027-01-26 [-bank MANDIRI] [-company 1] [-allow-open] [-actor cli-report] -out x.xlsx -dsn <payroll_app dsn> [-pii-key K | PAYROLL_PII_KEY]  (teks surat: PAYROLL_REPORT_*)
  report gl -yymm 202701 -division SOLO [-company 1] [-allow-open] [-actor cli-report] -out x.xlsx -dsn <payroll_app dsn>  (jurnal GL sheet upload, F-2)
  report slip -yymm 202701 -division SOLO [-nik N] [-nopassword] [-company 1] [-allow-open] [-actor cli-report] -out dir -dsn <payroll_app dsn> [-pii-key K | PAYROLL_PII_KEY]  (logo/teks: PAYROLL_REPORT_LOGO, PAYROLL_SLIP_PERHATIAN, PAYROLL_SLIP_CONTACT_IT)`

func main() {
	cmds := map[string]func(context.Context, []string) error{"recon-komponen": reconKomponen, "reco": reco, "bank": bank, "slip": slip, "gl": gl}
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
	dsn := fs.String("dsn", "", "DSN role payroll_app (atau env PAYROLL_APP_DSN)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	envDefault(dsn, "PAYROLL_APP_DSN")
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

// reco writes the F-2 reconciliation workbook (sheet detail, pra-cek, Ringkasan). The bank account is decrypted with
// the PII key, so the output is as sensitive as the DB itself.
func reco(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("reco", flag.ContinueOnError)
	yymm := fs.Int("yymm", 0, "periode yymm (wajib)")
	division := fs.String("division", "", "divisi (wajib)")
	company := fs.Int("company", 1, "pyr_company.pco_company_id")
	allowOpen := fs.Bool("allow-open", false, "izinkan periode belum APPROVED (gladi; tercatat di audit)")
	actor := fs.String("actor", "cli-report", "identitas aktor untuk audit")
	out := fs.String("out", "", "berkas xlsx keluaran (wajib; berisi gaji & rekening nyata, jangan di-commit)")
	dsn := fs.String("dsn", "", "DSN role payroll_app (atau env PAYROLL_APP_DSN)")
	key := fs.String("pii-key", "", "kunci PII (dekripsi rekening) (atau env PAYROLL_PII_KEY; utamakan env)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	envDefault(dsn, "PAYROLL_APP_DSN")
	envDefault(key, "PAYROLL_PII_KEY")
	if *yymm == 0 || *division == "" || *out == "" || *dsn == "" || *key == "" {
		return fmt.Errorf("-yymm, -division, -out, -dsn (atau PAYROLL_APP_DSN), dan -pii-key (atau PAYROLL_PII_KEY) wajib")
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
	tmp := *out + ".tmp" // renamed only after the audit row committed, as in recon-komponen
	var pre int
	err = svc.ExportReco(ctx, report.Query{CompanyID: int16(*company), YYMM: *yymm, //nolint:gosec // as above
		Division: *division, AllowOpen: *allowOpen}, *key, func(d report.RecoData) (int, error) {
		var buf bytes.Buffer
		n, err := report.WriteReco(d, &buf)
		if err != nil {
			return 0, err
		}
		pre = len(report.PreChecks(d.Rows))
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
	fmt.Printf("out=%s komponen_tanpa_setup_gl=%d\n", *out, pre)
	return nil
}

// bank writes the F-1 Mandiri file (COVER LETTER + DETAIL). Employees failing the pre-check are left out of the file and
// written to <out>.pra-cek.xlsx (NIK, name, reason; never the account number). The account is decrypted with the PII
// key (env preferred over the flag, which shows up in the process list) and is never printed or audited.
func bank(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("bank", flag.ContinueOnError)
	yymm := fs.Int("yymm", 0, "periode yymm (wajib)")
	division := fs.String("division", "", "divisi (wajib)")
	company := fs.Int("company", 1, "pyr_company.pco_company_id")
	tdate := fs.String("transfer-date", "", "tanggal transfer YYYY-MM-DD (wajib)")
	bankName := fs.String("bank", report.MandiriBank, "nama bank yang diterima di berkas (cocok sebagian, tanpa huruf besar/kecil)")
	allowOpen := fs.Bool("allow-open", false, "izinkan periode belum APPROVED (gladi; tercatat di audit)")
	actor := fs.String("actor", "cli-report", "identitas aktor untuk audit")
	out := fs.String("out", "", "berkas xlsx keluaran (wajib; berisi rekening & gaji nyata, jangan di-commit)")
	dsn := fs.String("dsn", "", "DSN role payroll_app (atau env PAYROLL_APP_DSN)")
	key := fs.String("pii-key", "", "kunci PII (dekripsi rekening) (atau env PAYROLL_PII_KEY; utamakan env)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	envDefault(dsn, "PAYROLL_APP_DSN")
	envDefault(key, "PAYROLL_PII_KEY")
	if *yymm == 0 || anyEmpty(*division, *out, *dsn, *key, *tdate) {
		return fmt.Errorf("-yymm, -division, -transfer-date, -out, -dsn (atau PAYROLL_APP_DSN), dan -pii-key (atau PAYROLL_PII_KEY) wajib")
	}
	text, err := bankText(*tdate)
	if err != nil {
		return err
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
	var issues int
	err = svc.ExportBank(ctx, report.Query{CompanyID: int16(*company), YYMM: *yymm, //nolint:gosec // as above
		Division: *division, AllowOpen: *allowOpen}, *key, *bankName, func(d report.BankData) (int, error) {
		issues = len(d.Issues)
		return writeBankFiles(d, text, *out)
	})
	if err == nil {
		err = commitBankFiles(*out, issues > 0)
	}
	if err != nil {
		for _, f := range []string{*out + ".tmp", *out + ".pra-cek.xlsx.tmp"} {
			if rerr := os.Remove(f); rerr != nil && !os.IsNotExist(rerr) {
				fmt.Fprintln(os.Stderr, "remove tmp:", rerr)
			}
		}
		return err
	}
	fmt.Printf("out=%s dilewati_pra_cek=%d\n", *out, issues)
	if issues > 0 {
		fmt.Printf("pra-cek=%s.pra-cek.xlsx\n", *out)
	}
	return nil
}

func anyEmpty(vals ...string) bool {
	for _, v := range vals {
		if v == "" {
			return true
		}
	}
	return false
}

// bankText builds the cover/title text from PAYROLL_REPORT_* and the transfer date.
func bankText(tdate string) (report.BankText, error) {
	td, err := time.Parse("2006-01-02", tdate)
	if err != nil {
		return report.BankText{}, fmt.Errorf("-transfer-date harus YYYY-MM-DD: %w", err)
	}
	rc, err := config.LoadReport()
	if err != nil {
		return report.BankText{}, err
	}
	return report.BankText{Letterhead: rc.Letterhead, CompanyName: rc.CompanyName, Branch: rc.BankBranch,
		EscrowAccount: rc.EscrowAccount, Signer1: rc.Signer1, Title1: rc.Title1, Signer2: rc.Signer2, Title2: rc.Title2,
		FeePerTransfer: rc.TransferFee, TransferDate: td, PeriodDate: td}, nil
}

// writeBankFiles writes the bank workbook (and the pre-check workbook when some rows were left out) to .tmp files; they
// are renamed only after the audit row committed.
func writeBankFiles(d report.BankData, text report.BankText, out string) (int, error) {
	if len(d.Rows) == 0 { // no bank file; the pre-check is written at its final name so the reasons are not lost
		var pb bytes.Buffer
		if err := report.WriteBankPreCheck(d.Issues, &pb); err != nil {
			return 0, err
		}
		if err := os.WriteFile(out+".pra-cek.xlsx", pb.Bytes(), 0o600); err != nil {
			return 0, err
		}
		return 0, report.ErrNoPayableRows
	}
	var buf bytes.Buffer
	n, err := report.WriteBank(d, text, &buf)
	if err != nil {
		return 0, err
	}
	if err := os.WriteFile(out+".tmp", buf.Bytes(), 0o600); err != nil {
		return 0, err
	}
	if len(d.Issues) == 0 {
		return n, nil
	}
	var pb bytes.Buffer
	if err := report.WriteBankPreCheck(d.Issues, &pb); err != nil {
		return 0, err
	}
	return n, os.WriteFile(out+".pra-cek.xlsx.tmp", pb.Bytes(), 0o600)
}

func commitBankFiles(out string, withPre bool) error {
	if err := os.Rename(out+".tmp", out); err != nil {
		return err
	}
	if withPre {
		return os.Rename(out+".pra-cek.xlsx.tmp", out+".pra-cek.xlsx")
	}
	return nil
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
	dsn := fs.String("dsn", "", "DSN role payroll_app (atau env PAYROLL_APP_DSN)")
	key := fs.String("pii-key", "", "kunci PII (dekripsi NPWP dan rekening) (atau env PAYROLL_PII_KEY; utamakan env)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	envDefault(dsn, "PAYROLL_APP_DSN")
	envDefault(key, "PAYROLL_PII_KEY")
	if *yymm == 0 || *division == "" || *out == "" || *dsn == "" || *key == "" {
		return fmt.Errorf("-yymm, -division, -out, -dsn (atau PAYROLL_APP_DSN), dan -pii-key (atau PAYROLL_PII_KEY) wajib")
	}
	cfg, err := slipConfig(*key)
	if err != nil {
		return err
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
		Division: *division, AllowOpen: *allowOpen}, cfg,
		report.SlipOptions{NIK: *nik, NoPassword: *noPassword, OutDir: *out, PeriodName: report.PeriodName(*yymm)})
	if err != nil {
		return err
	}
	fmt.Printf("slip ditulis=%d dilewati_tanpa_tanggal_lahir=%d out=%s\n", len(res.Written), len(res.Skipped), *out)
	for _, n := range res.Skipped {
		fmt.Printf("DILEWATI %s (pe_birth_date kosong; tindak lanjut HR)\n", n)
	}
	return nil
}

// envDefault fills an empty flag value from the environment; the secret never becomes a flag default, which
// flag.PrintDefaults would print.
func envDefault(v *string, name string) {
	if *v == "" {
		*v = os.Getenv(name)
	}
}

// slipConfig reads the F-3 layout settings from the environment: the logo file (PAYROLL_REPORT_LOGO, not in the
// repository; unset = text-only letterhead), the "Perhatian" text lines separated by "|" and the IT contact.
func slipConfig(piiKey string) (report.SlipConfig, error) {
	c := report.SlipConfig{PIIKey: piiKey, ContactIT: os.Getenv("PAYROLL_SLIP_CONTACT_IT")}
	if t := os.Getenv("PAYROLL_SLIP_PERHATIAN"); t != "" {
		c.Perhatian = strings.Split(t, "|")
	}
	if path := os.Getenv("PAYROLL_REPORT_LOGO"); path != "" {
		b, err := os.ReadFile(path) //nolint:gosec // operator-supplied path from the environment
		if err != nil {
			return c, fmt.Errorf("PAYROLL_REPORT_LOGO: %w", err)
		}
		c.Logo = b
	}
	return c, nil
}

// gl writes the GAJI journal (F-2 sheet `upload` + Check, 11§4, D-63). An invalid journal (unmapped line, missing
// sub-account, unbalanced, no BOOK rate) writes nothing and prints every problem. pyr_gl_export records the file hash.
func gl(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("gl", flag.ContinueOnError)
	yymm := fs.Int("yymm", 0, "periode yymm (wajib)")
	division := fs.String("division", "", "divisi (wajib)")
	company := fs.Int("company", 1, "pyr_company.pco_company_id")
	allowOpen := fs.Bool("allow-open", false, "izinkan periode belum APPROVED (gladi; tercatat di audit)")
	actor := fs.String("actor", "cli-report", "identitas aktor untuk audit")
	out := fs.String("out", "", "berkas xlsx keluaran (wajib; berisi nominal gaji, jangan di-commit)")
	dsn := fs.String("dsn", "", "DSN role payroll_app (atau env PAYROLL_APP_DSN)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	envDefault(dsn, "PAYROLL_APP_DSN")
	if *yymm == 0 || anyEmpty(*division, *out, *dsn, *actor) {
		return fmt.Errorf("-yymm, -division, -out, -actor, dan -dsn (atau PAYROLL_APP_DSN) wajib")
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
	tmp := *out + ".tmp"                                                              // renamed only after pyr_gl_export and the audit row committed
	j, err := svc.ExportGL(ctx, report.Query{CompanyID: int16(*company), YYMM: *yymm, //nolint:gosec // as above
		Division: *division, AllowOpen: *allowOpen}, func(j report.GLJournal) ([]byte, error) {
		var buf bytes.Buffer
		if err := report.WriteGL(j, &buf); err != nil {
			return nil, err
		}
		return buf.Bytes(), os.WriteFile(tmp, buf.Bytes(), 0o600)
	})
	if err == nil {
		err = os.Rename(tmp, *out)
	}
	if err != nil {
		if rerr := os.Remove(tmp); rerr != nil && !os.IsNotExist(rerr) {
			fmt.Fprintln(os.Stderr, "remove tmp:", rerr)
		}
		return glProblems(err, *out)
	}
	fmt.Printf("out=%s baris=%d debit=%s credit=%s nik=%d\n", *out, len(j.Rows), j.Debit, j.Credit, j.NIKCount)
	return nil
}

// glProblems writes the full problem list (with NIKs) to <out>.masalah.txt (0600) and returns the error, whose
// message only counts per code, so no NIK reaches stderr.
func glProblems(err error, out string) error {
	var ge *report.GLError
	if !errors.As(err, &ge) {
		return err
	}
	f := out + ".masalah.txt"
	if werr := os.WriteFile(f, []byte(strings.Join(ge.Details(), "\n")+"\n"), 0o600); werr != nil {
		return fmt.Errorf("%w (daftar masalah gagal ditulis: %v)", err, werr) //nolint:errorlint // primary error wrapped
	}
	return fmt.Errorf("%w; daftar lengkap: %s", err, f)
}
