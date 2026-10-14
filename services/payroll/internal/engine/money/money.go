// Package money provides Oracle-compatible numeric helpers for the payroll engine.
//
// All payroll money uses shopspring/decimal (never float). Rounding mimics Oracle
// ROUND(x, n): half away from zero, including negative n (D-07, spec 06§1).
// Division keeps 38 significant digits like Oracle NUMBER, without intermediate
// rounding to fixed decimal places, so results only get rounded at the end of a
// component.
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

// SigDigits is the number of significant digits kept by Div, matching Oracle NUMBER.
const SigDigits int32 = 38

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

// Div divides a by b keeping SigDigits significant digits (half away from zero on the
// last digit). It returns ErrDivByZero when b is zero.
func Div(a, b decimal.Decimal) (decimal.Decimal, error) {
	if b.IsZero() {
		return decimal.Zero, ErrDivByZero
	}
	if a.IsZero() {
		return decimal.Zero, nil
	}
	// The quotient's most significant digit sits at msd(a)-msd(b) or one above it.
	// Decide which one exactly (|a| >= |b|*10^(hi-1)) before dividing, so DivRound
	// rounds once at the 38th significant digit. Guessing from a rounded quotient is
	// wrong when rounding carries over a power of ten (0.99..95 -> 1).
	hi := msdPos(a) - msdPos(b) + 1
	if a.Abs().LessThan(b.Abs().Shift(hi - 1)) {
		hi--
	}
	return normalize(a.DivRound(b, SigDigits-hi)), nil
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

// normalize drops trailing fractional zeros so Div results have one canonical form
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

// Sum adds all values; Sum() is zero.
func Sum(vs ...decimal.Decimal) decimal.Decimal {
	s := decimal.Zero
	for _, v := range vs {
		s = s.Add(v)
	}
	return s
}

// Parse parses an exact decimal string (as stored in numeric columns or proto strings).
func Parse(s string) (decimal.Decimal, error) {
	return decimal.NewFromString(s)
}
