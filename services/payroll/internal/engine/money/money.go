// Package money provides Oracle-compatible numeric helpers for the payroll engine.
//
// All payroll money uses shopspring/decimal (never float). Rounding mimics Oracle
// ROUND(x, n): half away from zero, including negative n (D-07, spec 06§1).
// Division keeps 38 significant digits like Oracle NUMBER, without intermediate
// rounding to fixed decimal places, so results only get rounded at the end of a
// component.
//
// This package must not touch decimal.DivisionPrecision or any other global state.
package money

import (
	"errors"

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
	// Try the higher position first; if the quotient turns out smaller, redo with one
	// more place. Each attempt rounds exactly once, so there is no double rounding.
	hi := msdPos(a) - msdPos(b) + 1
	q := a.DivRound(b, SigDigits-hi)
	if msdPos(q) < hi {
		q = a.DivRound(b, SigDigits-hi+1)
	}
	return q, nil
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
