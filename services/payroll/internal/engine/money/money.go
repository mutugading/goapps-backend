// Package money provides Oracle-compatible numeric helpers for the payroll engine.
//
// All payroll money uses shopspring/decimal (never float). Rounding mimics Oracle
// ROUND(x, n): half away from zero, including negative n (D-07, spec 06§1).
// Div, Mul, Add and Sub round every result to Oracle NUMBER precision (20 base-100
// mantissa digits, 38-40 significant decimal digits, see Number), exactly like each
// arithmetic operation in the legacy PL/SQL (D-48). There is no rounding to fixed
// decimal places in between; components only get ROUNDed at the end.
//
// Compare values with Equal/Cmp, never with == or Exponent: decimals that are equal
// in value can carry different exponents (D-13).
//
// This package must not touch decimal.DivisionPrecision or any other global state.
package money

import (
	"errors"
	"math/big"

	"github.com/shopspring/decimal"
)

// SigDigits is the guaranteed number of significant decimal digits of Oracle NUMBER.
// The real mantissa is MantissaDigits base-100 digits, so 38 to 40 decimal digits
// survive depending on where the decimal point falls (see Number).
const SigDigits int32 = 38

// MantissaDigits is the Oracle NUMBER mantissa length in base-100 digits.
const MantissaDigits int32 = 20

// ErrDivByZero mirrors Oracle ORA-01476 (divisor is equal to zero).
var ErrDivByZero = errors.New("money: division by zero")

// Zero is a convenience zero value.
var Zero = decimal.Zero

// Round is Oracle ROUND(d, places): half away from zero.
// Negative places round left of the decimal point, e.g. Round(1234.5, -2) = 1200.
func Round(d decimal.Decimal, places int32) decimal.Decimal {
	return d.Round(places)
}

// Trunc is Oracle TRUNC(d, places): cut toward zero. Negative places are allowed,
// e.g. Trunc(12999, -3) = 12000 and Trunc(-12999, -3) = -12000.
func Trunc(d decimal.Decimal, places int32) decimal.Decimal {
	return d.Shift(places).Truncate(0).Shift(-places)
}

// Div is Oracle a / b: the exact quotient rounded to NUMBER precision (half away from
// zero on the last kept digit). It returns ErrDivByZero when b is zero.
func Div(a, b decimal.Decimal) (decimal.Decimal, error) {
	if b.IsZero() {
		return decimal.Zero, ErrDivByZero
	}
	if a.IsZero() {
		return decimal.Zero, nil
	}
	// The quotient's most significant digit sits at msd(a)-msd(b) or one above it.
	// Decide which one exactly (|a| >= |b|*10^(hi-1)) before dividing, so DivRound
	// rounds once at the last kept digit. Guessing from a rounded quotient is wrong
	// when rounding carries over a power of ten (0.99..95 -> 1).
	hi := msdPos(a) - msdPos(b) + 1
	if a.Abs().LessThan(b.Abs().Shift(hi - 1)) {
		hi--
	}
	return normalize(a.DivRound(b, numberPlaces(hi))), nil
}

// Mul is Oracle a * b: the exact product rounded to NUMBER precision.
func Mul(a, b decimal.Decimal) decimal.Decimal { return Number(a.Mul(b)) }

// Add is Oracle a + b: the exact sum rounded to NUMBER precision.
func Add(a, b decimal.Decimal) decimal.Decimal { return Number(a.Add(b)) }

// Sub is Oracle a - b: the exact difference rounded to NUMBER precision.
func Sub(a, b decimal.Decimal) decimal.Decimal { return Number(a.Sub(b)) }

// Number rounds an exact value to Oracle NUMBER precision: MantissaDigits base-100
// digits aligned on the decimal point, half away from zero. 1/3 keeps 40 decimals
// (0.33..3), 10/3 keeps 39 significant digits (3.33..3), 2022610/30 keeps 38.
// Values that already fit are returned unchanged (normalized, D-13).
func Number(d decimal.Decimal) decimal.Decimal {
	if d.IsZero() {
		return decimal.Zero
	}
	return normalize(d.Round(numberPlaces(msdPos(d))))
}

// numberPlaces is the count of fractional decimal places Oracle NUMBER keeps for a
// value whose most significant digit sits at position p (see msdPos): the mantissa
// starts at the base-100 digit holding that digit, so places = 2*(20 - ceil(p/2)).
func numberPlaces(p int32) int32 {
	e := p / 2 // Go truncates toward zero; this is ceil(p/2) for p <= 0
	if p > 0 && p%2 != 0 {
		e++
	}
	return 2 * (MantissaDigits - e)
}

// MustDiv is Div for divisors that are known non-zero (constants such as DAYS_MONTH).
// It panics on zero divisors; use Div for data-driven divisors.
func MustDiv(a, b decimal.Decimal) decimal.Decimal {
	q, err := Div(a, b)
	if err != nil {
		panic(err)
	}
	return q
}

// RoundSig rounds d to n significant digits (half away from zero).
func RoundSig(d decimal.Decimal, n int32) decimal.Decimal {
	if d.IsZero() || n <= 0 {
		return d
	}
	return d.Round(n - msdPos(d))
}

// msdPos returns the power-of-ten position just above the most significant digit:
// 1234.5 -> 4, 0.00123 -> -2.
// Coefficient digit counts of payroll values are far below the int32 range.
func msdPos(d decimal.Decimal) int32 {
	return int32(d.NumDigits()) + d.Exponent() //nolint:gosec // digit count is small
}

// normalize drops trailing fractional zeros so results have one canonical form
// (Div(10, 4) is 2.5, not 2.5000...), see D-13. Integer digits are left untouched.
func normalize(d decimal.Decimal) decimal.Decimal {
	if d.Exponent() >= 0 {
		return d
	}
	c := d.Coefficient()
	exp := d.Exponent()
	ten := big.NewInt(10)
	var q, r big.Int
	for exp < 0 {
		q.QuoRem(c, ten, &r)
		if r.Sign() != 0 {
			break
		}
		c.Set(&q)
		exp++
	}
	return decimal.NewFromBigInt(c, exp)
}

// Sum adds all values left to right with Add, like a chain of + in the legacy
// PL/SQL (each step rounded to NUMBER precision, D-48); Sum() is zero.
func Sum(vs ...decimal.Decimal) decimal.Decimal {
	s := decimal.Zero
	for _, v := range vs {
		s = Add(s, v)
	}
	return s
}

// Parse parses an exact decimal string (as stored in numeric columns or proto strings).
func Parse(s string) (decimal.Decimal, error) {
	return decimal.NewFromString(s)
}
