package report

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
)

// Sheet names of the bank workbook (F-1, GOLIVE-FORMATS) and the transfer sheet of the reco workbook (F-2).
const (
	SheetCover    = "COVER LETTER"
	SheetBank     = "DETAIL"
	SheetTransfer = "transfer"
)

// MandiriBank is the bank name (matched case-insensitively as a substring of pe_bank) accepted in the Mandiri file.
const MandiriBank = "MANDIRI"

const (
	bankDataRow    = 7  // first DETAIL data row (F-1)
	maxExcelDigits = 15 // an account number above this loses digits as an Excel number
)

// ErrNoPayableRows is returned when no employee passes the pre-check: a bank file without DETAIL rows would be a
// fake (circular SUM), so nothing is written to the bank file and nothing is audited.
var ErrNoPayableRows = errors.New("report: tidak ada baris yang layak ditransfer (lihat pra-cek)")

// BankRow is one employee paid by transfer. Account is the decrypted rekening: it must never reach a log or audit row.
type BankRow struct {
	NIK, Nama, Dept, Status, LevelStatus, MGTMB string
	Bank, AccountName, Account                  string
	Net                                         decimal.Decimal
}

// BankIssue is one employee excluded from the transfer list, with the reasons. It carries no account number.
type BankIssue struct {
	NIK, Nama, Reason string
}

// BankData is the transfer list of a period: Rows pass every check, Issues are listed in the pre-check sheet.
type BankData struct {
	YYMM   int
	Rows   []BankRow
	Issues []BankIssue
}

// Total is Σ net of the rows (the cover letter total, without the transfer fee).
func (d BankData) Total() decimal.Decimal {
	t := decimal.Zero
	for _, r := range d.Rows {
		t = t.Add(r.Net)
	}
	return t
}

// BankText is the fixed text of the cover letter and titles; it comes from configuration (PAYROLL_REPORT_*), never
// from the repository.
type BankText struct {
	Letterhead      []string // up to 4 lines
	CompanyName     string   // "PT. ..." as in the DETAIL title
	Branch          string   // Bank Mandiri branch
	EscrowAccount   string
	Signer1, Title1 string
	Signer2, Title2 string
	FeePerTransfer  decimal.Decimal
	TransferDate    time.Time
	PeriodDate      time.Time // value of "PERIODE :" in DETAIL
}

// bankIssues returns the reasons a row cannot go in the Mandiri file; empty means it is payable.
func bankIssues(r BankRow, bank string) []string {
	var out []string
	if !r.Net.IsPositive() {
		out = append(out, "net <= 0")
	} else if !r.Net.Equal(r.Net.Round(0)) {
		out = append(out, "net bukan bilangan bulat")
	}
	if strings.TrimSpace(r.AccountName) == "" {
		out = append(out, "nama rekening kosong")
	}
	switch a := r.Account; {
	case a == "":
		out = append(out, "rekening kosong")
	case !allDigits(a):
		out = append(out, "rekening bukan digit")
	case a[0] == '0':
		out = append(out, "rekening berawalan 0")
	case len(a) > maxExcelDigits:
		out = append(out, "rekening lebih dari 15 digit")
	}
	if !strings.Contains(strings.ToUpper(r.Bank), strings.ToUpper(bank)) {
		out = append(out, "bank bukan "+bank)
	}
	return out
}

func allDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return s != ""
}

// SplitBank applies the pre-checks: valid rows stay in the transfer list, the rest become issues.
func SplitBank(yymm int, all []BankRow, bank string) BankData {
	d := BankData{YYMM: yymm}
	for _, r := range all {
		if is := bankIssues(r, bank); len(is) > 0 {
			d.Issues = append(d.Issues, BankIssue{NIK: r.NIK, Nama: r.Nama, Reason: strings.Join(is, "; ")})
			continue
		}
		d.Rows = append(d.Rows, r)
	}
	return d
}

const bankSQL = `SELECT h.psh_nik, COALESCE(h.psh_nama,''), COALESCE(h.psh_dept_code,''),
  COALESCE(h.psh_emp_type, e.pe_employment_status, ''), COALESCE(g.pggm_gl_group,''), COALESCE(e.pe_cost_group,''),
  COALESCE(e.pe_bank,''), COALESCE(e.pe_bank_acnt_name,''),
  CASE WHEN e.pe_bank_acnt_enc IS NULL THEN '' ELSE payroll.pgp_sym_decrypt(e.pe_bank_acnt_enc, $3::text) END,
  h.psh_net_pay::text
FROM payroll.pyr_salary_head h
LEFT JOIN payroll.pyr_employee e ON e.pe_company_id = $2 AND e.pe_nik = h.psh_nik AND e.pe_division = h.psh_division
LEFT JOIN payroll.pyr_gl_group_map g ON g.pggm_company_id = $2 AND g.pggm_division = h.psh_division::text AND g.pggm_level = e.pe_level
WHERE h.psh_run_id = $1
ORDER BY h.psh_nik`

func loadBankRows(ctx context.Context, tx *sql.Tx, q Query, runID int64, key string) ([]BankRow, error) {
	rows, err := tx.QueryContext(ctx, bankSQL, runID, q.CompanyID, key)
	if err != nil {
		return nil, fmt.Errorf("read bank rows: %w", err)
	}
	defer closeRows(rows)
	var out []BankRow
	for rows.Next() {
		var r BankRow
		var net string
		if err := rows.Scan(&r.NIK, &r.Nama, &r.Dept, &r.Status, &r.LevelStatus, &r.MGTMB, &r.Bank, &r.AccountName, &r.Account, &net); err != nil {
			return nil, fmt.Errorf("scan bank row: %w", err)
		}
		if r.Net, err = decimal.NewFromString(net); err != nil {
			return nil, fmt.Errorf("bank net: %w", err)
		}
		r.Account = strings.TrimSpace(r.Account)
		out = append(out, r)
	}
	return out, rows.Err()
}

// ExportBank loads the run and the decrypted accounts, lets build write the bank file, and audits in the same
// transaction (counts only: no NIK, amount, or account number).
func (s *Service) ExportBank(ctx context.Context, q Query, key, bank string, build func(BankData) (rows int, err error)) error {
	if key == "" {
		return ErrNoPIIKey
	}
	return s.DB.ScopedTxOpts(ctx, run.SnapshotTx, func(tx *sql.Tx) error {
		d, err := Load(ctx, tx, q)
		if err != nil {
			return err
		}
		all, err := loadBankRows(ctx, tx, q, d.RunID, key)
		if err != nil {
			return err
		}
		bd := SplitBank(q.YYMM, all, bank)
		rows, err := build(bd)
		if err != nil {
			return err
		}
		return s.writeAudit(ctx, tx, q, "bank", rows, map[string]any{"pra_cek": len(bd.Issues)})
	})
}

// WriteBank writes the F-1 workbook (COVER LETTER + DETAIL) and returns the DETAIL row count. The workbook holds real
// account numbers and salaries: never commit it.
func WriteBank(d BankData, t BankText, w io.Writer) (int, error) {
	if len(d.Rows) == 0 {
		return 0, ErrNoPayableRows
	}
	f := excelize.NewFile()
	defer f.Close() //nolint:errcheck // in-memory workbook
	if err := f.SetSheetName("Sheet1", SheetCover); err != nil {
		return 0, fmt.Errorf("rename sheet: %w", err)
	}
	if _, err := f.NewSheet(SheetBank); err != nil {
		return 0, fmt.Errorf("new sheet: %w", err)
	}
	sw := &sheetWriter{f: f}
	sw.cover(d, t)
	sw.bankDetail(d, t)
	if sw.err != nil {
		return 0, sw.err
	}
	if err := f.Write(w); err != nil {
		return 0, err
	}
	return len(d.Rows), nil
}

// WriteBankPreCheck writes the pre-check workbook: NIK, name and reason only (no account number).
func WriteBankPreCheck(issues []BankIssue, w io.Writer) error {
	f := excelize.NewFile()
	defer f.Close() //nolint:errcheck // in-memory workbook
	if err := f.SetSheetName("Sheet1", "pra-cek"); err != nil {
		return err
	}
	sw := &sheetWriter{f: f}
	sw.start("pra-cek")
	sw.put("NIK", "Nama", "Alasan (tidak masuk file bank)")
	for _, i := range issues {
		sw.put(i.NIK, i.Nama, i.Reason)
	}
	if sw.err != nil {
		return sw.err
	}
	return f.Write(w)
}

// rupiah formats 1234567 as "1.234.567".
func rupiah(d decimal.Decimal) string {
	s := d.Round(0).String()
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

func (s *sheetWriter) cover(d BankData, t BankText) {
	s.start(SheetCover)
	for i := 0; i < 4; i++ {
		if i < len(t.Letterhead) {
			s.put(t.Letterhead[i])
		} else {
			s.put()
		}
	}
	s.put()
	s.put(t.TransferDate)
	s.put()
	s.put("To")
	s.put("The Manager, Bank Mandiri")
	s.put("Cabang " + t.Branch)
	s.put()
	s.put("Re: Salary transfer " + monthYear(d.YYMM))
	s.put()
	s.put("Dear Sir/Madam,")
	s.put(fmt.Sprintf("We hereby request you to process the salary payment by debiting our Escrow account number %s "+
		"with a total amount Rp %s", t.EscrowAccount, rupiah(d.Total())))
	s.put()
	s.put("Request to transfer on " + t.TransferDate.Weekday().String() + ", " + t.TransferDate.Format("2 January 2006"))
	s.put()
	s.put("Sincerely yours,")
	s.put()
	s.put()
	s.put()
	s.put(t.Signer1, nil, t.Signer2)
	s.put(t.Title1, nil, t.Title2)
	if s.err != nil {
		return
	}
	layout := "d mmmm yyyy"
	st, err := s.f.NewStyle(&excelize.Style{CustomNumFmt: &layout})
	if err != nil {
		s.err = err
		return
	}
	s.err = s.f.SetCellStyle(SheetCover, "A6", "A6", st)
}

func (s *sheetWriter) bankDetail(d BankData, t BankText) {
	s.start(SheetBank)
	s.put("DAFTAR PEMBAYARAN GAJI " + t.CompanyName)
	s.put("DI BANK MANDIRI CAB. " + t.Branch)
	s.put()
	s.put("PERIODE :", t.PeriodDate)
	s.put()
	s.put("No", "Name", "Account No.", "Nett Payable (Rp.)")
	for i, r := range d.Rows {
		acc, err := strconv.ParseInt(r.Account, 10, 64) // digits only and <= 15 by bankIssues
		if err != nil {
			s.err = err
			return
		}
		if i == 0 {
			s.put(1, r.AccountName, acc, r.Net)
			continue
		}
		s.put(nil, r.AccountName, acc, r.Net)
		s.formula(fmt.Sprintf("A%d", s.row), fmt.Sprintf("1+A%d", s.row-1))
	}
	if s.err != nil {
		return
	}
	n := len(d.Rows)
	last := bankDataRow + n - 1
	sumRow := last + 1
	s.put(nil, nil, nil)
	s.formula(fmt.Sprintf("D%d", s.row), fmt.Sprintf("SUM(D%d:D%d)", bankDataRow, last))
	s.put(nil, fmt.Sprintf("BIAYA TRANSFER Rp %s x %d", rupiah(t.FeePerTransfer), n))
	s.formula(fmt.Sprintf("D%d", s.row), fmt.Sprintf("%s*%d", t.FeePerTransfer.String(), n))
	s.put(nil, "TOTAL")
	s.formula(fmt.Sprintf("D%d", s.row), fmt.Sprintf("D%d+D%d", sumRow, sumRow+1))
	s.bankStyles(n, sumRow)
}

func (s *sheetWriter) formula(cell, f string) {
	if s.err == nil {
		s.err = s.f.SetCellFormula(s.sheet, cell, f)
	}
}

func (s *sheetWriter) bankStyles(n, sumRow int) {
	if s.err != nil {
		return
	}
	acc, err := s.f.NewStyle(&excelize.Style{NumFmt: 1}) // 0
	if err != nil {
		s.err = err
		return
	}
	amt, err := s.f.NewStyle(&excelize.Style{NumFmt: 3}) // #,##0
	if err != nil {
		s.err = err
		return
	}
	date, err := s.f.NewStyle(&excelize.Style{NumFmt: 15}) // d-mmm-yy
	if err != nil {
		s.err = err
		return
	}
	last := bankDataRow + n - 1
	for _, r := range []struct {
		a, b string
		st   int
	}{
		{fmt.Sprintf("C%d", bankDataRow), fmt.Sprintf("C%d", last), acc},
		{fmt.Sprintf("D%d", bankDataRow), fmt.Sprintf("D%d", sumRow+2), amt},
		{"B4", "B4", date},
	} {
		if s.err == nil {
			s.err = s.f.SetCellStyle(SheetBank, r.a, r.b, r.st)
		}
	}
}
