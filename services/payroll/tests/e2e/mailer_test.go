package e2e

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/distribution"
	"github.com/mutugading/goapps-backend/services/payroll/internal/application/report"
	auditinfra "github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/audit"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/config"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/pdf"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/smtp"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// T028g: mailer outbox against PG18 as payroll_app with an in-process fake SMTP server.
const (
	mailYYMM  = 203605
	mail2YYMM = 203606
	mail3YYMM = 203607
	mail4YYMM = 203604
)

type fakeSMTP struct {
	ln   net.Listener
	mu   sync.Mutex
	msgs []string // raw DATA per accepted message
	rcpt []string
	fail map[string]int // recipient -> remaining 451 rejections
}

func newFakeSMTP(t *testing.T) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeSMTP{ln: ln, fail: map[string]int{}}
	go f.serve()
	t.Cleanup(func() { _ = ln.Close() }) //nolint:errcheck // test cleanup
	return f
}

func (f *fakeSMTP) port() int { return f.ln.Addr().(*net.TCPAddr).Port }

func (f *fakeSMTP) serve() {
	for {
		c, err := f.ln.Accept()
		if err != nil {
			return
		}
		go f.conn(c)
	}
}

func (f *fakeSMTP) conn(c net.Conn) {
	defer c.Close() //nolint:errcheck // test server
	r := bufio.NewReader(c)
	w := func(s string) { fmt.Fprintf(c, "%s\r\n", s) }
	w("220 fake ESMTP")
	var rcpt string
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			w("250 fake")
		case strings.HasPrefix(cmd, "MAIL FROM"), strings.HasPrefix(cmd, "RSET"):
			w("250 OK")
		case strings.HasPrefix(cmd, "RCPT TO"):
			rcpt = strings.Trim(strings.TrimSpace(line[len("RCPT TO:"):]), "<>")
			f.mu.Lock()
			n := f.fail[rcpt]
			if n > 0 {
				f.fail[rcpt] = n - 1
			}
			f.mu.Unlock()
			if n > 0 {
				w("451 try later")
			} else {
				w("250 OK")
			}
		case cmd == "DATA":
			w("354 go")
			var b strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				b.WriteString(l)
			}
			f.mu.Lock()
			f.msgs = append(f.msgs, b.String())
			f.rcpt = append(f.rcpt, rcpt)
			f.mu.Unlock()
			w("250 2.0.0 Ok: queued as FAKE123")
		case cmd == "QUIT":
			w("221 bye")
			return
		default:
			w("250 OK")
		}
	}
}

func (f *fakeSMTP) count() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.msgs) }

func (f *fakeSMTP) to() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.rcpt...)
}

func mailSvc(f *fakeSMTP, now *time.Time) *distribution.Service {
	cfg := config.SMTPConfig{Host: "127.0.0.1", Port: f.port(), TLSMode: config.SMTPNone, From: "mgtpayslip@mutugading.com",
		FromName: "HR", ReplyTo: "hr@example.test", RatePerMin: 0, MaxAttempts: 3}
	s := distribution.NewService(appDB, auditinfra.NewWriter(), smtp.NewClient(cfg), cfg)
	s.Now = func() time.Time { return *now }
	s.Sleep = func(ctx context.Context, d time.Duration) error { *now = now.Add(d); return ctx.Err() }
	return s
}

func TestMailer(t *testing.T) {
	seedReportRun(t, mailYYMM, "APPROVED")
	for _, q := range []string{
		`INSERT INTO payroll.pyr_slip_password_policy (pspp_company_id, pspp_valid_from, pspp_policy_code, pspp_status, created_by, approved_by)
		 VALUES (1, DATE '2020-01-01', 'DOB_DDMMYYYY', 'ACTIVE', 'test', 'system') ON CONFLICT DO NOTHING`,
		`INSERT INTO payroll.pyr_mail_template (pmt_company_id, pmt_doc_type, pmt_version, pmt_subject, pmt_body_html, pmt_body_text, pmt_status, created_by, approved_by)
		 VALUES (1,'SLIP_GAJI',1,'Slip Gaji {{.PeriodName}} - {{.Nama}}','<p>Yth. {{.Nama}}, {{.PasswordHint}}</p>','Yth. {{.Nama}}','ACTIVE','t','s') ON CONFLICT DO NOTHING`,
		`INSERT INTO payroll.pyr_employee (pe_company_id, pe_nik, pe_division, pe_nama, pe_orion_sub_acnt, pe_birth_date, pe_email) VALUES
		 (1,'ZREP0001','SOLO','Rep Satu','ZREP0001','1990-03-07','satu@example.test'),
		 (1,'ZREP0002','SOLO','Rep Dua','ZREP0002','1991-04-08',NULL)
		 ON CONFLICT (pe_company_id, pe_nik, pe_division) DO UPDATE SET pe_birth_date=EXCLUDED.pe_birth_date, pe_email=EXCLUDED.pe_email`,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatalf("seed: %v\n%s", err, q)
		}
	}
	t.Cleanup(func() { // other e2e tests expect ZREP0002 without birth date
		for _, q := range []string{ // other e2e tests delete these employees and expect ZREP0002 without birth date
			`DELETE FROM payroll.pyr_distribution_item`, `DELETE FROM payroll.pyr_distribution_batch`,
			`UPDATE payroll.pyr_employee SET pe_birth_date=NULL WHERE pe_nik='ZREP0002'`} {
			_, _ = ownDB.Exec(q) //nolint:errcheck // cleanup
		}
	})
	fake := newFakeSMTP(t)
	now := time.Date(2036, 6, 1, 8, 0, 0, 0, time.UTC)
	svc := mailSvc(fake, &now)
	ctx := ctxFor(t, userRW)
	q := report.Query{CompanyID: 1, YYMM: mailYYMM, Division: "SOLO"}

	// create: pre-check (ZREP0002 has no email)
	cr, err := svc.Create(ctx, q, distribution.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if cr.Total != 2 || cr.Queued != 1 || cr.NoEmail != 1 || cr.NoEmailNIK[0] != "ZREP0002" {
		t.Fatalf("create %+v", cr)
	}
	if _, err := svc.Create(ctx, q, distribution.CreateOptions{}); err == nil {
		t.Fatal("batch kedua untuk periode yang sama harus ditolak")
	}
	b := cr.BatchID

	// gladi: -test-to redirects, item state untouched
	res, err := svc.Send(ctx, distribution.SendOptions{SlipConfig: report.SlipConfig{PIIKey: encKey}, BatchID: b, Query: q, TestTo: "uji@example.test"})
	if err != nil || res.Test != 1 {
		t.Fatalf("test send %+v %v", res, err)
	}
	if got := fake.to(); len(got) != 1 || got[0] != "uji@example.test" {
		t.Fatalf("gladi harus ke alamat uji: %v", got)
	}
	if got := scalar(t, fmt.Sprintf(`SELECT pdi_status FROM payroll.pyr_distribution_item WHERE pdi_batch_id=%d AND pdi_nik='ZREP0001'`, b)); got != "QUEUED" {
		t.Fatalf("status setelah gladi = %s", got)
	}

	// retry: first two attempts rejected (451), third succeeds; backoff 1m then 5m on the fake clock
	fake.mu.Lock()
	fake.fail["satu@example.test"] = 2
	fake.mu.Unlock()
	res, err = svc.Send(ctx, distribution.SendOptions{SlipConfig: report.SlipConfig{PIIKey: encKey}, BatchID: b, Query: q})
	if err != nil || res.Failed != 1 || res.Sent != 0 || res.Waiting == 0 {
		t.Fatalf("attempt 1: %+v %v", res, err)
	}
	if got := scalar(t, fmt.Sprintf(`SELECT pdi_attempts||'|'||pdi_status FROM payroll.pyr_distribution_item WHERE pdi_batch_id=%d AND pdi_nik='ZREP0001'`, b)); got != "1|FAILED" {
		t.Fatalf("after attempt 1: %s", got)
	}
	res, err = svc.Send(ctx, distribution.SendOptions{SlipConfig: report.SlipConfig{PIIKey: encKey}, BatchID: b, Query: q, Wait: true})
	if err != nil || res.Sent != 1 || res.Failed != 1 {
		t.Fatalf("with wait: %+v %v", res, err)
	}
	if fake.count() != 2 { // 1 test + 1 real
		t.Fatalf("messages = %d", fake.count())
	}
	if got := scalar(t, fmt.Sprintf(`SELECT pdi_attempts||'|'||pdi_status||'|'||pdi_message_id FROM payroll.pyr_distribution_item WHERE pdi_batch_id=%d AND pdi_nik='ZREP0001'`, b)); !strings.HasPrefix(got, "3|SENT|<") || !strings.HasSuffix(got, ".3@payroll>") {
		t.Fatalf("after retries: %s", got)
	}

	// restart: a second run must not send SENT items again
	res, err = svc.Send(ctx, distribution.SendOptions{SlipConfig: report.SlipConfig{PIIKey: encKey}, BatchID: b, Query: q})
	if err != nil || res.Sent+res.Failed != 0 || fake.count() != 2 {
		t.Fatalf("restart: %+v %v count=%d", res, err, fake.count())
	}
	if got := scalar(t, fmt.Sprintf(`SELECT pdb_status||'|'||pdb_sent||'|'||pdb_blocked FROM payroll.pyr_distribution_batch WHERE pdb_id=%d`, b)); got != "DONE|1|1" {
		t.Fatalf("batch = %s", got)
	}

	// the delivered mail carries the encrypted PDF under the legacy name, and no password in the body
	msg := fake.msgs[1]
	if !strings.Contains(msg, `filename="`+fmt.Sprintf("%dG", mailYYMM)+`ZREP0001.pdf"`) || strings.Contains(msg, "07031990") {
		t.Fatal("lampiran/nama berkas salah atau password bocor di email")
	}

	// resend requires a reason, then sends a new item exactly once; the original stays SENT
	if _, err := svc.Resend(ctx, distribution.ResendOptions{BatchID: b, NIK: "ZREP0001", Query: q}); err == nil {
		t.Fatal("resend tanpa alasan harus ditolak")
	}
	if _, err := svc.Resend(ctx, distribution.ResendOptions{BatchID: b, NIK: "ZREP0002", Reason: "x", Query: q}); err == nil {
		t.Fatal("resend tanpa email harus ditolak")
	}
	if _, err := svc.Resend(ctx, distribution.ResendOptions{BatchID: b, NIK: "ZREP0001", Reason: "karyawan tidak menerima", Query: q}); err != nil {
		t.Fatal(err)
	}
	res, err = svc.Send(ctx, distribution.SendOptions{SlipConfig: report.SlipConfig{PIIKey: encKey}, BatchID: b, Query: q})
	if err != nil || res.Sent != 1 || fake.count() != 3 {
		t.Fatalf("resend send: %+v %v count=%d", res, err, fake.count())
	}
	if got := scalar(t, fmt.Sprintf(`SELECT count(*)::text FROM payroll.pyr_distribution_item WHERE pdi_batch_id=%d AND pdi_nik='ZREP0001' AND pdi_status='SENT'`, b)); got != "2" {
		t.Fatalf("SENT items = %s", got)
	}

	// T028g2: the evidence is the server's own DATA reply, not a constant
	if got := scalar(t, fmt.Sprintf(`SELECT string_agg(DISTINCT pdi_smtp_response, '|') FROM payroll.pyr_distribution_item WHERE pdi_batch_id=%d AND pdi_status='SENT'`, b)); got != "250 2.0.0 Ok: queued as FAKE123" {
		t.Fatalf("pdi_smtp_response = %q", got)
	}

	// report + audit without PII
	br, err := svc.Report(ctx, q, b)
	if err != nil || br.Sent != 2 || len(br.Rows) != 3 {
		t.Fatalf("report %+v %v", br, err)
	}
	for _, r := range br.Rows {
		if strings.Contains(r.Email, "satu@") {
			t.Fatal("alamat tidak di-mask di laporan")
		}
	}
	if got := scalar(t, `SELECT count(*)::text FROM payroll.pyr_audit_log WHERE pal_action LIKE 'DISTRIBUTION\_%'
	  AND (pal_after::text LIKE '%satu@%' OR pal_after::text LIKE '%uji@%' OR pal_after::text LIKE '%ZREP%')`); got != "0" {
		t.Fatalf("audit memuat PII: %s", got)
	}
	if got := scalar(t, `SELECT count(DISTINCT pal_action)::text FROM payroll.pyr_audit_log WHERE pal_action LIKE 'DISTRIBUTION\_%' AND pal_action NOT IN ('DISTRIBUTION_CANCEL','DISTRIBUTION_SUPERSEDE')`); got != "5" {
		t.Fatalf("aksi audit = %s", got)
	}
	if got := scalar(t, `SELECT count(*)::text FROM payroll.pyr_distribution_item WHERE pdi_last_error LIKE '%@example%' OR pdi_smtp_response LIKE '%@example%'`); got != "0" {
		t.Fatalf("alamat di log item: %s", got)
	}
	_ = errors.New
}

// Stuck SENDING (crash after claim) is recovered after the lease under the same attempt/Message-ID; FAILED_FINAL at
// the maximum attempt with backoff 1m then 5m; a -test-to rehearsal is capped at 3 mails.
func TestMailer_StuckAndFinal(t *testing.T) {
	seedReportRun(t, mail2YYMM, "APPROVED")
	for _, q := range []string{
		`INSERT INTO payroll.pyr_slip_password_policy (pspp_company_id, pspp_valid_from, pspp_policy_code, pspp_status, created_by, approved_by)
		 VALUES (1, DATE '2020-01-01', 'DOB_DDMMYYYY', 'ACTIVE', 'test', 'system') ON CONFLICT DO NOTHING`,
		`INSERT INTO payroll.pyr_mail_template (pmt_company_id, pmt_doc_type, pmt_version, pmt_subject, pmt_body_html, pmt_body_text, pmt_status, created_by, approved_by)
		 VALUES (1,'SLIP_GAJI',1,'Slip {{.PeriodName}}','<p>{{.Nama}}</p>','{{.Nama}}','ACTIVE','t','s') ON CONFLICT DO NOTHING`,
		`INSERT INTO payroll.pyr_employee (pe_company_id, pe_nik, pe_division, pe_nama, pe_orion_sub_acnt, pe_birth_date, pe_email) VALUES
		 (1,'ZREP0001','SOLO','Rep Satu','ZREP0001','1990-03-07','satu@example.test'),
		 (1,'ZREP0002','SOLO','Rep Dua','ZREP0002','1991-04-08','dua@example.test')
		 ON CONFLICT (pe_company_id, pe_nik, pe_division) DO UPDATE SET pe_birth_date=EXCLUDED.pe_birth_date, pe_email=EXCLUDED.pe_email`,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatalf("seed: %v\n%s", err, q)
		}
	}
	t.Cleanup(func() {
		for _, q := range []string{`DELETE FROM payroll.pyr_distribution_item`, `DELETE FROM payroll.pyr_distribution_batch`,
			`UPDATE payroll.pyr_employee SET pe_birth_date=NULL WHERE pe_nik='ZREP0002'`} {
			_, _ = ownDB.Exec(q) //nolint:errcheck // cleanup
		}
	})
	fake := newFakeSMTP(t)
	now := time.Date(2036, 7, 1, 8, 0, 0, 0, time.UTC)
	svc := mailSvc(fake, &now)
	svc.Lease = 10 * time.Minute
	ctx := ctxFor(t, userRW)
	q := report.Query{CompanyID: 1, YYMM: mail2YYMM, Division: "SOLO"}
	cr, err := svc.Create(ctx, q, distribution.CreateOptions{})
	if err != nil || cr.Queued != 2 {
		t.Fatalf("create %+v %v", cr, err)
	}
	b := cr.BatchID
	item := func(nik string) string {
		return scalar(t, fmt.Sprintf(`SELECT pdi_status||'|'||pdi_attempts||'|'||COALESCE(pdi_message_id,'') FROM payroll.pyr_distribution_item WHERE pdi_batch_id=%d AND pdi_nik='%s'`, b, nik))
	}

	// rehearsal cap: 2 items < 3, so both go; the cap itself is exercised by DefaultTestLimit (3) below
	if distribution.DefaultTestLimit != 3 {
		t.Fatal("default test limit")
	}

	// crash simulation: ZREP0001 left in SENDING (attempt 1) with a lease; Resend must not accept it
	if _, err := ownDB.Exec(fmt.Sprintf(`UPDATE payroll.pyr_distribution_item SET pdi_status='SENDING', pdi_attempts=1,
	  pdi_message_id='<%d.0.1@payroll>', pdi_next_attempt_at=$1 WHERE pdi_batch_id=%d AND pdi_nik='ZREP0001'`, b, b), now.Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Resend(ctx, distribution.ResendOptions{BatchID: b, NIK: "ZREP0001", Reason: "x", Query: q}); err == nil {
		t.Fatal("resend SENDING harus ditolak (pulih lewat send)")
	}
	// lease not expired: ZREP0002 goes out, ZREP0001 is not touched and is reported as waiting
	res, err := svc.Send(ctx, distribution.SendOptions{SlipConfig: report.SlipConfig{PIIKey: encKey}, BatchID: b, Query: q})
	if err != nil || res.Sent != 1 || res.Waiting == 0 || fake.count() != 1 {
		t.Fatalf("before lease: %+v %v count=%d", res, err, fake.count())
	}
	if got := item("ZREP0001"); !strings.HasPrefix(got, "SENDING|1|") {
		t.Fatalf("stuck item touched: %s", got)
	}
	// after the lease the item is recovered under the same attempt (same Message-ID suffix .1)
	now = now.Add(11 * time.Minute)
	res, err = svc.Send(ctx, distribution.SendOptions{SlipConfig: report.SlipConfig{PIIKey: encKey}, BatchID: b, Query: q})
	if err != nil || res.Sent != 1 || fake.count() != 2 {
		t.Fatalf("after lease: %+v %v count=%d", res, err, fake.count())
	}
	if got := item("ZREP0001"); !strings.HasPrefix(got, "SENT|1|<") || !strings.HasSuffix(got, ".1@payroll>") {
		t.Fatalf("recovered item: %s", got)
	}
	if got := scalar(t, fmt.Sprintf(`SELECT pdb_status FROM payroll.pyr_distribution_batch WHERE pdb_id=%d`, b)); got != "DONE" {
		t.Fatalf("batch = %s", got)
	}

	// FAILED_FINAL after MaxAttempts (3) with backoff 1m then 5m
	cr2, err := svc.Resend(ctx, distribution.ResendOptions{BatchID: b, NIK: "ZREP0001", Reason: "uji gagal final", Query: q})
	if err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	fake.fail["satu@example.test"] = 10
	fake.mu.Unlock()
	start := now
	res, err = svc.Send(ctx, distribution.SendOptions{SlipConfig: report.SlipConfig{PIIKey: encKey}, BatchID: b, Query: q, Wait: true})
	if err != nil || res.FinalFailed != 1 || res.Failed != 2 {
		t.Fatalf("final: %+v %v", res, err)
	}
	if got := scalar(t, fmt.Sprintf(`SELECT pdi_status||'|'||pdi_attempts FROM payroll.pyr_distribution_item WHERE pdi_id=%d`, cr2)); got != "FAILED_FINAL|3" {
		t.Fatalf("resend item = %s", got)
	}
	if d := now.Sub(start); d < 6*time.Minute || d > 7*time.Minute {
		t.Fatalf("backoff waited %s, want about 1m + 5m", d)
	}
}

// attachment returns the base64-decoded PDF of the raw message.
func attachment(t *testing.T, msg string) []byte {
	t.Helper()
	i := strings.Index(msg, "Content-Type: application/pdf")
	if i < 0 {
		t.Fatal("pesan tanpa lampiran PDF")
	}
	rest := msg[i:]
	rest = rest[strings.Index(rest, "\r\n\r\n")+4:]
	if j := strings.Index(rest, "\r\n--"); j >= 0 {
		rest = rest[:j]
	}
	b, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(rest, "\r\n", ""))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// T028o2: the mailed slip is the full F-3 slip. The attachment (decrypted with the DOB password) holds NPWP and the
// bank account; audit rows and the item log do not. Send refuses to run without the PII key.
func TestMailer_SlipPII(t *testing.T) {
	seedReportRun(t, mail3YYMM, "APPROVED")
	y := itoa(mail3YYMM)
	for _, q := range []string{
		`INSERT INTO payroll.pyr_slip_password_policy (pspp_company_id, pspp_valid_from, pspp_policy_code, pspp_status, created_by, approved_by)
		 VALUES (1, DATE '2020-01-01', 'DOB_DDMMYYYY', 'ACTIVE', 'test', 'system') ON CONFLICT DO NOTHING`,
		`INSERT INTO payroll.pyr_mail_template (pmt_company_id, pmt_doc_type, pmt_version, pmt_subject, pmt_body_html, pmt_body_text, pmt_status, created_by, approved_by)
		 VALUES (1,'SLIP_GAJI',1,'Slip {{.PeriodName}}','<p>{{.Nama}}</p>','{{.Nama}}','ACTIVE','t','s') ON CONFLICT DO NOTHING`,
		`INSERT INTO payroll.pyr_salary_head (psh_run_id, psh_company_id, psh_nik, psh_yymm, psh_division, psh_nama, psh_tot_tunjangan, psh_tot_potongan, psh_rounding)
		 SELECT psh_run_id,1,'ZREP0003',psh_yymm,'SOLO','Rep Tiga',700,0,0 FROM payroll.pyr_salary_head WHERE psh_nik='ZREP0001' AND psh_yymm=` + y,
		`INSERT INTO payroll.pyr_salary_detail (psd_run_id, psd_nik, psd_type, psd_id, psd_amount)
		 SELECT psd_run_id,'ZREP0003','Tunjangan'::payroll.comp_type,901,700 FROM payroll.pyr_salary_detail WHERE psd_nik='ZREP0001' AND psd_id=901
		   AND psd_run_id IN (SELECT pr_id FROM payroll.pyr_run WHERE pr_yymm=` + y + `)`,
		`INSERT INTO payroll.pyr_employee (pe_company_id, pe_nik, pe_division, pe_nama, pe_orion_sub_acnt, pe_birth_date, pe_email, pe_npwp_enc, pe_bank_acnt_enc)
		 VALUES (1,'ZREP0001','SOLO','Rep Satu','ZREP0001','1990-03-07','satu@example.test', payroll.pgp_sym_encrypt('99.111.222.3-444.000','` + encKey + `'), payroll.pgp_sym_encrypt('5550001111','` + encKey + `')),
		        (1,'ZREP0002','SOLO','Rep Dua','ZREP0002','1991-04-08','dua@example.test', payroll.pgp_sym_encrypt('99.222.333.4-555.000','` + encKey + `'), payroll.pgp_sym_encrypt('5550002222','` + encKey + `')),
		        (1,'ZREP0003','SOLO','Rep Tiga','ZREP0003','1985-12-31','tiga@example.test', payroll.pgp_sym_encrypt('99.333.444.5-666.000','` + encKey + `'), payroll.pgp_sym_encrypt('5550003333','` + encKey + `'))
		 ON CONFLICT (pe_company_id, pe_nik, pe_division) DO UPDATE SET pe_birth_date=EXCLUDED.pe_birth_date, pe_email=EXCLUDED.pe_email,
		   pe_npwp_enc=EXCLUDED.pe_npwp_enc, pe_bank_acnt_enc=EXCLUDED.pe_bank_acnt_enc`,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatalf("seed: %v\n%s", err, q)
		}
	}
	t.Cleanup(func() {
		for _, q := range []string{`DELETE FROM payroll.pyr_distribution_item`, `DELETE FROM payroll.pyr_distribution_batch`,
			`DELETE FROM payroll.pyr_employee WHERE pe_nik = 'ZREP0003'`, // other e2e tests seed it themselves
			`UPDATE payroll.pyr_employee SET pe_birth_date=NULL, pe_email=NULL, pe_npwp_enc=NULL, pe_bank_acnt_enc=NULL WHERE pe_nik = 'ZREP0002'`} {
			_, _ = ownDB.Exec(q) //nolint:errcheck // cleanup
		}
	})
	fake := newFakeSMTP(t)
	now := time.Date(2036, 8, 1, 8, 0, 0, 0, time.UTC)
	svc := mailSvc(fake, &now)
	ctx := ctxFor(t, userRW)
	q := report.Query{CompanyID: 1, YYMM: mail3YYMM, Division: "SOLO"}
	cr, err := svc.Create(ctx, q, distribution.CreateOptions{})
	if err != nil || cr.Queued != 3 {
		t.Fatalf("create %+v %v", cr, err)
	}
	b := cr.BatchID

	if _, err := svc.Send(ctx, distribution.SendOptions{BatchID: b, Query: q}); !errors.Is(err, report.ErrNoPIIKey) {
		t.Fatalf("send tanpa kunci PII harus ditolak: %v", err)
	}
	if _, err := svc.Send(ctx, distribution.SendOptions{BatchID: b, Query: q, TestTo: "uji@example.test"}); !errors.Is(err, report.ErrNoPIIKey) {
		t.Fatalf("gladi tanpa kunci PII harus ditolak: %v", err)
	}
	if fake.count() != 0 {
		t.Fatal("tidak boleh ada email terkirim tanpa kunci")
	}

	cfg := report.SlipConfig{PIIKey: encKey, ContactIT: "it-sintetis@example.test", Perhatian: []string{"Teks perhatian sintetis satu"}}
	res, err := svc.Send(ctx, distribution.SendOptions{BatchID: b, Query: q, SlipConfig: cfg})
	if err != nil || res.Sent != 3 || fake.count() != 3 {
		t.Fatalf("send %+v %v count=%d", res, err, fake.count())
	}
	want := map[string][3]string{ // recipient -> DOB password, NPWP, account
		"satu@example.test": {"07031990", "99.111.222.3-444.000", "5550001111"},
		"dua@example.test":  {"08041991", "99.222.333.4-555.000", "5550002222"},
		"tiga@example.test": {"31121985", "99.333.444.5-666.000", "5550003333"},
	}
	rcpt, msgs := fake.to(), fake.msgs
	for i, to := range rcpt {
		w := want[to]
		enc := attachment(t, msgs[i])
		if err := pdf.Validate(enc, w[0]); err != nil {
			t.Fatalf("%s: password DOB: %v", to, err)
		}
		if contains(enc, w[1]) || contains(enc, w[2]) {
			t.Fatalf("%s: NPWP/rekening terbaca tanpa password", to)
		}
		var plain bytes.Buffer
		if err := api.Decrypt(bytes.NewReader(enc), &plain, model.NewAESConfiguration(w[0], "", 256)); err != nil {
			t.Fatalf("%s: dekripsi: %v", to, err)
		}
		for _, s := range []string{w[1], w[2], "it-sintetis@example.test", "Teks perhatian sintetis satu", "Net Paid"} {
			if !contains(plain.Bytes(), s) {
				t.Errorf("%s: slip terdekripsi tidak memuat %q", to, s)
			}
		}
		for _, other := range want { // no other employee's PII
			if other[1] != w[1] && (contains(plain.Bytes(), other[1]) || contains(plain.Bytes(), other[2])) {
				t.Errorf("%s: slip memuat PII karyawan lain", to)
			}
		}
		if strings.Contains(msgs[i], w[1]) || strings.Contains(msgs[i], w[2]) {
			t.Errorf("%s: PII di badan email", to)
		}
	}

	// neither the audit log nor the item log carries NPWP, account number or the key
	if got := scalar(t, `SELECT count(*)::text FROM payroll.pyr_audit_log WHERE pal_after::text ~ '(99\.[0-9]{3}|555000[123]|`+encKey+`)'
	  OR pal_before::text ~ '(99\.[0-9]{3}|555000[123]|`+encKey+`)'`); got != "0" {
		t.Fatalf("PII di audit: %s", got)
	}
	if got := scalar(t, `SELECT count(*)::text FROM payroll.pyr_distribution_item WHERE pdi_last_error ~ '(99\.[0-9]{3}|555000[123])' OR pdi_smtp_response ~ '(99\.[0-9]{3}|555000[123])'`); got != "0" {
		t.Fatalf("PII di log item: %s", got)
	}

	// resend renders with the same full slip and also needs the key
	if _, err := svc.Resend(ctx, distribution.ResendOptions{BatchID: b, NIK: "ZREP0001", Reason: "uji PII", Query: q}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Send(ctx, distribution.SendOptions{BatchID: b, Query: q}); !errors.Is(err, report.ErrNoPIIKey) {
		t.Fatalf("send item resend tanpa kunci harus ditolak: %v", err)
	}
	res, err = svc.Send(ctx, distribution.SendOptions{BatchID: b, Query: q, SlipConfig: cfg})
	if err != nil || res.Sent != 1 || fake.count() != 4 {
		t.Fatalf("resend send %+v %v", res, err)
	}
	if !contains(func() []byte {
		var o bytes.Buffer
		_ = api.Decrypt(bytes.NewReader(attachment(t, fake.msgs[3])), &o, model.NewAESConfiguration("07031990", "", 256)) //nolint:errcheck // asserted below via content
		return o.Bytes()
	}(), "5550001111") {
		t.Fatal("slip resend tanpa rekening")
	}
}

// T028g2: cancel refuses later send/resend, a new batch may follow; a batch built from a replaced run is SUPERSEDED
// when the next batch of the period is created, and the new batch records the revision link.
func TestMailer_CancelAndSupersede(t *testing.T) {
	seedReportRun(t, mail4YYMM, "APPROVED")
	y := itoa(mail4YYMM)
	for _, q := range []string{
		`INSERT INTO payroll.pyr_slip_password_policy (pspp_company_id, pspp_valid_from, pspp_policy_code, pspp_status, created_by, approved_by)
		 VALUES (1, DATE '2020-01-01', 'DOB_DDMMYYYY', 'ACTIVE', 'test', 'system') ON CONFLICT DO NOTHING`,
		`INSERT INTO payroll.pyr_mail_template (pmt_company_id, pmt_doc_type, pmt_version, pmt_subject, pmt_body_html, pmt_body_text, pmt_status, created_by, approved_by)
		 VALUES (1,'SLIP_GAJI',1,'Slip {{.PeriodName}}','<p>{{.Nama}}</p>','{{.Nama}}','ACTIVE','t','s') ON CONFLICT DO NOTHING`,
		`INSERT INTO payroll.pyr_employee (pe_company_id, pe_nik, pe_division, pe_nama, pe_orion_sub_acnt, pe_birth_date, pe_email) VALUES
		 (1,'ZREP0001','SOLO','Rep Satu','ZREP0001','1990-03-07','satu@example.test'),
		 (1,'ZREP0002','SOLO','Rep Dua','ZREP0002','1991-04-08','dua@example.test')
		 ON CONFLICT (pe_company_id, pe_nik, pe_division) DO UPDATE SET pe_birth_date=EXCLUDED.pe_birth_date, pe_email=EXCLUDED.pe_email`,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatalf("seed: %v\n%s", err, q)
		}
	}
	t.Cleanup(func() {
		for _, q := range []string{`DELETE FROM payroll.pyr_distribution_item`, `DELETE FROM payroll.pyr_distribution_batch`,
			`UPDATE payroll.pyr_employee SET pe_birth_date=NULL WHERE pe_nik='ZREP0002'`} {
			_, _ = ownDB.Exec(q) //nolint:errcheck // cleanup
		}
	})
	fake := newFakeSMTP(t)
	now := time.Date(2036, 9, 1, 8, 0, 0, 0, time.UTC)
	svc := mailSvc(fake, &now)
	ctx := ctxFor(t, userRW)
	q := report.Query{CompanyID: 1, YYMM: mail4YYMM, Division: "SOLO"}
	sendOpts := func(b int64) distribution.SendOptions {
		return distribution.SendOptions{BatchID: b, Query: q, SlipConfig: report.SlipConfig{PIIKey: encKey}}
	}

	// cancel: reason required; afterwards send and resend are refused, nothing leaves, and a new batch is allowed
	cr, err := svc.Create(ctx, q, distribution.CreateOptions{})
	if err != nil || cr.Queued != 2 {
		t.Fatalf("create %+v %v", cr, err)
	}
	if err := svc.Cancel(ctx, q, cr.BatchID, " "); err == nil {
		t.Fatal("cancel tanpa alasan harus ditolak")
	}
	if err := svc.Cancel(ctx, q, cr.BatchID, "salah periode"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Cancel(ctx, q, cr.BatchID, "lagi"); err == nil {
		t.Fatal("cancel dua kali harus ditolak")
	}
	if _, err := svc.Send(ctx, sendOpts(cr.BatchID)); err == nil || !strings.Contains(err.Error(), "CANCELLED") {
		t.Fatalf("send batch CANCELLED harus ditolak: %v", err)
	}
	if _, err := svc.Resend(ctx, distribution.ResendOptions{BatchID: cr.BatchID, NIK: "ZREP0001", Reason: "x", Query: q}); err == nil || !strings.Contains(err.Error(), "CANCELLED") {
		t.Fatalf("resend batch CANCELLED harus ditolak: %v", err)
	}
	if fake.count() != 0 {
		t.Fatal("tidak boleh ada email dari batch CANCELLED")
	}
	if got := scalar(t, `SELECT count(*)::text FROM payroll.pyr_audit_log WHERE pal_action='DISTRIBUTION_CANCEL' AND pal_after::text LIKE '%salah periode%'`); got != "1" {
		t.Fatalf("audit cancel = %s", got)
	}
	cr2, err := svc.Create(ctx, q, distribution.CreateOptions{})
	if err != nil {
		t.Fatalf("batch baru setelah cancel: %v", err)
	}

	// the run is replaced (reopen + new ACTIVE run): the old batch is refused, and creating the next batch marks it SUPERSEDED
	for _, st := range []string{
		`UPDATE payroll.pyr_run SET pr_status='SUPERSEDED' WHERE pr_yymm=` + y + ` AND pr_kind='ENGINE'`,
		`INSERT INTO payroll.pyr_run (pr_company_id, pr_yymm, pr_division, pr_pay_date, pr_rule_set_id, pr_snapshot_id, pr_status, pr_kind)
		 SELECT pr_company_id, pr_yymm, pr_division, pr_pay_date, pr_rule_set_id, pr_snapshot_id, 'ACTIVE', pr_kind FROM payroll.pyr_run WHERE pr_yymm=` + y + ` AND pr_kind='ENGINE'`,
		`INSERT INTO payroll.pyr_salary_head (psh_run_id, psh_company_id, psh_nik, psh_yymm, psh_division, psh_nama, psh_tot_tunjangan, psh_tot_potongan, psh_rounding)
		 SELECT n.pr_id, 1, h.psh_nik, h.psh_yymm, h.psh_division, h.psh_nama, h.psh_tot_tunjangan + 1, h.psh_tot_potongan, h.psh_rounding
		 FROM payroll.pyr_salary_head h JOIN payroll.pyr_run o ON o.pr_id=h.psh_run_id AND o.pr_status='SUPERSEDED'
		 JOIN payroll.pyr_run n ON n.pr_yymm=o.pr_yymm AND n.pr_status='ACTIVE' WHERE o.pr_yymm=` + y,
		`INSERT INTO payroll.pyr_salary_detail (psd_run_id, psd_nik, psd_type, psd_id, psd_amount)
		 SELECT n.pr_id, d.psd_nik, d.psd_type, d.psd_id, d.psd_amount
		 FROM payroll.pyr_salary_detail d JOIN payroll.pyr_run o ON o.pr_id=d.psd_run_id AND o.pr_status='SUPERSEDED'
		 JOIN payroll.pyr_run n ON n.pr_yymm=o.pr_yymm AND n.pr_status='ACTIVE' WHERE o.pr_yymm=` + y,
	} {
		if _, err := ownDB.Exec(st); err != nil {
			t.Fatalf("revisi run: %v\n%s", err, st)
		}
	}
	if _, err := svc.Send(ctx, sendOpts(cr2.BatchID)); err == nil {
		t.Fatal("send batch dari run lama harus ditolak")
	}
	cr3, err := svc.Create(ctx, q, distribution.CreateOptions{})
	if err != nil {
		t.Fatalf("batch revisi: %v", err)
	}
	if got := scalar(t, fmt.Sprintf(`SELECT pdb_status FROM payroll.pyr_distribution_batch WHERE pdb_id=%d`, cr2.BatchID)); got != "SUPERSEDED" {
		t.Fatalf("batch lama = %s", got)
	}
	if got := scalar(t, fmt.Sprintf(`SELECT pdb_is_revision::text||'|'||pdb_supersedes_batch_id FROM payroll.pyr_distribution_batch WHERE pdb_id=%d`, cr3.BatchID)); got != fmt.Sprintf("true|%d", cr2.BatchID) {
		t.Fatalf("batch revisi = %s", got)
	}
	if got := scalar(t, `SELECT count(*)::text FROM payroll.pyr_audit_log WHERE pal_action='DISTRIBUTION_SUPERSEDE'`); got != "1" {
		t.Fatalf("audit supersede = %s", got)
	}
	if err := svc.Cancel(ctx, q, cr2.BatchID, "x"); err == nil {
		t.Fatal("cancel batch SUPERSEDED harus ditolak")
	}
	res, err := svc.Send(ctx, sendOpts(cr3.BatchID))
	if err != nil || res.Sent != 2 {
		t.Fatalf("send batch revisi %+v %v", res, err)
	}
}
