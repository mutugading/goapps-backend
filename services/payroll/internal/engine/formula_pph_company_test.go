package engine

import (
	"errors"
	"testing"
)

// paidT is a taxable T component whose PPh the company bears (tax_paid_company = Y, e.g. T73).
func paidT(id int, amt string) EmpComponent {
	c := taxT(id, amt, "F")
	c.TaxPaidCompany = true
	return c
}

func lineOf(t *testing.T, in Input, k CompKey) Line {
	t.Helper()
	res, err := Calculate(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range res.Lines {
		if l.Key == k {
			return l
		}
	}
	t.Fatalf("no line %s", k)
	return Line{}
}

func amountStr(l Line) string {
	if l.Amount == nil {
		return ""
	}
	return l.Amount.String()
}

var keyP5 = CompKey{CompP, 5}

// Golden angka tangan (D-31). Non-Desember: P5 = TER(bruto tanpa baris ditanggung perusahaan)
// − TER(bruto semua), keduanya seperti P1 (D-28).
func TestPPhCompanyNonDecember(t *testing.T) {
	p5 := pComp(5, "0", "F", false)
	cases := []struct {
		name string
		in   Input
		want string // "" = NULL
	}{
		// semua 5.600.000 → A 0,25 % = 14.000; tanpa T73 5.000.000 → 0 %.
		{"seluruh PPh ditanggung", pphInput("TK", taxT(1, "5000000", "F"), paidT(73, "600000"), p5), "-14000"},
		// 9.900.000 × 2 % = 198.000; 9.700.000 × 2 % = 194.000.
		{"sebagian ditanggung", pphInput("TK", taxT(1, "9700000", "F"), paidT(73, "200000"), p5), "-4000"},
		{"tanpa baris ditanggung", pphInput("TK", taxT(1, "5600000", "F"), p5), "0"},
		// legacy SELECT INTO tanpa baris tax_paid_company = N (dan tanpa P4/P8) → NULL.
		{"hanya baris ditanggung", pphInput("TK", paidT(73, "600000"), p5), ""},
		// bruto 5.400.001–5.650.000 masuk bracket, 7.000.000 tidak ada di tabel uji → NULL.
		{"bracket hilang", pphInput("TK", taxT(1, "7000000", "F"), paidT(73, "100000"), p5), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := amountStr(lineOf(t, c.in, keyP5)); got != c.want {
				t.Errorf("P5 = %q, want %q", got, c.want)
			}
		})
	}
	t.Run("take-home tidak terpotong PPh", func(t *testing.T) {
		res, err := Calculate(pphInput("TK", taxT(1, "5000000", "F"), paidT(73, "600000"), p5))
		if err != nil {
			t.Fatal(err)
		}
		if res.Header.TotPotongan.String() != "0" {
			t.Errorf("tot_potongan = %s, want 0 (P1 14.000 + P5 −14.000)", res.Header.TotPotongan)
		}
	})
	t.Run("tabel TER kosong", func(t *testing.T) {
		in := pphInput("TK", taxT(1, "5000000", "F"), paidT(73, "600000"), p5)
		in.Tax = TaxTables{}
		if _, err := Calculate(in); !errors.Is(err, ErrNoTaxTable) {
			t.Fatalf("err = %v", err)
		}
	})
}

// Golden angka tangan (D-31). PTKP TK 54.000.000/th, BJ 5 % maks 6.000.000.
// Bruto 72.000.000 (YTD 66.000.000 termasuk T73 6.600.000; bulan ini T1 5.400.000 + T73 600.000),
// ditanggung perusahaan 7.200.000.
func TestPPhCompanyDecember(t *testing.T) {
	mk := func(legacy bool, p5Paid string) Input {
		in := decInput(legacy, "TK", "5400000", "0", "0", "0", paidT(73, "600000"), pComp(5, "0", "F", false))
		in.YTDCompany = YTDCompanyTax{Gross: d("66000000"), CompanyPaid: d("6600000"), P5Paid: d(p5Paid)}
		return in
	}
	cases := []struct {
		name string
		in   Input
		want string
	}{
		// v1: BJ 3.600.000 dari bruto penuh. PKP semua 14.400.000 → 720.000; ditanggung masuk
		// pengurang → PKP 7.200.000 → 360.000; P5 = 360.000 − 720.000 − (−330.000) = −30.000.
		{"v1 legacy, YTD P5 dikurangkan (D-31b)", mk(true, "-330000"), "-30000"},
		{"v1 legacy tanpa P5 sebelumnya", mk(true, "0"), "-360000"},
		// v2: bruto sendiri 64.800.000, BJ 3.240.000, PKP 7.560.000 → 378.000;
		// 378.000 − 720.000 − (−330.000) = −12.000.
		{"v2 YTD P5 dikurangkan", mk(false, "-330000"), "-12000"},
		// D-37: TK2 tidak ada di PTKPCompany (v1 → 0). v2 PTKP 63.000.000: semua PKP 5.400.000 → 270.000, sendiri < 0 → 0; P5 = −270.000.
		{"v1 TK2 tanpa PTKP P5 → 0 (D-37a)", func() Input { in := mk(true, "0"); in.Emp.TaxStatus = "TK2"; return in }(), "0"},
		{"v1 TK1 tanpa PTKP P5 → 0", func() Input { in := mk(true, "0"); in.Emp.TaxStatus = "TK1"; return in }(), "0"},
		{"v1 TK3 tanpa PTKP P5 → 0", func() Input { in := mk(true, "0"); in.Emp.TaxStatus = "TK3"; return in }(), "0"},
		{"v1 K1 tetap memakai PTKPCompany", func() Input { in := mk(true, "0"); in.Emp.TaxStatus = "K1"; return in }(), "-270000"},
		{"v2 TK2 memakai PTKP → bukan 0 (D-37b)", func() Input { in := mk(false, "0"); in.Emp.TaxStatus = "TK2"; return in }(), "-270000"},
		{"PTKP hilang → 0", func() Input { in := mk(true, "0"); in.Emp.TaxStatus = "X"; return in }(), "0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := amountStr(lineOf(t, c.in, keyP5)); got != c.want {
				t.Errorf("P5 = %q, want %q", got, c.want)
			}
		})
	}
	// Bruto 132.000.000 (THR setahun 60.000.000), BJ kena batas 6.000.000, pengurang YTD 8.000.000:
	// semua PKP 64.000.000 → 3.000.000 + 4.000.000 × 15 % = 3.600.000; sendiri 56.800.000 → 2.840.000.
	// Tanpa pengurang: 72.000.000 / 64.800.000 → −1.080.000. Tanpa THR: PKP 6.400.000 / <0 → −320.000.
	t.Run("v1 pengurang YTD, THR setahun, BJ kena batas, lintas lapis", func(t *testing.T) {
		in := mk(true, "0")
		in.YTDCompany.Deduct = d("8000000")
		in.YTDCompany.ThrBonus = d("60000000")
		l := lineOf(t, in, keyP5)
		if amountStr(l) != "-760000" {
			t.Errorf("P5 = %q, want -760000 (remark %q)", amountStr(l), l.Remark)
		}
	})
}

func frozenP(id int, amt string) EmpComponent {
	c := pComp(id, amt, "F", false)
	c.Frozen = true
	return c
}

// T016b2 (D-28d): legacy never filters FRZ_FLAG on the P4/P8 subqueries of any PPh branch
// (FNC_MGT_PAYROLL_PPH 139–142/478, _COMPANY 411/628), so a frozen P4 lowers the gross of
// P1 (non-December and December) and the December P5 deduction, but is no detail line.
func TestFrozenP4AllPPhPaths(t *testing.T) {
	t.Run("P1 non-Desember: P4 beku menurunkan bruto", func(t *testing.T) {
		in := pphInput("TK", taxT(1, "6000000", "F"), frozenP(4, "500000")) // 5.500.000 → 13.750
		if got := amountStr(lineOf(t, in, CompKey{CompP, 1})); got != "13750" {
			t.Errorf("P1 = %q, want 13750", got)
		}
	})
	t.Run("P1 Desember: P4 beku mengurangi bruto sebelum BJ", func(t *testing.T) {
		in := decInput(true, "TK", "6000000", "66000000", "2160000", "0", frozenP(4, "500000"))
		if got := amountStr(lineOf(t, in, CompKey{CompP, 1})); got != "588250" {
			t.Errorf("P1 = %q, want 588250", got)
		}
	})
	t.Run("P5 Desember: P4 beku masuk pengurang", func(t *testing.T) {
		// v2: bruto 120.200.000 (BJ semua kena batas 6 jt; sisi sendiri 113.000.000 → BJ 5.650.000), PTKP 54 jt.
		// Tanpa P4: semua PKP 60.200.000 → 3.030.000; sendiri 53.350.000 → 2.667.500; P5 −362.500.
		// P4 500.000 (pengurang): 59.700.000 → 2.985.000; 52.850.000 → 2.642.500; P5 −342.500.
		mk := func(p4 EmpComponent) Input {
			in := decInput(false, "TK", "5400000", "0", "0", "0", paidT(73, "600000"), pComp(5, "0", "F", false), p4)
			in.YTDCompany = YTDCompanyTax{Gross: d("114200000"), CompanyPaid: d("6600000")}
			return in
		}
		for name, c := range map[string]struct {
			p4   EmpComponent
			want string
		}{
			"beku":  {frozenP(4, "500000"), "-342500"},
			"biasa": {pComp(4, "500000", "F", false), "-342500"},
			"tanpa": {pComp(4, "0", "F", false), "-362500"},
		} {
			if got := amountStr(lineOf(t, mk(c.p4), keyP5)); got != c.want {
				t.Errorf("%s: P5 = %q, want %q", name, got, c.want)
			}
		}
	})
	t.Run("P5 Desember v1: P4 beku masuk pengurang (paritas)", func(t *testing.T) {
		// v1: nett semua = 120.200.000 − 500.000 − BJ 6.000.000 − 54.000.000 = 59.700.000 → 2.985.000;
		// sendiri 59.700.000 − 7.200.000 = 52.500.000 → 2.625.000; P5 = −360.000.
		in := decInput(true, "TK", "5400000", "0", "0", "0", paidT(73, "600000"), pComp(5, "0", "F", false), frozenP(4, "500000"))
		in.YTDCompany = YTDCompanyTax{Gross: d("114200000"), CompanyPaid: d("6600000")}
		if got := amountStr(lineOf(t, in, keyP5)); got != "-360000" {
			t.Errorf("P5 = %q, want -360000", got)
		}
	})
	t.Run("tidak masuk Lines maupun header", func(t *testing.T) {
		in := pphInput("TK", taxT(1, "6000000", "F"), frozenP(4, "500000"))
		res, err := Calculate(in)
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range res.Lines {
			if l.Key == (CompKey{CompP, 4}) {
				t.Fatalf("P4 beku muncul sebagai baris detail: %+v", l)
			}
		}
		if res.Header.TotPotongan.String() != "13750" { // hanya P1; P4 beku tidak ikut
			t.Errorf("tot_potongan = %s, want 13750", res.Header.TotPotongan)
		}
	})
}

// T016b3 (D-28d): legacy does not filter FRZ_FLAG on the P14/P18 (PPH.sql 253/289/436,
// PPH_COMPANY.sql 100/263/588/811) and P22/P23 (PPH.sql 436/502) subqueries either. A frozen
// row still lowers the December PKP but is no detail line. Dump: P14 frozen 13, P18 frozen 25,
// P22/P23 none (loader keeps them in the list anyway).
func TestFrozenAnnualDeductions(t *testing.T) {
	// bruto 72.000.000, BJ 3.600.000, PTKP 54 jt, pengurang YTD 2.160.000: PKP 12.240.000 × 5 % = 612.000.
	// TETAP 6.000.000: P14 1 % = 60.000, P18 1 % = 60.000 (BPJS, Amount = tarif); P22 50.000, P23 30.000
	// (addition). Pengurang +200.000: PKP 12.040.000 × 5 % = 602.000.
	for _, legacy := range []bool{true, false} {
		in := decInput(legacy, "TK", "6000000", "66000000", "2160000", "0",
			frozenP(14, "0.01"), frozenP(18, "0.01"), frozenP(22, "0"), frozenP(23, "0"))
		in.Rules = tRules{rules{legacy: legacy}}
		in.Additions = map[CompKey][]Addition{{CompP, 22}: {{Amount: d("50000")}}, {CompP, 23}: {{Amount: d("30000")}}}
		res, err := Calculate(in)
		if err != nil {
			t.Fatal(err)
		}
		var p1 string
		for _, l := range res.Lines {
			switch l.Key {
			case CompKey{CompP, 14}, CompKey{CompP, 18}, CompKey{CompP, 22}, CompKey{CompP, 23}:
				t.Fatalf("beku muncul sebagai baris detail: %+v", l)
			case CompKey{CompP, 1}:
				p1 = amountStr(l)
			}
		}
		if p1 != "602000" {
			t.Errorf("legacy=%v: P1 = %q, want 602000", legacy, p1)
		}
		if res.Header.TotPotongan.String() != "602000" { // hanya P1; baris beku tidak ikut
			t.Errorf("legacy=%v: tot_potongan = %s, want 602000", legacy, res.Header.TotPotongan)
		}
	}
}
