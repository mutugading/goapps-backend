package engine

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

var keyP0 = CompKey{CompP, 0}

// subT is a taxable T component in the subsidy basis (tax_subsidy = Y).
func subT(id int, amt string) EmpComponent {
	c := taxT(id, amt, "F")
	c.TaxSubsidy = true
	return c
}

// ruleV1 is the migrated MGT_PAYROLL_SUBSIDI_PPH row: SOLO, basis 202601, limit 10 jt, FULL_PPH.
func ruleV1() SubsidyRule {
	return SubsidyRule{ID: 7, Name: "SOLO 2026", Priority: 100, ValidFrom: 202601, Divisions: []string{"SOLO"},
		BasisPeriod: 202601, Limit: d("10000000"), Kind: SubsidyFullPPh}
}

// subInput: T1 5.600.000 (P1 TER A 0,25 % = 14.000) with a P0 row.
func subInput(basis string, rules ...SubsidyRule) Input {
	in := pphInput("TK", subT(1, "5600000"), pComp(0, "0", "F", false))
	in.Subsidy.Rules = rules
	if basis != "" {
		b := d(basis)
		in.Subsidy.Basis = &b
	}
	return in
}

// Golden angka tangan (06§6.4, 12§4): P0 = −P1 bila rule cocok dan basis ≤ batas, selain itu 0.
func TestPPhSubsidy(t *testing.T) {
	cases := []struct {
		name     string
		in       func() Input
		want     string
		ruleMeta string
	}{
		{"aktif, basis 202601 ≤ batas", func() Input { return subInput("9000000", ruleV1()) }, "-14000", "7"},
		{"basis tepat batas", func() Input { return subInput("10000000", ruleV1()) }, "-14000", "7"},
		{"di atas batas", func() Input { return subInput("10000001", ruleV1()) }, "0", "7"},
		{"tanpa rule (YN N / tanpa baris)", func() Input { return subInput("9000000") }, "0", ""},
		{"divisi lain", func() Input {
			in := subInput("9000000", ruleV1())
			in.Division = "JAKARTA"
			return in
		}, "0", ""},
		{"rule belum berlaku", func() Input {
			r := ruleV1()
			r.ValidFrom = 202611
			return subInput("9000000", r)
		}, "0", ""},
		// basis 202601 nil/0 → baris berjalan tax_subsidy = Y (5.600.000, T7 bukan basis).
		{"fallback komponen berjalan", func() Input {
			in := subInput("", ruleV1())
			in.Components = append(in.Components, taxT(7, "0", "F"))
			return in
		}, "-14000", "7"},
		{"fallback berjalan di atas batas", func() Input {
			in := subInput("0", ruleV1())
			in.Components = append(in.Components, subT(7, "4500000")) // basis 10.100.000
			return in
		}, "0", "7"},
		{"prioritas: rule pertama yang cocok", func() Input {
			low := ruleV1()
			low.ID, low.Priority, low.Limit = 9, 10, d("5000000")
			return subInput("9000000", ruleV1(), low)
		}, "0", "9"},
		{"override INCLUDE di atas batas", func() Input {
			in := subInput("20000000", ruleV1())
			in.Subsidy.Override = &SubsidyOverride{Action: OverrideInclude, Reason: "kebijakan direksi"}
			return in
		}, "-14000", ""},
		{"override EXCLUDE", func() Input {
			in := subInput("9000000", ruleV1())
			in.Subsidy.Override = &SubsidyOverride{Action: OverrideExclude, Reason: "PPh manual"}
			return in
		}, "0", ""},
		{"override AMOUNT", func() Input {
			in := subInput("9000000", ruleV1())
			in.Subsidy.Override = &SubsidyOverride{Action: OverrideAmount, Amount: d("5000"), Reason: "sebagian"}
			return in
		}, "-5000", ""},
		// P1 NULL (bracket hilang) → −NULL tetap NULL, seperti V_PPH × −1 legacy.
		{"P1 NULL", func() Input {
			in := subInput("9000000", ruleV1())
			in.Components[0] = subT(1, "7000000")
			return in
		}, "", "7"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l := lineOf(t, c.in(), keyP0)
			if got := amountStr(l); got != c.want {
				t.Errorf("P0 = %q, want %q (remark %q)", got, c.want, l.Remark)
			}
			if got := l.Meta[MetaSubsidyRule]; got != c.ruleMeta {
				t.Errorf("meta rule = %q, want %q", got, c.ruleMeta)
			}
		})
	}
	t.Run("meta mencatat basis, batas, override", func(t *testing.T) {
		l := lineOf(t, subInput("9000000", ruleV1()), keyP0)
		if l.Meta[MetaSubsidyBasis] != "9000000" || l.Meta[MetaSubsidyLimit] != "10000000" {
			t.Errorf("meta = %v", l.Meta)
		}
		in := subInput("9000000", ruleV1())
		in.Subsidy.Override = &SubsidyOverride{Action: OverrideExclude, Reason: "x"}
		if l = lineOf(t, in, keyP0); l.Meta[MetaSubsidyOverride] != OverrideExclude {
			t.Errorf("meta = %v", l.Meta)
		}
		if l = lineOf(t, in, CompKey{CompP, 1}); l.Meta != nil {
			t.Errorf("P1 meta = %v, want nil", l.Meta)
		}
	})
	t.Run("take-home tanpa potongan PPh", func(t *testing.T) {
		res, err := Calculate(subInput("9000000", ruleV1()))
		if err != nil {
			t.Fatal(err)
		}
		if res.Header == nil || res.Header.TotPotongan.String() != "0" {
			t.Errorf("header = %+v", res.Header)
		}
	})
	t.Run("jenis tidak dikenal", func(t *testing.T) {
		r := ruleV1()
		r.Kind = "EXPR"
		if _, err := Calculate(subInput("9000000", r)); !errors.Is(err, ErrSubsidyKind) {
			t.Fatalf("err = %v", err)
		}
		in := subInput("9000000", ruleV1())
		in.Subsidy.Override = &SubsidyOverride{Action: "X"}
		if _, err := Calculate(in); !errors.Is(err, ErrSubsidyKind) {
			t.Fatalf("override err = %v", err)
		}
	})
}

// p0Of returns the P0 line, ok false when the engine produced none.
func p0Of(t *testing.T, in Input) (Line, bool) {
	t.Helper()
	res, err := Calculate(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range res.Lines {
		if l.Key == keyP0 {
			return l, true
		}
	}
	return Line{}, false
}

// joinInput: joined 17/10 (9 days of 26/09–25/10), T1 12.000.000 STS D → 3.600.000, plus a taxable
// T5 2.000.000 outside the basis so P1 = TER A 0,25 % × 5.600.000 = 14.000.
func joinInput(legacy bool) Input {
	in := pphInput("TK", subT(1, "12000000"), taxT(5, "2000000", "F"), pComp(0, "0", "F", false))
	in.Components[0].Sts = "D"
	in.Emp.GapokSts = "D"
	in.Att = AttRekap{"pay_tot": d("9")}
	j := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)
	in.Emp.JoinDate = &j
	in.Subsidy.Rules = []SubsidyRule{ruleV1()}
	in.Rules = pphRules{rules: rules{legacy: legacy}}
	return in
}

// v2 is subInput under rule v2.
func v2(in Input) Input {
	in.Rules = pphRules{rules: rules{legacy: false}}
	return in
}

// withoutP0 drops the P0 component row.
func withoutP0(in Input) Input {
	in.Components = slices.DeleteFunc(in.Components, func(c EmpComponent) bool { return c.Key == keyP0 })
	return in
}

// Golden D-30: urutan basis v1 (202601 → periode lalu → berjalan), gerbang baris P0, basis v2 terkunci.
func TestPPhSubsidyD30(t *testing.T) {
	ptr := func(s string) *decimal.Decimal { v := d(s); return &v }
	cases := []struct {
		name, want, src, basis string
		in                     func() Input
	}{
		// v1
		{"v1 basis 202601 menang atas periode lalu", "-14000", SubsidySrcBasisPeriod, "9000000", func() Input {
			in := subInput("9000000", ruleV1())
			in.Subsidy.PrevBasis = ptr("11000000")
			return in
		}},
		{"v1 fallback periode lalu di atas batas", "0", SubsidySrcPrevPeriod, "11000000", func() Input {
			in := subInput("0", ruleV1()) // DETAIL 202601 ada tapi 0 → langkah 2
			in.Subsidy.PrevBasis = ptr("11000000")
			return in
		}},
		{"v1 fallback periode lalu ≤ batas", "-14000", SubsidySrcPrevPeriod, "9500000", func() Input {
			in := subInput("", ruleV1())
			in.Subsidy.PrevBasis = ptr("9500000")
			return in
		}},
		{"v1 periode lalu 0 → berjalan", "-14000", SubsidySrcCurrent, "5600000", func() Input {
			in := subInput("", ruleV1())
			in.Subsidy.PrevBasis = ptr("0")
			return in
		}},
		// defect D-30: join 9 hari gaji 12 jt → basis terprorata 3.600.000 → dapat subsidi.
		{"v1 join baru terprorata dapat", "-14000", SubsidySrcCurrent, "3600000", func() Input { return joinInput(true) }},
		// T8 lembur 4.450.000 (tax_subsidy) → basis 10.050.000 > batas; P1 2 % = 201.000 tidak disubsidi.
		{"v1 lembur mendorong basis > batas", "0", SubsidySrcCurrent, "10050000", func() Input {
			in := subInput("", ruleV1())
			in.Components = append(in.Components, subT(8, "4450000"))
			return in
		}},
		// v2
		{"v2 join 9 hari gaji 12 jt tidak dapat", "0", SubsidySrcProvisional, "12000000", func() Input { return joinInput(false) }},
		// basis sementara: nominal T1 8 jt sebelum prorata; T8 lembur dan T5 (bukan basis) tidak ikut.
		{"v2 basis sementara tanpa lembur", "-14000", SubsidySrcProvisional, "8000000", func() Input {
			in := joinInput(false)
			in.Components[0] = subT(1, "8000000")
			in.Components[0].Sts = "D" // 2.400.000; gross 2.400.000 + 2.000.000 + 1.200.000 = 5.600.000
			in.Components = append(in.Components, subT(8, "1200000"))
			return in
		}},
		{"v2 basis terkunci (lembur ikut) menang atas berjalan", "0", SubsidySrcLocked, "10500000", func() Input {
			in := v2(subInput("9000000", ruleV1())) // basis 202601 v1 diabaikan
			in.Subsidy.Locked, in.Subsidy.LockedYYMM = ptr("10500000"), 202601
			return in
		}},
		{"v2 kunci periode berjalan diabaikan (hitung ulang)", "-14000", SubsidySrcLockNow, "5600000", func() Input {
			in := v2(subInput("", ruleV1()))
			in.Subsidy.Locked, in.Subsidy.LockedYYMM = ptr("10500000"), 202610
			return in
		}},
		{"v2 basis 202601 & periode lalu v1 diabaikan", "-14000", SubsidySrcLockNow, "5600000", func() Input {
			in := v2(subInput("20000000", ruleV1()))
			in.Subsidy.PrevBasis = ptr("20000000")
			return in
		}},
		// join tepat di awal periode (jam 08:00) dengan gapok F = bulan penuh.
		{"v2 join tepat awal periode = bulan penuh", "-14000", SubsidySrcLockNow, "5600000", func() Input {
			in := v2(subInput("", ruleV1()))
			j := time.Date(2026, 9, 26, 8, 0, 0, 0, time.UTC)
			in.Emp.JoinDate = &j
			return in
		}},
		// D-30 teks: STS F cukup walau pay_tot < DM (mis. alpa/dirumahkan); basis tetap line berjalan.
		{"v2 gapok F pay_tot < DM = bulan penuh", "-14000", SubsidySrcLockNow, "5600000", func() Input {
			in := v2(subInput("", ruleV1()))
			in.Att = AttRekap{"pay_tot": d("20")}
			return in
		}},
		// basis sementara: komponen formula (BPJS T11) ikut dengan nilai hitungnya, bukan nominal 0.
		{"v2 basis sementara memakai line komponen formula", "0", SubsidySrcProvisional, "12000000", func() Input {
			in := joinInput(false)
			in.Components[0] = subT(1, "9000000")
			in.Components[0].Sts = "D"
			in.Components = append(in.Components, subT(10, "0")) // T10 SUBSIDI_PPH_JKT: SOLO → line ada
			in.Additions = map[CompKey][]Addition{{CompT, 19}: {{Amount: d("3000000")}}}
			in.Components = append(in.Components, subT(19, "0")) // addition 3 jt, bukan prorata nominal
			return in
		}},
		{"v2 bulan penuh pertama mengunci, lembur ikut", "0", SubsidySrcLockNow, "10050000", func() Input {
			in := v2(subInput("", ruleV1()))
			in.Components = append(in.Components, subT(8, "4450000"))
			return in
		}},
		{"v2 bulan penuh pertama ≤ batas", "-14000", SubsidySrcLockNow, "5600000", func() Input { return v2(subInput("", ruleV1())) }},
		{"v2 tanpa baris P0 tetap dihitung", "-14000", SubsidySrcLockNow, "5600000", func() Input {
			return withoutP0(v2(subInput("", ruleV1())))
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l, ok := p0Of(t, c.in())
			if !ok {
				t.Fatal("no P0 line")
			}
			if got := amountStr(l); got != c.want {
				t.Errorf("P0 = %q, want %q (remark %q)", got, c.want, l.Remark)
			}
			if l.Meta[MetaSubsidySource] != c.src || l.Meta[MetaSubsidyBasis] != c.basis {
				t.Errorf("meta = %v, want source %s basis %s", l.Meta, c.src, c.basis)
			}
		})
	}
	t.Run("v1 tanpa baris P0 → tidak ada P0", func(t *testing.T) {
		if _, ok := p0Of(t, withoutP0(subInput("9000000", ruleV1()))); ok {
			t.Error("P0 computed without a P0 row")
		}
	})
	t.Run("v2 override EXCLUDE tanpa baris P0", func(t *testing.T) {
		in := withoutP0(v2(subInput("9000000", ruleV1())))
		in.Subsidy.Override = &SubsidyOverride{Action: OverrideExclude, Reason: "warisan legacy"}
		l, ok := p0Of(t, in)
		if !ok || amountStr(l) != "0" || l.Meta[MetaSubsidyOverride] != OverrideExclude {
			t.Errorf("P0 = %+v ok %v", l, ok)
		}
	})
	t.Run("v2 tanpa rule dan override → tidak ada P0", func(t *testing.T) {
		if _, ok := p0Of(t, withoutP0(v2(subInput("9000000")))); ok {
			t.Error("P0 added without a rule")
		}
	})
}
