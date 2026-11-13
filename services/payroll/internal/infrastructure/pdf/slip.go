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

// ErrTooManyLines is returned when the lines do not fit one page even at the smallest font (no data in the message).
var ErrTooManyLines = errors.New("pdf: baris slip terlalu banyak untuk satu halaman")

// Line is one printed row. Basis is psd_basis_amount as a decimal string ("" = NULL, printed empty); Remarks is
// psd_remarks (wrapped over several lines).
type Line struct {
	Name    string
	Amount  decimal.Decimal
	Basis   string
	Remarks string
}

// Attendance is the attendance box of the slip (recap of the run snapshot). Valid is false when the NIK has no recap.
type Attendance struct {
	Valid                           bool
	Working, Off, Leave, Alpa, Ijin decimal.Decimal
}

// Total is the sum of the five printed categories.
func (a Attendance) Total() decimal.Decimal {
	return a.Working.Add(a.Off).Add(a.Leave).Add(a.Alpa).Add(a.Ijin)
}

// Slip is the content of one payslip (F-3). Zero-amount lines must already be omitted by the caller, but Render also
// drops them as a guard (11§2.1: baris 0 tidak dicetak). NPWP and BankAccount are decrypted plaintext: they live in
// memory and in the PDF only.
type Slip struct {
	Company, PeriodName, Period string // Period is the header text, e.g. "Jan - 27"
	NIK, Nama, Dept, Grade      string // Dept is "<dept_code> <section>"
	Jabatan, TaxCode, NPWP      string
	BankAccount, PayDate        string // PayDate is "DD-MON-YY"
	Currency                    string
	Tunjangan, Potongan         []Line // in pcm_serial order
	TotTunjangan, TotPotongan   decimal.Decimal
	Rounding, NetPay            decimal.Decimal
	Info                        Attendance
	Logo                        []byte   // PNG/JPEG; empty = text only
	Perhatian                   []string // lines of the "Perhatian" box
	ContactIT                   string
	ApprovedAt                  time.Time // PDF metadata dates
}

const (
	pageW, pageH = 297.0, 210.0
	margin       = 8.0
	colW         = 138.0
	colGap       = pageW - 2*margin - 2*colW
	rowsTop      = 49.0
	footH        = 15.0
	boxH         = 22.0
	boxTop       = pageH - margin - boxH
	rowsBottom   = boxTop - 2 - footH - 1
)

// side describes one column (PENGHASILAN with Basis, POTONGAN without).
type side struct {
	title string
	x     float64
	lines []Line
	basis bool
	w     [5]float64 // No, Description, Basis, Remarks, Amount (Basis 0 on the right side)
}

func newSides(s Slip) [2]side {
	return [2]side{
		{title: "PENGHASILAN", x: margin, lines: nonZero(s.Tunjangan), basis: true, w: [5]float64{8, 38, 22, 40, 30}},
		{title: "POTONGAN", x: margin + colW + colGap, lines: nonZero(s.Potongan), w: [5]float64{8, 52, 0, 48, 30}},
	}
}

func nonZero(in []Line) []Line {
	out := make([]Line, 0, len(in))
	for _, l := range in {
		if !l.Amount.IsZero() { // baris 0 tidak dicetak
			out = append(out, l)
		}
	}
	return out
}

// Render builds the unencrypted PDF.
func Render(s Slip) ([]byte, error) {
	p := fpdf.New("L", "mm", "A4", "")
	p.SetCompression(false)
	p.SetCatalogSort(true)
	p.SetCreationDate(s.ApprovedAt.UTC())
	p.SetModificationDate(s.ApprovedAt.UTC())
	p.SetProducer("goapps-payroll", true)
	p.SetCreator("goapps-payroll", true)
	p.SetTitle("Slip Gaji "+s.PeriodName, false)
	p.SetMargins(margin, margin, margin)
	p.SetAutoPageBreak(false, 0)
	tr := p.UnicodeTranslatorFromDescriptor("")
	p.AddPage()

	watermark(p, tr, s.Nama+" / "+s.NIK)
	header(p, tr, s)
	sides := newSides(s)
	size, maxRem, ok := fitSize(p, tr, sides)
	if !ok {
		return nil, ErrTooManyLines
	}
	for _, sd := range sides {
		drawSide(p, tr, sd, size, maxRem)
	}
	footer(p, tr, s)
	boxes(p, tr, s)

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
	p.SetFont("Helvetica", "B", 34)
	p.SetTextColor(200, 200, 200)
	p.SetAlpha(0.3, "Normal")
	t := tr(text)
	p.TransformBegin()
	p.TransformRotate(30, pageW/2, pageH/2)
	p.Text(pageW/2-p.GetStringWidth(t)/2, pageH/2, t)
	p.TransformEnd()
	p.SetAlpha(1, "Normal")
	p.SetTextColor(0, 0, 0)
}

// spaced renders a name letter-spaced like the legacy letterhead: "PT. MGT" -> "P T .   M G T".
func spaced(name string) string {
	words := strings.Fields(strings.ToUpper(name))
	for i, w := range words {
		words[i] = strings.Join(strings.Split(w, ""), " ")
	}
	return strings.Join(words, "   ")
}

func header(p *fpdf.Fpdf, tr func(string) string, s Slip) {
	if len(s.Logo) > 0 {
		opt := fpdf.ImageOptions{ImageType: imageType(s.Logo), ReadDpi: false}
		p.RegisterImageOptionsReader("logo", opt, bytes.NewReader(s.Logo))
		if p.Ok() {
			p.ImageOptions("logo", margin, 5, 17, 0, false, opt, 0, "")
		}
	}
	p.SetFont("Helvetica", "B", 13)
	p.SetXY(margin, 6)
	p.CellFormat(pageW-2*margin, 7, tr(spaced(s.Company)), "", 1, "C", false, 0, "")
	p.SetFont("Helvetica", "B", 11)
	p.SetX(margin)
	p.CellFormat(pageW-2*margin, 6, "SLIP GAJI", "", 1, "C", false, 0, "")

	pairs := [2][5][2]string{
		{{"NIK", s.NIK}, {"Dept", s.Dept}, {"Grade", s.Grade}, {"Bank Account", s.BankAccount}, {"Period", s.Period}},
		{{"Nama", s.Nama}, {"Jabatan", s.Jabatan}, {"Tax Code", s.TaxCode}, {"NPWP", s.NPWP}, {"Payment Date", s.PayDate}},
	}
	pw := (pageW - 2*margin) / 5
	p.SetDrawColor(120, 120, 120)
	p.Rect(margin, 26, pageW-2*margin, 12, "D")
	for r, row := range pairs {
		for c, kv := range row {
			x, y := margin+float64(c)*pw+1, 26.5+float64(r)*5.5
			p.SetFont("Helvetica", "B", 7)
			p.SetXY(x, y)
			p.CellFormat(17, 5, tr(kv[0]), "", 0, "L", false, 0, "")
			fitCell(p, tr, kv[0], ": "+kv[1], kv[0] == "NPWP" || kv[0] == "Bank Account", pw-19, x+17, y)
		}
	}
	p.SetDrawColor(0, 0, 0)
}

// imageType sniffs the logo format (fpdf needs it for a reader): JPEG by magic bytes, otherwise PNG.
func imageType(b []byte) string {
	if len(b) > 2 && b[0] == 0xFF && b[1] == 0xD8 {
		return "JPG"
	}
	return "PNG"
}

// fitCell writes text at (x, y) in the remaining width: the font shrinks to 5 pt first. A strict cell (NPWP, bank
// account) is never cut: if it still does not fit the render fails. Other cells end with "..." when cut.
func fitCell(p *fpdf.Fpdf, tr func(string) string, label, text string, strict bool, w, x, y float64) {
	t := tr(text)
	sz := 7.5
	for ; sz > 5; sz -= 0.5 {
		p.SetFont("Helvetica", "", sz)
		if p.GetStringWidth(t) <= w {
			break
		}
	}
	p.SetFont("Helvetica", "", sz)
	if p.GetStringWidth(t) > w {
		if strict {
			p.SetErrorf("pdf: nilai %s terlalu panjang untuk kolom header", label)
			return
		}
		for len(t) > 1 && p.GetStringWidth(t+"...") > w {
			t = t[:len(t)-1]
		}
		t += "..."
	}
	p.SetXY(x, y)
	p.CellFormat(w, 5, t, "", 0, "L", false, 0, "")
}

type sizing struct {
	size   float64
	maxRem int // 0 = unlimited
}

var sizings = []sizing{{7.5, 0}, {7, 0}, {6.5, 0}, {6, 0}, {6, 2}, {6, 1}}

func lineH(size float64) float64 { return size * 0.45 }

// fitSize picks the first font size at which both columns fit above the footer; ok is false when none does.
func fitSize(p *fpdf.Fpdf, tr func(string) string, sides [2]side) (size float64, maxRem int, ok bool) {
	for _, z := range sizings {
		fits := true
		for _, sd := range sides {
			h := 0.0
			for _, l := range sd.lines {
				_, _, _, n := wrapLine(p, tr, sd, l, z.size, z.maxRem)
				h += rowHeight(n, z.size)
			}
			if rowsTop+h > rowsBottom {
				fits = false
			}
		}
		if fits {
			return z.size, z.maxRem, true
		}
	}
	return 0, 0, false
}

func rowHeight(lines int, size float64) float64 { return float64(lines)*lineH(size) + 1 }

// wrapLine wraps description and remarks of one row; n is the number of text lines the row needs.
func wrapLine(p *fpdf.Fpdf, tr func(string) string, sd side, l Line, size float64, maxRem int) (desc, rem []string, basis string, n int) {
	p.SetFont("Helvetica", "", size)
	desc = split(p, tr(l.Name), sd.w[1]-2, 0)
	rem = split(p, tr(l.Remarks), sd.w[3]-2, maxRem)
	n = max(len(desc), len(rem), 1)
	return desc, rem, BasisText(l.Basis), n
}

func split(p *fpdf.Fpdf, text string, w float64, maxLines int) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	var out []string
	for _, para := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		if strings.TrimSpace(para) == "" {
			continue
		}
		out = append(out, p.SplitText(para, w)...)
	}
	if maxLines > 0 && len(out) > maxLines {
		out = out[:maxLines]
		out[maxLines-1] = strings.TrimRight(out[maxLines-1], " ") + "..."
	}
	return out
}

func drawSide(p *fpdf.Fpdf, tr func(string) string, sd side, size float64, maxRem int) {
	// title bar and column headings
	p.SetFillColor(225, 225, 225)
	p.SetFont("Helvetica", "B", 8.5)
	p.SetXY(sd.x, 39.5)
	p.CellFormat(colW, 5, sd.title, "1", 0, "C", true, 0, "")
	heads := []string{"No", "Description", "Basis", "Remarks", "Amount"}
	aligns := []string{"C", "L", "R", "L", "R"}
	p.SetFont("Helvetica", "B", 7.5)
	x, y := sd.x, 44.5
	for i, h := range heads {
		if sd.w[i] == 0 {
			continue
		}
		p.SetXY(x, y)
		p.CellFormat(sd.w[i], 4.5, h, "B", 0, aligns[i], false, 0, "")
		x += sd.w[i]
	}

	y = rowsTop
	lh := lineH(size)
	p.SetDrawColor(210, 210, 210)
	for i, l := range sd.lines {
		desc, rem, basis, n := wrapLine(p, tr, sd, l, size, maxRem)
		h := rowHeight(n, size)
		p.SetFont("Helvetica", "", size)
		cell := func(col int, x float64, txt, al string) {
			p.SetXY(x, y+0.5)
			p.CellFormat(sd.w[col], lh, txt, "", 0, al, false, 0, "")
		}
		x := sd.x
		cell(0, x, fmt.Sprint(i+1), "C")
		x += sd.w[0]
		for k, t := range desc {
			p.SetXY(x+1, y+0.5+float64(k)*lh)
			p.CellFormat(sd.w[1]-2, lh, t, "", 0, "L", false, 0, "")
		}
		x += sd.w[1]
		if sd.basis {
			cell(2, x, basis, "R")
			x += sd.w[2]
		}
		for k, t := range rem {
			p.SetXY(x+1, y+0.5+float64(k)*lh)
			p.CellFormat(sd.w[3]-2, lh, t, "", 0, "L", false, 0, "")
		}
		x += sd.w[3]
		cell(4, x, Money(l.Amount), "R")
		y += h
		p.Line(sd.x, y, sd.x+colW, y)
	}
	p.SetDrawColor(0, 0, 0)
}

func footer(p *fpdf.Fpdf, tr func(string) string, s Slip) {
	top := boxTop - 2 - footH
	left, right := margin, margin+colW+colGap
	tot := func(x, y float64, label string, v decimal.Decimal, bold bool) {
		style := ""
		if bold {
			style = "B"
		}
		p.SetFont("Helvetica", style, 8.5)
		p.SetXY(x, y)
		p.CellFormat(colW-30, 5, tr(label), "T", 0, "L", false, 0, "")
		p.CellFormat(30, 5, Money(v), "T", 0, "R", false, 0, "")
	}
	tot(left, top, "Total Penghasilan", s.TotTunjangan, true)
	tot(right, top, "Total Potongan", s.TotPotongan, true)
	tot(right, top+5, "Rounding", s.Rounding, false)
	net := "Net Paid"
	if c := cur(s.Currency); c != "IDR" {
		net += " (" + c + ")"
	}
	tot(right, top+10, net, s.NetPay, true)
}

func boxes(p *fpdf.Fpdf, tr func(string) string, s Slip) {
	left, right := margin, margin+colW+colGap
	p.SetDrawColor(120, 120, 120)
	p.Rect(left, boxTop, colW, boxH, "D")
	p.Rect(right, boxTop, colW, boxH, "D")
	p.SetDrawColor(0, 0, 0)

	p.SetFont("Helvetica", "B", 8)
	p.SetXY(left+1.5, boxTop+1)
	p.CellFormat(colW-3, 4, "Perhatian", "", 0, "L", false, 0, "")
	p.SetFont("Helvetica", "", 7)
	y := boxTop + 5.5
	var lines []string
	for _, t := range s.Perhatian {
		lines = append(lines, split(p, tr(t), colW-4, 0)...)
	}
	if s.ContactIT != "" {
		lines = append(lines, split(p, tr("Kontak IT: "+s.ContactIT), colW-4, 0)...)
	}
	for _, t := range lines {
		if y+3 > boxTop+boxH {
			break
		}
		p.SetXY(left+1.5, y)
		p.CellFormat(colW-3, 3, t, "", 0, "L", false, 0, "")
		y += 3
	}

	p.SetFont("Helvetica", "B", 8)
	p.SetXY(right+1.5, boxTop+1)
	p.CellFormat(colW-3, 4, "Informasi", "", 0, "L", false, 0, "")
	val := func(d decimal.Decimal) string {
		if !s.Info.Valid {
			return "-"
		}
		return d.String()
	}
	items := [][2]string{{"Working", val(s.Info.Working)}, {"Off", val(s.Info.Off)}, {"Leave", val(s.Info.Leave)},
		{"Alpa", val(s.Info.Alpa)}, {"Ijin", val(s.Info.Ijin)}, {"Total", val(s.Info.Total())}}
	cw := (colW - 3) / 3
	for i, kv := range items {
		x, y := right+1.5+float64(i%3)*cw, boxTop+6.5+float64(i/3)*6
		p.SetFont("Helvetica", "", 7.5)
		p.SetXY(x, y)
		p.CellFormat(16, 5, kv[0], "", 0, "L", false, 0, "")
		p.SetFont("Helvetica", "B", 7.5)
		p.CellFormat(cw-16, 5, ": "+kv[1], "", 0, "L", false, 0, "")
	}
}

// BasisText formats psd_basis_amount: a small rate (|v| < 1) as is (0.0089), a nominal with a thousands separator and
// no decimals; "" or an unparsable value prints empty.
func BasisText(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	d, err := decimal.NewFromString(raw)
	if err != nil {
		return ""
	}
	if !d.IsZero() && d.Abs().LessThan(decimal.NewFromInt(1)) {
		return d.String()
	}
	return group(d.Round(0).String())
}

// group inserts "." thousands separators into a (possibly negative) integer string.
func group(s string) string {
	neg := strings.HasPrefix(s, "-")
	ip := strings.TrimPrefix(s, "-")
	out := make([]byte, 0, len(ip)+len(ip)/3)
	for i, c := range []byte(ip) {
		if i > 0 && (len(ip)-i)%3 == 0 {
			out = append(out, '.')
		}
		out = append(out, c)
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

// Money formats with a thousands separator "." and two decimals ",": 1234567.5 → "1.234.567,50".
func Money(v decimal.Decimal) string {
	ip, fp, _ := strings.Cut(v.StringFixed(2), ".")
	return group(ip) + "," + fp
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
