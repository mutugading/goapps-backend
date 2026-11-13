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

// SlipResult reports a slip run. Skipped lists NIKs without a birth date (no password can be derived).
type SlipResult struct {
	Written []string
	Skipped []string
}

// slipExtra is the per-NIK data outside RunData.
type slipExtra struct {
	Jabatan, Pembayaran, Level, Currency string
	PayDate                              string
	Rate                                 decimal.Decimal
	Jam                                  map[string]decimal.Decimal
	DOB                                  *time.Time
}

// lemburLabels fixes the print order and labels of the psh_jam_lembur keys.
var lemburLabels = []struct{ Key, Label string }{
	{"lembur1", "Lembur 1"}, {"lembur2", "Lembur 2"},
	{"off1", "Lembur Off 1"}, {"off2", "Lembur Off 2"}, {"off3", "Lembur Off 3"},
	{"nasp1", "Lembur NAS+ 1"}, {"nasp2", "Lembur NAS+ 2"}, {"nasp3", "Lembur NAS+ 3"},
}

// RoundMoney is Oracle ROUND half-up to 2 decimals (D-07).
func roundMoney(d decimal.Decimal) decimal.Decimal { return d.Round(2) }

// BuildOvertime computes the overtime rows: round(jam × rate) per column; Executive NAS = (nas1+nas2+nas3) × 2 in one
// row (D11). Zero rows are kept here and dropped by the renderer.
func BuildOvertime(jam map[string]decimal.Decimal, rate decimal.Decimal, level string) []pdf.Line {
	out := make([]pdf.Line, 0, len(lemburLabels)+3)
	for _, l := range lemburLabels {
		out = append(out, pdf.Line{Name: l.Label, Amount: roundMoney(jam[l.Key].Mul(rate))})
	}
	nas := jam["nas1"].Add(jam["nas2"]).Add(jam["nas3"])
	if strings.EqualFold(strings.TrimSpace(level), "executive") {
		out = append(out, pdf.Line{Name: "Lembur NAS (Executive)", Amount: roundMoney(nas.Mul(decimal.NewFromInt(2)).Mul(rate))})
	} else {
		for i, k := range []string{"nas1", "nas2", "nas3"} {
			out = append(out, pdf.Line{Name: fmt.Sprintf("Lembur NAS %d", i+1), Amount: roundMoney(jam[k].Mul(rate))})
		}
	}
	return out
}

// BuildSlip assembles the content of one slip from a loaded run.
func BuildSlip(d RunData, e Emp, x slipExtra, company, division, periodName string, approvedAt time.Time) pdf.Slip {
	s := pdf.Slip{Company: company, PeriodName: periodName, Division: division, NIK: e.NIK, Nama: e.Nama, Dept: e.Dept,
		Jabatan: x.Jabatan, PayDate: x.PayDate, Rekening: x.Pembayaran, Currency: x.Currency,
		Overtime:     BuildOvertime(x.Jam, x.Rate, x.Level),
		TotTunjangan: e.TotTunjangan, TotPotongan: e.TotPotongan, Rounding: e.Rounding, NetPay: e.NetPay, ApprovedAt: approvedAt}
	for _, c := range d.Comps { // already in pcm_serial order
		v, ok := e.Amounts[c.Key()]
		if !ok || v.IsZero() {
			continue
		}
		if c.Type == "T" {
			s.Tunjangan = append(s.Tunjangan, pdf.Line{Name: c.Name, Amount: v})
		} else {
			s.Potongan = append(s.Potongan, pdf.Line{Name: c.Name, Amount: v})
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
	approvedAt time.Time
	extra      map[string]slipExtra
	idx        map[string]int
	division   string
}

// LoadSlipSource loads the ACTIVE run of q (same checks as Load) and the slip header data inside tx.
func LoadSlipSource(ctx context.Context, tx *sql.Tx, q Query) (*SlipSource, error) {
	d, err := Load(ctx, tx, q)
	if err != nil {
		return nil, err
	}
	company, policy, approvedAt, err := slipHeader(ctx, tx, q, d.RunID)
	if err != nil {
		return nil, err
	}
	extra, err := loadSlipExtra(ctx, tx, q, d.RunID)
	if err != nil {
		return nil, err
	}
	idx := make(map[string]int, len(d.Emps))
	for i, e := range d.Emps {
		idx[e.NIK] = i
	}
	return &SlipSource{d: d, company: company, policy: policy, approvedAt: approvedAt, extra: extra, idx: idx, division: q.Division}, nil
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

// Render returns the plain PDF (for the content hash) and the password-protected PDF of nik. A missing birth date
// yields pdf.ErrNoBirthDate; an unknown NIK is an error.
func (s *SlipSource) Render(nik, periodName string) (plain, enc []byte, err error) {
	i, ok := s.idx[nik]
	if !ok {
		return nil, nil, fmt.Errorf("NIK %s tidak ada di run", nik)
	}
	x := s.extra[nik]
	plain, err = pdf.Render(BuildSlip(s.d, s.d.Emps[i], x, s.company, s.division, periodName, s.approvedAt))
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
// and reported. The audit row holds counts only.
func (s *Service) ExportSlips(ctx context.Context, q Query, o SlipOptions) (SlipResult, error) {
	var res SlipResult
	var files stagedFiles
	err := s.DB.ScopedTxOpts(ctx, run.SnapshotTx, func(tx *sql.Tx) error {
		res, files = SlipResult{}, stagedFiles{} // the tx may be retried
		if err := s.slipsInTx(ctx, tx, q, o, &res, &files); err != nil {
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

func (s *Service) slipsInTx(ctx context.Context, tx *sql.Tx, q Query, o SlipOptions, res *SlipResult, files *stagedFiles) error {
	d, err := Load(ctx, tx, q)
	if err != nil {
		return err
	}
	company, policy, approvedAt, err := slipHeader(ctx, tx, q, d.RunID)
	if err != nil {
		return err
	}
	extra, err := loadSlipExtra(ctx, tx, q, d.RunID)
	if err != nil {
		return err
	}
	if d.Emps, err = filterNIK(d.Emps, o.NIK); err != nil {
		return err
	}
	for _, e := range d.Emps {
		x := extra[e.NIK]
		out, err := renderSlip(BuildSlip(d, e, x, company, q.Division, o.PeriodName, approvedAt), x, policy, o.NoPassword)
		if errors.Is(err, pdf.ErrNoBirthDate) {
			res.Skipped = append(res.Skipped, e.NIK)
			continue
		}
		if err != nil {
			return err
		}
		final, err := slipPath(o.OutDir, q.YYMM, e.NIK)
		if err != nil {
			return err
		}
		if err := files.add(final, out); err != nil {
			return err
		}
		res.Written = append(res.Written, e.NIK)
	}
	return nil
}

// filterNIK keeps only nik when set; an unknown NIK is an error.
func filterNIK(emps []Emp, nik string) ([]Emp, error) {
	if nik == "" {
		return emps, nil
	}
	for _, e := range emps {
		if e.NIK == nik {
			return []Emp{e}, nil
		}
	}
	return nil, fmt.Errorf("NIK %s tidak ada di run", nik)
}

// renderSlip renders one slip and encrypts it unless noPassword. A missing birth date yields pdf.ErrNoBirthDate.
func renderSlip(sl pdf.Slip, x slipExtra, policy string, noPassword bool) ([]byte, error) {
	plain, err := pdf.Render(sl)
	if err != nil || noPassword {
		return plain, err
	}
	pw, err := pdf.Password(policy, x.DOB)
	if err != nil {
		return nil, err
	}
	return pdf.Encrypt(plain, pw)
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

func loadSlipExtra(ctx context.Context, tx *sql.Tx, q Query, runID int64) (map[string]slipExtra, error) {
	rows, err := tx.QueryContext(ctx, `SELECT h.psh_nik, COALESCE(h.psh_jabatan,''), COALESCE(h.psh_pembayaran,''),
  COALESCE(to_char(h.psh_pay_date,'DD-MM-YYYY'),''), COALESCE(h.psh_rate_perjam::text,'0'), h.psh_jam_lembur::text,
  COALESCE(e.pe_level,''), COALESCE(e.pe_pay_currency::text,'IDR'), e.pe_birth_date
FROM payroll.pyr_salary_head h
LEFT JOIN payroll.pyr_employee e ON e.pe_company_id = $2 AND e.pe_nik = h.psh_nik AND e.pe_division = h.psh_division
WHERE h.psh_run_id = $1`, runID, q.CompanyID)
	if err != nil {
		return nil, fmt.Errorf("read slip extra: %w", err)
	}
	defer closeRows(rows)
	out := map[string]slipExtra{}
	for rows.Next() {
		var nik, rate string
		var jam sql.NullString
		var dob sql.NullTime
		var x slipExtra
		if err := rows.Scan(&nik, &x.Jabatan, &x.Pembayaran, &x.PayDate, &rate, &jam, &x.Level, &x.Currency, &dob); err != nil {
			return nil, fmt.Errorf("scan slip extra: %w", err)
		}
		if x.Rate, err = decimal.NewFromString(rate); err != nil {
			return nil, fmt.Errorf("rate %s: %w", nik, err)
		}
		if x.Jam, err = parseJam(jam.String); err != nil {
			return nil, fmt.Errorf("jam_lembur %s: %w", nik, err)
		}
		if dob.Valid {
			t := dob.Time
			x.DOB = &t
		}
		out[nik] = x
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
