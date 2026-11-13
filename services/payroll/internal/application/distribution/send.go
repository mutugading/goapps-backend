package distribution

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/textproto"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/report"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/pdf"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/smtp"
)

// SendOptions controls one send run.
type SendOptions struct {
	BatchID   int64
	Query     report.Query // company/division/yymm of the batch (AllowOpen for rehearsal)
	TestTo    string       // when set every mail goes here, items are not touched (gladi)
	Limit     int          // 0 = all (test mode: number of mails)
	Wait      bool         // keep going until FAILED items whose retry time has not come are done
	ReplyToHR string       // optional override of the template ReplyTo
	// SlipConfig carries the F-3 settings (PII key for NPWP/account, logo, texts). The PII key is required: the slip
	// attachment is rendered here.
	SlipConfig report.SlipConfig
}

// SendResult counts one run.
type SendResult struct {
	Sent, Failed, FinalFailed, Test, Waiting int
}

type claimed struct {
	id      int64
	nik     string
	email   string
	attempt int
	name    string
}

// Send delivers the due items of a batch. Items are claimed one by one with FOR UPDATE SKIP LOCKED and set to
// SENDING in their own committed transaction, so parallel workers never take the same item and a restart never
// re-sends an item that already left. A crash between SMTP and the status update leaves SENDING with a lease
// (pdi_next_attempt_at); after the lease the next send run recovers it under the same attempt and Message-ID.
func (s *Service) Send(ctx context.Context, o SendOptions) (SendResult, error) {
	var res SendResult
	if o.SlipConfig.PIIKey == "" {
		return res, fmt.Errorf("send: %w", report.ErrNoPIIKey)
	}
	src, tmpl, err := s.prepare(ctx, o)
	if err != nil {
		return res, err
	}
	period := report.PeriodName(o.Query.YYMM)
	if o.TestTo != "" {
		return s.sendTest(ctx, o, src, tmpl, period)
	}
	if err := s.markRunning(ctx, o); err != nil {
		return res, err
	}
	var loopErr error
	for o.Limit == 0 || res.Sent+res.Failed+res.FinalFailed < o.Limit {
		more, err := s.step(ctx, o, src, tmpl, period, &res)
		if err != nil {
			loopErr = err
			break
		}
		if !more {
			break
		}
	}
	// Counters and the DISTRIBUTION_SEND audit row are written even after Ctrl-C/SIGTERM or an error, so a mail that
	// left is always audited.
	ferr := s.finish(context.WithoutCancel(ctx), o)
	if loopErr != nil {
		return res, loopErr
	}
	return res, ferr
}

// step delivers one due item (or waits for a retry); more=false when the batch has nothing left to do now.
func (s *Service) step(ctx context.Context, o SendOptions, src *report.SlipSource, tmpl *mailTemplate, period string, res *SendResult) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	c, waiting, err := s.claim(ctx, o)
	if err != nil {
		return false, err
	}
	if c == nil {
		if !o.Wait || waiting == 0 {
			res.Waiting = waiting
			return false, nil
		}
		return true, s.Sleep(ctx, time.Second*time.Duration(waiting))
	}
	outcome, err := s.deliver(ctx, o, src, tmpl, period, *c, c.email)
	if err != nil {
		return false, err
	}
	res.tally(outcome)
	return true, s.throttle(ctx)
}

func (r *SendResult) tally(outcome string) {
	switch outcome {
	case StSent:
		r.Sent++
	case StFinal:
		r.FinalFailed++
	default:
		r.Failed++
	}
}

// throttle waits one rate-limit slot (pma_rate_per_minute).
func (s *Service) throttle(ctx context.Context) error {
	if s.Cfg.RatePerMin <= 0 {
		return nil
	}
	return s.Sleep(ctx, time.Minute/time.Duration(s.Cfg.RatePerMin))
}

func (s *Service) prepare(ctx context.Context, o SendOptions) (*report.SlipSource, *mailTemplate, error) {
	var src *report.SlipSource
	var tmpl *mailTemplate
	err := s.DB.ScopedTxOpts(ctx, snapshot(), func(tx *sql.Tx) error {
		var err error
		if src, err = loadBatchSource(ctx, tx, o.BatchID, o.Query, "send", o.SlipConfig); err != nil {
			return err
		}
		_, tmpl, err = loadMailTemplate(ctx, tx, o.Query.CompanyID)
		return err
	})
	return src, tmpl, err
}

func (s *Service) markRunning(ctx context.Context, o SendOptions) error {
	return s.DB.ScopedTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE payroll.pyr_distribution_batch SET pdb_status='RUNNING' WHERE pdb_id=$1 AND pdb_status='DRAFT'`, o.BatchID)
		return err
	})
}

// claim takes the next due item (QUEUED or FAILED with next_attempt_at <= now). waiting = seconds until the next
// retry when nothing is due but FAILED items remain (0 when none).
func (s *Service) claim(ctx context.Context, o SendOptions) (*claimed, int, error) {
	var c *claimed
	waiting := 0
	err := s.DB.ScopedTx(ctx, func(tx *sql.Tx) error {
		c, waiting = nil, 0
		now := s.Now().UTC()
		var cl claimed
		var prev string
		err := tx.QueryRowContext(ctx, `SELECT pdi_id, pdi_nik, COALESCE(pdi_email_to,''), pdi_attempts, COALESCE(pdi_attachment_name,''), pdi_status
FROM payroll.pyr_distribution_item
WHERE pdi_batch_id=$1 AND pdi_status IN ('QUEUED','FAILED','SENDING') AND COALESCE(pdi_next_attempt_at, $2) <= $2
ORDER BY pdi_id LIMIT 1 FOR UPDATE SKIP LOCKED`, o.BatchID, now).Scan(&cl.id, &cl.nik, &cl.email, &cl.attempt, &cl.name, &prev)
		if errors.Is(err, sql.ErrNoRows) {
			waiting, err = nextWait(ctx, tx, o.BatchID, now)
			return err
		}
		if err != nil {
			return fmt.Errorf("claim: %w", err)
		}
		// A SENDING item whose lease expired (process died, SIGTERM, outcome not recorded) is retried under the SAME
		// attempt number, hence the same Message-ID: if the first mail did leave, the recipient's mailbox can dedupe it.
		if prev != StSending {
			cl.attempt++
		}
		if _, err := tx.ExecContext(ctx, `UPDATE payroll.pyr_distribution_item SET pdi_status='SENDING', pdi_attempts=$2,
pdi_message_id=$3, pdi_next_attempt_at=$4 WHERE pdi_id=$1`, cl.id, cl.attempt, messageID(o.BatchID, cl.id, cl.attempt),
			now.Add(s.lease())); err != nil {
			return fmt.Errorf("mark sending: %w", err)
		}
		c = &cl
		return nil
	})
	return c, waiting, err
}

func nextWait(ctx context.Context, tx *sql.Tx, batch int64, now time.Time) (int, error) {
	var next sql.NullTime
	if err := tx.QueryRowContext(ctx, `SELECT min(pdi_next_attempt_at) FROM payroll.pyr_distribution_item
WHERE pdi_batch_id=$1 AND pdi_status IN ('QUEUED','FAILED','SENDING')`, batch).Scan(&next); err != nil {
		return 0, fmt.Errorf("next attempt: %w", err)
	}
	if !next.Valid {
		return 0, nil
	}
	return max(int(next.Time.Sub(now).Seconds())+1, 1), nil
}

// DefaultTestLimit is the number of slips a -test-to rehearsal sends unless -limit says otherwise (16§2.6 step 3).
const DefaultTestLimit = 3

// DefaultLease is how long a SENDING item is considered in flight before another run recovers it.
const DefaultLease = 15 * time.Minute

func (s *Service) lease() time.Duration {
	if s.Lease > 0 {
		return s.Lease
	}
	return DefaultLease
}

// messageID is <batch.item.attempt@payroll> (16§2.5): unique per attempt and stable for the same attempt.
func messageID(batch, item int64, attempt int) string {
	return fmt.Sprintf("<%d.%d.%d@payroll>", batch, item, attempt)
}

func (s *Service) buildMail(src *report.SlipSource, tmpl *mailTemplate, o SendOptions, period string, c claimed, to string, testPrefix bool) (smtp.Mail, []byte, error) {
	plain, enc, err := src.Render(c.nik, period)
	if err != nil {
		return smtp.Mail{}, nil, err
	}
	reply := s.Cfg.ReplyTo
	if o.ReplyToHR != "" {
		reply = o.ReplyToHR
	}
	subj, text, html, err := tmpl.render(MailData{PeriodName: period, Nama: src.Nama(c.nik), PasswordHint: passHint,
		Division: o.Query.Division, ReplyTo: reply, Yymm: fmt.Sprint(o.Query.YYMM), DocCode: docCode, NIK: c.nik})
	if err != nil {
		return smtp.Mail{}, nil, err
	}
	if testPrefix {
		subj = "[UJI] " + subj
	}
	return smtp.Mail{MessageID: messageID(o.BatchID, c.id, c.attempt), From: s.Cfg.From, FromName: s.Cfg.FromName,
		ReplyTo: reply, To: to, Subject: subj, Text: text, HTML: html, AttachName: c.name, Attach: enc, Date: s.Now()}, plain, nil
}

// deliver renders and sends one claimed item and records the outcome.
func (s *Service) deliver(ctx context.Context, o SendOptions, src *report.SlipSource, tmpl *mailTemplate, period string, c claimed, to string) (string, error) {
	m, plain, err := s.buildMail(src, tmpl, o, period, c, to, false)
	var resp string
	if err == nil {
		resp, err = s.Sender.Send(ctx, m)
	}
	status, next, lastErr := s.outcome(err, c.attempt)
	if err != nil {
		log.Warn().Int64("item", c.id).Int("attempt", c.attempt).Str("status", status).Str("error", lastErr).Msg("slip mail not sent")
	}
	attSHA := ""
	if status == StSent {
		attSHA = sha(m.Attach)
	}
	wctx := context.WithoutCancel(ctx) // a delivered mail must be recorded even when the run is interrupted
	uerr := s.DB.ScopedTx(wctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(wctx, `UPDATE payroll.pyr_distribution_item SET pdi_status=$2::text, pdi_next_attempt_at=$3,
pdi_smtp_response=NULLIF($4,''), pdi_last_error=NULLIF($5,''),
pdi_sent_at=CASE WHEN $2::text='SENT' THEN $6::timestamptz END,
pdi_attachment_sha256=NULLIF($7,''), pdi_content_sha256=NULLIF($8,'')
WHERE pdi_id=$1`, c.id, status, next, scrub(resp), lastErr, s.Now().UTC(), attSHA, hashOrEmpty(plain))
		return err
	})
	if uerr != nil {
		return "", fmt.Errorf("record outcome item %d: %w", c.id, uerr)
	}
	return status, nil
}

// outcome maps a send error to the item status, the next retry time, and a masked error text.
func (s *Service) outcome(err error, attempt int) (status string, next sql.NullTime, lastErr string) {
	switch {
	case err == nil:
		return StSent, next, ""
	case errors.Is(err, pdf.ErrNoBirthDate):
		return StNoPass, next, "" // data changed since create; no retry
	case attempt >= s.Cfg.MaxAttempts || permanent(err):
		return StFinal, next, scrub(err.Error())
	}
	d := backoff[min(attempt-1, len(backoff)-1)]
	return StFailed, sql.NullTime{Time: s.Now().UTC().Add(d), Valid: true}, scrub(err.Error())
}

// permanent reports an SMTP 5xx reply (no retry will help).
func permanent(err error) bool {
	var te *textproto.Error
	return errors.As(err, &te) && te.Code >= 500
}

func hashOrEmpty(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return pdf.ContentHash(b)
}

// finish recomputes the batch counters and marks it DONE when nothing is pending.
func (s *Service) finish(ctx context.Context, o SendOptions) error {
	return s.DB.ScopedTx(ctx, func(tx *sql.Tx) error {
		if err := recount(ctx, tx, o.BatchID); err != nil {
			return err
		}
		var pending int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM payroll.pyr_distribution_item
WHERE pdi_batch_id=$1 AND pdi_status IN ('QUEUED','SENDING','FAILED')`, o.BatchID).Scan(&pending); err != nil {
			return fmt.Errorf("pending: %w", err)
		}
		if pending == 0 {
			if _, err := tx.ExecContext(ctx, `UPDATE payroll.pyr_distribution_batch SET pdb_status='DONE', pdb_finished_at=$2
WHERE pdb_id=$1 AND pdb_status IN ('DRAFT','RUNNING')`, o.BatchID, s.Now().UTC()); err != nil {
				return err
			}
		}
		var sent, failed, blocked int
		if err := tx.QueryRowContext(ctx, `SELECT pdb_sent, pdb_failed, pdb_blocked FROM payroll.pyr_distribution_batch WHERE pdb_id=$1`, o.BatchID).
			Scan(&sent, &failed, &blocked); err != nil {
			return err
		}
		return s.audit(ctx, tx, o.Query.CompanyID, o.Query.Division, "DISTRIBUTION_SEND", o.BatchID,
			map[string]any{"sent": sent, "failed": failed, "blocked": blocked, "pending": pending, "allow_open": o.Query.AllowOpen})
	})
}

func recount(ctx context.Context, tx *sql.Tx, batch int64) error {
	_, err := tx.ExecContext(ctx, `UPDATE payroll.pyr_distribution_batch b SET
pdb_sent=(SELECT count(*) FROM payroll.pyr_distribution_item WHERE pdi_batch_id=b.pdb_id AND pdi_status='SENT'),
pdb_failed=(SELECT count(*) FROM payroll.pyr_distribution_item WHERE pdi_batch_id=b.pdb_id AND pdi_status IN ('FAILED','FAILED_FINAL')),
pdb_blocked=(SELECT count(*) FROM payroll.pyr_distribution_item WHERE pdi_batch_id=b.pdb_id AND pdi_status IN ('BLOCKED_NO_EMAIL','BLOCKED_NO_PASSWORD','EXCLUDED'))
WHERE b.pdb_id=$1`, batch)
	if err != nil {
		return fmt.Errorf("recount: %w", err)
	}
	return nil
}

// sendTest mails every QUEUED item to TestTo without touching item state (gladi). The recipient's own address is
// never used. The audit row records the count only.
func (s *Service) sendTest(ctx context.Context, o SendOptions, src *report.SlipSource, tmpl *mailTemplate, period string) (SendResult, error) {
	var res SendResult
	items, err := s.testItems(ctx, o.BatchID)
	if err != nil {
		return res, err
	}
	limit := o.Limit
	if limit <= 0 {
		limit = DefaultTestLimit
	}
	for i, c := range items {
		if i >= limit {
			break
		}
		c.attempt = 0
		m, _, err := s.buildMail(src, tmpl, o, period, c, o.TestTo, true)
		if err != nil {
			return res, fmt.Errorf("render item %d: %w", c.id, err)
		}
		m.MessageID = fmt.Sprintf("<test.%d.%d.%d@payroll>", o.BatchID, c.id, s.Now().UnixNano())
		if _, err := s.Sender.Send(ctx, m); err != nil {
			return res, fmt.Errorf("kirim uji item %d: %s", c.id, scrub(err.Error()))
		}
		res.Test++
		if err := s.throttle(ctx); err != nil {
			return res, err
		}
	}
	err = s.DB.ScopedTx(ctx, func(tx *sql.Tx) error {
		return s.audit(ctx, tx, o.Query.CompanyID, o.Query.Division, "DISTRIBUTION_TEST_SEND", o.BatchID,
			map[string]any{"jumlah": res.Test, "tujuan": MaskEmail(o.TestTo)})
	})
	return res, err
}

// ResendOptions selects one NIK of a batch.
type ResendOptions struct {
	BatchID int64
	NIK     string
	Reason  string
	Query   report.Query
	// AllowDuplicateEmail permits resending an item that was EXCLUDED for a shared address.
	AllowDuplicateEmail bool
}

// Resend queues a new item for nik (explicit action, reason required) using the employee's current address. The
// original item keeps its history. The next send run delivers it.
func (s *Service) Resend(ctx context.Context, o ResendOptions) (int64, error) {
	if strings.TrimSpace(o.Reason) == "" {
		return 0, errors.New("resend: alasan wajib (-reason)")
	}
	actor, err := actorOf(ctx)
	if err != nil {
		return 0, err
	}
	var newID int64
	err = s.DB.ScopedTxOpts(ctx, snapshot(), func(tx *sql.Tx) error {
		src, err := loadBatchSource(ctx, tx, o.BatchID, o.Query, "resend")
		if err != nil {
			return err
		}
		origID, status, err := lastItem(ctx, tx, o)
		if err != nil {
			return err
		}
		if status == StQueued || status == StSending || status == StFailed {
			return fmt.Errorf("item NIK %s masih %s; jalankan send (SENDING pulih otomatis setelah lease %s)", o.NIK, status, s.lease())
		}
		if status == StExcluded && !o.AllowDuplicateEmail {
			return fmt.Errorf("item NIK %s dikecualikan (email ganda); gunakan -allow-dup-email bila sudah dikonfirmasi", o.NIK)
		}
		e, err := resendTarget(ctx, tx, src, o)
		if err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `INSERT INTO payroll.pyr_distribution_item
(pdi_batch_id, pdi_employee_id, pdi_nik, pdi_email_to, pdi_status, pdi_next_attempt_at, pdi_attachment_name,
 pdi_resend_of_item_id, pdi_resend_reason, pdi_resend_by)
VALUES ($1,$2,$3,$4,'QUEUED',$5,$6,$7,$8,$9) RETURNING pdi_id`,
			o.BatchID, e.id, o.NIK, e.email, s.Now().UTC(), attachName(o.Query.YYMM, o.NIK), origID, o.Reason, actor).Scan(&newID); err != nil {
			return fmt.Errorf("insert resend: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE payroll.pyr_distribution_batch SET pdb_status='RUNNING', pdb_finished_at=NULL
WHERE pdb_id=$1 AND pdb_status='DONE'`, o.BatchID); err != nil {
			return err
		}
		return s.audit(ctx, tx, o.Query.CompanyID, o.Query.Division, "DISTRIBUTION_RESEND", o.BatchID,
			map[string]any{"item": newID, "asal": origID, "alasan": o.Reason})
	})
	return newID, err
}

// resendTarget finds the employee of a resend and checks that email and birth date exist.
// loadBatchSource checks that the batch is usable (exists in scope, not CANCELLED/SUPERSEDED) and still belongs to the
// ACTIVE run, and returns the slip source of that run. A SlipConfig (send only) makes the source decrypt NPWP/account.
func loadBatchSource(ctx context.Context, tx *sql.Tx, batchID int64, q report.Query, what string, cfg ...report.SlipConfig) (*report.SlipSource, error) {
	var status string
	var batchRun sql.NullInt64
	err := tx.QueryRowContext(ctx, `SELECT pdb_status, pdb_run_id FROM payroll.pyr_distribution_batch
WHERE pdb_id=$1 AND pdb_company_id=$2 AND pdb_division=$3 AND pdb_yymm=$4`, batchID, q.CompanyID, q.Division, q.YYMM).Scan(&status, &batchRun)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("batch %d tidak ada (atau di luar scope)", batchID)
	}
	if err != nil {
		return nil, fmt.Errorf("read batch: %w", err)
	}
	if status == "CANCELLED" || status == "SUPERSEDED" {
		return nil, fmt.Errorf("batch %d berstatus %s; %s ditolak", batchID, status, what)
	}
	src, err := report.LoadSlipSource(ctx, tx, q, cfg...)
	if err != nil {
		return nil, err
	}
	if batchRun.Valid && batchRun.Int64 != src.RunID() {
		return nil, fmt.Errorf("batch %d dibuat dari run %d, run ACTIVE sekarang %d; buat batch baru (slip lama tidak boleh dikirim dari run baru)", batchID, batchRun.Int64, src.RunID())
	}
	return src, nil
}

// lastItem locks the latest item of o.NIK in the batch.
func lastItem(ctx context.Context, tx *sql.Tx, o ResendOptions) (id int64, status string, err error) {
	err = tx.QueryRowContext(ctx, `SELECT i.pdi_id, i.pdi_status FROM payroll.pyr_distribution_item i
JOIN payroll.pyr_distribution_batch b ON b.pdb_id=i.pdi_batch_id
WHERE i.pdi_batch_id=$1 AND i.pdi_nik=$2 AND b.pdb_company_id=$3 ORDER BY i.pdi_id DESC LIMIT 1 FOR UPDATE OF i`,
		o.BatchID, o.NIK, o.Query.CompanyID).Scan(&id, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", fmt.Errorf("NIK %s tidak ada di batch %d", o.NIK, o.BatchID)
	}
	if err != nil {
		return 0, "", fmt.Errorf("read item: %w", err)
	}
	return id, status, nil
}

func resendTarget(ctx context.Context, tx *sql.Tx, src *report.SlipSource, o ResendOptions) (empRow, error) {
	emps, err := loadEmployees(ctx, tx, o.Query)
	if err != nil {
		return empRow{}, err
	}
	e, ok := emps[o.NIK]
	switch {
	case !ok || e.email == "":
		return empRow{}, fmt.Errorf("NIK %s tidak punya email; perbaiki data lalu ulangi", o.NIK)
	case !src.HasBirthDate(o.NIK):
		return empRow{}, fmt.Errorf("NIK %s tidak punya tanggal lahir; perbaiki data lalu ulangi", o.NIK)
	}
	return e, nil
}

// testItems lists the QUEUED/FAILED items of the batch for a rehearsal send.
func (s *Service) testItems(ctx context.Context, batch int64) ([]claimed, error) {
	var items []claimed
	err := s.DB.ScopedTx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT pdi_id, pdi_nik, COALESCE(pdi_attachment_name,'') FROM payroll.pyr_distribution_item
WHERE pdi_batch_id=$1 AND pdi_status IN ('QUEUED','FAILED') ORDER BY pdi_id`, batch)
		if err != nil {
			return fmt.Errorf("read items: %w", err)
		}
		defer closeRows(rows)
		for rows.Next() {
			var c claimed
			if err := rows.Scan(&c.id, &c.nik, &c.name); err != nil {
				return err
			}
			items = append(items, c)
		}
		return rows.Err()
	})
	return items, err
}
