package report

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"
)

func glBase() GLInput {
	return GLInput{
		YYMM: 202610,
		Rate: dec("0.000055561729"),
		Heads: []GLHead{
			{NIK: "ZN001", Dept: "TXT", Group: "Operator", CostGroup: "MGT", SubAcnt: "0101", Tunj: dec("1000"), Pot: dec("100"), Rounding: dec("-50")},
			{NIK: "ZN002", Dept: "TXT", Group: "Operator", CostGroup: "MGT", SubAcnt: "0102", Tunj: dec("500"), Pot: dec("0"), Rounding: dec("20")},
		},
		Lines: []GLLine{
			{NIK: "ZN001", Type: "Tunjangan", No: 1, Amount: dec("1000"), HasSetup: true, Main: "403000", Sub: "403001", DrCr: "D", Desc: "Gaji Solo"},
			{NIK: "ZN001", Type: "Potongan", No: 9, Amount: dec("100"), HasSetup: true, Main: "205037", DrCr: "C", Desc: "Hutang BPJS"},
			{NIK: "ZN002", Type: "Tunjangan", No: 1, Amount: dec("500"), HasSetup: true, Main: "403000", Sub: "403001", DrCr: "D", Desc: "Gaji Solo"},
		},
		Payables: map[string]GLAccount{"Operator": {Main: "205012", DrCr: "C", Desc: "Salary Payables"}},
	}
}

func build(in GLInput) (GLJournal, error) { return BuildGL(withNet(in)) }

func netOf(h GLHead) *decimal.Decimal {
	n := h.Tunj.Sub(h.Pot).Sub(h.Rounding)
	return &n
}

// withNet fills psh_net_pay = T − P − R on every head (as the generated column does).
func withNet(in GLInput) GLInput {
	for i := range in.Heads {
		in.Heads[i].NetPay = netOf(in.Heads[i])
	}
	return in
}

func findRow(j GLJournal, main string) (GLRow, bool) {
	for _, r := range j.Rows {
		if r.Main == main {
			return r, true
		}
	}
	return GLRow{}, false
}

func TestBuildGL_PayablesRoundingBalanced(t *testing.T) {
	j, err := build(glBase())
	if err != nil {
		t.Fatal(err)
	}
	// Gaji Solo = 1500 − Σ rounding (−30) = 1530; payables = Σ (T − P − R) = 950 + 480 = 1430; BPJS 100.
	g, _ := findRow(j, "403000")
	p, _ := findRow(j, "205012")
	if !g.FC.Equal(dec("1530")) || g.DrCr != "D" || g.Desc != "Gaji Solo Oct 2026" {
		t.Fatalf("gaji %+v", g)
	}
	if !p.FC.Equal(dec("1430")) || p.DrCr != "C" || p.Desc != "Salary Payables Oct 2026" || !p.FC.Equal(j.NetPay) {
		t.Fatalf("payables %+v net %s", p, j.NetPay)
	}
	if !j.Debit.Equal(j.Credit) || !j.Debit.Equal(dec("1530")) || len(j.Rows) != 3 {
		t.Fatalf("D %s C %s rows %d", j.Debit, j.Credit, len(j.Rows))
	}
	if !g.LC.Equal(dec("0.09")) { // ROUND(1530 × 0.000055561729, 2) = 0.085009... → 0.09
		t.Fatalf("lc %s", g.LC)
	}
}

func TestBuildGL_NegativeFlipsDrCr(t *testing.T) {
	in := glBase()
	// A negative earning (correction) larger than the other earnings of its account flips D → C, amount abs.
	in.Lines = append(in.Lines, GLLine{NIK: "ZN001", Type: "Tunjangan", No: 5, Amount: dec("-300"), HasSetup: true, Main: "406000", Sub: "406001", DrCr: "D", Desc: "Koreksi"})
	in.Heads[0].Tunj = dec("700")
	j, err := build(in)
	if err != nil {
		t.Fatal(err)
	}
	r, ok := findRow(j, "406000")
	if !ok || r.DrCr != "C" || !r.FC.Equal(dec("300")) {
		t.Fatalf("flip %+v", r)
	}
	if !j.Debit.Equal(j.Credit) {
		t.Fatalf("unbalanced %s %s", j.Debit, j.Credit)
	}
}

func TestBuildGL_SubFromEmployeeAndMissing(t *testing.T) {
	in := glBase()
	in.Lines = append(in.Lines, GLLine{NIK: "ZN002", Type: "Potongan", No: 70, Amount: dec("10"), HasSetup: true, Main: "105013", DrCr: "C", Desc: "Other Deduction", SubFromEmployee: true})
	in.Heads[1].Pot = dec("10")
	j, err := build(in)
	if err != nil {
		t.Fatal(err)
	}
	if r, _ := findRow(j, "105013"); r.Sub != "0102" {
		t.Fatalf("sub %+v", r)
	}
	in.Heads[1].SubAcnt = ""
	_, err = build(in)
	var ge *GLError
	if !errors.As(err, &ge) || !errors.Is(err, ErrGL) || !strings.Contains(err.Error(), GLSubacntMissing) {
		t.Fatalf("want %s, got %v", GLSubacntMissing, err)
	}
}

func TestBuildGL_Rejects(t *testing.T) {
	cases := []struct {
		name string
		mod  func(*GLInput)
		code string
	}{
		{"unmapped", func(in *GLInput) { in.Lines[1].HasSetup = false }, GLUnmapped},
		{"group", func(in *GLInput) { in.Heads[0].Group = "" }, GLGroupUnmapped},
		{"cost group", func(in *GLInput) { in.Heads[0].CostGroup = "" }, GLCostGroupMissing},
		{"payable", func(in *GLInput) { in.Payables = map[string]GLAccount{} }, GLPayableUnmapped},
		{"loan", func(in *GLInput) {
			in.Lines = append(in.Lines, GLLine{NIK: "ZN001", Type: "Potongan", No: 32, Amount: dec("5"), HasSetup: true, Main: "105001", DrCr: "C", Desc: "Cicilan"})
		}, GLLoanUnsupported},
		{"rounding without gaji line", func(in *GLInput) {
			for i := range in.Lines {
				in.Lines[i].Desc = "Gaji Pokok"
			}
		}, GLRoundingUnplaced},
		{"unbalanced", func(in *GLInput) { in.Heads[0].Tunj = dec("1001") }, GLUnbalanced},
	}
	for _, c := range cases {
		in := glBase()
		c.mod(&in)
		if _, err := build(in); !errors.Is(err, ErrGL) || !strings.Contains(err.Error(), c.code) {
			t.Errorf("%s: want %s, got %v", c.name, c.code, err)
		}
	}
	in := glBase()
	in.Rate = dec("0")
	if _, err := build(in); !errors.Is(err, ErrGL) || !strings.Contains(err.Error(), GLRateInvalid) {
		t.Errorf("rate 0: %v", err)
	}
	in = glBase()
	in.Heads, in.Lines = nil, nil
	if _, err := build(in); !strings.Contains(fmt.Sprint(err), GLEmpty) {
		t.Errorf("no heads: %v", err)
	}
	// heads whose lines and payables are all zero → no journal line
	in = glBase()
	in.Lines = nil
	for i := range in.Heads {
		in.Heads[i].Tunj, in.Heads[i].Pot, in.Heads[i].Rounding = dec("0"), dec("0"), dec("0")
	}
	if _, err := build(in); !strings.Contains(fmt.Sprint(err), GLEmpty) {
		t.Errorf("no rows: %v", err)
	}
}

func TestBuildGL_NetPayAndMaskedError(t *testing.T) {
	in := withNet(glBase())
	wrong := dec("1")
	in.Heads[0].NetPay = &wrong
	in.Heads[1].NetPay = nil
	_, err := BuildGL(in)
	var ge *GLError
	if !errors.As(err, &ge) || !strings.Contains(err.Error(), GLNetPayMismatch+"=2") {
		t.Fatalf("net pay: %v", err)
	}
	if strings.Contains(err.Error(), "ZN00") {
		t.Fatalf("error carries NIK: %v", err)
	}
	if d := ge.Details(); len(d) != 2 || d[0] != GLNetPayMismatch+" ZN001" {
		t.Fatalf("details %q", d)
	}
	j, err := build(glBase())
	if err != nil || !j.NetPay.Equal(j.Payables) || !j.NetPay.Equal(dec("1430")) {
		t.Fatalf("net %s payables %s %v", j.NetPay, j.Payables, err)
	}
	if !j.LCDebit.Equal(dec("0.09")) || !j.LCCredit.Equal(dec("0.09")) { // 1530 D vs 1430+100 C, each ROUND(×rate,2)
		t.Fatalf("lc %s %s", j.LCDebit, j.LCCredit)
	}
}

func TestWriteGL_Layout(t *testing.T) {
	j, err := build(glBase())
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := WriteGL(j, &buf); err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	for cell, want := range map[string]string{"A1": "SALARY SETUP Gaji", "A3": "Processing Month 202610", "A4": "No.",
		"M4": "LC Amount", "A5": "1", "F5": "MGT", "B5": "202610"} {
		if v, _ := f.GetCellValue(SheetUpload, cell); v != want { //nolint:errcheck // asserted
			t.Errorf("%s = %q, want %q", cell, v, want)
		}
	}
	if v, _ := f.GetCellValue(SheetUpload, "A8"); v != "" { //nolint:errcheck // asserted
		t.Errorf("extra row %q", v)
	}
	rows, err := f.GetRows(SheetGLCheck)
	if err != nil || len(rows) < 12 {
		t.Fatalf("check rows %d %v", len(rows), err)
	}
	if (GLRow{CostGroup: "CONTRACT"}).MGTMB() != "Contract" {
		t.Error("MGTMB")
	}
}
