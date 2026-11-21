package engine

import (
	"fmt"
	"sort"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/engine/money"
)

// WarnPPhNoPTKP is the warning when the tax status has no PAYROLLSETUP PTKP row (P1 NULL, legacy
// inner join finds no row).
const WarnPPhNoPTKP = "PPH_NO_PTKP"

// idTHRBonus is T75/P75 THR and Bonus, left out of the December current-month gross (legacy
// MPCE_ID NOT IN (75)): THR enters the annual gross through YTD and Input.Thr instead.
const idTHRBonus = 75

// Ids of the annual deduction: P14 JHT, P18 pension, P22/P23 BPJS Naker JHT/pension debt.
var annualDeductIDs = []int{14, 18, 22, 23}

// JobCost is PAYROLLSETUP BIAYAJABATAN: Rate percent of the gross, capped at MaxMonthly × 12.
type JobCost struct {
	Rate       decimal.Decimal // e.g. 5
	MaxMonthly decimal.Decimal // e.g. 500000
}

// TaxLayer is one PAYROLLSETUP TARIFxx row: Min ≤ PKP ≤ Max taxed at Rate percent.
type TaxLayer struct {
	Min  decimal.Decimal // legacy VSSV_NAME, e.g. 60000001
	Max  decimal.Decimal // legacy VSSV_SHORT_NAME, e.g. 250000000
	Rate decimal.Decimal // percent, e.g. 15
}

// annualBasis is the December computation, kept for the remark.
type annualBasis struct {
	gross, deduct, jobCost, ptkp, pkp, tax, paid decimal.Decimal
}

// pph21Annual is P1 in December (06§6.2, D-29): annual tax on gross Jan..Dec minus PPh paid
// Jan..Nov, unfloored (negative = over-withheld, refunded in take-home).
func pph21Annual(ctx *EvalCtx) (*decimal.Decimal, string, error) {
	in := ctx.In
	cur, ok := pphGrossExcept(ctx, map[int]bool{idTHRBonus: true})
	curDeduct, okDed := annualDeduct(ctx)
	if !ok && !okDed && allZero(in.YTD.Gross, in.YTD.Deduct, in.YTD.TaxPaid, in.TaxAdj.Earn, in.TaxAdj.Deduct, in.Thr.Gross, in.Thr.PPhPaid) {
		return nil, "", nil
	}
	t := in.Tax
	if len(t.PTKP) == 0 || t.JobCost.Rate.IsZero() || t.JobCost.MaxMonthly.IsZero() || len(t.Progressive) == 0 {
		return nil, "", ErrNoAnnualTaxTable
	}
	layers := sortedLayers(t.Progressive)
	ptkp, okPTKP := t.PTKP[in.Emp.TaxStatus]
	if !okPTKP {
		ctx.warn(fmt.Sprintf("%s: %s status pajak %q", WarnPPhNoPTKP, in.Emp.NIK, in.Emp.TaxStatus))
		return nil, "", nil
	}
	twelve := decimal.NewFromInt(12)
	b := annualBasis{
		gross:  money.Sum(in.YTD.Gross, cur, in.Thr.Gross, in.TaxAdj.Earn),
		deduct: money.Sum(in.YTD.Deduct, curDeduct, in.TaxAdj.Deduct),
		ptkp:   money.Mul(ptkp, twelve),
		paid:   money.Add(in.YTD.TaxPaid, in.Thr.PPhPaid),
	}
	b.jobCost = decimal.Min(money.Mul(b.gross, t.JobCost.Rate).Shift(-2), money.Mul(t.JobCost.MaxMonthly, twelve))
	nett := money.Sub(money.Sub(money.Sub(b.gross, b.deduct), b.jobCost), b.ptkp)
	if in.Rules.LegacyCompat() {
		b.pkp, b.tax = nett, progressiveLegacy(layers, nett)
	} else {
		b.pkp = decimal.Max(money.Trunc(nett, -3), decimal.Zero)
		b.tax = progressive(layers, b.pkp)
	}
	p := money.Round(money.Sub(b.tax, b.paid), 0)
	remark := fmt.Sprintf("Bruto : %s | Pengurang : %s | Biaya Jabatan : %s | PTKP : %s | PKP : %s | PPh Setahun : %s | PPh Dibayar : %s",
		thousands(b.gross), thousands(b.deduct), thousands(b.jobCost), thousands(b.ptkp), thousands(b.pkp), thousands(b.tax), thousands(b.paid))
	return &p, remark, nil
}

// annualDeduct sums the current-month P14/P18/P22/P23 lines; ok is false when none exists.
func annualDeduct(ctx *EvalCtx) (sum decimal.Decimal, ok bool) {
	for _, id := range annualDeductIDs {
		if l, found := ctx.lines[CompKey{CompP, id}]; found {
			sum, ok = money.Add(sum, l.AmountOrZero()), true
		}
	}
	return sum, ok
}

// sortedLayers returns the layers ordered by Min; loaders may read TARIFxx in code order
// (TARIF15 before TARIF5), while progressive needs ascending layers.
func sortedLayers(ls []TaxLayer) []TaxLayer {
	out := append([]TaxLayer(nil), ls...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Min.LessThan(out[j].Min) })
	return out
}

// progressiveLegacy reproduces the PL/SQL layer sum (D-29, rule v1): a PKP above a layer's Max
// is taxed on the whole Max instead of the layer width, a PKP equal to Max gets nothing from
// that layer, and a PKP above the last Max (9 M) pays a flat Max × 35 %. The result is not rounded.
func progressiveLegacy(layers []TaxLayer, pkp decimal.Decimal) decimal.Decimal {
	var tax decimal.Decimal
	for _, l := range layers {
		var base decimal.Decimal
		lower := money.Sub(l.Min, decimal.NewFromInt(1))
		switch {
		case pkp.GreaterThan(l.Max):
			base = l.Max
		case pkp.LessThan(l.Max) && pkp.GreaterThanOrEqual(lower):
			base = money.Sub(pkp, lower)
		}
		tax = money.Add(tax, money.Mul(base, l.Rate).Shift(-2))
	}
	return tax
}

// progressive is the statutory layer sum (rule v2): each layer taxes the PKP slice between
// Min−1 and Max; the last layer has no upper bound (above 5 M stays 35 %, 06§6.2).
func progressive(layers []TaxLayer, pkp decimal.Decimal) decimal.Decimal {
	var tax decimal.Decimal
	for i, l := range layers {
		lower := money.Sub(l.Min, decimal.NewFromInt(1))
		if !pkp.GreaterThan(lower) {
			break
		}
		upper := pkp
		if i < len(layers)-1 {
			upper = decimal.Min(pkp, l.Max)
		}
		tax = money.Add(tax, money.Mul(money.Sub(upper, lower), l.Rate).Shift(-2))
	}
	return tax
}

// allZero reports whether every value is zero (decimal values cannot be compared with ==).
func allZero(vs ...decimal.Decimal) bool {
	for _, v := range vs {
		if !v.IsZero() {
			return false
		}
	}
	return true
}
