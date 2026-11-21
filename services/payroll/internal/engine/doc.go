// Package engine is the pure payroll calculation core (spec 03§2.1, 06).
//
// Rules for this package and its sub-packages (D-03):
//   - import only the standard library, shopspring/decimal and expr-lang/expr;
//   - no I/O, no time.Now(), no global mutable state: time and data come in via Input;
//   - money is decimal.Decimal, never float; rounding goes through engine/money (D-07);
//   - output must be deterministic: lines are ordered by (type, id), never by map iteration.
package engine
