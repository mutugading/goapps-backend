// Package main is the payslip mailer CLI (D-62 butir 5, 16§2.5). Run by IT on the server as payroll_app with an
// explicit scope. Subcommands: create, send, resend, cancel, report. SMTP settings come from PAYROLL_SMTP_*.
package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	_ "github.com/jackc/pgx/v5/stdlib" // driver

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/distribution"
	"github.com/mutugading/goapps-backend/services/payroll/internal/application/report"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/scope"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/audit"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/config"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/postgres"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/smtp"
)

const usage = `usage:
  mailer create -yymm 202701 -division SOLO [-company 1] [-allow-open] [-allow-dup-email] [-actor cli-mailer] -dsn <payroll_app dsn>
  mailer send   -batch ID -yymm 202701 -division SOLO [-company 1] [-allow-open] [-test-to addr] [-limit N] [-wait] [-actor cli-mailer] [-pii-key K | PAYROLL_PII_KEY] -dsn <dsn>   (SMTP: PAYROLL_SMTP_*; kunci PII wajib, utamakan env; logo/teks: PAYROLL_REPORT_LOGO, PAYROLL_SLIP_PERHATIAN, PAYROLL_SLIP_CONTACT_IT)
  mailer resend -batch ID -yymm 202701 -division SOLO -nik N -reason "..." [-allow-dup-email] [-company 1] [-allow-open] [-actor cli-mailer] -dsn <dsn>
  mailer cancel -batch ID -yymm 202701 -division SOLO -reason "..." [-company 1] [-actor cli-mailer] -dsn <dsn>   (batch CANCELLED; send/resend ditolak; batch baru boleh dibuat)
  mailer report -batch ID -yymm 202701 -division SOLO -out x.xlsx [-company 1] [-actor cli-mailer] -dsn <dsn>`

type common struct {
	fs        *flag.FlagSet
	yymm      *int
	division  *string
	company   *int
	allowOpen *bool
	actor     *string
	dsn       *string
}

func newCommon(name string) *common {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	return &common{fs: fs,
		yymm:      fs.Int("yymm", 0, "periode yymm (wajib)"),
		division:  fs.String("division", "", "divisi (wajib)"),
		company:   fs.Int("company", 1, "pyr_company.pco_company_id"),
		allowOpen: fs.Bool("allow-open", false, "izinkan periode belum APPROVED (gladi; tercatat di audit)"),
		actor:     fs.String("actor", "cli-mailer", "identitas aktor untuk audit"),
		dsn:       fs.String("dsn", "", "DSN role payroll_app (atau env PAYROLL_APP_DSN)"), // never a default: -h would print it
	}
}

// open validates the common flags and returns the scoped context, query, and a closer.
func (c *common) open(ctx context.Context) (context.Context, report.Query, *postgres.DB, func(), error) {
	if *c.dsn == "" { // the DSN holds the password: read from the environment after flag parsing, never as a flag default
		*c.dsn = os.Getenv("PAYROLL_APP_DSN")
	}
	if *c.yymm == 0 || anyEmpty(*c.division, *c.dsn, *c.actor) {
		return nil, report.Query{}, nil, nil, errors.New("-yymm, -division, dan -dsn (atau PAYROLL_APP_DSN) wajib")
	}
	sc, err := scope.New(int16(*c.company), *c.division) //nolint:gosec // range checked by FK
	if err != nil {
		return nil, report.Query{}, nil, nil, err
	}
	raw, err := sql.Open("pgx", *c.dsn)
	if err != nil {
		return nil, report.Query{}, nil, nil, err
	}
	closer := func() {
		if cerr := raw.Close(); cerr != nil {
			fmt.Fprintln(os.Stderr, "close:", cerr)
		}
	}
	ctx = scope.WithIdentity(ctx, scope.Identity{UserID: *c.actor, Scopes: []scope.Scope{sc}})
	q := report.Query{CompanyID: int16(*c.company), YYMM: *c.yymm, Division: *c.division, AllowOpen: *c.allowOpen} //nolint:gosec // as above
	return ctx, q, postgres.NewDBFromSQL(raw), closer, nil
}

func anyEmpty(vals ...string) bool {
	for _, v := range vals {
		if v == "" {
			return true
		}
	}
	return false
}

func main() { os.Exit(realMain()) }

func realMain() int {
	cmds := map[string]func(context.Context, []string) error{"create": create, "send": send, "resend": resend, "cancel": cancel, "report": batchReport}
	var fn func(context.Context, []string) error
	if len(os.Args) >= 2 {
		fn = cmds[os.Args[1]]
	}
	if fn == nil {
		fmt.Fprintln(os.Stderr, usage)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := fn(ctx, os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, os.Args[1]+":", err)
		return 1
	}
	return 0
}

func create(ctx context.Context, args []string) error {
	c := newCommon("create")
	dup := c.fs.Bool("allow-dup-email", false, "kirim juga ke alamat yang dipakai >1 NIK (default: dikecualikan)")
	if err := c.fs.Parse(args); err != nil {
		return err
	}
	ctx, q, db, closeDB, err := c.open(ctx)
	if err != nil {
		return err
	}
	defer closeDB()
	svc := distribution.NewService(db, audit.NewWriter(), nil, config.SMTPConfig{})
	res, err := svc.Create(ctx, q, distribution.CreateOptions{AllowDuplicateEmail: *dup})
	if err != nil {
		return err
	}
	fmt.Printf("batch=%d total=%d antre=%d tanpa_email=%d tanpa_tanggal_lahir=%d email_ganda_dikecualikan=%d\n",
		res.BatchID, res.Total, res.Queued, res.NoEmail, res.NoPassword, res.Dup)
	for _, g := range []struct {
		label string
		niks  []string
	}{{"TANPA_EMAIL", res.NoEmailNIK}, {"TANPA_TANGGAL_LAHIR", res.NoPasswordNIK}, {"EMAIL_GANDA", res.DuplicateEmailNIK}} {
		if len(g.niks) > 0 {
			fmt.Printf("%s (tindak lanjut HR): %s\n", g.label, strings.Join(g.niks, " "))
		}
	}
	return nil
}

func send(ctx context.Context, args []string) error {
	c := newCommon("send")
	batch := c.fs.Int64("batch", 0, "id batch (wajib)")
	testTo := c.fs.String("test-to", "", "gladi: kirim semua email ke alamat ini saja; status item tidak berubah")
	limit := c.fs.Int("limit", 0, "maksimum email dalam satu jalan (0 = semua; dengan -test-to default 3)")
	wait := c.fs.Bool("wait", false, "tunggu jadwal retry sampai semua item selesai")
	key := c.fs.String("pii-key", "", "kunci PII (dekripsi NPWP dan rekening di slip) (atau env PAYROLL_PII_KEY; utamakan env)")
	if err := c.fs.Parse(args); err != nil {
		return err
	}
	if *batch == 0 {
		return errors.New("-batch wajib")
	}
	if *key == "" { // the secret never becomes a flag default, which flag.PrintDefaults would print
		*key = os.Getenv("PAYROLL_PII_KEY")
	}
	if strings.TrimSpace(*key) == "" {
		return errors.New("-pii-key (atau PAYROLL_PII_KEY) wajib: slip memuat NPWP dan rekening")
	}
	if err := config.ValidatePIIKey(config.EnvFromOS(), "-pii-key (PAYROLL_PII_KEY)", *key); err != nil {
		return err
	}
	slipCfg, err := report.SlipConfigFromEnv(*key)
	if err != nil {
		return err
	}
	ctx, q, db, closeDB, err := c.open(ctx)
	if err != nil {
		return err
	}
	defer closeDB()
	cfg, err := config.LoadSMTP()
	if err != nil {
		return err
	}
	svc := distribution.NewService(db, audit.NewWriter(), smtp.NewClient(cfg), cfg)
	res, err := svc.Send(ctx, distribution.SendOptions{BatchID: *batch, Query: q, TestTo: *testTo, Limit: *limit, Wait: *wait, SlipConfig: slipCfg})
	fmt.Printf("terkirim=%d gagal=%d gagal_final=%d uji=%d menunggu_retry=%d\n", res.Sent, res.Failed, res.FinalFailed, res.Test, res.Waiting)
	return err
}

func resend(ctx context.Context, args []string) error {
	c := newCommon("resend")
	batch := c.fs.Int64("batch", 0, "id batch (wajib)")
	nik := c.fs.String("nik", "", "NIK (wajib)")
	reason := c.fs.String("reason", "", "alasan kirim ulang (wajib)")
	dup := c.fs.Bool("allow-dup-email", false, "izinkan kirim ulang item yang dikecualikan karena email ganda")
	if err := c.fs.Parse(args); err != nil {
		return err
	}
	if *batch == 0 || *nik == "" || strings.TrimSpace(*reason) == "" {
		return errors.New("-batch, -nik, dan -reason wajib")
	}
	ctx, q, db, closeDB, err := c.open(ctx)
	if err != nil {
		return err
	}
	defer closeDB()
	svc := distribution.NewService(db, audit.NewWriter(), nil, config.SMTPConfig{})
	id, err := svc.Resend(ctx, distribution.ResendOptions{BatchID: *batch, NIK: *nik, Reason: *reason, Query: q, AllowDuplicateEmail: *dup})
	if err != nil {
		return err
	}
	fmt.Printf("item=%d diantre; jalankan: mailer send -batch %d ...\n", id, *batch)
	return nil
}

func cancel(ctx context.Context, args []string) error {
	c := newCommon("cancel")
	batch := c.fs.Int64("batch", 0, "id batch (wajib)")
	reason := c.fs.String("reason", "", "alasan pembatalan (wajib)")
	if err := c.fs.Parse(args); err != nil {
		return err
	}
	if *batch == 0 || strings.TrimSpace(*reason) == "" {
		return errors.New("-batch dan -reason wajib")
	}
	ctx, q, db, closeDB, err := c.open(ctx)
	if err != nil {
		return err
	}
	defer closeDB()
	svc := distribution.NewService(db, audit.NewWriter(), nil, config.SMTPConfig{})
	if err := svc.Cancel(ctx, q, *batch, *reason); err != nil {
		return err
	}
	fmt.Printf("batch=%d CANCELLED\n", *batch)
	return nil
}

func batchReport(ctx context.Context, args []string) error {
	c := newCommon("report")
	batch := c.fs.Int64("batch", 0, "id batch (wajib)")
	out := c.fs.String("out", "", "berkas xlsx keluaran (wajib; berisi NIK dan nama)")
	if err := c.fs.Parse(args); err != nil {
		return err
	}
	if *batch == 0 || *out == "" {
		return errors.New("-batch dan -out wajib")
	}
	ctx, q, db, closeDB, err := c.open(ctx)
	if err != nil {
		return err
	}
	defer closeDB()
	svc := distribution.NewService(db, audit.NewWriter(), nil, config.SMTPConfig{})
	br, err := svc.Report(ctx, q, *batch)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := distribution.WriteReport(br, &buf); err != nil {
		return err
	}
	if err := os.WriteFile(*out, buf.Bytes(), 0o600); err != nil {
		return err
	}
	fmt.Printf("out=%s total=%d terkirim=%d gagal=%d diblokir=%d\n", *out, br.Total, br.Sent, br.Failed, br.Blocked)
	return nil
}
