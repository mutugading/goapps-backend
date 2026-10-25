package engine

import (
	"errors"
	"fmt"
	"sort"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/engine/money"
)

// Calculate errors.
var (
	ErrNoFormula = errors.New("engine: no formula registered")
	ErrNoRekap   = errors.New("engine: employee has no attendance recap (rule v2)")
)

// Calculate computes one employee's payroll. It is pure and deterministic (06§9):
// bases → all T → P non-PPh → P1 → P5 → P0 → header.
func Calculate(in Input) (Result, error) {
	return CalculateWith(DefaultRegistry(), in)
}

// CalculateWith is Calculate with an explicit formula registry.
func CalculateWith(reg Registry, in Input) (Result, error) {
	if err := in.Validate(); err != nil {
		return Result{}, err
	}
	basis, err := computeBasis(in)
	if err != nil {
		return Result{}, err
	}

	comps := append([]EmpComponent(nil), in.Components...)
	sort.SliceStable(comps, func(i, j int) bool { return comps[i].Key.Less(comps[j].Key) })

	ctx := &EvalCtx{In: in, Basis: basis, lines: make(map[CompKey]Line, len(comps))}
	res := Result{NIK: in.Emp.NIK}

	var tList, pList, pphList []EmpComponent
	for _, c := range comps {
		switch {
		case c.Key.Type == CompT:
			tList = append(tList, c)
		case isPPh(c.Key):
			pphList = append(pphList, c)
		default:
			pList = append(pList, c)
		}
	}
	sort.SliceStable(pphList, func(i, j int) bool { return pphRank(pphList[i].Key) < pphRank(pphList[j].Key) })

	for _, c := range tList {
		if err := evalLine(reg, ctx, &res, c, false); err != nil {
			return Result{}, err
		}
	}
	for _, c := range pList {
		if err := evalLine(reg, ctx, &res, c, false); err != nil {
			return Result{}, err
		}
	}
	for _, c := range pphList {
		if err := evalLine(reg, ctx, &res, c, true); err != nil {
			return Result{}, err
		}
	}

	SortLines(res.Lines)
	if in.HasRekap() || !in.Rules.LegacyCompat() {
		res.Header = buildHeader(in, basis, res.Lines)
	}
	return res, nil
}

// evalLine evaluates one component and appends its line. A PPh component whose formula
// is not registered yet is skipped with a warning instead of failing the whole run.
func evalLine(reg Registry, ctx *EvalCtx, res *Result, c EmpComponent, pph bool) error {
	spec, mapped := specFor(c.Key)
	if !mapped {
		res.Incomplete = true
		res.Warnings = append(res.Warnings, fmt.Sprintf("%s: belum dipetakan ke formula, dihitung sebagai nominal tetap", c.Key))
	}
	f, ok := reg.Get(spec.Code)
	if !ok {
		if pph {
			res.Incomplete = true
			res.Warnings = append(res.Warnings, fmt.Sprintf("%s: formula %s belum tersedia, dilewati", c.Key, spec.Code))
			return nil
		}
		return fmt.Errorf("%w: %s for %s", ErrNoFormula, spec.Code, c.Key)
	}
	amt, remark, err := f.Eval(ctx, c, spec.Params)
	if err != nil {
		return fmt.Errorf("engine: %s (%s): %w", c.Key, spec.Code, err)
	}
	l := Line{Key: c.Key, Base: c.Amount, Tax: c.Tax, Remark: remark, Formula: spec.Code}
	if amt != nil {
		r := money.Round(*amt, 0)
		l.Amount = &r
	}
	ctx.lines[c.Key] = l
	res.Lines = append(res.Lines, l)
	return nil
}

// buildHeader derives the header of 06§7 from the final lines.
func buildHeader(in Input, b Basis, lines []Line) *Header {
	var totT, totP decimal.Decimal
	for _, l := range lines {
		if l.Key.Type == CompT {
			totT = totT.Add(l.AmountOrZero())
		} else {
			totP = totP.Add(l.AmountOrZero())
		}
	}
	net := totT.Sub(totP)
	takeHome := money.Round(net, -2)
	h := &Header{
		BasicPay:     b.Gapok,
		RatePerJam:   b.rateJamBase,
		TotTunjangan: totT,
		TotPotongan:  totP,
		Rounding:     net.Sub(takeHome),
		TakeHome:     takeHome,
	}
	if bd := in.Emp.BirthDate; bd != nil && int(bd.Month()) == in.Period.Month() {
		h.Remarks = fmt.Sprintf("Selamat Ulang Tahun Ke %d", in.Period.Year()-bd.Year())
	}
	return h
}
