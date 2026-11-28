package report

import (
	"bytes"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"
)

func TestBuildOTRowActualHoursAndRupiah(t *testing.T) {
	jam := map[string]decimal.Decimal{"lembur1": dec("3"), "nas1": dec("4"), "nas2": dec("6"), "nas3": dec("8"), "off2": dec("6")}
	rate := dec("10000.33")

	non := BuildOTRow("N1", "Non", "D", "Staff", rate, jam)
	if !non.Actual[0].Equal(dec("2")) || !non.Actual[2].Equal(dec("2")) || !non.Actual[3].Equal(dec("2")) || !non.Actual[4].Equal(dec("2")) || !non.Actual[6].Equal(dec("2")) {
		t.Fatalf("non-exec actual %v", non.Actual)
	}
	exec := BuildOTRow("E1", "Exec", "D", "Executive", rate, jam)
	// Executive NAS columns are all x2: paid 4/6/8 -> actual 2/3/4.
	if !exec.Actual[2].Equal(dec("2")) || !exec.Actual[3].Equal(dec("3")) || !exec.Actual[4].Equal(dec("4")) {
		t.Fatalf("exec actual %v", exec.Actual)
	}
	if !non.Rupiah[0].Equal(dec("30000.99")) { // round(3 x 10000.33, 2)
		t.Fatalf("rupiah %s", non.Rupiah[0])
	}
	if !non.TotalRupiah().Equal(sumDec(non.Rupiah[:])) || !non.TotalPaid().Equal(dec("27")) {
		t.Fatalf("totals %s %s", non.TotalRupiah(), non.TotalPaid())
	}
}

func TestRegTotalIsFixedIncomeOnly(t *testing.T) {
	r := RegRow{Amt: [8]decimal.Decimal{dec("100"), dec("10"), dec("5"), dec("1"), dec("2"), dec("3"), dec("50"), dec("60")}}
	if !r.Total().Equal(dec("121")) {
		t.Fatalf("total %s", r.Total())
	}
}

func TestWriteRecoNewSheets(t *testing.T) {
	tax := 2
	d := RecoData{RunData: sample(), YYMM: 202610,
		Additions: []AdditionRow{{NIK: "A1", Nama: "A", Type: "Tunjangan", ID: 100, Name: "Award", Sts: "F", Tax: &tax, Amount: dec("500"), Remarks: "Award", Division: "SOLO"}},
		Reg:       []RegRow{{NIK: "A1", Nama: "A", Amt: [8]decimal.Decimal{dec("100"), {}, {}, {}, {}, {}, {}, {}}}},
		Overtime:  []OTRow{BuildOTRow("A1", "A", "D", "Staff", dec("100"), map[string]decimal.Decimal{"lembur1": dec("3")})}}
	var buf bytes.Buffer
	if _, err := WriteReco(d, &buf); err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	add, _ := f.GetRows(SheetAdditional) //nolint:errcheck // content checked
	if len(add) != 2 || len(add[0]) != 11 || add[1][8] != "500" || add[1][7] != "2" {
		t.Fatalf("additional %v", add)
	}
	reg, _ := f.GetRows(SheetReg) //nolint:errcheck // content checked
	if len(reg) != 4 || len(reg[2]) != 17 || reg[3][8] != "100" || reg[3][14] != "100" {
		t.Fatalf("reg %v", reg)
	}
	ot, _ := f.GetRows(SheetOvertime) //nolint:errcheck // content checked
	if len(ot) != 5 || ot[4][1] != "A1" {
		t.Fatalf("ot %v", ot)
	}
}
