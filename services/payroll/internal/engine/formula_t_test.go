package engine

import (
	"testing"

	"github.com/shopspring/decimal"
)

// tRules adds the T constants of 06§1/06§4 (SOLO values) to the test rule set.
type tRules struct{ rules }

func (r tRules) Constant(k string) (decimal.Decimal, bool) {
	switch k {
	case ConstBerasDays:
		return decimal.NewFromInt(25), true
	case ConstNakMin, ConstKesMin, ConstPenMin:
		return d("2592154"), true
	case ConstKesMax:
		return d("12000000"), true
	case ConstPenMax:
		return d("11086300"), true
	}
	return r.rules.Constant(k)
}

// tLine runs Calculate and returns the line of T id; amt "" means NULL is expected.
func tLine(t *testing.T, in Input, id int) Line {
	t.Helper()
	res, err := Calculate(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range res.Lines {
		if l.Key == (CompKey{CompT, id}) {
			return l
		}
	}
	t.Fatalf("no line T%03d", id)
	return Line{}
}

func checkAmt(t *testing.T, name string, l Line, want string) {
	t.Helper()
	switch {
	case want == "" && l.Amount != nil:
		t.Errorf("%s: amount = %s, want NULL", name, l.Amount)
	case want != "" && l.Amount == nil:
		t.Errorf("%s: amount = NULL, want %s", name, want)
	case want != "" && !l.Amount.Equal(d(want)):
		t.Errorf("%s: amount = %s, want %s", name, l.Amount, want)
	}
}

// tInput is a SOLO staff with T1 3.000.000 (F) and the given extra components.
func tInput(att AttRekap, comps ...EmpComponent) Input {
	in := baseInput("SOLO", "staff", "F", append([]EmpComponent{tc(1, "3000000", "F")}, comps...)...)
	in.Att = att
	in.Rules = tRules{rules{legacy: true}}
	return in
}

// Golden cases computed by hand from FNC_MGT_PAYROLL_COMPNT_CALC (D-25); 09§3 golden from DETAIL follows in B-GOLD.
func TestTFormulas(t *testing.T) {
	att := AttRekap{"pay_tot": d("26"), "day_work": d("22"), "dirumahkan": d("6"),
		"sakit_75": d("2"), "sakit_50": d("4"), "sakit_25": d("1"), "alpa": d("1"), "ijin": d("1")}
	add := func(id int, amt, rem string) map[CompKey][]Addition {
		return map[CompKey][]Addition{{CompT, id}: {{Amount: d(amt), Remark: rem}}}
	}
	cases := []struct {
		name string
		id   int
		mod  func(*Input)
		comp EmpComponent
		want string
	}{
		{"FIXED F", 2, nil, tc(2, "300000", "F"), "300000"},
		{"PRORATE D", 2, nil, tc(2, "300000", "D"), "260000"}, // 300000/30*26
		{"PRORATE D no rekap → NULL", 2, func(in *Input) { in.Att = nil }, tc(2, "300000", "D"), ""},
		{"PRORATE other STS → NULL", 2, nil, tc(2, "300000", "X"), ""},
		{"ADDITION F", 41, func(in *Input) { in.Additions = add(41, "150000", "insentif") }, tc(41, "0", "F"), "150000"},
		{"ADDITION D", 41, func(in *Input) { in.Additions = add(41, "300000", "") }, tc(41, "0", "D"), "260000"},
		{"ADDITION none → NULL", 41, nil, tc(41, "0", "F"), ""},
		{"T19 addition", 19, func(in *Input) { in.Additions = add(19, "12345", "") }, tc(19, "0", "F"), "12345"},
		// (200000 + 250000 − 250000/25×2) / 0.95 × 5/100 = 22631.578… → 22632
		{"T10 subsidi PPh", 10, func(in *Input) {
			in.Additions = add(38, "200000", "")
			in.Components = append(in.Components, tc(74, "250000", "D"))
		}, tc(10, "0", "F"), "22632"},
		{"T10 nothing → NULL", 10, nil, tc(10, "0", "F"), ""},
		{"T10 STS D → NULL", 10, func(in *Input) { in.Additions = add(38, "200000", "") }, tc(10, "0", "D"), ""},
		{"T40 F", 40, nil, tc(40, "150000", "F"), "150000"},
		{"T40 D", 40, nil, tc(40, "150000", "D"), "110000"}, // 150000/30*22
		{"T40 premi fail → NULL (G1, D10)", 40, func(in *Input) { in.Daily.PremiFail = 2 }, tc(40, "150000", "F"), ""},
		{"T74 F", 74, func(in *Input) { in.Daily.BerasFail = 3 }, tc(74, "250000", "F"), "250000"},
		{"T74 D", 74, func(in *Input) { in.Daily.BerasFail = 3 }, tc(74, "250000", "D"), "220000"}, // 250000 − 250000/25×3
		{"T101 F", 101, nil, tc(101, "3000000", "F"), "1500000"},
		{"T101 D dirumahkan", 101, nil, tc(101, "3000000", "D"), "300000"}, // 3000000/30×6×0.5
		{"T155 D own amount", 155, nil, tc(155, "300000", "D"), "30000"},
		{"T401 D sakit 75", 401, nil, tc(401, "3000000", "D"), "150000"}, // /30×2×0.75
		{"T456 D sakit 50", 456, nil, tc(456, "300000", "D"), "20000"},   // /30×4×0.5
		{"T427 D sakit 25", 427, nil, tc(427, "600000", "D"), "5000"},    // /30×1×0.25
		{"T411 F", 411, nil, tc(411, "3000000", "F"), "1500000"},
		{"T201 F", 201, nil, tc(201, "3000000", "F"), "1500000"},
		{"T201 D NW", 201, func(in *Input) { in.Sch = SchRekap{"NW": d("3")} }, tc(201, "3000000", "D"), "150000"},
		{"T201 D no schedule → NULL", 201, nil, tc(201, "3000000", "D"), ""},
		{"T301 D NWS", 301, func(in *Input) { in.Sch = SchRekap{"NW": d("9"), "NWS": d("2")} }, tc(301, "3000000", "D"), "100000"},
		// BPJS: TETAP = 3000000 (T1 only); TETAP_D = 3000000/30×26 + 6×3000000/30/2 = 2900000
		{"T11 naker F", 11, nil, tc(11, "0.0024", "F"), "7200"},
		{"T12 naker D", 12, nil, tc(12, "0.0030", "D"), "8700"},
		{"T13 naker min", 13, func(in *Input) { in.Components[0] = tc(1, "2000000", "F") }, tc(13, "0.02", "F"), "51843"}, // 2592154×0.02=51843.08
		{"T15 kes F", 15, nil, tc(15, "0.04", "F"), "120000"},
		{"T15 kes max", 15, func(in *Input) { in.Components[0] = tc(1, "15000000", "F") }, tc(15, "0.04", "F"), "480000"},
		{"T15 kes January JAN_BASE (G7)", 15, func(in *Input) {
			in.Period.YYMM = 202601
			in.Components = append(in.Components, tc(901, "13000000", "F"))
		}, tc(15, "0.04", "F"), "480000"},
		// JAN_BASE_D = 9000000/30×26 + 6×9000000/30/2 = 8700000 → ×0.04
		{"T15 kes January D JAN_BASE_D", 15, func(in *Input) {
			in.Period.YYMM = 202601
			in.Components = append(in.Components, tc(901, "9000000", "F"))
		}, tc(15, "0.04", "D"), "348000"},
		{"T15 kes min", 15, func(in *Input) { in.Components[0] = tc(1, "1000000", "F") }, tc(15, "0.04", "F"), "103686"}, // 2592154×0.04=103686.16
		// Recap without T1: T10 still reads recap alpa+ijin; STS D components get NULL.
		{"T10 without T1", 10, func(in *Input) {
			in.Components = []EmpComponent{tc(10, "0", "F"), tc(74, "250000", "F")}
		}, tc(10, "0", "F"), "12105"}, // (250000 − 250000/25×2)/0.95×5/100 = 12105.26… → 12105
		{"PRORATE D without T1 → NULL", 2, func(in *Input) { in.Components = in.Components[1:] }, tc(2, "300000", "D"), ""},
		{"T15 kes January without JAN_BASE → NULL", 15, func(in *Input) { in.Period.YYMM = 202601 }, tc(15, "0.04", "F"), ""},
		{"T17 pensiun D", 17, nil, tc(17, "0.02", "D"), "58000"},
		{"T17 pensiun max", 17, func(in *Input) { in.Components[0] = tc(1, "12000000", "F") }, tc(17, "0.02", "F"), "221726"},
		{"T37 no legacy branch → NULL", 37, nil, tc(37, "500000", "F"), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := tInput(att, c.comp)
			if c.mod != nil {
				c.mod(&in)
			}
			checkAmt(t, c.name, tLine(t, in, c.id), c.want)
		})
	}
}

func TestTFormulaRemarkAddition(t *testing.T) {
	in := tInput(AttRekap{"pay_tot": d("30")}, tc(44, "0", "F"))
	in.Additions = map[CompKey][]Addition{{CompT, 44}: {{Amount: d("100"), Remark: "a"}, {Amount: d("50"), Remark: "b"}}}
	l := tLine(t, in, 44)
	checkAmt(t, "sum", l, "150")
	if l.Remark != "a; b" || l.Formula != FormulaAddition {
		t.Errorf("remark = %q formula = %q", l.Remark, l.Formula)
	}
}

func TestTSpecMapping(t *testing.T) {
	// Every T id present in spec/dump/MGT_PAYROLL_COMPNT_EMP.psv is mapped; overtime/shift waits for T011b.
	used := []int{1, 2, 3, 4, 5, 7, 9, 10, 11, 12, 13, 15, 17, 19, 32, 33, 37, 38, 40, 41, 42, 43, 44, 45,
		46, 47, 48, 49, 55, 57, 70, 71, 72, 73, 74, 75, 76, 77, 78, 101, 107, 155, 401, 405, 407, 411,
		415, 417, 421, 427, 455, 456, 457, 901}
	for _, id := range used {
		if _, ok := specFor(CompKey{CompT, id}); !ok {
			t.Errorf("T%03d unmapped", id)
		}
	}
	for _, id := range []int{6, 8, 20, 21, 31, 34, 35, 36, 134, 135, 136} {
		if _, ok := specFor(CompKey{CompT, id}); ok {
			t.Errorf("T%03d is overtime/shift (T011b) but mapped", id)
		}
	}
	for id, code := range map[int]string{106: FormulaLegacyNull, 206: FormulaLegacyNull, 406: FormulaLegacyNull,
		465: FormulaLegacyNull, 105: FormulaDirumahkan, 255: FormulaNotWorking, 425: FormulaSakit, 901: FormulaLegacyNull} {
		if s, _ := specFor(CompKey{CompT, id}); s.Code != code {
			t.Errorf("T%03d = %s, want %s", id, s.Code, code)
		}
	}
}

func TestTIncompleteOnlyForOvertime(t *testing.T) {
	in := tInput(AttRekap{"pay_tot": d("30")}, tc(37, "1", "F"), tc(6, "0.0877", "D"))
	res, err := Calculate(in)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Incomplete || len(res.Warnings) != 1 {
		t.Errorf("incomplete = %v warnings = %v, want only T006", res.Incomplete, res.Warnings)
	}
}
