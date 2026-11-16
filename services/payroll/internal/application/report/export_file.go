package report

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/config"
)

// File is one finished workbook: the bytes are only handed out after the audit row committed (T030d).
type File struct {
	Name    string
	Content []byte
	Rows    int
	// Skipped is the number of rows left out of the file (BANK: employees failing the pre-check) or flagged
	// (RECO: components without a GL setup); the CLI lists them in a separate workbook.
	Skipped int
}

// NewBankText builds the cover/title text of the bank file from the report configuration and the transfer date; the
// CLI and the API share it so both produce the same workbook.
func NewBankText(rc config.ReportConfig, transferDate time.Time) BankText {
	return BankText{Letterhead: rc.Letterhead, CompanyName: rc.CompanyName, Branch: rc.BankBranch,
		EscrowAccount: rc.EscrowAccount, Signer1: rc.Signer1, Title1: rc.Title1, Signer2: rc.Signer2, Title2: rc.Title2,
		FeePerTransfer: rc.TransferFee, TransferDate: transferDate, PeriodDate: transferDate}
}

// RecoFile builds the F-2 reconciliation workbook in memory (same ExportReco + WriteReco as `cmd/report reco`).
func (s *Service) RecoFile(ctx context.Context, q Query, key string) (File, error) {
	var buf bytes.Buffer
	f := File{Name: fileName("reco", q)}
	err := s.ExportReco(ctx, q, key, func(d RecoData) (int, error) {
		n, err := WriteReco(d, &buf)
		f.Rows, f.Skipped = n, len(PreChecks(d.Rows))
		return n, err
	})
	if err != nil {
		return File{}, err
	}
	f.Content = buf.Bytes()
	return f, nil
}

// TextLoader returns the bank cover/title text for a transfer date. It runs only after the period check, so a missing
// PAYROLL_REPORT_* setting never hides a period error (and nothing is audited when it fails).
type TextLoader func(transferDate time.Time) (BankText, error)

// BankFile builds the F-1 Mandiri workbook in memory (same ExportBank + WriteBank as `cmd/report bank`). Employees
// failing the pre-check are counted in Skipped, not written; a period without a single payable row is
// ErrNoPayableRows and nothing is audited.
func (s *Service) BankFile(ctx context.Context, q Query, key, bank string, transferDate time.Time, load TextLoader) (File, error) {
	var buf bytes.Buffer
	f := File{Name: fileName("bank", q)}
	q.Extra = map[string]any{"transfer_date": transferDate.Format("2006-01-02")}
	err := s.ExportBank(ctx, q, key, bank, func(d BankData) (int, error) {
		f.Skipped = len(d.Issues)
		text, err := load(transferDate)
		if err != nil {
			return 0, fmt.Errorf("%w: %w", ErrBankConfig, err)
		}
		n, err := WriteBank(d, text, &buf)
		f.Rows = n
		return n, err
	})
	if err != nil {
		return File{}, err
	}
	f.Content = buf.Bytes()
	return f, nil
}

// fileName is <kind>_<division>_<yymm>.xlsx with the division reduced to [A-Za-z0-9_-] (it ends up in a header).
func fileName(kind string, q Query) string {
	div := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return -1
	}, q.Division)
	return fmt.Sprintf("%s_%s_%d.xlsx", kind, div, q.YYMM)
}
