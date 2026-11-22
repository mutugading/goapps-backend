package engine

import (
	"strings"
	"testing"
)

// T033n (D-80 (3)): PTKP tahunan memakai status pajak per 1 Januari. Karyawan TK di 1 Jan menikah Juni (K0):
// bruto 120 jt, BJ 6 jt. Pajak v2: PTKP TK 54 jt → PKP 60 jt → 3.000.000 (bukan K0 58,5 jt → 2.775.000).
// v1 (parity legacy, tanpa riwayat) tetap memakai status terkini.
func TestAnnualPTKPUsesJanuaryStatus(t *testing.T) {
	mk := func(from int64, jan1 string) Input {
		in := decInput(true, "K0", "10000000", "110000000", "0", "0")
		in.Emp.TaxStatusJan1 = jan1
		return withTaxV2From(in, from)
	}
	cases := []struct {
		name, jan1, want, ptkp string
		from                   int64
	}{
		{"v2, menikah Juni → PTKP TK 1 Jan", "TK", "3000000", "PTKP : 54,000,000", 202612},
		{"v2, tanpa riwayat → status terkini", "", "2775000", "PTKP : 58,500,000", 202612},
		{"v1 parity → status terkini", "TK", "2775000", "PTKP : 58,500,000", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l, _ := p1(t, mk(c.from, c.jan1))
			if amountStr(l) != c.want || !strings.Contains(l.Remark, c.ptkp) {
				t.Errorf("P1 = %q, want %q (remark %q)", amountStr(l), c.want, l.Remark)
			}
		})
	}
}

// P5 Desember v2 ikut status 1 Januari; status 1 Jan yang tidak dikenal PTKP memberi PPH_NO_PTKP dengan kode itu.
func TestAnnualPTKPJanuaryStatusP5AndWarning(t *testing.T) {
	in := decInput(true, "K0", "10000000", "110000000", "0", "0", paidT(73, "600000"), pComp(5, "0", "F", false))
	in.Emp.TaxStatusJan1 = "TK"
	in = withTaxV2From(in, 202612)
	if l := lineOf(t, in, keyP5); !strings.Contains(l.Remark, "PTKP : 54,000,000") {
		t.Errorf("P5 remark %q, want PTKP TK", l.Remark)
	}
	for _, c := range []struct{ cur, jan1, want string }{
		{"K0", "XX", `"XX"`}, // status 1 Jan tidak dikenal PTKP
		{"", "TK", `""`},     // status terkini dikosongkan: riwayat tidak dipakai diam-diam
	} {
		in.Emp.TaxStatus, in.Emp.TaxStatusJan1 = c.cur, c.jan1
		_, res := p1(t, in)
		found := false
		for _, w := range res.Warnings {
			found = found || strings.Contains(w, WarnPPhNoPTKP) && strings.Contains(w, c.want)
		}
		if !found {
			t.Errorf("cur %q jan1 %q: warnings %v, want PPH_NO_PTKP %s", c.cur, c.jan1, res.Warnings, c.want)
		}
	}
}
