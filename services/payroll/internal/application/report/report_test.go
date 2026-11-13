package report

import (
	"bytes"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"
)

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func sample() RunData {
	comps := []Comp{{Type: "P", ID: 2, Name: "BPJS", Serial: 20}, {Type: "T", ID: 1, Name: "Gaji", Serial: 10}, {Type: "T", ID: 5, Name: "Unknown", Serial: unknownSerial}}
	sortComps(comps)
	return RunData{Comps: comps, Emps: []Emp{
		{NIK: "A1", Nama: "A", TotTunjangan: dec("1500.50"), TotPotongan: dec("100"), Rounding: dec("0.50"), NetPay: dec("1400"),
			Amounts: map[string]decimal.Decimal{"T1": dec("1000.50"), "T5": dec("500"), "P2": dec("100")}},
		{NIK: "B2", Nama: "B", TotTunjangan: dec("2000"), TotPotongan: dec("0"), Rounding: dec("0"), NetPay: dec("2000"),
			Amounts: map[string]decimal.Decimal{"T1": dec("2000")}},
	}}
}

func TestSortCompsBySerial(t *testing.T) {
	got := sample().Comps
	want := []string{"T1", "P2", "T5"}
	for i, k := range want {
		if got[i].Key() != k {
			t.Fatalf("order[%d] = %s, want %s", i, got[i].Key(), k)
		}
	}
}

func TestSummarizeMatchesHead(t *testing.T) {
	s := Summarize(sample())
	if !s.SumT.Equal(s.HeadTunj) || !s.SumP.Equal(s.HeadPot) {
		t.Fatalf("T %s/%s P %s/%s", s.SumT, s.HeadTunj, s.SumP, s.HeadPot)
	}
	if !s.HeadNet.Equal(s.HeadTunj.Sub(s.HeadPot).Sub(s.Rounding)) {
		t.Fatalf("net %s", s.HeadNet)
	}
	if s.NIKCount != 2 || s.CountPerComp["T1"] != 2 || s.CountPerComp["P2"] != 1 || !s.PerComp["T1"].Equal(dec("3000.50")) {
		t.Fatalf("summary %+v", s)
	}
}

func TestWriteReconKomponen(t *testing.T) {
	var buf bytes.Buffer
	n, err := WriteReconKomponen(sample(), &buf)
	if err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	f, err := excelize.OpenReader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := f.GetRows(SheetDetail) //nolint:errcheck // checked by content
	if len(rows) != 3 || rows[0][3] != "T1 Gaji" || rows[0][4] != "P2 BPJS" {
		t.Fatalf("header/rows %v", rows)
	}
	// Zero lines are left empty: B2 has no P2 and no T5.
	if (len(rows[2]) > 4 && rows[2][4] != "") || (len(rows[2]) > 5 && rows[2][5] != "") {
		t.Fatalf("zero line printed: %v", rows[2])
	}
	if sum, _ := f.GetRows(SheetSummary); len(sum) < 8 { //nolint:errcheck // length check
		t.Fatalf("summary %v", sum)
	}
}
