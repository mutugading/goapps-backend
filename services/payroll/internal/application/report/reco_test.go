package report

import (
	"bytes"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"
)

func TestRecoEarnDeducAndDescription(t *testing.T) {
	tun := DetailRow{Type: "Tunjangan", Amount: dec("100"), HasSetup: true, SetupDesc: "Gaji Pokok"}
	pot := DetailRow{Type: "Potongan", Amount: dec("40"), HasSetup: true, SetupDesc: "BPJS"}
	if !tun.EarnDeduc().Equal(dec("100")) || !pot.EarnDeduc().Equal(dec("-40")) {
		t.Fatalf("earnduduc %s %s", tun.EarnDeduc(), pot.EarnDeduc())
	}
	if got := tun.MPSDescription(202610); got != "Gaji Pokok Oct 2026" {
		t.Fatalf("desc %q", got)
	}
	if got := (DetailRow{Type: "Tunjangan"}).MPSDescription(202610); got != "" {
		t.Fatalf("no setup desc %q", got)
	}
}

func TestWriteRecoNoSetupRowsAndPreCheck(t *testing.T) {
	join := time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)
	b := dec("500")
	d := RecoData{RunData: sample(), YYMM: 202610, Rows: []DetailRow{
		{NIK: "A1", Nama: "A", Type: "Tunjangan", No: 1, Description: "Gaji", Amount: dec("1000.50"), DateJoin: &join, BankAcnt: "0123",
			Basis: &b, HasSetup: true, Akun: "405300-405301", Main: "405300", Sub: "405301", DrCr: "D", SetupDesc: "Gaji Solo"},
		{NIK: "A1", Nama: "A", Type: "Potongan", No: 2, Description: "BPJS", Amount: dec("100"), HasSetup: true, Main: "211", DrCr: "C", SetupDesc: "BPJS"},
		{NIK: "B2", Nama: "B", Type: "Tunjangan", No: 9, Description: "Tanpa Setup", Amount: dec("7")},
		{NIK: "C3", Nama: "C", Type: "Tunjangan", No: 9, Description: "Tanpa Setup", Amount: dec("8")},
	}}
	var buf bytes.Buffer
	n, err := WriteReco(d, &buf)
	if err != nil || n != 4 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	f, err := excelize.OpenReader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := f.GetRows(SheetRecoDetail) //nolint:errcheck // content checked
	if len(rows) != 5 || len(rows[0]) != 25 || rows[0][21] != "EARNDEDUC" {
		t.Fatalf("detail shape: %d rows %v", len(rows), rows[0])
	}
	if rows[1][11] != "0123" || rows[1][16] != "405300-405301" || rows[1][20] != "Gaji Solo Oct 2026" || rows[1][21] != "1000.5" {
		t.Fatalf("row1 %v", rows[1])
	}
	if rows[2][21] != "-100" || rows[2][19] != "C" {
		t.Fatalf("row2 %v", rows[2])
	}
	for _, i := range []int{16, 17, 18, 19, 20} { // MPS_* empty when no setup
		if len(rows[3]) > i && rows[3][i] != "" {
			t.Fatalf("row3 MPS col %d = %q", i, rows[3][i])
		}
	}
	pre, _ := f.GetRows(SheetPreCheck) //nolint:errcheck // content checked
	if len(pre) != 2 || pre[1][1] != "9" || pre[1][3] != "2" {
		t.Fatalf("pra-cek %v", pre)
	}
	if got := PreChecks(d.Rows); len(got) != 1 || got[0].Lines != 2 {
		t.Fatalf("prechecks %+v", got)
	}
	_ = decimal.Zero
}
