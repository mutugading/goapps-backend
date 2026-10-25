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
