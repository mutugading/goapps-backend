package e2e

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/distribution"
	"github.com/mutugading/goapps-backend/services/payroll/internal/application/report"
)

// T033k (review #283, tes): mailer on a period that is not APPROVED (AllowOpen) and double-send of one batch.
const (
	mail7YYMM = 203704 // OPEN period: refused unless AllowOpen
	mail8YYMM = 203705 // APPROVED period: two workers send the same batch
)

// seedMailerBase seeds password policy, template and two employees with an e-mail address, and resets them after.
func seedMailerBase(t *testing.T) {
	t.Helper()
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
	t.Cleanup(func() { // other e2e tests expect ZREP0002 without e-mail and birth date
		for _, q := range []string{`DELETE FROM payroll.pyr_distribution_item`, `DELETE FROM payroll.pyr_distribution_batch`,
			`UPDATE payroll.pyr_employee SET pe_birth_date=NULL, pe_email=NULL WHERE pe_nik='ZREP0002'`} {
			_, _ = ownDB.Exec(q) //nolint:errcheck // cleanup
		}
	})
}

// An OPEN period cannot be mailed: create and send are refused without AllowOpen, nothing is written or sent; with
// AllowOpen (rehearsal) both work and the audit rows record allow_open=true.
func TestMailer_AllowOpen(t *testing.T) {
	seedReportRun(t, mail7YYMM, "OPEN")
	seedMailerBase(t)
	fake := newFakeSMTP(t)
	now := time.Date(2037, 4, 1, 8, 0, 0, 0, time.UTC)
	svc := mailSvc(fake, &now)
	ctx := ctxFor(t, userRW)
	q := report.Query{CompanyID: 1, YYMM: mail7YYMM, Division: "SOLO"}
	batches := `SELECT count(*)::text FROM payroll.pyr_distribution_batch WHERE pdb_yymm=` + itoa(mail7YYMM)

	if _, err := svc.Create(ctx, q, distribution.CreateOptions{}); !errors.Is(err, report.ErrPeriodNotApproved) {
		t.Fatalf("create on OPEN period without AllowOpen: %v", err)
	}
	if got := scalar(t, batches); got != "0" {
		t.Fatalf("refused create left %s batches", got)
	}

	q.AllowOpen = true
	cr, err := svc.Create(ctx, q, distribution.CreateOptions{})
	if err != nil {
		t.Fatalf("create with AllowOpen: %v", err)
	}
	noOpen := q
	noOpen.AllowOpen = false
	_, err = svc.Send(ctx, distribution.SendOptions{SlipConfig: report.SlipConfig{PIIKey: encKey}, BatchID: cr.BatchID, Query: noOpen})
	if !errors.Is(err, report.ErrPeriodNotApproved) || fake.count() != 0 {
		t.Fatalf("send on OPEN period without AllowOpen: err=%v sent=%d", err, fake.count())
	}
	res, err := svc.Send(ctx, distribution.SendOptions{SlipConfig: report.SlipConfig{PIIKey: encKey}, BatchID: cr.BatchID, Query: q})
	if err != nil || res.Sent != 2 || fake.count() != 2 {
		t.Fatalf("send with AllowOpen: %+v %v count=%d", res, err, fake.count())
	}
	for _, act := range []string{"DISTRIBUTION_CREATE", "DISTRIBUTION_SEND"} {
		got := scalar(t, `SELECT count(*)::text FROM payroll.pyr_audit_log WHERE pal_action='`+act+`' AND pal_after->>'allow_open'='true'`)
		if got == "0" {
			t.Fatalf("%s audit row does not record allow_open", act)
		}
	}
}

// Two workers sending the same batch at once (a second terminal, a Job restart) deliver each slip exactly once:
// items are claimed with FOR UPDATE SKIP LOCKED, and a second full run afterwards sends nothing.
func TestMailer_DoubleSend(t *testing.T) {
	seedReportRun(t, mail8YYMM, "APPROVED")
	seedMailerBase(t)
	fake := newFakeSMTP(t)
	nowA, nowB := time.Date(2037, 5, 1, 8, 0, 0, 0, time.UTC), time.Date(2037, 5, 1, 8, 0, 0, 0, time.UTC)
	a, b := mailSvc(fake, &nowA), mailSvc(fake, &nowB)
	ctx := ctxFor(t, userRW)
	q := report.Query{CompanyID: 1, YYMM: mail8YYMM, Division: "SOLO"}
	cr, err := a.Create(ctx, q, distribution.CreateOptions{})
	if err != nil || cr.Queued != 2 {
		t.Fatalf("create: %+v %v", cr, err)
	}

	opts := distribution.SendOptions{SlipConfig: report.SlipConfig{PIIKey: encKey}, BatchID: cr.BatchID, Query: q}
	var wg sync.WaitGroup
	results := make([]distribution.SendResult, 2)
	errs := make([]error, 2)
	for i, svc := range []*distribution.Service{a, b} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = svc.Send(context.WithoutCancel(ctx), opts)
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("worker %d: %v", i, err)
		}
	}
	if sent := results[0].Sent + results[1].Sent; sent != 2 {
		t.Fatalf("sent by both workers = %d, want 2 (%+v)", sent, results)
	}
	if fake.count() != 2 {
		t.Fatalf("SMTP messages = %d, want exactly one per employee", fake.count())
	}
	to := strings.Join(fake.to(), ",")
	if strings.Count(to, "satu@example.test") != 1 || strings.Count(to, "dua@example.test") != 1 {
		t.Fatalf("recipients = %s", to)
	}
	if got := scalar(t, `SELECT count(*)::text FROM payroll.pyr_distribution_item WHERE pdi_batch_id=`+itoa(int(cr.BatchID))+` AND pdi_status='SENT' AND pdi_attempts=1`); got != "2" {
		t.Fatalf("items SENT with one attempt = %s", got)
	}
	again, err := a.Send(ctx, opts)
	if err != nil || again.Sent+again.Failed != 0 || fake.count() != 2 {
		t.Fatalf("third run: %+v %v count=%d", again, err, fake.count())
	}
}
