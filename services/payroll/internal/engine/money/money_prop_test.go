package money

import (
	"math/big"
	"math/rand"
	"testing"
	"testing/quick"

	"github.com/shopspring/decimal"
)

// randDec builds a decimal with up to 18 digits and an exponent in [-12, 12],
// covering tiny fractions up to amounts far beyond payroll range.
func randDec(r *rand.Rand) decimal.Decimal {
	coef := r.Int63n(1_000_000_000_000_000_000)
	if r.Intn(2) == 0 {
		coef = -coef
	}
	return decimal.New(coef, int32(r.Intn(25)-12))
}

func quickCfg() *quick.Config {
	return &quick.Config{MaxCount: 5000, Rand: rand.New(rand.NewSource(20261014))}
}

func TestRound_Property_SymmetricAndBounded(t *testing.T) {
	half := decimal.New(5, -1)
	f := func(seed int64, n int8) bool {
		x := randDec(rand.New(rand.NewSource(seed)))
		places := int32(n%8) - 3 // [-10, 4]
		got := Round(x, places)
		if !Round(x.Neg(), places).Equal(got.Neg()) {
			return false
		}
		// |Round(x) - x| <= 0.5 * 10^-places
		return got.Sub(x).Abs().LessThanOrEqual(half.Shift(-places))
	}
	if err := quick.Check(f, quickCfg()); err != nil {
		t.Fatal(err)
	}
}

func TestTrunc_Property_TowardZero(t *testing.T) {
	f := func(seed int64, n int8) bool {
		x := randDec(rand.New(rand.NewSource(seed)))
		places := int32(n%8) - 3
		got := Trunc(x, places)
		if got.Abs().GreaterThan(x.Abs()) {
			return false
		}
		if !got.IsZero() && got.Sign() != x.Sign() {
			return false
		}
		// |x - Trunc(x)| < 10^-places
		return x.Sub(got).Abs().LessThan(decimal.New(1, -places))
	}
	if err := quick.Check(f, quickCfg()); err != nil {
		t.Fatal(err)
	}
}

// TestDiv_Property_MatchesExactQuotient checks Div against the exact rational a/b:
// at most SigDigits significant digits, error within half a unit in the 38th digit,
// and no trailing fractional zeros.
func TestDiv_Property_MatchesExactQuotient(t *testing.T) {
	f := func(seedA, seedB int64) bool {
		a := randDec(rand.New(rand.NewSource(seedA)))
		b := randDec(rand.New(rand.NewSource(seedB)))
		if b.IsZero() {
			return true
		}
		q, err := Div(a, b)
		if err != nil {
			return false
		}
		return checkQuotient(t, a, b, q)
	}
	if err := quick.Check(f, quickCfg()); err != nil {
		t.Fatal(err)
	}
}

func checkQuotient(t *testing.T, a, b, q decimal.Decimal) bool {
	t.Helper()
	if a.IsZero() {
		return q.IsZero()
	}
	if int32(q.NumDigits()) > SigDigits { //nolint:gosec // test only
		t.Logf("Div(%s, %s) = %s has %d digits", a, b, q, q.NumDigits())
		return false
	}
	if q.Exponent() < 0 && new(big.Int).Rem(q.Coefficient(), big.NewInt(10)).Sign() == 0 {
		t.Logf("Div(%s, %s) = %s keeps trailing zeros", a, b, q)
		return false
	}
	exact := new(big.Rat).Quo(a.Rat(), b.Rat())
	diff := new(big.Rat).Abs(new(big.Rat).Sub(q.Rat(), exact))
	// Half ulp of the 38th significant digit of the exact quotient (msd taken from q,
	// which is within one ulp of the exact value, or from q/10 after a carry).
	halfUlp := decimal.New(5, msdPos(q)-SigDigits-1).Rat()
	if diff.Cmp(halfUlp) > 0 {
		t.Logf("Div(%s, %s) = %s, off by %s", a, b, q, diff.FloatString(50))
		return false
	}
	return true
}
