package recon

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"
)

func TestDecimalCell(t *testing.T) {
	if v, ok := decimalCell(decimal.RequireFromString("1234567.50")).(float64); !ok || v != 1234567.5 {
		t.Errorf("short value must be numeric, got %v", v)
	}
	long := "12345678901234567.89"
	if v, ok := decimalCell(decimal.RequireFromString(long)).(string); !ok || v != long {
		t.Errorf("long value must stay exact text, got %v", v)
	}
}

func TestTopLines_Order(t *testing.T) {
	d := func(s string) decimal.Decimal { return decimal.RequireFromString(s) }
	got := TopLines([]LineDiff{
		{NIK: "B", Component: "T001", Delta: d("-5")},
		{NIK: "A", Component: "T002", Delta: d("5")},
		{NIK: "A", Component: "T001", Delta: d("5")},
		{NIK: "C", Component: "P001", Delta: d("9")},
		{NIK: "D", Component: "P001", Delta: d("1")},
	}, 4)
	want := []string{"C/P001", "A/T001", "A/T002", "B/T001"}
	if len(got) != len(want) {
		t.Fatalf("len %d", len(got))
	}
	for i, l := range got {
		if g := l.NIK + "/" + l.Component; g != want[i] {
			t.Errorf("pos %d: %s want %s", i, g, want[i])
		}
	}
}

func TestWriteXLSX(t *testing.T) {
	rep := Diff(diffFixture())
	// pad with 60 FORMULA lines so Top50 is truncated
	for i := 0; i < 60; i++ {
		rep.Lines = append(rep.Lines, LineDiff{NIK: fmt.Sprintf("X%03d", i), Component: "T001",
			Delta: decimal.NewFromInt(int64(5000 + i)), Category: CatFormula})
	}
	var buf bytes.Buffer
	if err := WriteXLSX(rep, &buf); err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() //nolint:errcheck // test
	if got := f.GetSheetList(); len(got) != 4 || got[0] != SheetSummary || got[1] != SheetTop50 || got[2] != SheetPerNIK || got[3] != SheetDerived {
		t.Fatalf("sheets %v", got)
	}
	top, _ := f.GetRows(SheetTop50)
	if len(top) != topN+1 {
		t.Fatalf("Top50 rows %d", len(top))
	}
	if top[1][0] != "X059" || top[1][4] != "5059" {
		t.Errorf("first Top50 row %v", top[1])
	}
	per, _ := f.GetRows(SheetPerNIK)
	niks := map[string]bool{}
	for _, l := range rep.Lines {
		niks[l.NIK] = true
	}
	for _, h := range rep.Heads {
		niks[h.NIK] = true
	}
	lines := len(rep.Lines)
	extra := 0
	seen := map[string]bool{}
	for _, l := range rep.Lines {
		seen[l.NIK] = true
	}
	for _, h := range rep.Heads {
		if !seen[h.NIK] {
			extra++
		}
	}
	if len(per) != 1+lines+extra {
		t.Errorf("PerNIK rows %d want %d", len(per), 1+lines+extra)
	}
	der, _ := f.GetRows(SheetDerived)
	if len(der) != 1+len(rep.AttDerived) || len(rep.AttDerived) != 1 || der[1][0] != "D-ATT" || der[1][1] != DerivedShift {
		t.Errorf("AttDerived rows %v (%+v)", der, rep.AttDerived)
	}
	sum, _ := f.GetRows(SheetSummary)
	found := false
	for _, r := range sum {
		if len(r) > 1 && r[0] == "Verdict paritas" && r[1] == "TIDAK PARITAS" {
			found = true
		}
	}
	if !found {
		t.Errorf("verdict missing: %v", sum)
	}
}
