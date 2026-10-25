package engine

import (
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

type rules struct{ legacy bool }

func (r rules) ID() int            { return 1 }
func (r rules) LegacyCompat() bool { return r.legacy }
func (rules) Constant(k string) (decimal.Decimal, bool) {
	switch k {
	case ConstDaysMonth:
		return decimal.NewFromInt(30), true
	case ConstHoursMonth:
		return decimal.NewFromInt(173), true
	}
	return decimal.Zero, false
}

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func tc(id int, amt, sts string) EmpComponent {
	return EmpComponent{Key: CompKey{CompT, id}, Amount: d(amt), Sts: sts}
}

func baseInput(div, level, sts string, comps ...EmpComponent) Input {
	return Input{
		Period:     Period{YYMM: 202610, Start: time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)},
		Division:   div,
		Emp:        Employee{NIK: "1", Level: level, GapokSts: sts},
		Components: comps,
		Att:        AttRekap{"pay_tot": d("26")},
		Rules:      rules{legacy: true},
	}
}

func TestBasis(t *testing.T) {
	comps := []EmpComponent{
		tc(1, "3000000", "F"), tc(2, "300000", "F"), tc(3, "150000", "F"), tc(4, "90000", "F"),
		tc(7, "600000", "F"), tc(9, "60000", "F"), tc(55, "30000", "F"), tc(901, "4000000", "F"),
		tc(5, "999", "F"), // not part of TETAP
	}
	cases := []struct {
		name                      string
		div, level, sts           string
		gapok, tetap, tetapD, jan string
		rateOT                    string // exact 38-digit value computed via Div
	}{
		// TETAP_D is always prorated (D-25): 4230000/30*26 = 3666000.
		// Others: (T1+T7+T55)/173 = 3630000/173
		{"solo staff F", "SOLO", "staff", "F", "3000000", "4230000", "3666000", "4000000", "20982.658959537572254335260115606936416185"},
		// JAKARTA: T1/173 = 3000000/173
		{"jakarta", "JAKARTA", "staff", "F", "3000000", "4230000", "3666000", "4000000", "17341.040462427745664739884393063583815029"},
		// Executive: (T1+T2+T3+T4)/173 = 3540000/173
		{"executive", "SOLO", "Executive", "F", "3000000", "4230000", "3666000", "4000000", "20462.427745664739884393063583815028901734"},
		// D: each / 30 × 26 → 4230000/30*26 = 3666000
		{"solo staff D", "SOLO", "staff", "D", "3000000", "4230000", "3666000", "4000000", "20982.658959537572254335260115606936416185"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cs := append([]EmpComponent(nil), comps...)
			cs[0].Sts = c.sts // STS(T,1) decides TETAP_D
			in := baseInput(c.div, c.level, c.sts, cs...)
			b, err := computeBasis(in)
			if err != nil {
				t.Fatal(err)
			}
			eq := func(name string, got decimal.Decimal, want string) {
				if !got.Equal(d(want)) {
					t.Errorf("%s = %s, want %s", name, got, want)
				}
			}
			eq("gapok", b.Gapok, c.gapok)
			eq("tetap", b.Tetap, c.tetap)
			eq("janBase", b.JanBase, c.jan)
			// TETAP_D tolerance: legacy order is divide then multiply; allow 1e-30 drift.
			if diff := b.TetapD.Sub(d(c.tetapD)).Abs(); diff.GreaterThan(d("0.000000000000000000000000001")) {
				t.Errorf("tetapD = %s, want %s", b.TetapD, c.tetapD)
			}
			if diff := b.RateOT.Sub(d(c.rateOT)).Abs(); diff.GreaterThan(d("0.0000000000000000001")) {
				t.Errorf("rateOT = %s, want %s", b.RateOT, c.rateOT)
			}
			if b.DaysPeriod != 30 {
				t.Errorf("daysPeriod = %d, want 30", b.DaysPeriod)
			}
		})
	}
}

func TestDaysPeriod(t *testing.T) {
	cases := []struct {
		s, e time.Time
		want int
	}{
		{time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC), 30},
		{time.Date(2026, 1, 26, 0, 0, 0, 0, time.UTC), time.Date(2026, 2, 25, 0, 0, 0, 0, time.UTC), 31},
		{time.Date(2026, 2, 26, 0, 0, 0, 0, time.UTC), time.Date(2026, 3, 25, 0, 0, 0, 0, time.UTC), 28},
		{time.Time{}, time.Time{}, 0},
	}
	for _, c := range cases {
		if got := daysBetween(c.s, c.e); got != c.want {
			t.Errorf("daysBetween(%v,%v) = %d, want %d", c.s, c.e, got, c.want)
		}
	}
}

func TestMissingConstant(t *testing.T) {
	in := baseInput("SOLO", "staff", "F", tc(1, "1", "F"))
	in.Rules = noConst{}
	if _, err := Calculate(in); !errors.Is(err, ErrMissingConstant) {
		t.Fatalf("err = %v, want ErrMissingConstant", err)
	}
}

type noConst struct{}

func (noConst) ID() int                                 { return 1 }
func (noConst) LegacyCompat() bool                      { return true }
func (noConst) Constant(string) (decimal.Decimal, bool) { return decimal.Zero, false }

// Golden mini: fixed + prorate lines, header rounding to -2.
func TestCalculateGoldenMini(t *testing.T) {
	in := baseInput("SOLO", "staff", "D",
		tc(2, "300000", "F"),
		tc(1, "3000100", "D"), // 3000100/30*26 = 2600086.666… → 2600087
		EmpComponent{Key: CompKey{CompP, 24}, Amount: d("12345"), Sts: "F"},
	)
	res, err := Calculate(in)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		key CompKey
		amt string
	}{{CompKey{CompT, 1}, "2600087"}, {CompKey{CompT, 2}, "300000"}, {CompKey{CompP, 24}, "12345"}}
	if len(res.Lines) != len(want) {
		t.Fatalf("lines = %d", len(res.Lines))
	}
	for i, w := range want {
		l := res.Lines[i]
		if l.Key != w.key || l.Amount == nil || !l.Amount.Equal(d(w.amt)) {
			t.Errorf("line %d = %v %v, want %v %s", i, l.Key, l.Amount, w.key, w.amt)
		}
	}
	// totT=2900087 totP=12345 net=2887742 → take_home 2887700, rounding 42.
	h := res.Header
	if h == nil {
		t.Fatal("header nil")
	}
	for name, p := range map[string][2]string{
		"totT": {h.TotTunjangan.String(), "2900087"}, "totP": {h.TotPotongan.String(), "12345"},
		"takeHome": {h.TakeHome.String(), "2887700"}, "rounding": {h.Rounding.String(), "42"},
	} {
		if p[0] != p[1] {
			t.Errorf("%s = %s, want %s", name, p[0], p[1])
		}
	}
}

func TestCalculateDeterministicAndOrder(t *testing.T) {
	comps := []EmpComponent{
		{Key: CompKey{CompP, 1}, Amount: d("1")}, {Key: CompKey{CompP, 24}, Amount: d("5")},
		tc(7, "7", "F"), tc(1, "10", "F"), {Key: CompKey{CompP, 5}, Amount: d("1")},
		{Key: CompKey{CompP, 0}, Amount: d("1")}, tc(2, "2", "F"),
	}
	in := baseInput("SOLO", "staff", "F", comps...)
	first, err := Calculate(in)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		// reverse input order each time
		rev := append([]EmpComponent(nil), comps...)
		for a, b := 0, len(rev)-1; a < b; a, b = a+1, b-1 {
			rev[a], rev[b] = rev[b], rev[a]
		}
		in.Components = rev
		got, err := Calculate(in)
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Lines) != len(first.Lines) {
			t.Fatalf("len differ")
		}
		for j := range got.Lines {
			if got.Lines[j].Key != first.Lines[j].Key {
				t.Fatalf("order differs at %d", j)
			}
		}
	}
	for j := 1; j < len(first.Lines); j++ {
		if !first.Lines[j-1].Key.Less(first.Lines[j].Key) {
			t.Errorf("not sorted at %d: %v", j, first.Lines)
		}
	}
	// P1 has no taxable line → NULL; P5/P0 skipped (no formula yet): 2 warnings, incomplete.
	if len(first.Warnings) != 2 || !first.Incomplete {
		t.Errorf("warnings = %v", first.Warnings)
	}
}

// pphProbe records evaluation order to verify T → P non-PPh → P1 → P5 → P0.
type probe struct {
	code string
	log  *[]CompKey
}

func (p probe) Code() string { return p.code }
func (p probe) Eval(_ *EvalCtx, c EmpComponent, _ Params) (*decimal.Decimal, string, error) {
	*p.log = append(*p.log, c.Key)
	a := c.Amount
	return &a, "", nil
}

func TestEvaluationOrder(t *testing.T) {
	var log []CompKey
	reg, err := NewRegistry(probe{FormulaFixedOrProrate, &log}, probe{FormulaPPh21, &log},
		probe{FormulaPPhCompany, &log}, probe{FormulaPPhSubsidy, &log}, probe{FormulaAbsen, &log})
	if err != nil {
		t.Fatal(err)
	}
	in := baseInput("SOLO", "staff", "F",
		EmpComponent{Key: CompKey{CompP, 0}, Amount: d("1")}, EmpComponent{Key: CompKey{CompP, 5}, Amount: d("1")},
		EmpComponent{Key: CompKey{CompP, 1}, Amount: d("1")}, EmpComponent{Key: CompKey{CompP, 4}, Amount: d("1")},
		tc(2, "1", "F"), tc(1, "1", "F"),
	)
	if _, err := CalculateWith(reg, in); err != nil {
		t.Fatal(err)
	}
	want := []CompKey{{CompT, 1}, {CompT, 2}, {CompP, 4}, {CompP, 1}, {CompP, 5}, {CompP, 0}}
	if len(log) != len(want) {
		t.Fatalf("log = %v", log)
	}
	for i := range want {
		if log[i] != want[i] {
			t.Fatalf("order = %v, want %v", log, want)
		}
	}
}

func TestHeaderPresence(t *testing.T) {
	in := baseInput("SOLO", "staff", "F", tc(1, "100", "F"))
	in.Att = nil
	if res, err := Calculate(in); err != nil || res.Header != nil || len(res.Lines) != 1 {
		t.Fatalf("legacy no rekap: header=%v lines=%d err=%v", res.Header, len(res.Lines), err)
	}
	in.Att = AttRekap{}
	if res, _ := Calculate(in); res.Header == nil {
		t.Fatal("empty non-nil rekap must get header")
	}
	in.Att = nil
	in.Rules = rules{legacy: false}
	if res, _ := Calculate(in); res.Header == nil {
		t.Fatal("v2 always gets header")
	}
}

func TestHeaderBirthday(t *testing.T) {
	bd := time.Date(1990, 10, 27, 0, 0, 0, 0, time.UTC) // window of 202610 is 26/10–25/11
	in := baseInput("SOLO", "staff", "F", tc(1, "100", "F"))
	in.Emp.BirthDate = &bd
	res, _ := Calculate(in)
	want := "Working : 0 Days, Off : 0 Days, Leave : 0 Days, Alpa : 0 Days, Ijin : 0 Days, Total : 26 Days" +
		"\n\nSelamat Ulang Tahun Ke 36 Tanggal 27 Oktober 2026"
	if res.Header.Remarks != want {
		t.Errorf("remarks = %q", res.Header.Remarks)
	}
}

func TestNullAmountTreatedAsZero(t *testing.T) {
	l := Line{}
	if !l.AmountOrZero().IsZero() {
		t.Fatal("nil amount must be zero")
	}
}

func TestJakartaBeatsExecutiveAndAcceptsJKT(t *testing.T) {
	comps := []EmpComponent{tc(1, "1730000", "F"), tc(2, "1730000", "F")}
	for _, div := range []string{"JAKARTA", "JKT", "jkt"} {
		b, err := computeBasis(baseInput(div, "Executive", "F", comps...))
		if err != nil {
			t.Fatal(err)
		}
		if !b.RateOT.Equal(d("10000")) { // T1/173 only
			t.Errorf("%s: rateOT = %s, want 10000", div, b.RateOT)
		}
	}
}

func TestTetapDLegacy(t *testing.T) {
	// Legacy V_GPTUNJTETAP_D: TETAP/30 × pay_tot + dirumahkan × TETAP/30/2, whatever STS(T,1) is.
	in := baseInput("SOLO", "staff", "F", tc(1, "3000000", "F"), tc(2, "300000", "F"), tc(901, "600000", "F"))
	in.Att = AttRekap{"pay_tot": d("20"), "dirumahkan": d("6")}
	b, err := computeBasis(in)
	if err != nil {
		t.Fatal(err)
	}
	// 3300000/30*20 + 6*3300000/30/2 = 2200000 + 330000; JAN: 600000/30*20 + 6*600000/30/2 = 460000
	if !b.TetapD.Equal(d("2530000")) || !b.JanBaseD.Equal(d("460000")) {
		t.Errorf("tetapD = %s janBaseD = %s, want 2530000 460000", b.TetapD, b.JanBaseD)
	}
	// Without T1 the legacy recap join yields no row: recap columns are NULL → TETAP_D 0.
	in.Components = []EmpComponent{tc(2, "300000", "F")}
	if b, _ = computeBasis(in); !b.TetapD.IsZero() || b.hasRekap {
		t.Errorf("no T1: tetapD = %s hasRekap = %v, want 0 false", b.TetapD, b.hasRekap)
	}
}

func TestHeaderRoundingHalfAwayFromZero(t *testing.T) {
	for _, c := range []struct{ t, p, th, rnd string }{
		{"1250", "0", "1300", "-50"},
		{"0", "1250", "-1300", "50"},
		{"1249", "0", "1200", "49"},
	} {
		h := buildHeader(baseInput("SOLO", "staff", "F"), Basis{}, []Line{
			{Key: CompKey{CompT, 1}, Amount: ptr(d(c.t))}, {Key: CompKey{CompP, 24}, Amount: ptr(d(c.p))},
		})
		if !h.TakeHome.Equal(d(c.th)) || !h.Rounding.Equal(d(c.rnd)) {
			t.Errorf("t=%s p=%s: takeHome=%s rounding=%s, want %s %s", c.t, c.p, h.TakeHome, h.Rounding, c.th, c.rnd)
		}
	}
}

func ptr(v decimal.Decimal) *decimal.Decimal { return &v }
