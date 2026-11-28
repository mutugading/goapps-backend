package etl

import (
	"fmt"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

type memSource map[string]*Table

func (m memSource) Read(name string) (*Table, error) {
	if t, ok := m[name]; ok {
		return t, nil
	}
	return nil, fmt.Errorf("no table %s", name)
}

func tbl(name, header string, rows ...string) *Table {
	t := &Table{Name: name, Cols: strings.Split(header, "|")}
	for _, r := range rows {
		t.Rows = append(t.Rows, strings.Split(r, "|"))
	}
	return t
}

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// T032b: the dump aggregate follows the loader: D-35.1 skip reported apart, D-35.2 synthetic MAIN (round once per
// NIK, rounding 0), per-cell 2-decimal rounding, net = tunjangan - potongan - rounding, range cut.
func TestAggregateSalaryLegacy(t *testing.T) {
	src := memSource{
		TableSalaryMain: tbl(TableSalaryMain, "PM_YYMM|PM_NIK|PM_DIVISION|PM_TOT_TUNJANGAN|PM_TOT_POTONGAN|PM_ROUNDING",
			"202603|A1|SOLO|100.50|10|0.25",
			"202603|A2|SOLO||5|",
			"202512|J1|JAKARTA|99|0|0", // D-35.1 skip
			"202604|A1|SOLO|7|0|0",     // out of range
			"2026|A1|SOLO|7|0|0"),      // bad key
		TableSalaryDetail: tbl(TableSalaryDetail, "PD_YYMM|PD_NIK|PD_TYPE|PD_NO|PD_DIVISION|PD_AMOUNT",
			"202603|A1|Tunjangan|1|SOLO|100.504",
			"202603|A1|Potongan|32|SOLO|10",
			"202603|A9|Tunjangan|1|SOLO|100.005", // orphan: synthetic MAIN
			"202603|A9|Potongan|32|SOLO|1.234",
			"202603|A9|Tunjangan|2|SOLO|",
			"202512|J1|Tunjangan|1|JAKARTA|99", // D-35.1 skip
			"202604|A1|Tunjangan|1|SOLO|5"),
	}
	got, err := AggregateSalaryLegacy(src, 202501, 202603)
	if err != nil {
		t.Fatal(err)
	}
	if got.Rejected != 1 { // the MAIN row with yymm "2026"
		t.Errorf("rejected = %d, want 1", got.Rejected)
	}
	if len(got.Periods) != 1 {
		t.Fatalf("periods %+v", got.Periods)
	}
	p := got.Periods[0]
	if p.YYMM != 202603 || p.Division != "SOLO" || p.DetailLines != 5 || p.HeadRows != 3 || p.Synthetic != 1 {
		t.Fatalf("period %+v", p)
	}
	// detail T: 100.50 + 100.01 (100.005 rounds half-up per cell) + 0 = 200.51; P: 10 + 1.23 = 11.23
	if !p.DetailT.Equal(d("200.51")) || !p.DetailP.Equal(d("11.23")) {
		t.Errorf("detail T %s P %s", p.DetailT, p.DetailP)
	}
	// head: real 100.50 + 0 (empty) + synthetic ROUND(100.005)=100.01 ; potongan 10 + 5 + 1.23
	if !p.HeadTunj.Equal(d("200.51")) || !p.HeadPot.Equal(d("16.23")) {
		t.Errorf("head tunj %s pot %s", p.HeadTunj, p.HeadPot)
	}
	// net: (100.50-10-0.25) + (0-5-0) + (100.01-1.23)
	if !p.HeadNet.Equal(d("184.03")) {
		t.Errorf("head net %s", p.HeadNet)
	}
	if len(got.Skipped) != 1 || got.Skipped[0].Division != "JAKARTA" || got.Skipped[0].YYMM != 202512 ||
		got.Skipped[0].MainRows != 1 || got.Skipped[0].DetailRows != 1 || !got.Skipped[0].DetailT.Equal(d("99")) {
		t.Errorf("skipped %+v", got.Skipped)
	}
}

func TestAggregateSalaryLegacy_MissingTableAndBadAmount(t *testing.T) {
	if _, err := AggregateSalaryLegacy(memSource{}, 1, 2); err == nil {
		t.Error("missing MAIN: want error")
	}
	src := memSource{
		TableSalaryMain: tbl(TableSalaryMain, "PM_YYMM|PM_NIK|PM_DIVISION|PM_TOT_TUNJANGAN|PM_TOT_POTONGAN|PM_ROUNDING"),
		TableSalaryDetail: tbl(TableSalaryDetail, "PD_YYMM|PD_NIK|PD_TYPE|PD_NO|PD_DIVISION|PD_AMOUNT",
			"202603|A1|Tunjangan|1|SOLO|abc"),
	}
	if _, err := AggregateSalaryLegacy(src, 202603, 202603); err == nil {
		t.Error("bad amount: want error")
	}
}
