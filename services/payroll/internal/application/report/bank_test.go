package report

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func bankRow(nik, acc, name, bank, net string) BankRow {
	return BankRow{NIK: nik, Nama: name, AccountName: name, Account: acc, Bank: bank, Net: dec(net)}
}

func TestSplitBankPreChecks(t *testing.T) {
	all := []BankRow{
		bankRow("A", "1234567890", "A", "Bank Mandiri", "1000000"),
		bankRow("B", "1234567890", "B", "Bank Mandiri", "0"),
		bankRow("C", "", "C", "Bank Mandiri", "10"),
		bankRow("D", "12a45", "D", "Bank Mandiri", "10"),
		bankRow("E", "0123", "E", "Bank Mandiri", "10"),
		bankRow("F", "1234567890", "F", "BCA", "10"),
		bankRow("G", "1234567890", "", "Bank Mandiri", "10"),
		bankRow("H", "1234567890", "H", "Bank Mandiri", "100.50"),
	}
	d := SplitBank(202701, all, MandiriBank)
	if len(d.Rows) != 1 || d.Rows[0].NIK != "A" || len(d.Issues) != 7 {
		t.Fatalf("rows=%d issues=%d", len(d.Rows), len(d.Issues))
	}
	for _, i := range d.Issues {
		if strings.Contains(i.Reason, "1234567890") || strings.Contains(i.Reason, "0123") {
			t.Fatalf("issue leaks account: %+v", i)
		}
	}
}

func TestWriteBankFormulasAndTotals(t *testing.T) {
	d := BankData{YYMM: 202701, Rows: []BankRow{
		bankRow("A", "1234567890", "Aa", "MANDIRI", "1500000"),
		bankRow("B", "9876543210", "Bb", "MANDIRI", "2500000"),
		bankRow("C", "5555555555", "Cc", "MANDIRI", "100"),
	}}
	td := time.Date(2027, 1, 26, 0, 0, 0, 0, time.UTC)
	txt := BankText{Letterhead: []string{"PT X"}, CompanyName: "PT. X", Branch: "SOLO", EscrowAccount: "9999",
		Signer1: "S1", Title1: "T1", Signer2: "S2", Title2: "T2", FeePerTransfer: dec("2200"), TransferDate: td, PeriodDate: td}
	var buf bytes.Buffer
	n, err := WriteBank(d, txt, &buf)
	if err != nil || n != 3 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	f, err := excelize.OpenReader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	for cell, want := range map[string]string{"A8": "1+A7", "A9": "1+A8", "D10": "SUM(D7:D9)", "D11": "2200*3", "D12": "D10+D11"} {
		if got, _ := f.GetCellFormula(SheetBank, cell); got != want { //nolint:errcheck // asserted
			t.Fatalf("%s formula = %q, want %q", cell, got, want)
		}
	}
	if v, _ := f.GetCellValue(SheetBank, "A7", excelize.Options{RawCellValue: true}); v != "1" { //nolint:errcheck // asserted
		t.Fatalf("A7 = %q", v)
	}
	if v, _ := f.GetCellValue(SheetBank, "C7", excelize.Options{RawCellValue: true}); v != "1234567890" { //nolint:errcheck // asserted
		t.Fatalf("C7 = %q", v)
	}
	if v, _ := f.GetCellValue(SheetBank, "B11"); v != "BIAYA TRANSFER Rp 2.200 x 3" { //nolint:errcheck // asserted
		t.Fatalf("B11 = %q", v)
	}
	rows, _ := f.GetRows(SheetCover) //nolint:errcheck // asserted
	var found bool
	for _, r := range rows {
		if len(r) > 0 && strings.Contains(r[0], "Rp 4.000.100") {
			found = true
		}
	}
	if !found {
		t.Fatalf("cover total missing: %v", rows)
	}
}

func TestRupiah(t *testing.T) {
	for in, want := range map[string]string{"0": "0", "999": "999", "1000": "1.000", "1234567": "1.234.567"} {
		if got := rupiah(dec(in)); got != want {
			t.Fatalf("%s -> %s", in, got)
		}
	}
}

func TestWriteBankNoRows(t *testing.T) {
	var buf bytes.Buffer
	if _, err := WriteBank(BankData{YYMM: 202701}, BankText{}, &buf); !errors.Is(err, ErrNoPayableRows) || buf.Len() != 0 {
		t.Fatalf("err=%v len=%d", err, buf.Len())
	}
}
