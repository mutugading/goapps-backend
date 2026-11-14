package money

import (
	"bufio"
	"os"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

// TestOracleOps_Golden replays testdata/oracle_ops.psv: 500 random operand sets
// evaluated by Oracle 11.2 PL/SQL NUMBER on 2026-11-05 (T018c); the first 400 are
// payroll-shaped positives, the last 100 add negatives and very small/large values. Columns:
// a|b|c|p|d|a/b|a/b*c|a/b*c*p|a*c/b|a/b*c+d/b*p|a/b/2. Every value must match exactly,
// not just after ROUND, so each operation reproduces Oracle precision (D-48).
func TestOracleOps_Golden(t *testing.T) {
	f, err := os.Open("testdata/oracle_ops.psv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	rows := 0
	for sc.Scan() {
		x := strings.Split(sc.Text(), "|")
		if len(x) != 11 {
			t.Fatalf("line %d: %d columns", rows+1, len(x))
		}
		rows++
		v := make([]decimal.Decimal, len(x))
		for i, s := range x {
			v[i] = decimal.RequireFromString(s)
		}
		a, b, c, p, dd := v[0], v[1], v[2], v[3], v[4]
		q := MustDiv(a, b)
		got := []decimal.Decimal{
			q,
			Mul(q, c),
			Mul(Mul(q, c), p),
			MustDiv(Mul(a, c), b),
			Add(Mul(q, c), Mul(MustDiv(dd, b), p)),
			MustDiv(q, decimal.NewFromInt(2)),
		}
		for i, g := range got {
			if want := v[5+i]; !g.Equal(want) {
				t.Errorf("line %d op %d (%s): got %s, Oracle %s", rows, i+1, strings.Join(x[:5], ","), g, want)
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if rows != 500 {
		t.Fatalf("rows = %d, want 500", rows)
	}
}

// TestMul_Dirumahkan is the T019f SOLO 202610 case (T101/T155): legacy
// 2022610/30*3*0.5 is 101130.5 in Oracle (202260.99..99 rounds up to 202261 at the
// 40th digit), so ROUND gives 101131; exact multiplication gave 101130.
func TestMul_Dirumahkan(t *testing.T) {
	got := Round(Mul(Mul(MustDiv(d("2022610"), d("30")), d("3")), d("0.5")), 0)
	if want := d("101131"); !got.Equal(want) {
		t.Fatalf("got %s, want %s", got, want)
	}
}

// TestSum_Oracle: Oracle TO_CHAR(1/3+1/7+100000+2/3), each + rounded to NUMBER.
func TestSum_Oracle(t *testing.T) {
	got := Sum(MustDiv(d("1"), d("3")), MustDiv(d("1"), d("7")), d("100000"), MustDiv(d("2"), d("3")))
	if want := d("100001.1428571428571428571428571428571429"); !got.Equal(want) {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestNumber_Layout(t *testing.T) {
	cases := []struct{ in, want string }{
		{"0", "0"},
		{"202260.99999999999999999999999999999999999", "202261"},
		{"0.33333333333333333333333333333333333333333", "0.3333333333333333333333333333333333333333"},
		{"3.33333333333333333333333333333333333333333", "3.33333333333333333333333333333333333333"},
		{"-3.333333333333333333333333333333333333335", "-3.33333333333333333333333333333333333334"},
		{"12.50", "12.5"},
		// carry across a base-100 pair boundary, Oracle TO_CHAR(x*1) on 2026-11-05
		{"99.99999999999999999999999999999999999999999", "100"},
		{"9.999999999999999999999999999999999999999999", "10"},
		{"0.99999999999999999999999999999999999999999999", "1"},
		{"-99.99999999999999999999999999999999999999999", "-100"},
		{"1234567", "1234567"},
	}
	for _, c := range cases {
		if got := Number(d(c.in)); got.String() != d(c.want).String() {
			t.Errorf("Number(%s) = %s, want %s", c.in, got, c.want)
		}
	}
}
