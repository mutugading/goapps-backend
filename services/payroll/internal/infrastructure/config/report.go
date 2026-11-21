package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/shopspring/decimal"
)

// ReportConfig is the fixed text of the bank file (F-1, D-62 (3)): company letterhead, branch, escrow account, signers.
// It is read from PAYROLL_REPORT_* at runtime and never lives in the repository.
type ReportConfig struct {
	Letterhead      []string        // PAYROLL_REPORT_LETTERHEAD, lines separated by "|" (max 4 used)
	CompanyName     string          // PAYROLL_REPORT_COMPANY_NAME
	BankBranch      string          // PAYROLL_REPORT_BANK_BRANCH
	EscrowAccount   string          // PAYROLL_REPORT_ESCROW_ACCOUNT
	Signer1, Title1 string          // PAYROLL_REPORT_SIGNER1_NAME / _TITLE
	Signer2, Title2 string          // PAYROLL_REPORT_SIGNER2_NAME / _TITLE
	TransferFee     decimal.Decimal // PAYROLL_REPORT_TRANSFER_FEE, default 2200
}

// DefaultTransferFee is the Mandiri fee per transfer in IDR (F-1).
const DefaultTransferFee = "2200"

// LoadReport reads and validates the report text from the environment.
func LoadReport() (ReportConfig, error) {
	c := ReportConfig{
		CompanyName:   os.Getenv("PAYROLL_REPORT_COMPANY_NAME"),
		BankBranch:    os.Getenv("PAYROLL_REPORT_BANK_BRANCH"),
		EscrowAccount: os.Getenv("PAYROLL_REPORT_ESCROW_ACCOUNT"),
		Signer1:       os.Getenv("PAYROLL_REPORT_SIGNER1_NAME"),
		Title1:        os.Getenv("PAYROLL_REPORT_SIGNER1_TITLE"),
		Signer2:       os.Getenv("PAYROLL_REPORT_SIGNER2_NAME"),
		Title2:        os.Getenv("PAYROLL_REPORT_SIGNER2_TITLE"),
	}
	if lh := os.Getenv("PAYROLL_REPORT_LETTERHEAD"); lh != "" {
		c.Letterhead = strings.Split(lh, "|")
	}
	fee := os.Getenv("PAYROLL_REPORT_TRANSFER_FEE")
	if fee == "" {
		fee = DefaultTransferFee
	}
	var err error
	if c.TransferFee, err = decimal.NewFromString(fee); err != nil || c.TransferFee.IsNegative() {
		return c, fmt.Errorf("config: PAYROLL_REPORT_TRANSFER_FEE %q bukan angka >= 0", fee)
	}
	return c, c.Validate()
}

// Validate requires the text without which the bank letter would be incomplete.
func (c ReportConfig) Validate() error {
	var missing []string
	for _, f := range []struct{ name, v string }{
		{"PAYROLL_REPORT_COMPANY_NAME", c.CompanyName}, {"PAYROLL_REPORT_BANK_BRANCH", c.BankBranch},
		{"PAYROLL_REPORT_ESCROW_ACCOUNT", c.EscrowAccount}, {"PAYROLL_REPORT_SIGNER1_NAME", c.Signer1},
		{"PAYROLL_REPORT_SIGNER1_TITLE", c.Title1}, {"PAYROLL_REPORT_SIGNER2_NAME", c.Signer2},
		{"PAYROLL_REPORT_SIGNER2_TITLE", c.Title2},
	} {
		if strings.TrimSpace(f.v) == "" {
			missing = append(missing, f.name)
		}
	}
	if len(missing) > 0 {
		return errors.New("config: wajib diisi: " + strings.Join(missing, ", "))
	}
	return nil
}
