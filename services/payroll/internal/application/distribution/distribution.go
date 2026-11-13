// Package distribution is the slip e-mail outbox of cmd/mailer (16§2.5-2.6, D-62 butir 5): create a batch with the
// pre-check, send with SKIP LOCKED / rate / retry, resend with a reason, and a batch report. The password-protected
// PDF is rendered at send time and never stored; passwords, PDF bytes and full addresses never reach logs or audit.
package distribution

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	htmltpl "html/template"
	"regexp"
	"strings"
	texttpl "text/template"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/report"
	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/audit"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/config"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/smtp"
)

// DocType is the only document distributed in this task.
const DocType = "SLIP_GAJI"

// docCode is the DocCode of the attachment name <yymm>G<nik>.pdf (same as legacy).
const docCode = "G"

// Item statuses (pyr_distribution_item.pdi_status).
const (
	StQueued   = "QUEUED"
	StSending  = "SENDING"
	StSent     = "SENT"
	StFailed   = "FAILED"
	StFinal    = "FAILED_FINAL"
	StNoEmail  = "BLOCKED_NO_EMAIL"
	StNoPass   = "BLOCKED_NO_PASSWORD" //nolint:gosec // status name, not a credential
	StExcluded = "EXCLUDED"

	// Batch statuses that end a batch (pyr_distribution_batch.pdb_status).
	StCancelled  = "CANCELLED"
	StSuperseded = "SUPERSEDED"

	reasonDupe  = "EMAIL_DUPLIKAT"
	passHint    = "tanggal lahir Anda, format DDMMYYYY" //nolint:gosec // hint text, not a credential
	auditEntity = "pyr_distribution_batch"
)

// backoff is the retry delay after attempt n (16§2.5: 1m, 5m, 30m, 2j).
var backoff = []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour}

// Service runs the distribution use cases.
type Service struct {
	DB     run.TxRunner
	Audit  audit.Writer
	Sender smtp.Sender
	Cfg    config.SMTPConfig
	Now    func() time.Time
	Sleep  func(ctx context.Context, d time.Duration) error
	Lease  time.Duration // SENDING lease; 0 = DefaultLease
}

// NewService creates a Service with the wall clock and a context-aware sleep.
func NewService(db run.TxRunner, w audit.Writer, sender smtp.Sender, cfg config.SMTPConfig) *Service {
	return &Service{DB: db, Audit: w, Sender: sender, Cfg: cfg, Now: time.Now, Sleep: sleepCtx}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// CreateOptions tunes the pre-check.
type CreateOptions struct {
	// AllowDuplicateEmail sends to addresses shared by several NIKs (otherwise those items are EXCLUDED, 16§2.7).
	AllowDuplicateEmail bool
}

// CreateResult is the batch with its pre-check counts.
type CreateResult struct {
	BatchID                                      int64
	Total, Queued, NoEmail, NoPassword, Dup      int
	NoEmailNIK, NoPasswordNIK, DuplicateEmailNIK []string // for the HR follow-up list (stdout only, never audited)
}

// Create builds a DRAFT-to-send batch for the ACTIVE run of q: one item per employee, pre-checked (no email, shared
// email, no birth date). Creating twice for the same run is refused; use resend for individual NIKs.
func (s *Service) Create(ctx context.Context, q report.Query, o CreateOptions) (CreateResult, error) {
	var res CreateResult
	err := s.DB.ScopedTxOpts(ctx, run.SnapshotTx, func(tx *sql.Tx) error {
		res = CreateResult{}
		return s.createInTx(ctx, tx, q, o, &res)
	})
	return res, err
}

type empRow struct {
	id    int64
	email string
}

func (s *Service) createInTx(ctx context.Context, tx *sql.Tx, q report.Query, o CreateOptions, res *CreateResult) error {
	src, err := report.LoadSlipSource(ctx, tx, q)
	if err != nil {
		return err
	}
	superseded, err := s.supersedeStale(ctx, tx, q, src.RunID())
	if err != nil {
		return err
	}
	var existing int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM payroll.pyr_distribution_batch
WHERE pdb_company_id=$1 AND pdb_division=$2 AND pdb_doc_type=$3 AND pdb_yymm=$4 AND pdb_status NOT IN ('CANCELLED','SUPERSEDED')`,
		q.CompanyID, q.Division, DocType, q.YYMM).Scan(&existing); err != nil {
		return fmt.Errorf("check batch: %w", err)
	}
	if existing > 0 {
		return fmt.Errorf("batch %s %d %s sudah ada; gunakan resend untuk NIK tertentu atau batalkan batch lama", DocType, q.YYMM, q.Division)
	}
	tmplID, _, err := loadMailTemplate(ctx, tx, q.CompanyID)
	if err != nil {
		return err
	}
	emps, err := loadEmployees(ctx, tx, q)
	if err != nil {
		return err
	}
	niks := src.NIKs()
	count := map[string]int{}
	for _, n := range niks {
		if e, ok := emps[n]; ok && e.email != "" {
			count[strings.ToLower(e.email)]++
		}
	}
	id, err := insertBatch(ctx, tx, q, src.RunID(), tmplID, len(niks), superseded)
	if err != nil {
		return err
	}
	res.BatchID, res.Total = id, len(niks)
	for _, n := range niks {
		e, ok := emps[n]
		if !ok {
			return fmt.Errorf("NIK %s ada di run tetapi tidak di master karyawan", n)
		}
		status, errTxt := classify(src, e, n, count, o, res)
		if _, err := tx.ExecContext(ctx, `INSERT INTO payroll.pyr_distribution_item
(pdi_batch_id, pdi_employee_id, pdi_nik, pdi_email_to, pdi_status, pdi_next_attempt_at, pdi_last_error, pdi_attachment_name)
VALUES ($1,$2,$3,NULLIF($4,''),$5::text,CASE WHEN $5::text='QUEUED' THEN $6::timestamptz END,NULLIF($7,''),$8)`,
			id, e.id, n, e.email, status, s.Now().UTC(), errTxt, attachName(q.YYMM, n)); err != nil {
			return fmt.Errorf("insert item: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE payroll.pyr_distribution_batch SET pdb_blocked=$2 WHERE pdb_id=$1`,
		id, res.NoEmail+res.NoPassword+res.Dup); err != nil {
		return fmt.Errorf("update batch: %w", err)
	}
	return s.audit(ctx, tx, q.CompanyID, q.Division, "DISTRIBUTION_CREATE", id, map[string]any{
		"yymm": q.YYMM, "total": res.Total, "queued": res.Queued, "tanpa_email": res.NoEmail,
		"tanpa_password": res.NoPassword, "email_ganda": res.Dup, "allow_open": q.AllowOpen})
}

// classify is the pre-check of one item and counts it into res.
func classify(src *report.SlipSource, e empRow, nik string, count map[string]int, o CreateOptions, res *CreateResult) (status, errTxt string) {
	switch {
	case e.email == "":
		res.NoEmail++
		res.NoEmailNIK = append(res.NoEmailNIK, nik)
		return StNoEmail, ""
	case !src.HasBirthDate(nik):
		res.NoPassword++
		res.NoPasswordNIK = append(res.NoPasswordNIK, nik)
		return StNoPass, ""
	case count[strings.ToLower(e.email)] > 1 && !o.AllowDuplicateEmail:
		res.Dup++
		res.DuplicateEmailNIK = append(res.DuplicateEmailNIK, nik)
		return StExcluded, reasonDupe
	}
	res.Queued++
	return StQueued, ""
}

func attachName(yymm int, nik string) string { return fmt.Sprintf("%d%s%s.pdf", yymm, docCode, nik) }

func insertBatch(ctx context.Context, tx *sql.Tx, q report.Query, runID, tmplID int64, total int, supersedes int64) (int64, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return 0, err
	}
	var id int64
	err = tx.QueryRowContext(ctx, `INSERT INTO payroll.pyr_distribution_batch
(pdb_company_id, pdb_division, pdb_doc_type, pdb_yymm, pdb_run_id, pdb_status, pdb_total, pdb_mail_template_id, created_by,
 pdb_is_revision, pdb_supersedes_batch_id)
VALUES ($1,$2,$3,$4,$5,'DRAFT',$6,$7,$8,$9::bigint IS NOT NULL,$9) RETURNING pdb_id`,
		q.CompanyID, q.Division, DocType, q.YYMM, runID, total, tmplID, actor, sql.NullInt64{Int64: supersedes, Valid: supersedes > 0}).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert batch: %w", err)
	}
	return id, nil
}

func loadEmployees(ctx context.Context, tx *sql.Tx, q report.Query) (map[string]empRow, error) {
	rows, err := tx.QueryContext(ctx, `SELECT pe_nik, pe_id, COALESCE(btrim(pe_email),'') FROM payroll.pyr_employee
WHERE pe_company_id=$1 AND pe_division=$2`, q.CompanyID, q.Division)
	if err != nil {
		return nil, fmt.Errorf("read employees: %w", err)
	}
	defer closeRows(rows)
	out := map[string]empRow{}
	for rows.Next() {
		var nik string
		var e empRow
		if err := rows.Scan(&nik, &e.id, &e.email); err != nil {
			return nil, fmt.Errorf("scan employee: %w", err)
		}
		out[nik] = e
	}
	return out, rows.Err()
}

func closeRows(rows *sql.Rows) {
	if err := rows.Close(); err != nil {
		log.Warn().Err(err).Msg("failed to close rows")
	}
}

func actorOf(ctx context.Context) (string, error) {
	// the audit writer enforces the identity; here only the actor name is needed for created_by
	id, ok := identityFrom(ctx)
	if !ok {
		return "", errors.New("distribution: tanpa identitas")
	}
	return id, nil
}

func (s *Service) audit(ctx context.Context, tx *sql.Tx, company int16, division, action string, batchID int64, after map[string]any) error {
	return s.Audit.Write(ctx, tx, audit.Entry{CompanyID: company, Division: division, Action: action,
		Entity: auditEntity, EntityKey: fmt.Sprint(batchID), After: after})
}

// mailTemplate holds the parsed ACTIVE template.
type mailTemplate struct {
	subject *texttpl.Template
	text    *texttpl.Template
	html    *htmltpl.Template
}

// MailData are the fields available to the template (16§2.5).
type MailData struct {
	PeriodName, Nama, PasswordHint, Division, ReplyTo, Yymm, DocCode, NIK string
}

func loadMailTemplate(ctx context.Context, tx *sql.Tx, company int16) (int64, *mailTemplate, error) {
	var id int64
	var subj, bodyHTML string
	var bodyText sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT pmt_id, pmt_subject, pmt_body_html, pmt_body_text FROM payroll.pyr_mail_template
WHERE pmt_company_id=$1 AND pmt_doc_type=$2 AND pmt_status='ACTIVE' ORDER BY pmt_version DESC LIMIT 1`, company, DocType).
		Scan(&id, &subj, &bodyHTML, &bodyText)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil, errors.New("tidak ada template email SLIP_GAJI ACTIVE")
	}
	if err != nil {
		return 0, nil, fmt.Errorf("read mail template: %w", err)
	}
	mt := &mailTemplate{}
	if mt.subject, err = texttpl.New("subject").Option("missingkey=error").Parse(subj); err != nil {
		return 0, nil, fmt.Errorf("template subject: %w", err)
	}
	if mt.html, err = htmltpl.New("html").Option("missingkey=error").Parse(bodyHTML); err != nil {
		return 0, nil, fmt.Errorf("template html: %w", err)
	}
	if bodyText.Valid && bodyText.String != "" {
		if mt.text, err = texttpl.New("text").Option("missingkey=error").Parse(bodyText.String); err != nil {
			return 0, nil, fmt.Errorf("template text: %w", err)
		}
	}
	return id, mt, nil
}

func (t *mailTemplate) render(d MailData) (subject, text, html string, err error) {
	var sb, tb, hb bytes.Buffer
	if err = t.subject.Execute(&sb, d); err != nil {
		return "", "", "", err
	}
	if t.text != nil {
		if err = t.text.Execute(&tb, d); err != nil {
			return "", "", "", err
		}
	}
	if err = t.html.Execute(&hb, d); err != nil {
		return "", "", "", err
	}
	return strings.TrimSpace(sb.String()), tb.String(), hb.String(), nil
}

var emailRe = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+`)

// MaskEmail turns s***@gmail.com style (16§2.7).
func MaskEmail(a string) string {
	i := strings.LastIndex(a, "@")
	if i <= 0 {
		return "***"
	}
	return a[:1] + "***" + a[i:]
}

// scrub masks every address in an error text before it is stored or logged.
func scrub(s string) string {
	s = emailRe.ReplaceAllStringFunc(s, MaskEmail)
	s = strings.ToValidUTF8(s, "?")
	if r := []rune(s); len(r) > 500 {
		s = string(r[:500])
	}
	return s
}

func sha(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
