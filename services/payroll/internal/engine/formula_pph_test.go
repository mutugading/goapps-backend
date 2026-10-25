package engine

import (
	"errors"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

// pphRules adds ALLOW_NEGATIVE_PPH to the test rule set; nil leaves it absent (default allow).
type pphRules struct {
	rules
	allowNeg *int64
}

func (r pphRules) Constant(k string) (decimal.Decimal, bool) {
	if k == ConstAllowNegativePPh && r.allowNeg != nil {
		return decimal.NewFromInt(*r.allowNeg), true
	}
	return r.rules.Constant(k)
}

// testTaxTables holds the legacy EMPTAX rows and the PAYROLLSETUP TER brackets used below
// (values from IM_VS_STATIC_VALUE, spec/_analysis/ddl2.txt).
func testTaxTables() TaxTables {
	br := func(rows ...[3]string) []TERBracket {
		out := make([]TERBracket, 0, len(rows))
		for _, r := range rows {
			out = append(out, TERBracket{Min: d(r[0]), Max: d(r[1]), Rate: d(r[2])})
		}
		return out
	}
	return TaxTables{
		Category: map[string]string{
			"K0": "A", "K1": "B", "K2": "B", "K3": "C",
			"TK": "A", "TK1": "A", "TK2": "B", "TK3": "B",
		},
		TER: map[string][]TERBracket{
			"A": br([3]string{"0", "5400000", "0"}, [3]string{"5400001", "5650000", "0.25"},
				[3]string{"9650001", "10050000", "2"}),
			"B": br([3]string{"0", "6200000", "0"}, [3]string{"6200001", "6500000", "0.25"},
				[3]string{"6500001", "6850000", "0.5"}),
			"C": br([3]string{"0", "6600000", "0"}, [3]string{"6600001", "6950000", "0.25"}),
		},
	}
}

// taxT is a taxable T component (MPCE_TAX = 2).
func taxT(id int, amt, sts string) EmpComponent {
	c := tc(id, amt, sts)
	c.Tax = true
	return c
}

func pComp(id int, amt, sts string, tax bool) EmpComponent {
	return EmpComponent{Key: CompKey{CompP, id}, Amount: d(amt), Sts: sts, Tax: tax}
}

func pphInput(taxID string, comps ...EmpComponent) Input {
	in := baseInput("SOLO", "staff", "F", append(comps, pComp(1, "0", "F", false))...)
	in.Emp.TaxStatus = taxID
	in.Tax = testTaxTables()
	in.Rules = pphRules{rules: rules{legacy: true}}
	return in
}

func p1(t *testing.T, in Input) (Line, Result) {
	t.Helper()
	res, err := Calculate(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range res.Lines {
		if l.Key == (CompKey{CompP, 1}) {
			return l, res
		}
	}
	t.Fatal("no line P001")
	return Line{}, res
}

// Golden angka tangan: P1 = ROUND(G × TER(kat, G) / 100, 0), G = Σ ROUND(komponen tax=2) − P4 − P8.
func TestPPh21NonDecember(t *testing.T) {
	neg0 := int64(0)
	cases := []struct {
		name   string
		in     func() Input
		want   string // "" = NULL
		remark string
	}{
		{"A 5.6jt → 0.25%", func() Input {
			return pphInput("TK", taxT(1, "5000000", "F"), taxT(2, "600000", "F"))
		}, "14000", "Kategori : A | Penghasilan : 5,400,001 - 5,650,000 | Tarif : 0.25 %"},
		{"A batas atas A0 inklusif", func() Input { return pphInput("K0", taxT(1, "5400000", "F")) }, "0", ""},
		{"A batas bawah 0.25%", func() Input { return pphInput("TK1", taxT(1, "5400001", "F")) }, "13500", ""},
		{"B K1 0.5%", func() Input {
			return pphInput("K1", taxT(1, "6000000", "F"), taxT(2, "566753", "F")) // 6,566,753 × 0.5% = 32833.765
		}, "32834", ""},
		{"C batas atas C0", func() Input { return pphInput("K3", taxT(1, "6600000", "F")) }, "0", ""},
		{"C 0.25%", func() Input { return pphInput("K3", taxT(1, "6600001", "F")) }, "16500", ""},
		{"komponen non-pajak tidak masuk gross", func() Input {
			return pphInput("TK", taxT(1, "5400000", "F"), tc(7, "1000000", "F"))
		}, "0", ""},
		// Legacy sums ROUND(·,0) per component: 7,500,001/30×26 = 6,500,000.8667 → 6,500,001 (B0.5).
		// The raw sum would fall into the 6,500,000–6,500,001 gap and find no bracket.
		{"ROUND per komponen sebelum dijumlah", func() Input {
			in := pphInput("K2", taxT(1, "7500001", "D"))
			in.Emp.GapokSts = stsProrate
			return in
		}, "32500", ""},
		{"P4 dikurangi", func() Input {
			return pphInput("TK", taxT(1, "6000000", "F"), pComp(4, "500000", "F", false)) // 5,500,000
		}, "13750", ""},
		{"P8 legacy NULL tidak mengurangi", func() Input {
			return pphInput("TK", taxT(1, "5450000", "F"), pComp(8, "300000", "F", false))
		}, "13625", ""},
		{"P tax=2 menambah gross (legacy)", func() Input {
			in := pphInput("TK", taxT(1, "5350000", "F"), pComp(19, "0", "F", true))
			in.Additions = map[CompKey][]Addition{{CompP, 19}: {{Amount: d("100000")}}}
			return in
		}, "13625", ""},
		{"bulan THR", func() Input {
			in := pphInput("TK", taxT(1, "5000000", "F"))
			in.Thr = ThrTax{Gross: d("5000000")} // G = 10,000,000 → A2
			return in
		}, "200000", "Kategori : A | Penghasilan : 9,650,001 - 10,050,000 | Tarif : 2 % | THR : 5,000,000 | PPh THR : 0"},
		{"THR sudah dipotong → negatif diizinkan (default)", func() Input {
			in := pphInput("TK", taxT(1, "5000000", "F"))
			in.Thr = ThrTax{Gross: d("5000000"), PPhPaid: d("250000")}
			return in
		}, "-50000", ""},
		{"negatif dilarang → 0", func() Input {
			in := pphInput("TK", taxT(1, "5000000", "F"))
			in.Thr = ThrTax{Gross: d("5000000"), PPhPaid: d("250000")}
			in.Rules = pphRules{rules: rules{legacy: true}, allowNeg: &neg0}
			return in
		}, "0", ""},
		{"tanpa komponen pajak & P4/P8 → NULL", func() Input { return pphInput("TK", tc(1, "5000000", "F")) }, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l, res := p1(t, c.in())
			if res.Incomplete {
				t.Errorf("incomplete: %v", res.Warnings)
			}
			got := ""
			if l.Amount != nil {
				got = l.Amount.String()
			}
			if got != c.want {
				t.Errorf("P1 = %q, want %q", got, c.want)
			}
			if c.remark != "" && l.Remark != c.remark {
				t.Errorf("remark = %q, want %q", l.Remark, c.remark)
			}
		})
	}
}

// Legacy SELECT INTO without a category or bracket raises NO_DATA_FOUND, which SQL turns into NULL.
func TestPPh21NoBracket(t *testing.T) {
	for name, in := range map[string]Input{
		"status pajak tak dikenal": pphInput("XX", taxT(1, "6000000", "F")),
		"gross negatif":            pphInput("TK", taxT(1, "100000", "F"), pComp(4, "500000", "F", false)),
	} {
		t.Run(name, func(t *testing.T) {
			l, res := p1(t, in)
			if l.Amount != nil {
				t.Errorf("P1 = %s, want NULL", l.Amount)
			}
			if len(res.Warnings) != 1 || !strings.HasPrefix(res.Warnings[0], WarnPPhNoBracket) {
				t.Errorf("warnings = %v", res.Warnings)
			}
			if res.Incomplete {
				t.Error("incomplete")
			}
		})
	}
}

func TestPPh21NoTaxTable(t *testing.T) {
	in := pphInput("TK", taxT(1, "6000000", "F"))
	in.Tax = TaxTables{}
	if _, err := Calculate(in); !errors.Is(err, ErrNoTaxTable) {
		t.Fatalf("err = %v, want ErrNoTaxTable", err)
	}
}

func TestThousands(t *testing.T) {
	for in, want := range map[string]string{"0": "0", "999": "999", "1000": "1,000", "1400000001": "1,400,000,001", "-50000": "-50,000"} {
		if got := thousands(d(in)); got != want {
			t.Errorf("thousands(%s) = %s, want %s", in, got, want)
		}
	}
}

// A negative P1 (THR PPh over-withheld) lowers total deductions, i.e. it is paid back in take-home.
func TestPPh21NegativeInHeader(t *testing.T) {
	in := pphInput("TK", taxT(1, "5000000", "F"))
	in.Thr = ThrTax{Gross: d("5000000"), PPhPaid: d("250000")}
	_, res := p1(t, in)
	if res.Header == nil || res.Header.TotPotongan.String() != "-50000" || res.Header.TakeHome.String() != "5050000" {
		t.Fatalf("header = %+v", res.Header)
	}
}
