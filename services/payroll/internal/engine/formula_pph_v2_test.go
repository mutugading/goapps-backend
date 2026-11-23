package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// taxV2Rules is rule v1 (LegacyCompat) with TAX_V2_FROM set (D-81); from 0 leaves it absent.
type taxV2Rules struct {
	pphRules
	from int64
}

func (r taxV2Rules) Constant(k string) (decimal.Decimal, bool) {
	if k == ConstTaxV2From && r.from != 0 {
		return decimal.NewFromInt(r.from), true
	}
	return r.pphRules.Constant(k)
}

func withTaxV2From(in Input, from int64) Input {
	in.Rules = taxV2Rules{pphRules: pphRules{rules: rules{legacy: true}}, from: from}
	return in
}

func date(y int, m time.Month, dd int) *time.Time {
	t := time.Date(y, m, dd, 0, 0, 0, 0, time.UTC)
	return &t
}

// periodYM is the payroll period of yymm (26/MM-1 through 25/MM).
func periodYM(yymm int) Period {
	end := time.Date(yymm/100, time.Month(yymm%100), 25, 0, 0, 0, 0, time.UTC)
	return Period{YYMM: yymm, Start: end.AddDate(0, -1, 1), End: end}
}

// D-80 (1): rule set v1 + TAX_V2_FROM = 202612 → lapis benar + TRUNC −3 di Desember; sebelum TAX_V2_FROM tetap D-29 v1.
func TestTaxV2FromDecember(t *testing.T) {
	cases := []struct {
		name string
		in   Input
		from int64
		want string
	}{
		// PKP 300.000.000: v2 3 + 28,5 + 12,5 jt = 44 jt; v1 (bug Max penuh) 53 jt.
		{"PKP 300 jt, v2 aktif", decInput(true, "K1", "29000000", "340000000", "0", "0"), 202612, "44000000"},
		{"PKP 300 jt, v2 belum aktif (202701)", decInput(true, "K1", "29000000", "340000000", "0", "0"), 202701, "53000000"},
		{"PKP 300 jt, tanpa TAX_V2_FROM", decInput(true, "K1", "29000000", "340000000", "0", "0"), 0, "53000000"},
		// PKP tepat 60.000.000: v2 60 jt × 5 % = 3 jt; v1 0.
		{"PKP 60 jt, v2 aktif", decInput(true, "TK", "10000000", "110000000", "0", "0"), 202612, "3000000"},
		{"PKP 60 jt, v1", decInput(true, "TK", "10000000", "110000000", "0", "0"), 0, "0"},
		// TRUNC −3: PKP 12.240.949,05 → 12.240.000 × 5 % − 550.000.
		{"TRUNC −3, v2 aktif", decInput(true, "TK", "6000999", "66000000", "2160000", "550000"), 202612, "62000"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l, _ := p1(t, withTaxV2From(c.in, c.from))
			if got := amountStr(l); got != c.want {
				t.Errorf("P1 = %q, want %q (remark %q)", got, c.want, l.Remark)
			}
		})
	}
}

// D-80 (4b): batas biaya jabatan = 500.000 × bulan kerja. Masuk 1 Juli: Jul..Des = 6 bulan → batas 3.000.000.
// Bruto 6 × 20 jt = 120 jt, BJ 5 % = 6 jt → 3 jt; PKP 120 − 3 − 54 = 63 jt → 3 jt + 3 jt × 15 % = 3.450.000.
// v1 (× 12, batas 6 jt): PKP 60 jt → bug lapis legacy 0.
func TestTaxV2JobCostMonthsWorked(t *testing.T) {
	mk := func(from int64) Input {
		in := decInput(true, "TK", "20000000", "100000000", "0", "0")
		in.Emp.JoinDate = date(2026, time.July, 1)
		in.YTD.Months = 5
		return withTaxV2From(in, from)
	}
	l, _ := p1(t, mk(202612))
	if amountStr(l) != "3450000" || !strings.Contains(l.Remark, "Biaya Jabatan : 3,000,000") || !strings.Contains(l.Remark, "Bulan Kerja : 6") {
		t.Errorf("v2 masuk Juli: P1 %q remark %q", amountStr(l), l.Remark)
	}
	l, _ = p1(t, mk(0))
	if amountStr(l) != "0" || !strings.Contains(l.Remark, "Biaya Jabatan : 6,000,000") {
		t.Errorf("v1 masuk Juli: P1 %q remark %q", amountStr(l), l.Remark)
	}
	// Karyawan lama setahun penuh tetap × 12 (BJ 6 jt): bruto 120 jt, PKP 60 jt → 3 jt.
	in := mk(202612)
	in.Emp.JoinDate = date(2015, time.March, 2)
	in.YTD.Months = 11
	if l, _ := p1(t, in); amountStr(l) != "3000000" {
		t.Errorf("v2 setahun penuh: P1 %q remark %q", amountStr(l), l.Remark)
	}
}

func TestWorkMonths(t *testing.T) {
	cases := []struct {
		name   string
		yymm   int
		join   *time.Time
		months int
		want   int
	}{
		{"masuk 1 Juli, run Jul..Nov", 202612, date(2026, time.July, 1), 5, 6},
		{"masuk 26 Juni = periode Juli", 202612, date(2026, time.June, 26), 5, 6},
		{"masuk 25 Juni = periode Juni", 202612, date(2026, time.June, 25), 0, 7},
		{"run migrasi bolong, join menang", 202612, date(2026, time.July, 1), 3, 6},
		{"pindah NIK: run penempatan lama menang", 202612, date(2026, time.September, 1), 11, 12},
		{"masuk tahun lalu", 202612, date(2025, time.December, 30), 0, 12},
		{"masuk 26 Des tahun lalu = Januari", 202606, date(2025, time.December, 26), 0, 6},
		{"tanpa tanggal join", 202606, nil, 4, 5},
		{"join sesudah periode (data salah) → minimal 1", 202603, date(2026, time.May, 1), 0, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := Input{Period: Period{YYMM: c.yymm}, Emp: Employee{JoinDate: c.join}, YTD: YTDTax{Months: c.months}}
			if got := workMonths(in); got != c.want {
				t.Errorf("workMonths = %d, want %d", got, c.want)
			}
		})
	}
}

// D-80 (4a): resign Juni → masa pajak terakhir, P1 = PPh setahun (tidak disetahunkan) − PPh dibayar Jan..Mei.
// Bruto 5 × 10 jt + 10 jt = 60 jt, 6 bulan → BJ min(3 jt, 3 jt); PTKP TK 54 jt; PKP 3 jt → 150.000 − 1.000.000 = −850.000.
// Bulan non-leaver: TER A 2 % × 10.000.000 = 200.000 (bracket 9.650.001–10.050.000 ada di tabel uji).
func TestTaxV2LeaverTrueUp(t *testing.T) {
	mk := func(from int64, resign *time.Time) Input {
		in := decInput(true, "TK", "10000000", "50000000", "0", "1000000")
		in.Period = periodYM(202606)
		in.Emp.JoinDate = date(2019, time.April, 1)
		in.Emp.ResignDate = resign
		in.YTD.Months = 5
		return withTaxV2From(in, from)
	}
	cases := []struct {
		name   string
		in     Input
		want   string
		annual bool
	}{
		{"resign 10 Juni, v2: tahunan", mk(202606, date(2026, time.June, 10)), "-850000", true},
		{"resign 25 Juni (batas periode), v2: tahunan", mk(202606, date(2026, time.June, 25)), "-850000", true},
		{"resign 26 Juni = periode Juli: TER", mk(202606, date(2026, time.June, 26)), "200000", false},
		{"resign Juni, TAX_V2_FROM belum berlaku: TER (paritas)", mk(202612, date(2026, time.June, 10)), "200000", false},
		{"tanpa resign, v2: TER", mk(202606, nil), "200000", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l, _ := p1(t, c.in)
			if amountStr(l) != c.want || strings.Contains(l.Remark, "Masa Pajak Terakhir") != c.annual {
				t.Errorf("P1 %q (want %s), remark %q", amountStr(l), c.want, l.Remark)
			}
		})
	}
	// P5 ikut tahunan di bulan resign: tanpa baris ditanggung → 0 dengan remark tahunan.
	in := mk(202606, date(2026, time.June, 10))
	in.Components = append(in.Components, pComp(5, "0", "F", false))
	if l := lineOf(t, in, keyP5); amountStr(l) != "0" || !strings.Contains(l.Remark, "PPh Setahun") {
		t.Errorf("P5 resign v2: %q remark %q", amountStr(l), l.Remark)
	}
}

// Review T033m: karyawan resign Juni yang masih aktif di master (v1 tidak men-drop, D-55) tidak boleh mendapat
// BJ 12 bulan di Desember. Bruto 120 jt, bulan kerja 6 → BJ 3 jt; PKP 63 jt → 3.450.000 (bukan 3.000.000 dengan BJ 6 jt).
func TestTaxV2DecemberAfterResign(t *testing.T) {
	in := decInput(true, "TK", "20000000", "100000000", "0", "0")
	in.Emp.JoinDate = date(2015, time.March, 2)
	in.Emp.ResignDate = date(2026, time.June, 10)
	in.YTD.Months = 5
	in = withTaxV2From(in, 202612)
	if l, _ := p1(t, in); amountStr(l) != "3450000" || !strings.Contains(l.Remark, "Bulan Kerja : 6") {
		t.Errorf("Desember sesudah resign: P1 %q remark %q", amountStr(l), l.Remark)
	}
}

func TestLeaverMonth(t *testing.T) {
	p := periodYM(202606)
	cases := []struct {
		resign *time.Time
		want   bool
	}{
		{nil, false}, {date(2026, time.May, 26), true}, {date(2026, time.June, 25), true},
		{date(2026, time.May, 25), false}, {date(2026, time.June, 26), false},
	}
	for _, c := range cases {
		if got := LeaverMonth(p, c.resign); got != c.want {
			t.Errorf("LeaverMonth(%v) = %v, want %v", c.resign, got, c.want)
		}
	}
	if LeaverMonth(periodYM(202612), date(2026, time.December, 1)) {
		t.Error("December is annual anyway, LeaverMonth must be false")
	}
}

// D-80 (2): pajak v2 → P0 = −(P1 + P5). T73 600.000 ditanggung: P1 14.000, P5 −14.000, P0 0 (v1: −14.000).
func TestTaxV2SubsidyNetOfCompany(t *testing.T) {
	mk := func(from int64) Input {
		in := pphInput("TK", subT(1, "5000000"), paidT(73, "600000"), pComp(5, "0", "F", false), pComp(0, "0", "F", false))
		b := d("5600000")
		in.Subsidy.Rules, in.Subsidy.Basis = []SubsidyRule{ruleV1()}, &b
		return withTaxV2From(in, from)
	}
	cases := []struct {
		name string
		from int64
		want string
	}{
		{"v2 aktif: P0 = −(14.000 − 14.000)", 202610, "0"},
		{"v1: P0 = −P1", 0, "-14000"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := mk(c.from)
			if l := lineOf(t, in, keyP0); amountStr(l) != c.want {
				t.Errorf("P0 = %q, want %q", amountStr(l), c.want)
			}
			if l := lineOf(t, in, keyP5); amountStr(l) != "-14000" {
				t.Errorf("P5 = %q, want -14000", amountStr(l))
			}
		})
	}
	// Override AMOUNT dibatasi P1 + P5 (0) di v2.
	in := mk(202610)
	in.Subsidy.Override = &SubsidyOverride{Action: OverrideAmount, Amount: d("10000"), Reason: "uji"}
	if l := lineOf(t, in, keyP0); amountStr(l) != "0" {
		t.Errorf("override AMOUNT v2: P0 = %q, want 0", amountStr(l))
	}
}

func TestTaxV2Flag(t *testing.T) {
	v1 := rules{legacy: true}
	if TaxV2(v1, 202612) {
		t.Error("v1 without TAX_V2_FROM must stay legacy")
	}
	if !TaxV2(rules{legacy: false}, 202601) {
		t.Error("v2 rule set always uses tax v2")
	}
	r := taxV2Rules{pphRules: pphRules{rules: v1}, from: 202612}
	if TaxV2(r, 202611) || !TaxV2(r, 202612) || !TaxV2(r, 202701) {
		t.Error("TAX_V2_FROM boundary wrong")
	}
}

// Review T033m: P0 Desember pajak v2 = −(P1 + P5). Bruto 72 jt (YTD 66 jt + T1 5,4 jt + T73 0,6 jt ditanggung), P1 dibayar 0.
func TestTaxV2SubsidyDecember(t *testing.T) {
	in := decInput(true, "TK", "5400000", "66000000", "0", "0", paidT(73, "600000"), pComp(5, "0", "F", false), pComp(0, "0", "F", false))
	in.YTDCompany = YTDCompanyTax{Gross: d("66000000"), CompanyPaid: d("6600000")}
	b := d("5000000")
	in.Subsidy.Rules, in.Subsidy.Basis = []SubsidyRule{ruleV1()}, &b
	in = withTaxV2From(in, 202612)
	res, err := Calculate(in)
	if err != nil {
		t.Fatal(err)
	}
	get := func(k CompKey) decimal.Decimal {
		for _, l := range res.Lines {
			if l.Key == k {
				return l.AmountOrZero()
			}
		}
		t.Fatalf("no line %s", k)
		return decimal.Zero
	}
	p1v, p5v, p0v := get(CompKey{CompP, 1}), get(keyP5), get(keyP0)
	if !p0v.Equal(p1v.Add(p5v).Neg()) || p5v.IsZero() {
		t.Errorf("P0 %s, want −(P1 %s + P5 %s)", p0v, p1v, p5v)
	}
}
