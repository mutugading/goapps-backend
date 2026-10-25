package engine

import (
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/engine/money"
)

// Potongan (P) formula codes without PPh (06§4, legacy FNC_MGT_PAYROLL_COMPNT_CALC, D-27).
const (
	FormulaAbsen = "ABSEN" // P4
	FormulaLoan  = "LOAN"  // P32 principal, P33 interest: addition (legacy) + LOAN_DUE (15§2.4)
)

// Loan component ids and the formula parameter selecting the LOAN_DUE part.
const (
	idLoanPrincipal = 32
	idLoanInterest  = 33
	paramLoanPart   = "part"
	loanPrincipal   = "principal"
	loanInterest    = "interest"
)

// AdditionSourceLoan is pa_source of installments generated from a loan (migrated, 15§2.4).
const AdditionSourceLoan = "LOAN"

// divSolo is the legacy division name whose P4 divides by DAYS_MONTH (D12).
const divSolo = "SOLO"

// P formula errors.
var (
	// ErrLoanDouble rejects an employee with LOAN-sourced P32/P33 additions and LOAN_DUE at once (15§2.4).
	ErrLoanDouble = errors.New("engine: LOAN_DOUBLE: loan installment both as addition and LOAN_DUE")
	// ErrNoWorkdays mirrors legacy ORA-01476: JAKARTA P4 divides by NVL(ATTD_DAYS, 0).
	ErrNoWorkdays = errors.New("engine: P4 JAKARTA needs ATTD_DAYS (workdays) > 0")
)

// WarnLoanNetNegative is the run warning when loan deductions leave a negative net (15§2.3).
const WarnLoanNetNegative = "LOAN_NET_NEGATIVE"

func isSolo(div string) bool { return strings.EqualFold(strings.TrimSpace(div), divSolo) }

// absen is P4. F → A. D → GAPOK/DM × (alpa+ijin) for SOLO, GAPOK/WD × (alpa+ijin) for
// JAKARTA (D12). alpa+ijin exists only when the recap row exists and STS(T1) = 'F'; without
// it SOLO is NULL and JAKARTA multiplies by NVL(·, 0). Other divisions are NULL.
type absen struct{}

func (absen) Code() string { return FormulaAbsen }

func (absen) Eval(ctx *EvalCtx, c EmpComponent, _ Params) (*decimal.Decimal, string, error) {
	switch c.Sts {
	case stsFull:
		a := c.Amount
		return &a, "", nil
	case stsProrate:
	default:
		return nil, "", nil
	}
	b := ctx.Basis
	if !b.hasGapok { // NULL GAPOK: NULL / x is NULL, even for x = 0
		return nil, "", nil
	}
	var days *decimal.Decimal
	if b.hasRekap && b.GapokSts == stsFull {
		v := ctx.In.Att.Get(colAlpa).Add(ctx.In.Att.Get(colIjin))
		days = &v
	}
	var div decimal.Decimal
	switch {
	case isSolo(ctx.In.Division):
		if days == nil {
			return nil, "", nil
		}
		div = b.DaysMonth // legacy literal 30, read from DAYS_MONTH (D-25)
	case isJakarta(ctx.In.Division):
		if ctx.In.Workdays == nil || *ctx.In.Workdays == 0 {
			return nil, "", ErrNoWorkdays
		}
		div = decimal.NewFromInt(int64(*ctx.In.Workdays))
		if days == nil {
			z := decimal.Zero
			days = &z
		}
	default:
		return nil, "", nil
	}
	q, err := money.Div(b.Gapok, div)
	if err != nil {
		return nil, "", err
	}
	a := q.Mul(*days)
	return &a, fmt.Sprintf("%s / %s × %s hari alpa+ijin", b.Gapok, div, *days), nil
}

// loan is P32/P33: the legacy addition part PRO(ADD) plus the LOAN_DUE installments of the
// period (15§2.4). The LOAN_DUE part is never prorated and does not depend on STS, so a due
// installment is never dropped silently. NULL when neither part exists.
type loan struct{}

func (loan) Code() string { return FormulaLoan }

func (loan) Eval(ctx *EvalCtx, c EmpComponent, p Params) (*decimal.Decimal, string, error) {
	amt, rem, err := addition{}.Eval(ctx, c, p)
	if err != nil {
		return nil, "", err
	}
	if len(ctx.In.Loans) == 0 {
		return amt, rem, nil
	}
	sum := decimal.Zero
	rems := make([]string, 0, len(ctx.In.Loans)+1)
	if amt != nil {
		sum = *amt
		if rem != "" {
			rems = append(rems, rem)
		}
	}
	for _, l := range ctx.In.Loans {
		v := l.Principal
		if p[paramLoanPart] == loanInterest {
			v = l.Interest
		}
		sum = sum.Add(v)
		r := l.Remark
		if r == "" {
			r = fmt.Sprintf("Pinjaman %d ke-%d", l.LoanID, l.Seq)
		}
		rems = append(rems, r)
	}
	return &sum, strings.Join(rems, "; "), nil
}

// pFormulas returns the non-PPh P formulas (BPJS reuses the T implementations, D-25).
func pFormulas() []Formula { return []Formula{absen{}, loan{}} }

// pSpecFor maps a P component id to its formula (legacy FNC_MGT_PAYROLL_COMPNT_CALC, D-27).
// Every id is mapped: ids without a legacy branch (e.g. P8, P10) get LEGACY_NULL.
func pSpecFor(id int) formulaSpec {
	switch id {
	case 0:
		return formulaSpec{Code: FormulaPPhSubsidy}
	case 1:
		return formulaSpec{Code: FormulaPPh21}
	case 5:
		return formulaSpec{Code: FormulaPPhCompany}
	case 2, 3, 6, 7, 9, 19, 21, 22, 23, 31, 38, 70, 71, 75, 76, 77:
		return formulaSpec{Code: FormulaAddition}
	case idLoanPrincipal:
		return formulaSpec{Code: FormulaLoan, Params: Params{paramLoanPart: loanPrincipal}}
	case idLoanInterest:
		return formulaSpec{Code: FormulaLoan, Params: Params{paramLoanPart: loanInterest}}
	case 4:
		return formulaSpec{Code: FormulaAbsen}
	case 24, 34, 49, 62, 73: // legacy prorates the component amount, not an addition
		return formulaSpec{Code: FormulaFixedOrProrate}
	case 11, 12, 13, 14:
		return formulaSpec{Code: FormulaBPJSNaker}
	case 15, 16:
		return formulaSpec{Code: FormulaBPJSKes}
	case 17, 18:
		return formulaSpec{Code: FormulaBPJSPensiun}
	}
	return formulaSpec{Code: FormulaLegacyNull}
}

// withLoanComponents adds P32/P33 when LOAN_DUE exists but the employee has no such
// component row, so a due installment always produces a line (15§2.4).
func withLoanComponents(in Input, comps []EmpComponent) []EmpComponent {
	if len(in.Loans) == 0 {
		return comps
	}
	for _, id := range []int{idLoanPrincipal, idLoanInterest} {
		k := CompKey{CompP, id}
		found := false
		for _, c := range comps {
			if c.Key == k {
				found = true
				break
			}
		}
		if !found {
			comps = append(comps, EmpComponent{Key: k, Sts: stsFull})
		}
	}
	return comps
}

// checkLoanDouble implements LOAN_DOUBLE (15§2.4).
func checkLoanDouble(in Input) error {
	if len(in.Loans) == 0 {
		return nil
	}
	for _, id := range []int{idLoanPrincipal, idLoanInterest} {
		for _, a := range in.Additions[CompKey{CompP, id}] {
			if strings.EqualFold(a.Source, AdditionSourceLoan) {
				return fmt.Errorf("%w: P%03d", ErrLoanDouble, id)
			}
		}
	}
	return nil
}
