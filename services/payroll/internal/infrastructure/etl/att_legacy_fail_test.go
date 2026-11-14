package etl

import (
	"strings"
	"testing"
)

func psvTable(name, body string) *Table {
	lines := strings.Split(strings.TrimSpace(body), "\n")
	t := &Table{Name: name, Cols: strings.Split(lines[0], "|")}
	for _, l := range lines[1:] {
		t.Rows = append(t.Rows, strings.Split(l, "|"))
	}
	return t
}

// T009e9 (D-46): T40 attached (frz=2) without a DETAIL row ⇒ PF=1, only for OPEN periods.
func TestMarkMissingPremi(t *testing.T) {
	ce := psvTable(TableEmployeeComponent, `MPCE_NIK|MPCE_TYPE|MPCE_ID|MPCE_AMOUNT|MPCE_FRZ_FLAG|MPCE_DIVISION
A|Tunjangan|40|100|2|SOLO
B|Tunjangan|40|100||SOLO
C|Tunjangan|40|100|1|SOLO
E|Potongan|40|100|2|SOLO
F|Tunjangan|40|100|2|SOLO
G|Tunjangan|40|100|2|SOLO`)
	dt := psvTable(TableSalaryDetail, `PD_YYMM|PD_NIK|PD_NO|PD_TYPE|PD_AMOUNT|PD_STATUS|PD_DIVISION
202610|F|40|Tunjangan|5|F|SOLO
202610|G|40|Tunjangan||F|SOLO`)
	mn := psvTable(TableSalaryMain, `PM_YYMM|PM_NIK|PM_DIVISION
202610|A|SOLO
202610|B|SOLO
202610|C|SOLO
202610|D|SOLO
202610|E|SOLO
202610|F|SOLO
202610|G|SOLO
202609|A|SOLO
202610|A|JAKARTA`)
	var rep AttLegacyFailReport
	c := &failCollector{rep: &rep, amounts: map[failKey]string{}, premiOn: map[failKey]bool{}, vals: map[failKey]*failVal{}}
	if err := c.loadAmounts(ce); err != nil {
		t.Fatal(err)
	}
	if err := c.scanDetail(dt); err != nil {
		t.Fatal(err)
	}
	if err := c.markMissingPremi(mn, map[string]bool{"202610": true}); err != nil {
		t.Fatal(err)
	}
	pf := func(yymm, div, nik string) string {
		v := c.vals[failKey{yymm, div, nik}]
		if v == nil || v.premi == nil {
			return "NULL"
		}
		if v.noRow {
			return "1norow"
		}
		return map[int]string{0: "0", 1: "1"}[*v.premi]
	}
	for _, tc := range []struct{ yymm, div, nik, want string }{
		{"202610", "SOLO", "A", "1norow"},  // frz=2, no row
		{"202610", "SOLO", "B", "1norow"},  // frz empty = 2
		{"202610", "SOLO", "C", "NULL"},    // frz=1
		{"202610", "SOLO", "D", "NULL"},    // no T40 component
		{"202610", "SOLO", "E", "NULL"},    // T40 is Potongan, not Tunjangan
		{"202610", "SOLO", "F", "0"},       // DETAIL row with value (D-33)
		{"202610", "SOLO", "G", "1"},       // DETAIL row NULL (D-33)
		{"202609", "SOLO", "A", "NULL"},    // not an OPEN period
		{"202610", "JAKARTA", "A", "NULL"}, // component is per division
	} {
		if got := pf(tc.yymm, tc.div, tc.nik); got != tc.want {
			t.Errorf("%s/%s %s: PF %s, want %s", tc.yymm, tc.div, tc.nik, got, tc.want)
		}
	}
}

func TestOpenSet(t *testing.T) {
	if s, err := openSet(nil); err != nil || len(s) != 0 {
		t.Fatalf("nil: %v %v", s, err)
	}
	if s, err := openSet([]string{"202610", " 202609"}); err != nil || !s["202610"] || !s["202609"] {
		t.Fatalf("valid: %v %v", s, err)
	}
	for _, bad := range []string{"2026", "202613", "x"} {
		if _, err := openSet([]string{bad}); err == nil {
			t.Errorf("%q diterima", bad)
		}
	}
}

func TestLoadAmountsNeedsFrzOnlyWhenOpen(t *testing.T) {
	ce := psvTable(TableEmployeeComponent, `MPCE_NIK|MPCE_TYPE|MPCE_ID|MPCE_AMOUNT|MPCE_DIVISION
A|Tunjangan|40|100|SOLO`)
	c := &failCollector{amounts: map[failKey]string{}}
	if err := c.loadAmounts(ce); err != nil {
		t.Fatalf("tanpa periode OPEN, MPCE_FRZ_FLAG tidak wajib: %v", err)
	}
	c.premiOn = map[failKey]bool{}
	if err := c.loadAmounts(ce); err == nil {
		t.Fatal("periode OPEN tanpa MPCE_FRZ_FLAG harus gagal")
	}
}
