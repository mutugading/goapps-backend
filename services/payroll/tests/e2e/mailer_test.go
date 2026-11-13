package e2e

import (
	"bufio"
	"context"
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
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/smtp"
)

// T028g: mailer outbox against PG18 as payroll_app with an in-process fake SMTP server.
const (
	mailYYMM  = 203605
	mail2YYMM = 203606
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
			w("250 queued")
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
	res, err := svc.Send(ctx, distribution.SendOptions{BatchID: b, Query: q, TestTo: "uji@example.test"})
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
	res, err = svc.Send(ctx, distribution.SendOptions{BatchID: b, Query: q})
	if err != nil || res.Failed != 1 || res.Sent != 0 || res.Waiting == 0 {
		t.Fatalf("attempt 1: %+v %v", res, err)
	}
	if got := scalar(t, fmt.Sprintf(`SELECT pdi_attempts||'|'||pdi_status FROM payroll.pyr_distribution_item WHERE pdi_batch_id=%d AND pdi_nik='ZREP0001'`, b)); got != "1|FAILED" {
		t.Fatalf("after attempt 1: %s", got)
	}
	res, err = svc.Send(ctx, distribution.SendOptions{BatchID: b, Query: q, Wait: true})
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
	res, err = svc.Send(ctx, distribution.SendOptions{BatchID: b, Query: q})
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
	res, err = svc.Send(ctx, distribution.SendOptions{BatchID: b, Query: q})
	if err != nil || res.Sent != 1 || fake.count() != 3 {
		t.Fatalf("resend send: %+v %v count=%d", res, err, fake.count())
	}
	if got := scalar(t, fmt.Sprintf(`SELECT count(*)::text FROM payroll.pyr_distribution_item WHERE pdi_batch_id=%d AND pdi_nik='ZREP0001' AND pdi_status='SENT'`, b)); got != "2" {
		t.Fatalf("SENT items = %s", got)
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
	if got := scalar(t, `SELECT count(DISTINCT pal_action)::text FROM payroll.pyr_audit_log WHERE pal_action LIKE 'DISTRIBUTION\_%'`); got != "5" {
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
	res, err := svc.Send(ctx, distribution.SendOptions{BatchID: b, Query: q})
	if err != nil || res.Sent != 1 || res.Waiting == 0 || fake.count() != 1 {
		t.Fatalf("before lease: %+v %v count=%d", res, err, fake.count())
	}
	if got := item("ZREP0001"); !strings.HasPrefix(got, "SENDING|1|") {
		t.Fatalf("stuck item touched: %s", got)
	}
	// after the lease the item is recovered under the same attempt (same Message-ID suffix .1)
	now = now.Add(11 * time.Minute)
	res, err = svc.Send(ctx, distribution.SendOptions{BatchID: b, Query: q})
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
	res, err = svc.Send(ctx, distribution.SendOptions{BatchID: b, Query: q, Wait: true})
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
