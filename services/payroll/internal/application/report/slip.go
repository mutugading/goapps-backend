package report

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/pdf"
)

// SlipOptions selects the slip output. NoPassword writes unencrypted PDFs for finance (printing); the audit row
// records it.
type SlipOptions struct {
	NIK        string // empty = all employees of the division
	NoPassword bool
	OutDir     string
	PeriodName string
}

// SlipConfig is the runtime configuration of the F-3 layout (read from env by the CLI, never stored in the DB or the
// repository). PIIKey decrypts NPWP and the bank account in memory; without it both print empty (cmd/mailer until it
// is wired). Logo is the PNG/JPEG bytes of the letterhead (empty = text only).
type SlipConfig struct {
	PIIKey    string
	OnlyNIK   string // when set, NPWP/account are decrypted for this NIK only (PII minimisation)
	Logo      []byte
	Perhatian []string
	ContactIT string
}

// DefaultPerhatian is the confidentiality text used when PAYROLL_SLIP_PERHATIAN is not set.
var DefaultPerhatian = []string{
	"Slip gaji ini bersifat RAHASIA dan hanya untuk yang bersangkutan. Dilarang menyebarkan isi slip gaji kepada pihak lain.",
	"Bila ada ketidaksesuaian data, segera hubungi bagian HR atau IT.",
}

// SlipResult reports a slip run. Skipped lists NIKs without a birth date (no password can be derived).
type SlipResult struct {
	Written []string
	Skipped []string
}

// slipExtra is the per-NIK data outside RunData.
type slipExtra struct {
	Jabatan, Dept, Grade, TaxCode string
	NPWP, BankAccount, Currency   string // NPWP and BankAccount are decrypted plaintext: memory only
	PayDate                       *time.Time
	DOB                           *time.Time
	Info                          pdf.Attendance
}

// slipDetail is the Basis/Remarks of one detail line.
type slipDetail struct{ Basis, Remarks string }

// RoundMoney is Oracle ROUND half-up to 2 decimals (D-07).
func roundMoney(d decimal.Decimal) decimal.Decimal { return d.Round(2) }

// periodHeader renders 202701 as "Jan - 27" (F-3 header).
func periodHeader(yymm int) string {
	m := time.Month(yymm % 100)
	if m < 1 || m > 12 {
		return fmt.Sprint(yymm)
	}
	return fmt.Sprintf("%s - %02d", m.String()[:3], (yymm/100)%100)
}

// payDateText renders the payment date as DD-MON-YY (upper case, e.g. 26-JAN-27); "" when unknown.
func payDateText(t *time.Time) string {
	if t == nil {
		return ""
	}
	return strings.ToUpper(t.Format("02-Jan-06"))
}

// BuildSlip assembles the content of one slip from a loaded run (F-3). Lines follow pcm_serial; zero lines are
// dropped; Basis and Remarks come from pyr_salary_detail.
func BuildSlip(d RunData, e Emp, x slipExtra, det map[string]slipDetail, cfg SlipConfig, company string, yymm int, periodName string, approvedAt time.Time) pdf.Slip {
	perhatian := cfg.Perhatian
	if len(perhatian) == 0 {
		perhatian = DefaultPerhatian
	}
	s := pdf.Slip{Company: company, PeriodName: periodName, Period: periodHeader(yymm),
		NIK: e.NIK, Nama: e.Nama, Dept: x.Dept, Grade: x.Grade, Jabatan: x.Jabatan, TaxCode: x.TaxCode,
		NPWP: x.NPWP, BankAccount: x.BankAccount, PayDate: payDateText(x.PayDate), Currency: x.Currency,
		TotTunjangan: e.TotTunjangan, TotPotongan: e.TotPotongan, Rounding: e.Rounding, NetPay: e.NetPay,
		Info: x.Info, Logo: cfg.Logo, Perhatian: perhatian, ContactIT: cfg.ContactIT, ApprovedAt: approvedAt}
	for _, c := range d.Comps { // already in pcm_serial order
		v, ok := e.Amounts[c.Key()]
		if !ok || v.IsZero() {
			continue
		}
		l := pdf.Line{Name: c.Name, Amount: v, Basis: det[c.Key()].Basis, Remarks: det[c.Key()].Remarks}
		if c.Type == "T" {
			s.Tunjangan = append(s.Tunjangan, l)
		} else {
			s.Potongan = append(s.Potongan, l)
		}
	}
	return s
}

// PeriodName renders 202701 as "Januari 2027".
func PeriodName(yymm int) string {
	months := []string{"Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}
	m := yymm % 100
	if m < 1 || m > 12 {
		return fmt.Sprint(yymm)
	}
	return fmt.Sprintf("%s %d", months[m-1], 2000+yymm/100)
}

// SlipSource renders the slips of one loaded run (cmd/mailer renders per item without reloading the run).
type SlipSource struct {
	d          RunData
	company    string
	policy     string
	yymm       int
	approvedAt time.Time
	cfg        SlipConfig
	extra      map[string]slipExtra
	detail     map[string]map[string]slipDetail
	idx        map[string]int
}

// LoadSlipSource loads the ACTIVE run of q (same checks as Load) and the slip header data inside tx. At most one
// SlipConfig may be given; without it the NPWP and bank account stay empty and the slip has the default text.
func LoadSlipSource(ctx context.Context, tx *sql.Tx, q Query, cfgs ...SlipConfig) (*SlipSource, error) {
	var cfg SlipConfig
	if len(cfgs) > 0 {
		cfg = cfgs[0]
	}
	d, err := Load(ctx, tx, q)
	if err != nil {
		return nil, err
	}
	company, policy, approvedAt, err := slipHeader(ctx, tx, q, d.RunID)
	if err != nil {
		return nil, err
	}
	extra, err := loadSlipExtra(ctx, tx, q, d.RunID, cfg.PIIKey, cfg.OnlyNIK)
	if err != nil {
		return nil, err
	}
	detail, err := loadSlipDetail(ctx, tx, d.RunID)
	if err != nil {
		return nil, err
	}
	idx := make(map[string]int, len(d.Emps))
	for i, e := range d.Emps {
		idx[e.NIK] = i
	}
	return &SlipSource{d: d, company: company, policy: policy, yymm: q.YYMM, approvedAt: approvedAt, cfg: cfg,
		extra: extra, detail: detail, idx: idx}, nil
}

// NIKs lists the employees of the run in NIK order.
func (s *SlipSource) NIKs() []string {
	out := make([]string, len(s.d.Emps))
	for i, e := range s.d.Emps {
		out[i] = e.NIK
	}
	return out
}

// Nama is the employee name on the slip ("" for an unknown NIK).
func (s *SlipSource) Nama(nik string) string {
	if i, ok := s.idx[nik]; ok {
		return s.d.Emps[i].Nama
	}
	return ""
}

// HasBirthDate reports whether the password can be derived for nik.
func (s *SlipSource) HasBirthDate(nik string) bool {
	x, ok := s.extra[nik]
	return ok && x.DOB != nil && !x.DOB.IsZero()
}

// RunID is the run the slips come from.
func (s *SlipSource) RunID() int64 { return s.d.RunID }

func (s *SlipSource) renderPlain(nik, periodName string) ([]byte, slipExtra, error) {
	i, ok := s.idx[nik]
	if !ok {
		return nil, slipExtra{}, fmt.Errorf("NIK %s tidak ada di run", nik)
	}
	x := s.extra[nik]
	plain, err := pdf.Render(BuildSlip(s.d, s.d.Emps[i], x, s.detail[nik], s.cfg, s.company, s.yymm, periodName, s.approvedAt))
	return plain, x, err
}

// Render returns the plain PDF (for the content hash) and the password-protected PDF of nik. A missing birth date
// yields pdf.ErrNoBirthDate; an unknown NIK is an error.
func (s *SlipSource) Render(nik, periodName string) (plain, enc []byte, err error) {
	plain, x, err := s.renderPlain(nik, periodName)
	if err != nil {
		return nil, nil, err
	}
	pw, err := pdf.Password(s.policy, x.DOB)
	if err != nil {
		return nil, nil, err
	}
	enc, err = pdf.Encrypt(plain, pw)
	return plain, enc, err
}

// stagedFiles holds files written under a .tmp name until the audit row has committed.
type stagedFiles struct{ finals []string }

func (f *stagedFiles) add(final string, data []byte) error {
	f.finals = append(f.finals, final)
	return os.WriteFile(final+".tmp", data, 0o600)
}

func (f *stagedFiles) discard() {
	for _, p := range f.finals {
		_ = os.Remove(p + ".tmp") //nolint:errcheck // best effort, may not exist
	}
}

func (f *stagedFiles) commit() error {
	for _, p := range f.finals {
		if err := os.Rename(p+".tmp", p); err != nil {
			return err
		}
	}
	return nil
}

// ExportSlips renders (and unless NoPassword encrypts) one PDF per employee into OutDir as <yymm>G<nik>.pdf. Files are
// written to a temp name and renamed only after the audit row committed. Employees without a birth date are skipped
// and reported. The PII key is required (NPWP and bank account are decrypted in memory). The audit row holds counts
// only: no NIK, NPWP, or account number.
func (s *Service) ExportSlips(ctx context.Context, q Query, cfg SlipConfig, o SlipOptions) (SlipResult, error) {
	if cfg.PIIKey == "" {
		return SlipResult{}, ErrNoPIIKey
	}
	var res SlipResult
	var files stagedFiles
	err := s.DB.ScopedTxOpts(ctx, run.SnapshotTx, func(tx *sql.Tx) error {
		res, files = SlipResult{}, stagedFiles{} // the tx may be retried
		if err := s.slipsInTx(ctx, tx, q, cfg, o, &res, &files); err != nil {
			return err
		}
		return s.writeAudit(ctx, tx, q, "slip", len(res.Written), map[string]any{
			"tanpa_password": o.NoPassword, "dilewati": len(res.Skipped), "filter_nik": o.NIK != ""})
	})
	if err == nil {
		err = files.commit()
	}
	if err != nil {
		files.discard()
		return SlipResult{}, err
	}
	sort.Strings(res.Skipped)
	return res, nil
}

func (s *Service) slipsInTx(ctx context.Context, tx *sql.Tx, q Query, cfg SlipConfig, o SlipOptions, res *SlipResult, files *stagedFiles) error {
	cfg.OnlyNIK = o.NIK
	src, err := LoadSlipSource(ctx, tx, q, cfg)
	if err != nil {
		return err
	}
	niks := src.NIKs()
	if o.NIK != "" {
		if _, ok := src.idx[o.NIK]; !ok {
			return fmt.Errorf("NIK %s tidak ada di run", o.NIK)
		}
		niks = []string{o.NIK}
	}
	for _, nik := range niks {
		var out []byte
		if o.NoPassword {
			out, _, err = src.renderPlain(nik, o.PeriodName)
		} else {
			_, out, err = src.Render(nik, o.PeriodName)
		}
		if errors.Is(err, pdf.ErrNoBirthDate) {
			res.Skipped = append(res.Skipped, nik)
			continue
		}
		if err != nil {
			return err
		}
		final, err := slipPath(o.OutDir, q.YYMM, nik)
		if err != nil {
			return err
		}
		if err := files.add(final, out); err != nil {
			return err
		}
		res.Written = append(res.Written, nik)
	}
	return nil
}

// slipPath is <dir>/<yymm>G<nik>.pdf; a NIK that could escape dir is rejected.
func slipPath(dir string, yymm int, nik string) (string, error) {
	if nik == "" || strings.ContainsAny(nik, `/\`) || strings.Contains(nik, "..") {
		return "", fmt.Errorf("NIK tidak valid untuk nama berkas: %q", nik)
	}
	return filepath.Join(dir, fmt.Sprintf("%dG%s.pdf", yymm, nik)), nil
}

func slipHeader(ctx context.Context, tx *sql.Tx, q Query, runID int64) (company, policy string, approvedAt time.Time, err error) {
	var ap sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT c.pco_name, p.approved_at FROM payroll.pyr_company c
JOIN payroll.pyr_period p ON p.pp_company_id = c.pco_company_id AND p.pp_yymm = $2 AND p.pp_division = $3
WHERE c.pco_company_id = $1`, q.CompanyID, q.YYMM, q.Division).Scan(&company, &ap)
	if err != nil {
		return "", "", time.Time{}, fmt.Errorf("read company/period: %w", err)
	}
	if ap.Valid {
		approvedAt = ap.Time
	} else { // gladi (-allow-open): deterministic fallback tied to the run, not to the wall clock
		if err = tx.QueryRowContext(ctx, `SELECT COALESCE(pr_finished_at, pr_started_at) FROM payroll.pyr_run WHERE pr_id = $1`, runID).Scan(&approvedAt); err != nil {
			return "", "", time.Time{}, fmt.Errorf("read run time: %w", err)
		}
	}
	err = tx.QueryRowContext(ctx, `SELECT pspp_policy_code FROM payroll.pyr_slip_password_policy
WHERE pspp_company_id = $1 AND pspp_status = 'ACTIVE' AND pspp_valid_from <= $2::date
ORDER BY pspp_valid_from DESC LIMIT 1`, q.CompanyID, approvedAt).Scan(&policy)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", time.Time{}, errors.New("tidak ada kebijakan password slip ACTIVE")
	}
	if err != nil {
		return "", "", time.Time{}, fmt.Errorf("read policy: %w", err)
	}
	return company, policy, approvedAt.UTC(), nil
}

func loadSlipExtra(ctx context.Context, tx *sql.Tx, q Query, runID int64, piiKey, onlyNIK string) (map[string]slipExtra, error) {
	// NPWP and bank account are decrypted only with a key; the snapshot is the one the run was computed from.
	rows, err := tx.QueryContext(ctx, `SELECT h.psh_nik, COALESCE(h.psh_jabatan,''),
  btrim(COALESCE(h.psh_dept_code,'') || ' ' || COALESCE(h.psh_section,'')), COALESCE(h.psh_grade,''), COALESCE(h.psh_tax_code,''),
  h.psh_pay_date, COALESCE(e.pe_pay_currency::text,'IDR'), e.pe_birth_date,
  CASE WHEN $3::text = '' OR e.pe_npwp_enc IS NULL THEN '' ELSE payroll.pgp_sym_decrypt(e.pe_npwp_enc, $3::text) END,
  CASE WHEN $3::text = '' OR e.pe_bank_acnt_enc IS NULL THEN '' ELSE payroll.pgp_sym_decrypt(e.pe_bank_acnt_enc, $3::text) END,
  r.pasr_nik IS NOT NULL,
  CASE WHEN r.pasr_data->>'MPAR_DAY_WORK' ~ '^-?[0-9]+(\.[0-9]+)?$' THEN (r.pasr_data->>'MPAR_DAY_WORK')::numeric ELSE 0 END::text, CASE WHEN r.pasr_data->>'MPAR_DAYOFF' ~ '^-?[0-9]+(\.[0-9]+)?$' THEN (r.pasr_data->>'MPAR_DAYOFF')::numeric ELSE 0 END::text, CASE WHEN r.pasr_data->>'MPAR_CUTI' ~ '^-?[0-9]+(\.[0-9]+)?$' THEN (r.pasr_data->>'MPAR_CUTI')::numeric ELSE 0 END::text, CASE WHEN r.pasr_data->>'MPAR_ALPA' ~ '^-?[0-9]+(\.[0-9]+)?$' THEN (r.pasr_data->>'MPAR_ALPA')::numeric ELSE 0 END::text, CASE WHEN r.pasr_data->>'MPAR_IJIN' ~ '^-?[0-9]+(\.[0-9]+)?$' THEN (r.pasr_data->>'MPAR_IJIN')::numeric ELSE 0 END::text
FROM payroll.pyr_salary_head h
LEFT JOIN payroll.pyr_employee e ON e.pe_company_id = $2 AND e.pe_nik = h.psh_nik AND e.pe_division = h.psh_division
LEFT JOIN payroll.pyr_att_snapshot_row r ON r.pasr_snapshot_id = (SELECT pr_snapshot_id FROM payroll.pyr_run WHERE pr_id = $1)
  AND r.pasr_kind = 'REKAP' AND r.pasr_nik = h.psh_nik
WHERE h.psh_run_id = $1 AND ($4::text = '' OR h.psh_nik = $4)`, runID, q.CompanyID, piiKey, onlyNIK)
	if err != nil {
		return nil, redact("read slip extra", err, piiKey)
	}
	defer closeRows(rows)
	out := map[string]slipExtra{}
	for rows.Next() {
		var nik string
		var pay, dob sql.NullTime
		var has bool
		var days [5]string
		var x slipExtra
		if err := rows.Scan(&nik, &x.Jabatan, &x.Dept, &x.Grade, &x.TaxCode, &pay, &x.Currency, &dob, &x.NPWP, &x.BankAccount,
			&has, &days[0], &days[1], &days[2], &days[3], &days[4]); err != nil {
			return nil, redact("scan slip extra", err, piiKey)
		}
		x.NPWP, x.BankAccount = strings.TrimSpace(x.NPWP), strings.TrimSpace(x.BankAccount)
		if pay.Valid {
			t := pay.Time
			x.PayDate = &t
		}
		if dob.Valid {
			t := dob.Time
			x.DOB = &t
		}
		if x.Info, err = attendance(has, days); err != nil {
			return nil, fmt.Errorf("absensi %s: %w", nik, err)
		}
		out[nik] = x
	}
	if err := rows.Err(); err != nil {
		return nil, redact("read slip extra", err, piiKey)
	}
	return out, nil
}

// redactedError carries the original error in its chain (errors.As finds *pgconn.PgError, errors.Is finds
// context.Canceled: ScopedTxOpts relies on SQLSTATE 40001) but its text never holds the PII key.
type redactedError struct {
	msg string
	err error
}

func (e *redactedError) Error() string { return e.msg }
func (e *redactedError) Unwrap() error { return e.err }

// redact wraps err with a prefix and masks the PII key in the message (a driver error may echo statement arguments).
func redact(prefix string, err error, key string) error {
	return &redactedError{msg: prefix + ": " + sanitizeErr(err, key), err: err}
}

// sanitizeErr keeps the PII key out of an error message.
func sanitizeErr(err error, key string) string {
	m := err.Error()
	if key != "" {
		m = strings.ReplaceAll(m, key, "***")
	}
	return m
}

func attendance(has bool, days [5]string) (pdf.Attendance, error) {
	a := pdf.Attendance{Valid: has}
	for i, dst := range []*decimal.Decimal{&a.Working, &a.Off, &a.Leave, &a.Alpa, &a.Ijin} {
		d, err := decimal.NewFromString(days[i])
		if err != nil {
			return a, err
		}
		*dst = d
	}
	return a, nil
}

// loadSlipDetail reads Basis and Remarks of every detail line of the run: NIK → component key → text.
func loadSlipDetail(ctx context.Context, tx *sql.Tx, runID int64) (map[string]map[string]slipDetail, error) {
	rows, err := tx.QueryContext(ctx, `SELECT psd_nik, psd_type::text, psd_id, COALESCE(psd_basis_amount::text,''), COALESCE(psd_remarks,'')
FROM payroll.pyr_salary_detail WHERE psd_run_id = $1`, runID)
	if err != nil {
		return nil, fmt.Errorf("read slip detail: %w", err)
	}
	defer closeRows(rows)
	out := map[string]map[string]slipDetail{}
	for rows.Next() {
		var nik, typ string
		var id int
		var d slipDetail
		if err := rows.Scan(&nik, &typ, &id, &d.Basis, &d.Remarks); err != nil {
			return nil, fmt.Errorf("scan slip detail: %w", err)
		}
		c := Comp{Type: "T", ID: id}
		if typ == "Potongan" || typ == "P" {
			c.Type = "P"
		}
		if out[nik] == nil {
			out[nik] = map[string]slipDetail{}
		}
		out[nik][c.Key()] = d
	}
	return out, rows.Err()
}

func parseJam(raw string) (map[string]decimal.Decimal, error) {
	out := map[string]decimal.Decimal{}
	if raw == "" {
		return out, nil
	}
	m := map[string]json.Number{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, err
	}
	for k, v := range m {
		d, err := decimal.NewFromString(v.String())
		if err != nil {
			return nil, fmt.Errorf("%s: %w", k, err)
		}
		out[k] = d
	}
	return out, nil
}
