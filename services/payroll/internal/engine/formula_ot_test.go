package engine

import "testing"

// otInput is a SOLO operator with T1 3.000.000, T7 300.000, T55 200.000 (all F), so
// RATEOTOPR = Σ T{1,7,55} = 3.500.000 and the operator hourly rate = 3.500.000/173.
func otInput(att AttRekap, comps ...EmpComponent) Input {
	in := tInput(att, append([]EmpComponent{tc(7, "300000", "F"), tc(55, "200000", "F")}, comps...)...)
	in.Emp.Level = "Operator Shift I,II,III"
	return in
}

func intp(v int) *int { return &v }

// Golden cases computed by hand from FNC_MGT_PAYROLL_COMPNT_CALC (D-25), incl. G4 (T36) and G5 (D11).
func TestOTFormulas(t *testing.T) {
	att := AttRekap{"pay_tot": d("26"), "otiw_decr": d("0"), "shift": d("20"),
		"lembur_1": d("4"), "lembur_2": d("4"), "lembur_off_1": d("4"), "lembur_off_2": d("4"), "lembur_off_3": d("4"),
		"lembur_nasp_2": d("4"), "lembur_nasp_3": d("4"), "lembur_nas_1": d("8"), "lembur_nas_2": d("8"), "lembur_nas_3": d("8"), "lembur_nasp_1": d("4")}
	exec := func(in *Input) { in.Emp.Level = "Executive" }
	otiw := func(v string) func(*Input) { return func(in *Input) { in.Att["otiw_decr"] = d(v) } }
	cases := []struct {
		name string
		id   int
		mod  func(*Input)
		comp EmpComponent
		want string
	}{
		// T6: RATEOTOPR × A × ratio; 0.0877: TOT = 3.5×52/12, ratio = (TOT − 3.5×fail)/TOT
		{"T6 F → A (rounded)", 6, nil, tc(6, "0.0877", "F"), "0"},
		{"T6 0.0877 fail 0", 6, nil, tc(6, "0.0877", "D"), "306950"},        // 3500000×0.0877
		{"T6 0.0877 fail 2", 6, otiw("2"), tc(6, "0.0877", "D"), "165281"},  // 306950×98/182 = 165280.77
		{"T6 0.0877 fail 4", 6, otiw("4"), tc(6, "0.0877", "D"), "23612"},   // 306950×14/182 = 23611.54
		{"T6 0.0877 fail 5 → 0", 6, otiw("5"), tc(6, "0.0877", "D"), "0"},   // > 4
		{"T6 0.075 fail 1", 6, otiw("1"), tc(6, "0.075", "D"), "201923"},    // TOT 13; 262500×10/13 = 201923.08
		{"T6 0.9017 fail 3", 6, otiw("3"), tc(6, "0.9017", "D"), "2791802"}, // TOT 156; 3155950×138/156 = 2791801.92
		{"T6 0.9017 fail 26 → 0", 6, otiw("26"), tc(6, "0.9017", "D"), "0"}, // (156 − 156)/156
		{"T6 unknown amount → NULL", 6, nil, tc(6, "0", "D"), ""},           // V_OTINWORK stays NULL
		{"T6 JAKARTA still RATEOTOPR", 6, func(in *Input) { in.Division = divJakarta }, tc(6, "0.0877", "D"), "306950"},
		{"T6 no recap → fail 0", 6, func(in *Input) { in.Att = nil }, tc(6, "0.0877", "D"), "306950"},
		// T36 (G4, D9): only A = 0.0127 has a variant; TOT = 0.75×4×52/12 = 13
		{"G4 T36 0.0127 fail 0", 36, nil, tc(36, "0.0127", "D"), "44450"},       // 3500000×0.0127
		{"G4 T36 0.0127 fail 2", 36, otiw("2"), tc(36, "0.0127", "D"), "23935"}, // 44450×7/13 = 23934.62
		{"G4 T36 other amount → NULL", 36, nil, tc(36, "0.0877", "D"), ""},
		// T8: A/WD × (WD − otiw_decr)
		{"T8 D fail 0 → A", 8, func(in *Input) { in.Workdays = intp(26) }, tc(8, "520000", "D"), "520000"},
		{"T8 D fail 2", 8, func(in *Input) { in.Workdays = intp(26); in.Att["otiw_decr"] = d("2") }, tc(8, "520000", "D"), "480000"},
		{"T8 D no WD → NULL", 8, nil, tc(8, "520000", "D"), ""},
		// T20: rate/173 × ADD hours
		{"T20 opr", 20, func(in *Input) { in.Additions = map[CompKey][]Addition{{CompT, 20}: {{Amount: d("10")}}} }, tc(20, "0", "D"), "202312"},
		{"T20 exec", 20, func(in *Input) {
			exec(in)
			in.Additions = map[CompKey][]Addition{{CompT, 20}: {{Amount: d("10")}}}
		}, tc(20, "0", "D"), "173410"}, // Σ T{1..4} = 3000000 /173×10
		{"T20 F", 20, func(in *Input) { in.Additions = map[CompKey][]Addition{{CompT, 20}: {{Amount: d("10")}}} }, tc(20, "0", "F"), "10"},
		{"T20 no addition → NULL", 20, nil, tc(20, "0", "D"), ""},
		// T21–31
		{"T21 opr ×1.5", 21, nil, tc(21, "0", "D"), "121387"}, // 3500000/173×4×1.5 = 121387.28
		{"T21 exec → NULL (D11)", 21, exec, tc(21, "0", "D"), ""},
		{"T21 JAKARTA exec T1 rate", 21, func(in *Input) { exec(in); in.Division = divJakarta }, tc(21, "0", "D"), "104046"}, // 3000000/173×6
		{"T21 NULL level → NULL", 21, func(in *Input) { in.Emp.Level = "" }, tc(21, "0", "D"), ""},
		{"T21 no recap → NULL", 21, func(in *Input) { in.Att = nil }, tc(21, "0", "D"), ""},
		{"T21 F", 21, nil, tc(21, "75000", "F"), "75000"},
		{"T26 exec ×2", 26, exec, tc(26, "0", "D"), "277457"},                                           // 3000000/173×8×2 = 277456.65
		{"T26 NULL level → opr", 26, func(in *Input) { in.Emp.Level = "" }, tc(26, "0", "D"), "323699"}, // 3500000/173×16 = 323699.42
		{"G5 T27 exec ×2", 27, exec, tc(27, "0", "D"), "277457"},
		{"G5 T28 exec ×2", 28, exec, tc(28, "0", "D"), "277457"},
		{"G5 T27 opr ×3", 27, nil, tc(27, "0", "D"), "485549"}, // 3500000/173×24 = 485549.13
		{"G5 T28 opr ×4", 28, nil, tc(28, "0", "D"), "647399"}, // 3500000/173×32 = 647398.84
		{"T29 opr ×2", 29, nil, tc(29, "0", "D"), "161850"},    // 3500000/173×8 = 161849.71
		// Shift
		{"T34 D", 34, nil, tc(34, "15000", "D"), "300000"},
		{"T35 F", 35, nil, tc(35, "450000", "F"), "450000"},
		{"T34 no recap → NULL", 34, func(in *Input) { in.Att = nil }, tc(34, "15000", "D"), ""},
		{"T134 D", 134, func(in *Input) { in.AttAdd = AttRekapAdd{"shift_add": d("3")} }, tc(134, "15000", "D"), "45000"},
		{"T134 no add recap → NULL", 134, nil, tc(134, "15000", "D"), ""},
		{"T136 D", 136, func(in *Input) { in.AttAdd = AttRekapAdd{"pay_tot_add": d("5")} }, tc(136, "0.0127", "D"), "7408"}, // 3500000/30×0.0127×5 = 7408.33
		{"T136 F → A (rounded)", 136, nil, tc(136, "0.0127", "F"), "0"},
		{"T136 no add recap → NULL", 136, nil, tc(136, "0.0127", "D"), ""},
		{"T34 other STS → NULL", 34, nil, tc(34, "15000", "X"), ""},
		// Multipliers per id: opr rate 3500000/173 × 4 hours × mult
		{"T22 opr ×2", 22, nil, tc(22, "0", "D"), "161850"}, // ×8 = 161849.71
		{"T23 opr ×2", 23, nil, tc(23, "0", "D"), "161850"},
		{"T24 opr ×3", 24, nil, tc(24, "0", "D"), "242775"}, // ×12 = 242774.57
		{"T25 opr ×4", 25, nil, tc(25, "0", "D"), "323699"}, // ×16 = 323699.42
		{"T30 opr ×3", 30, nil, tc(30, "0", "D"), "242775"},
		{"T31 opr ×4", 31, nil, tc(31, "0", "D"), "323699"},
		{"T29 exec → NULL (D11)", 29, exec, tc(29, "0", "D"), ""},
		{"T31 exec → NULL (D11)", 31, exec, tc(31, "0", "D"), ""},
		// JAKARTA uses T1 only, even with T7/T55 present
		{"T20 JAKARTA opr T1 rate", 20, func(in *Input) {
			in.Division = divJakarta
			in.Additions = map[CompKey][]Addition{{CompT, 20}: {{Amount: d("10")}}}
		}, tc(20, "0", "D"), "173410"}, // 3000000/173×10 = 173410.40
		{"T22 JAKARTA opr T1 rate", 22, func(in *Input) { in.Division = divJakarta }, tc(22, "0", "D"), "138728"}, // 3000000/173×8 = 138728.32
		{"T6 0.075 fail 5 → 0", 6, otiw("5"), tc(6, "0.075", "D"), "0"},
		{"T6 without T1/T7/T55 → NULL", 6, func(in *Input) { in.Components = []EmpComponent{tc(6, "0.0877", "D")} }, tc(6, "0.0877", "D"), ""},
		{"T36 without T1/T7/T55 → NULL", 36, func(in *Input) { in.Components = []EmpComponent{tc(36, "0.0127", "D")} }, tc(36, "0.0127", "D"), ""},
		{"T136 without T1/T7/T55 → NULL", 136, func(in *Input) {
			in.Components = []EmpComponent{tc(136, "0.0127", "D")}
			in.AttAdd = AttRekapAdd{"pay_tot_add": d("5")}
		}, tc(136, "0.0127", "D"), ""},
		{"T8 no recap with WD → A", 8, func(in *Input) { in.Att = nil; in.Workdays = intp(26) }, tc(8, "520000", "D"), "520000"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := otInput(cloneRekap(att), c.comp)
			if c.mod != nil {
				c.mod(&in)
			}
			checkAmt(t, c.name, tLine(t, in, c.id), c.want)
		})
	}
}

func cloneRekap(r AttRekap) AttRekap {
	out := make(AttRekap, len(r))
	for k, v := range r {
		out[k] = v
	}
	return out
}
