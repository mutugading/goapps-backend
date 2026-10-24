package etl

import "testing"

// Tabel 02§5.1 apa adanya: level → (rate, otSts, adjSts, tsSts, tsRate, grup).
func TestLevelRules_Spec021(t *testing.T) {
	want := map[string]LevelRuleRow{
		"Operator Kontrak Shift I,II,III": {Group: "SHIFT", OtinworkRate: "0.0877", OtinworkSts: 2, ShiftadjSts: 1, TunjshiftSts: 2, TunjshiftRt: "2200"},
		"Operator Shift I,II,III":         {Group: "SHIFT", OtinworkRate: "0.0877", OtinworkSts: 2, ShiftadjSts: 1, TunjshiftSts: 2, TunjshiftRt: "2200"},
		"Operator General Shift":          {Group: "GS", OtinworkRate: "0.075", OtinworkSts: 2, ShiftadjSts: 2, TunjshiftSts: 1, TunjshiftRt: "2200"},
		"Operator PKWT GS Less 3 Month":   {Group: "GS", OtinworkRate: "0.075", OtinworkSts: 2, ShiftadjSts: 2, TunjshiftSts: 1, TunjshiftRt: "2200"},
		"Operator Kontrak GS":             {Group: "GS", OtinworkRate: "0.075", OtinworkSts: 2, ShiftadjSts: 2, TunjshiftSts: 1, TunjshiftRt: "2200"},
		"Driver":                          {Group: "DRIVER", OtinworkRate: "0.9017", OtinworkSts: 2, ShiftadjSts: 1, TunjshiftSts: 2, TunjshiftRt: "2200"},
		"Dirumahkan":                      {Group: "DIRUMAHKAN", OtinworkRate: "0", OtinworkSts: 1, ShiftadjSts: 1, TunjshiftSts: 1, TunjshiftRt: "2200"},
	}
	for _, l := range []string{"Executive", "Driver HO", "Staff HO", "Office Boy", "Solo Asmen", "Driver Bandung", "Staff Bandung", "OB Bandung"} {
		want[l] = LevelRuleRow{Group: "STAFF", OtinworkRate: "0", OtinworkSts: 1, ShiftadjSts: 1, TunjshiftSts: 1, TunjshiftRt: "3300"}
	}
	got := LevelRules()
	if len(got) != len(want) {
		t.Fatalf("jumlah level %d, spec %d", len(got), len(want))
	}
	for _, g := range got {
		w, ok := want[g.Level]
		if !ok {
			t.Errorf("level tak ada di spec: %q", g.Level)
			continue
		}
		w.Level, w.RateOTBasis = g.Level, g.RateOTBasis
		if g != w {
			t.Errorf("%q: got %+v want %+v", g.Level, g, w)
		}
		wantBasis := "T1_7_55"
		if g.Level == "Executive" {
			wantBasis = "T1_2_3_4"
		}
		if g.RateOTBasis != wantBasis {
			t.Errorf("%q basis %s, want %s", g.Level, g.RateOTBasis, wantBasis)
		}
	}
}

func TestCompCodeAndTaxClass(t *testing.T) {
	if c, _ := CompCode("Tunjangan", 1); c != "T001" {
		t.Fatal(c)
	}
	if c, _ := CompCode("Potongan", 32); c != "P032" {
		t.Fatal(c)
	}
	if _, err := CompCode("X", 1); err == nil {
		t.Fatal("tipe asing harus ditolak")
	}
	if taxClass("2", "N") != "TAXABLE" || taxClass("2", "Y") != "TAX_PAID_COMPANY" || taxClass("1", "N") != "NONE" {
		t.Fatal("taxClass")
	}
}

func TestChecksumOrderIndependent(t *testing.T) {
	a := &Table{Rows: [][]string{{"1", "a"}, {"2", "b"}}}
	b := &Table{Rows: [][]string{{"2", "b"}, {"1", "a"}}}
	if a.Checksum() != b.Checksum() {
		t.Fatal("checksum harus tidak peduli urutan")
	}
}
