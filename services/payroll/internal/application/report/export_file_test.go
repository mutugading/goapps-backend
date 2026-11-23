package report

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/config"
)

func TestFileNameSanitized(t *testing.T) {
	if got := fileName("reco", Query{Division: "SO\r\nLO\"x", YYMM: 202701}); got != "reco_SOLOx_202701.xlsx" {
		t.Fatal(got)
	}
}

func TestNewBankText(t *testing.T) {
	td := time.Date(2027, 1, 26, 0, 0, 0, 0, time.UTC)
	got := NewBankText(config.ReportConfig{CompanyName: "PT X", BankBranch: "SOLO", EscrowAccount: "9", Signer1: "A", Title1: "a",
		Signer2: "B", Title2: "b", TransferFee: decimal.NewFromInt(2200), Letterhead: []string{"L1"}}, td)
	if got.CompanyName != "PT X" || got.Branch != "SOLO" || !got.TransferDate.Equal(td) || !got.PeriodDate.Equal(td) ||
		!got.FeePerTransfer.Equal(decimal.NewFromInt(2200)) || len(got.Letterhead) != 1 {
		t.Fatalf("%+v", got)
	}
}
