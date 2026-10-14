package money

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// Expected values follow Oracle ROUND/TRUNC semantics (half away from zero).
func TestRound_Oracle(t *testing.T) {
	cases := []struct {
		in     string
		places int32
		want   string
	}{
		{"2.5", 0, "3"},
		{"-2.5", 0, "-3"},
		{"1.5", 0, "2"},
		{"0.5", 0, "1"},
		{"-0.5", 0, "-1"},
		{"2.4999999999", 0, "2"},
		{"-2.4999999999", 0, "-2"},
		{"1234.5", -2, "1200"},
		{"1250", -2, "1300"},
		{"-1250", -2, "-1300"},
		{"1249.99", -2, "1200"},
		{"49", -2, "0"},
		{"50", -2, "100"},
		{"-50", -2, "-100"},
		{"1234567.891", -3, "1235000"},
		{"1.005", 2, "1.01"},
		{"-1.005", 2, "-1.01"},
		{"123.456", 1, "123.5"},
		{"0", -2, "0"},
	}
	for _, c := range cases {
		got := Round(d(c.in), c.places)
		if !got.Equal(d(c.want)) {
			t.Errorf("Round(%s, %d) = %s, want %s", c.in, c.places, got, c.want)
		}
	}
}

func TestRound_NotBanker(t *testing.T) {
	// Banker's rounding would give 2 and 4; Oracle gives 3 and 5.
	for in, want := range map[string]string{"2.5": "3", "4.5": "5", "-4.5": "-5"} {
		if got := Round(d(in), 0); !got.Equal(d(want)) {
			t.Errorf("Round(%s) = %s, want %s", in, got, want)
		}
	}
}

func TestTrunc_Oracle(t *testing.T) {
	cases := []struct {
		in     string
		places int32
		want   string
	}{
		{"12999", -3, "12000"},
		{"-12999", -3, "-12000"},
		{"999", -3, "0"},
		{"2.99", 0, "2"},
		{"-2.99", 0, "-2"},
		{"1.239", 2, "1.23"},
		{"-1.239", 2, "-1.23"},
	}
	for _, c := range cases {
		got := Trunc(d(c.in), c.places)
		if !got.Equal(d(c.want)) {
			t.Errorf("Trunc(%s, %d) = %s, want %s", c.in, c.places, got, c.want)
		}
	}
}

func TestDiv_Precision(t *testing.T) {
	cases := []struct{ a, b, want string }{
		{"10", "4", "2.5"},
		{"1", "3", "0.33333333333333333333333333333333333333"},
		{"2", "3", "0.66666666666666666666666666666666666667"},
		{"-2", "3", "-0.66666666666666666666666666666666666667"},
		{"5000000", "30", "166666.66666666666666666666666666666667"},
		{"1", "7", "0.14285714285714285714285714285714285714"},
		{"9", "3", "3"},
		{"0", "3", "0"},
		{"99", "100", "0.99"},
		{"10", "4", "2.5"},
		// negative divisor and both negative
		{"2", "-3", "-0.66666666666666666666666666666666666667"},
		{"-2", "-3", "0.66666666666666666666666666666666666667"},
		// exact quotient just below 1: 38 nines, must not collapse to 1 with 37 digits
		{"1", "1.00000000000000000000000000000000000001", "0.99999999999999999999999999999999999999"},
		// quotient carries over a power of ten at the 38th digit
		{"1", "1.000000000000000000000000000000000000004", "1"},
		{"999999999999999999999999999999999999995", "1000000000000000000000000000000000000000", "1"},
		// very large and very small operands
		{"1000000000000000000000000000000", "7", "142857142857142857142857142857.14285714"},
		{"0.000000000001", "3", "0.00000000000033333333333333333333333333333333333333"},
		{"123456789012345678", "0.000000001", "123456789012345678000000000"},
	}
	for _, c := range cases {
		got, err := Div(d(c.a), d(c.b))
		if err != nil {
			t.Fatalf("Div(%s, %s): %v", c.a, c.b, err)
		}
		if got.String() != d(c.want).String() {
			t.Errorf("Div(%s, %s) = %s, want %s (exact text, D-13)", c.a, c.b, got, c.want)
		}
	}
}

func TestDiv_Zero(t *testing.T) {
	if _, err := Div(d("1"), Zero); !errors.Is(err, ErrDivByZero) {
		t.Fatalf("want ErrDivByZero, got %v", err)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("MustDiv by zero must panic")
		}
	}()
	MustDiv(d("1"), Zero)
}

// Legacy computes amount / 30 * pay_tot then ROUND(0); dividing first must not
// lose precision compared with multiplying first.
func TestDiv_ThenMultiply_LegacyOrder(t *testing.T) {
	amount, payTot := d("4750000"), d("23")
	got := Round(MustDiv(amount, d("30")).Mul(payTot), 0)
	if want := d("3641667"); !got.Equal(want) {
		t.Fatalf("prorate = %s, want %s", got, want)
	}
}

func TestRoundSig(t *testing.T) {
	cases := []struct {
		in   string
		n    int32
		want string
	}{
		{"123456", 2, "120000"},
		{"0.0012345", 3, "0.00123"},
		{"-0.0012355", 3, "-0.00124"},
		{"9.99", 2, "10"},
	}
	for _, c := range cases {
		if got := RoundSig(d(c.in), c.n); !got.Equal(d(c.want)) {
			t.Errorf("RoundSig(%s, %d) = %s, want %s", c.in, c.n, got, c.want)
		}
	}
}

func TestSum(t *testing.T) {
	if !Sum().IsZero() {
		t.Fatal("empty sum must be zero")
	}
	if got := Sum(d("0.1"), d("0.2")); !got.Equal(d("0.3")) {
		t.Fatalf("0.1+0.2 = %s", got)
	}
}
