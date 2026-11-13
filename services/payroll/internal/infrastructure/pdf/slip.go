// Package pdf renders payslips in pure Go (fpdf) and encrypts them with pdfcpu AES-256 (D-62 butir 4, 16§2.3-2.4).
// Rendering is deterministic: the PDF dates are the run approval time, so the same run yields the same bytes
// before encryption. Passwords exist only in memory.
package pdf

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/shopspring/decimal"
)

// PolicyDOB is the only password policy implemented (Q-P2, pyr_slip_password_policy).
const PolicyDOB = "DOB_DDMMYYYY"

// ErrNoBirthDate is returned when the password cannot be derived (pe_birth_date NULL).
var ErrNoBirthDate = errors.New("pdf: tanggal lahir kosong, password tidak dapat diturunkan")

// ErrPolicy is returned for a password policy that is not implemented yet.
var ErrPolicy = errors.New("pdf: kebijakan password belum didukung")

// Line is one printed row (label + amount).
type Line struct {
	Name   string
	Amount decimal.Decimal
}

// Slip is the content of one payslip. Zero-amount lines must already be omitted by the caller, but Render also
// drops them as a guard (11§2.1: baris 0 tidak dicetak).
type Slip struct {
	Company, PeriodName, Division string
	NIK, Nama, Dept, Jabatan      string
	PayDate, Rekening, Currency   string
	Overtime                      []Line // jam × rate, already rounded
	Tunjangan, Potongan           []Line // in pcm_serial order
	TotTunjangan, TotPotongan     decimal.Decimal
	Rounding, NetPay              decimal.Decimal
	ApprovedAt                    time.Time // PDF metadata dates
}

// Render builds the unencrypted PDF.
func Render(s Slip) ([]byte, error) {
	p := fpdf.New("P", "mm", "A4", "")
	p.SetCompression(false)
	p.SetCatalogSort(true)
	p.SetCreationDate(s.ApprovedAt.UTC())
	p.SetModificationDate(s.ApprovedAt.UTC())
	p.SetProducer("goapps-payroll", true)
	p.SetCreator("goapps-payroll", true)
	p.SetTitle("Slip Gaji "+s.PeriodName, false)
	p.SetMargins(15, 15, 15)
	p.SetAutoPageBreak(true, 20)
	tr := p.UnicodeTranslatorFromDescriptor("")
	p.AddPage()

	watermark(p, tr, s.Nama+" / "+s.NIK)

	p.SetFont("Helvetica", "B", 14)
	p.CellFormat(0, 8, tr(s.Company+" - Slip Gaji "+s.PeriodName), "", 1, "L", false, 0, "")
	p.SetFont("Helvetica", "", 10)
	for _, kv := range [][2]string{{"NIK", s.NIK}, {"Nama", s.Nama}, {"Departemen", s.Dept}, {"Jabatan", s.Jabatan},
		{"Divisi", s.Division}, {"Tanggal bayar", s.PayDate}, {"Pembayaran", s.Rekening}} {
		if kv[1] == "" {
			continue
		}
		p.CellFormat(35, 5.5, tr(kv[0]), "", 0, "L", false, 0, "")
		p.CellFormat(0, 5.5, tr(": "+kv[1]), "", 1, "L", false, 0, "")
	}
	p.Ln(3)
	section(p, tr, "Lembur", s.Overtime, nil)
	section(p, tr, "Tunjangan", s.Tunjangan, &s.TotTunjangan)
	section(p, tr, "Potongan", s.Potongan, &s.TotPotongan)
	if !s.Rounding.IsZero() {
		row(p, tr, "Pembulatan", s.Rounding, false)
	}
	p.Ln(1)
	row(p, tr, "TAKE HOME PAY ("+cur(s.Currency)+")", s.NetPay, true)

	p.SetY(-18)
	p.SetFont("Helvetica", "I", 8)
	p.CellFormat(0, 5, tr("Dokumen ini dibuat otomatis oleh sistem, tidak memerlukan tanda tangan."), "", 0, "C", false, 0, "")

	var b bytes.Buffer
	if err := p.Output(&b); err != nil {
		return nil, fmt.Errorf("render slip: %w", err)
	}
	return b.Bytes(), nil
}

func cur(c string) string {
	if c == "" {
		return "IDR"
	}
	return c
}

func watermark(p *fpdf.Fpdf, tr func(string) string, text string) {
	p.SetFont("Helvetica", "B", 30)
	p.SetTextColor(200, 200, 200)
	p.SetAlpha(0.3, "Normal")
	p.TransformBegin()
	p.TransformRotate(45, 105, 148)
	p.Text(30, 148, tr(text))
	p.TransformEnd()
	p.SetAlpha(1, "Normal")
	p.SetTextColor(0, 0, 0)
}

func section(p *fpdf.Fpdf, tr func(string) string, title string, lines []Line, total *decimal.Decimal) {
	var kept []Line
	for _, l := range lines {
		if !l.Amount.IsZero() { // baris 0 tidak dicetak
			kept = append(kept, l)
		}
	}
	if len(kept) == 0 && total == nil {
		return
	}
	p.SetFont("Helvetica", "B", 10)
	p.CellFormat(0, 6, tr(title), "B", 1, "L", false, 0, "")
	for _, l := range kept {
		row(p, tr, l.Name, l.Amount, false)
	}
	if total != nil {
		row(p, tr, "Total "+title, *total, true)
	}
	p.Ln(2)
}

func row(p *fpdf.Fpdf, tr func(string) string, name string, v decimal.Decimal, bold bool) {
	style := ""
	if bold {
		style = "B"
	}
	p.SetFont("Helvetica", style, 10)
	p.CellFormat(120, 5.5, tr(name), "", 0, "L", false, 0, "")
	p.CellFormat(0, 5.5, Money(v), "", 1, "R", false, 0, "")
}

// Money formats with a thousands separator "." and two decimals ",": 1234567.5 → "1.234.567,50".
func Money(v decimal.Decimal) string {
	s := v.StringFixed(2)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	ip, fp, _ := strings.Cut(s, ".")
	out := make([]byte, 0, len(ip)+len(ip)/3)
	for i, c := range []byte(ip) {
		if i > 0 && (len(ip)-i)%3 == 0 {
			out = append(out, '.')
		}
		out = append(out, c)
	}
	r := string(out) + "," + fp
	if neg {
		r = "-" + r
	}
	return r
}

// Password derives the user password from the policy. Only DOB_DDMMYYYY is supported.
func Password(policy string, dob *time.Time) (string, error) {
	if policy != PolicyDOB {
		return "", fmt.Errorf("%w: %s", ErrPolicy, policy)
	}
	if dob == nil || dob.IsZero() {
		return "", ErrNoBirthDate
	}
	return dob.Format("02012006"), nil
}

// ContentHash is the sha256 (hex) of the unencrypted PDF bytes.
func ContentHash(plain []byte) string {
	h := sha256.Sum256(plain)
	return hex.EncodeToString(h[:])
}

// Encrypt applies AES-256 with the user password and a random 32-byte owner password (not stored). Print is allowed,
// modify/copy are not (PermissionsPrint).
func Encrypt(plain []byte, userPW string) ([]byte, error) {
	if userPW == "" {
		return nil, errors.New("pdf: user password kosong")
	}
	ob := make([]byte, 32)
	if _, err := rand.Read(ob); err != nil {
		return nil, fmt.Errorf("owner password: %w", err)
	}
	conf := model.NewAESConfiguration(userPW, hex.EncodeToString(ob), 256)
	conf.Permissions = model.PermissionsPrint
	var out bytes.Buffer
	if err := api.Encrypt(bytes.NewReader(plain), &out, conf); err != nil {
		return nil, fmt.Errorf("encrypt slip: %w", err)
	}
	return out.Bytes(), nil
}

// Validate reports whether the encrypted PDF opens with pw (false for a wrong password).
func Validate(enc []byte, pw string) error {
	conf := model.NewAESConfiguration(pw, "", 256)
	return api.Validate(bytes.NewReader(enc), conf)
}
