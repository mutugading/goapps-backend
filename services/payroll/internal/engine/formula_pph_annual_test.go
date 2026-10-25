package engine

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// annualTaxTables adds the legacy PAYROLLSETUP PTKP/BIAYAJABATAN/TARIF rows (spec/_analysis/ddl2.txt).
func annualTaxTables() TaxTables {
	t := testTaxTables()
	t.PTKP = map[string]decimal.Decimal{
		"TK": d("4500000"), "TK1": d("4875000"), "TK2": d("5250000"), "TK3": d("5625000"),
		"K": d("4875000"), "K0": d("4875000"), "K1": d("5250000"), "K2": d("5625000"), "K3": d("6000000"),
	}
	t.JobCost = JobCost{Rate: d("5"), MaxMonthly: d("500000")}
	t.Progressive = []TaxLayer{
		{Min: d("1"), Max: d("60000000"), Rate: d("5")},
		{Min: d("60000001"), Max: d("250000000"), Rate: d("15")},
		{Min: d("250000001"), Max: d("500000000"), Rate: d("25")},
		{Min: d("500000001"), Max: d("5000000000"), Rate: d("30")},
		{Min: d("5000000001"), Max: d("9000000000"), Rate: d("35")},
	}
	return t
}

// decInput is a December input: T1 = cur (taxable), YTD gross/deduct/paid as given.
func decInput(legacy bool, taxID, cur, ytdGross, ytdDeduct, ytdPaid string, comps ...EmpComponent) Input {
	in := pphInput(taxID, append([]EmpComponent{taxT(1, cur, "F")}, comps...)...)
	in.Period = Period{YYMM: 202612, Start: time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 12, 25, 0, 0, 0, 0, time.UTC)}
	in.Tax = annualTaxTables()
	in.Rules = pphRules{rules: rules{legacy: legacy}}
	in.YTD = YTDTax{Gross: d(ytdGross), Deduct: d(ytdDeduct), TaxPaid: d(ytdPaid), Months: 11}
	return in
}

// Golden angka tangan (D-29). PTKP TK = 54.000.000/th, K1 = 63.000.000/th; BJ = MIN(5 %, 6.000.000).
// v1 = legacy PL/SQL (tanpa TRUNC, lapis progresif ber-bug), v2 = 06§6.2.
func TestPPh21December(t *testing.T) {
	neg0 := int64(0)
	cases := []struct {
		name   string
		in     Input
		v1, v2 string
	}{
		// bruto 72.000.000, BJ 3.600.000, pengurang 2.160.000 → PKP 12.240.000 × 5 % = 612.000 − 550.000.
		{"lapis 5 %", decInput(true, "TK", "6000000", "66000000", "2160000", "550000"), "62000", "62000"},
		// bruto 72.000.999, BJ 3.600.049,95 → PKP 12.240.949,05: v1 612.047,45 → 62.047; v2 TRUNC 12.240.000 → 62.000.
		{"TRUNC −3", decInput(true, "TK", "6000999", "66000000", "2160000", "550000"), "62047", "62000"},
		// bruto 240.000.000, BJ kena batas 6.000.000, PKP 171.000.000 → 3.000.000 + 111.000.000 × 15 %.
		{"lapis 15 % + BJ kena batas", decInput(true, "K1", "20000000", "220000000", "0", "18000000"), "1650000", "1650000"},
		// PKP 300.000.000: v1 lapis 15 % dikenakan atas Max penuh 250.000.000 (37.500.000), v2 atas 190.000.000.
		{"lapis 25 %", decInput(true, "K1", "29000000", "340000000", "0", "0"), "53000000", "44000000"},
		// PKP 6.000.000.000: v1 3 + 37,5 + 125 + 1.500 + 350 jt; v2 3 + 28,5 + 62,5 + 1.350 + 350 jt.
		{"lapis 30 % & 35 %", decInput(true, "K1", "69000000", "6000000000", "0", "0"), "2015500000", "1794000000"},
		// PKP tepat 60.000.000: v1 tidak masuk lapis mana pun (> Max salah, < Max salah) → 0.
		{"PKP tepat batas lapis", decInput(true, "TK", "10000000", "110000000", "0", "0"), "0", "3000000"},
		// Lebih bayar: 612.000 − 700.000, tidak di-floor walau ALLOW_NEGATIVE_PPH = 0.
		{"negatif (lebih bayar)", func() Input {
			in := decInput(true, "TK", "6000000", "66000000", "2160000", "700000")
			in.Rules = pphRules{rules: rules{legacy: true}, allowNeg: &neg0}
			return in
		}(), "-88000", "-88000"},
		// bruto 30.000.000 → PKP negatif → pajak 0, P1 = −PPh dibayar.
		{"PKP negatif", decInput(true, "TK", "2500000", "27500000", "0", "100000"), "-100000", "-100000"},
		// T75 keluar dari gross berjalan; THR + tax_adj masuk bruto, P22 + tax_adj Potongan masuk pengurang:
		// bruto 66 + 6 + 2 + 1 = 75 jt, BJ 3.750.000, pengurang 2.000.000 + 100.000 + 300.000 → PKP 14.850.000
		// → 742.500 − (550.000 + 50.000).
		{"THR, tax_adj, P22, T75 dikecualikan", func() Input {
			in := decInput(true, "TK", "6000000", "66000000", "2000000", "550000",
				taxT(75, "0", "F"), pComp(22, "0", "F", false))
			in.Additions = map[CompKey][]Addition{{CompT, 75}: {{Amount: d("5000000")}}, {CompP, 22}: {{Amount: d("100000")}}}
			in.Thr = ThrTax{Gross: d("2000000"), PPhPaid: d("50000")}
			in.TaxAdj = TaxAdj{Earn: d("1000000"), Deduct: d("300000")}
			return in
		}(), "142500", "142500"},
		// P4 mengurangi bruto sebelum BJ (legacy), bukan pengurang sesudah BJ: bruto 71.500.000, BJ 3.575.000
		// → PKP 11.765.000 × 5 % = 588.250.
		{"P4 mengurangi bruto", decInput(true, "TK", "6000000", "66000000", "2160000", "0", pComp(4, "500000", "F", false)), "588250", "588250"},
	}
	for _, c := range cases {
		for _, v := range []struct {
			name   string
			legacy bool
			want   string
		}{{"v1", true, c.v1}, {"v2", false, c.v2}} {
			t.Run(c.name+"/"+v.name, func(t *testing.T) {
				in := c.in
				if r, ok := in.Rules.(pphRules); ok {
					r.legacy = v.legacy
					in.Rules = r
				}
				l, res := p1(t, in)
				if res.Incomplete {
					t.Fatalf("incomplete: %v", res.Warnings)
				}
				if l.Amount == nil || l.Amount.String() != v.want {
					t.Errorf("P1 = %v, want %s (remark %q)", l.Amount, v.want, l.Remark)
				}
			})
		}
	}
}

// v2: above the last TARIF Max (9 M) the PKP stays at 35 % (06§6.2).
func TestPPh21DecemberAboveLastLayer(t *testing.T) {
	// PKP 10.000.000.000 → 3 + 28,5 + 62,5 + 1.350 + 1.750 jt.
	l, _ := p1(t, decInput(false, "K1", "69000000", "10000000000", "0", "0"))
	if l.Amount == nil || l.Amount.String() != "3194000000" {
		t.Errorf("P1 = %v", l.Amount)
	}
}

func TestPPh21DecemberRemark(t *testing.T) {
	l, _ := p1(t, decInput(true, "TK", "6000000", "66000000", "2160000", "550000"))
	want := "Bruto : 72,000,000 | Pengurang : 2,160,000 | Biaya Jabatan : 3,600,000 | PTKP : 54,000,000 | PKP : 12,240,000 | PPh Setahun : 612,000 | PPh Dibayar : 550,000"
	if l.Remark != want {
		t.Errorf("remark = %q", l.Remark)
	}
}

func TestPPh21DecemberNoPTKP(t *testing.T) {
	l, res := p1(t, decInput(true, "XX", "6000000", "66000000", "0", "0"))
	if l.Amount != nil {
		t.Errorf("P1 = %s, want NULL", l.Amount)
	}
	if len(res.Warnings) != 1 || !strings.HasPrefix(res.Warnings[0], WarnPPhNoPTKP) || res.Incomplete {
		t.Errorf("warnings = %v incomplete = %v", res.Warnings, res.Incomplete)
	}
}

func TestPPh21DecemberNoTable(t *testing.T) {
	in := decInput(true, "TK", "6000000", "66000000", "0", "0")
	in.Tax.Progressive = nil
	if _, err := Calculate(in); !errors.Is(err, ErrNoAnnualTaxTable) {
		t.Fatalf("err = %v, want ErrNoAnnualTaxTable", err)
	}
}

// Nothing taxable this month and no YTD at all → NULL like legacy (no row).
func TestPPh21DecemberNothing(t *testing.T) {
	in := decInput(true, "TK", "0", "0", "0", "0")
	in.Components = []EmpComponent{tc(1, "5000000", "F"), pComp(1, "0", "F", false)}
	l, res := p1(t, in)
	if l.Amount != nil || res.Incomplete {
		t.Errorf("P1 = %v incomplete = %v", l.Amount, res.Incomplete)
	}
}

// v1: above the last TARIF Max the legacy tax is flat (Max × 35 % for that layer); layers given
// out of order are sorted first.
func TestPPh21DecemberLegacyAboveLastLayer(t *testing.T) {
	in := decInput(true, "K1", "69000000", "10000000000", "0", "0") // PKP 10.000.000.000
	ls := in.Tax.Progressive
	in.Tax.Progressive = []TaxLayer{ls[1], ls[4], ls[0], ls[3], ls[2]}
	// 3 + 37,5 + 125 + 1.500 + 3.150 jt
	l, _ := p1(t, in)
	if l.Amount == nil || l.Amount.String() != "4815500000" {
		t.Errorf("P1 = %v", l.Amount)
	}
}

// v2: negative PKP is truncated toward zero then floored at 0; the remark shows PKP 0.
func TestPPh21DecemberV2NegativePKP(t *testing.T) {
	l, _ := p1(t, decInput(false, "TK", "2500000", "27500000", "0", "100000"))
	if l.Amount == nil || l.Amount.String() != "-100000" || !strings.Contains(l.Remark, "PKP : 0 |") {
		t.Errorf("P1 = %v remark %q", l.Amount, l.Remark)
	}
}

func TestPPh21DecemberNoJobCostMax(t *testing.T) {
	in := decInput(true, "TK", "6000000", "66000000", "0", "0")
	in.Tax.JobCost.MaxMonthly = decimal.Zero
	if _, err := Calculate(in); !errors.Is(err, ErrNoAnnualTaxTable) {
		t.Fatalf("err = %v, want ErrNoAnnualTaxTable", err)
	}
}
