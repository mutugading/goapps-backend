package engine

import (
	"strings"
	"testing"
	"time"
)

func countWarn(res Result, code string) int {
	n := 0
	for _, w := range res.Warnings {
		if strings.HasPrefix(w, code) {
			n++
		}
	}
	return n
}

// Temuan review T015 (D-31): satu warning per karyawan untuk bracket hilang, walau P1 dan P5
// sama-sama memanggil terTax pada bruto yang sama.
func TestPPhNoBracketWarnsOnce(t *testing.T) {
	// 7.100.000 tidak ada di tabel uji kategori A → P1 dan P5 NULL.
	in := pphInput("TK", taxT(1, "7000000", "F"), paidT(73, "100000"), pComp(5, "0", "F", false))
	res, err := Calculate(in)
	if err != nil {
		t.Fatal(err)
	}
	if n := countWarn(res, WarnPPhNoBracket); n != 1 {
		t.Errorf("PPH_NO_BRACKET = %d kali, want 1: %v", n, res.Warnings)
	}
}

// P5 non-Desember: baris ditanggung selain T73, THR terisi, STS D, dan periode Januari.
func TestPPhCompanyNonDecemberExtra(t *testing.T) {
	p5 := pComp(5, "0", "F", false)
	t.Run("baris ditanggung T74", func(t *testing.T) {
		in := pphInput("TK", taxT(1, "5000000", "F"), paidT(74, "600000"), p5)
		if got := amountStr(lineOf(t, in, keyP5)); got != "-14000" {
			t.Errorf("P5 = %q", got)
		}
	})
	t.Run("Thr.Gross menggeser bracket di kedua sisi", func(t *testing.T) {
		// T1 4.000.000 + T73 600.000 + THR 1.000.000: semua 5.600.000 → 14.000; tanpa T73 5.000.000 → 0.
		mk := func(thr string) Input {
			in := pphInput("TK", taxT(1, "4000000", "F"), paidT(73, "600000"), p5)
			in.Thr.Gross = d(thr)
			return in
		}
		if got := amountStr(lineOf(t, mk("1000000"), keyP5)); got != "-14000" {
			t.Errorf("dengan THR P5 = %q, want -14000", got)
		}
		if got := amountStr(lineOf(t, mk("0"), keyP5)); got != "0" {
			t.Errorf("tanpa THR P5 = %q, want 0", got)
		}
	})
	t.Run("STS D baris ditanggung terprorata", func(t *testing.T) {
		// T73 1.200.000 STS D × pay_tot/DM; DM periode uji dipakai agar hasil 600.000.
		in := pphInput("TK", taxT(1, "5000000", "F"), paidT(73, "1200000"), p5)
		in.Components[1].Sts = "D"
		in.Att = AttRekap{"pay_tot": d("15")}
		in.Components[1].Amount = d("1200000")
		l := lineOf(t, in, CompKey{CompT, 73})
		want := l.AmountOrZero()
		// P5 = TER(bruto − T73) − TER(bruto): kedua sisi ikut nilai T73 yang sudah terprorata.
		all := d("5000000").Add(want)
		var wantP5 string
		switch {
		case all.GreaterThan(d("5400000")) && all.LessThanOrEqual(d("5650000")):
			wantP5 = "-" + all.Mul(d("0.25")).Shift(-2).Round(0).String()
		default:
			wantP5 = "0"
		}
		if got := amountStr(lineOf(t, in, keyP5)); got != wantP5 {
			t.Errorf("P5 = %q, want %q (T73 %s)", got, wantP5, want)
		}
	})
	t.Run("periode Januari non-Desember", func(t *testing.T) {
		in := pphInput("TK", taxT(1, "5000000", "F"), paidT(73, "600000"), p5)
		in.Period = Period{YYMM: 202601, Start: time.Date(2025, 12, 26, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 1, 25, 0, 0, 0, 0, time.UTC)}
		if got := amountStr(lineOf(t, in, keyP5)); got != "-14000" {
			t.Errorf("P5 = %q", got)
		}
	})
}

// P5 Desember: pengurang P4/P14/P18 bulan berjalan, v2 PKP negatif dan TRUNC.
func TestPPhCompanyDecemberExtra(t *testing.T) {
	t.Run("v1 pengurang P4/P14/P18 bulan berjalan", func(t *testing.T) {
		// P4/P14/P18 dihitung formula masing-masing; yang diuji: ketiganya (bukan P8) masuk pengurang
		// bersama YTD, dilihat dari remark P5.
		in := decInput(true, "TK", "5400000", "0", "0", "0", paidT(73, "600000"), pComp(5, "0", "F", false),
			pComp(4, "1000000", "F", false), pComp(8, "700000", "F", false),
			pComp(14, "500000", "F", false), pComp(18, "500000", "F", false))
		in.YTDCompany = YTDCompanyTax{Gross: d("66000000"), CompanyPaid: d("6600000"), Deduct: d("6000000")}
		res, err := Calculate(in)
		if err != nil {
			t.Fatal(err)
		}
		cur := d("0")
		var p5 Line
		for _, l := range res.Lines {
			switch l.Key {
			case CompKey{CompP, 4}, CompKey{CompP, 14}, CompKey{CompP, 18}:
				cur = cur.Add(l.AmountOrZero())
			case keyP5:
				p5 = l
			}
		}
		want := "Pengurang : " + thousands(d("6000000").Add(cur))
		if !strings.Contains(p5.Remark, want) {
			t.Errorf("remark %q tidak memuat %q", p5.Remark, want)
		}
	})
	t.Run("v2 PKP negatif → 0, hanya P5 Jan-Nov yang dikembalikan", func(t *testing.T) {
		in := decInput(false, "TK", "1000000", "0", "0", "0", paidT(73, "100000"), pComp(5, "0", "F", false))
		in.YTDCompany = YTDCompanyTax{Gross: d("10000000"), CompanyPaid: d("1000000"), P5Paid: d("-330000")}
		if got := amountStr(lineOf(t, in, keyP5)); got != "330000" {
			t.Errorf("P5 = %q, want 330000", got)
		}
	})
	t.Run("v2 PKP di-TRUNC ribuan", func(t *testing.T) {
		// bruto 72.000.999 → PKP 14.400.949,05 → 14.400.000 → 720.000;
		// sendiri 64.800.999 → PKP 7.560.949,05 → 7.560.000 → 378.000.
		in := decInput(false, "TK", "5400999", "0", "0", "0", paidT(73, "600000"), pComp(5, "0", "F", false))
		in.YTDCompany = YTDCompanyTax{Gross: d("66000000"), CompanyPaid: d("6600000")}
		if got := amountStr(lineOf(t, in, keyP5)); got != "-342000" {
			t.Errorf("P5 = %q, want -342000", got)
		}
	})
}

// P0: Desember, Januari, JAKARTA, override v1 tanpa baris P0, dan flag OT komponen.
func TestPPhSubsidyExtra(t *testing.T) {
	t.Run("Desember v2 memakai P1 tahunan", func(t *testing.T) {
		in := decInput(false, "TK", "6000000", "66000000", "2160000", "550000")
		in.Components[0] = subT(1, "6000000")
		in.Subsidy.Rules = []SubsidyRule{ruleV1()}
		l, ok := p0Of(t, in)
		if !ok || amountStr(l) != "-62000" || l.Meta[MetaSubsidySource] != SubsidySrcLockNow {
			t.Errorf("P0 = %+v ok %v, want -62000 %s", l, ok, SubsidySrcLockNow)
		}
	})
	t.Run("Desember v1 dengan baris P0", func(t *testing.T) {
		in := decInput(true, "TK", "6000000", "66000000", "2160000", "550000", pComp(0, "0", "F", false))
		in.Components[0] = subT(1, "6000000")
		in.Subsidy.Rules = []SubsidyRule{ruleV1()}
		l, ok := p0Of(t, in)
		if !ok || amountStr(l) != "-62000" || l.Meta[MetaSubsidySource] != SubsidySrcCurrent {
			t.Errorf("P0 = %+v ok %v", l, ok)
		}
	})
	t.Run("Januari v2", func(t *testing.T) {
		in := v2(subInput("", ruleV1()))
		in.Period = Period{YYMM: 202601, Start: time.Date(2025, 12, 26, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 1, 25, 0, 0, 0, 0, time.UTC)}
		l, ok := p0Of(t, in)
		if !ok || amountStr(l) != "-14000" || l.Meta[MetaSubsidySource] != SubsidySrcLockNow {
			t.Errorf("P0 = %+v ok %v", l, ok)
		}
	})
	t.Run("JAKARTA tidak cocok rule SOLO → 0", func(t *testing.T) {
		in := subInput("", ruleV1())
		in.Division = "JAKARTA"
		l, ok := p0Of(t, in)
		if !ok || amountStr(l) != "0" || l.Meta[MetaSubsidyRule] != "" {
			t.Errorf("P0 = %+v ok %v", l, ok)
		}
	})
	t.Run("JAKARTA cocok rule JAKARTA", func(t *testing.T) {
		r := ruleV1()
		r.Divisions = []string{"JAKARTA"}
		in := subInput("", r)
		in.Division = "JAKARTA"
		l, ok := p0Of(t, in)
		if !ok || amountStr(l) != "-14000" {
			t.Errorf("P0 = %+v ok %v", l, ok)
		}
	})
	t.Run("v1 override tanpa baris P0 → warning", func(t *testing.T) {
		in := withoutP0(subInput("9000000", ruleV1()))
		in.Subsidy.Override = &SubsidyOverride{Action: OverrideInclude, Reason: "x"}
		res, err := Calculate(in)
		if err != nil {
			t.Fatal(err)
		}
		if countWarn(res, WarnSubsidyOverrideIgnored) != 1 {
			t.Errorf("warnings = %v", res.Warnings)
		}
		in = subInput("9000000", ruleV1()) // dengan baris P0 → override berlaku, tanpa warning
		in.Subsidy.Override = &SubsidyOverride{Action: OverrideInclude, Reason: "x"}
		if res, _ = Calculate(in); countWarn(res, WarnSubsidyOverrideIgnored) != 0 {
			t.Errorf("warning tak perlu: %v", res.Warnings)
		}
	})
	t.Run("flag SubsidyOT mengeluarkan komponen dari basis sementara", func(t *testing.T) {
		in := joinInput(false)
		ot := subT(55, "1200000") // id 55 bukan daftar OT D-30
		ot.Sts = "D"
		ot.SubsidyOT = true
		in.Components = append(in.Components, ot)
		l, _ := p0Of(t, in)
		if l.Meta[MetaSubsidyBasis] != "12000000" {
			t.Errorf("basis = %s, want 12000000", l.Meta[MetaSubsidyBasis])
		}
		in.Components[len(in.Components)-1].SubsidyOT = false
		if l, _ = p0Of(t, in); l.Meta[MetaSubsidyBasis] != "13200000" {
			t.Errorf("tanpa flag basis = %s, want 13200000", l.Meta[MetaSubsidyBasis])
		}
	})
}
