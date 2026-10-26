package engine

import (
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/engine/money"
)

// Ids of the December P5 deduction (legacy FNC_MGT_PAYROLL_PPH_COMPANY): P4 Absensi, P14 JHT,
// P18 pension. Unlike P1 (D-29) P4 is a deduction after the job cost and P8/P22/P23 are absent.
var companyDeductIDs = []int{idAbsen, 14, 18}

// pphCompany is P5, PPh borne by the company (06§6.3, D-31). Non-December:
// TER(gross without tax_paid_company rows) − TER(gross), both like P1 (D-28). December: annual
// tax with the company-paid rows deducted minus annual tax on everything. The result is usually
// negative: a negative deduction hands the borne PPh back in take-home.
type pphCompany struct{}

func (pphCompany) Code() string { return FormulaPPhCompany }

func (pphCompany) Eval(ctx *EvalCtx, _ EmpComponent, _ Params) (*decimal.Decimal, string, error) {
	in := ctx.In
	if in.Period.IsDecember() {
		return pphCompanyAnnual(ctx)
	}
	all, okAll := pphGross(ctx)
	own, okOwn := pphGrossWhere(ctx, func(_ CompKey, l Line) bool { return l.TaxPaidCompany }, false)
	if !okAll || !okOwn { // legacy SELECT INTO without a row: NO_DATA_FOUND → NULL
		return nil, "", nil
	}
	if len(in.Tax.Category) == 0 || len(in.Tax.TER) == 0 {
		return nil, "", ErrNoTaxTable
	}
	// THR enters both sides like P1 (D-28c), so P5 stays the difference of what P1 withholds.
	pAll, _, _, ok := terTax(ctx, all.Add(in.Thr.Gross))
	if !ok {
		return nil, "", nil
	}
	pOwn, _, _, ok := terTax(ctx, own.Add(in.Thr.Gross))
	if !ok {
		return nil, "", nil
	}
	p := pOwn.Sub(pAll)
	remark := fmt.Sprintf("Bruto : %s | Bruto Ditanggung Karyawan : %s | PPh : %s | PPh Ditanggung Karyawan : %s",
		thousands(all), thousands(own), thousands(pAll), thousands(pOwn))
	return &p, remark, nil
}

// pphCompanyAnnual is P5 in December. Legacy (rule v1) computes both annual taxes on the same
// gross and job cost; the company-paid rows only join the deduction, after the job cost. Both
// rules subtract the P5 of earlier months (D-31b, 06§6.3); legacy did not (D21, 10§1). Rule v2
// also lowers the gross itself, truncates the PKP and uses the statutory layers.
func pphCompanyAnnual(ctx *EvalCtx) (*decimal.Decimal, string, error) {
	in := ctx.In
	y := in.YTDCompany
	var cur, curPaid, curDeduct decimal.Decimal
	for k, l := range ctx.lines {
		if isPPh(k) {
			continue
		}
		if l.Tax {
			cur = cur.Add(l.AmountOrZero())
			if l.TaxPaidCompany {
				curPaid = curPaid.Add(l.AmountOrZero())
			}
		}
	}
	for _, id := range companyDeductIDs {
		if l, ok := ctx.lines[CompKey{CompP, id}]; ok {
			curDeduct = curDeduct.Add(l.AmountOrZero())
		}
	}
	t := in.Tax
	ptkpTable := t.PTKP // v2 (D-37b): all 9 codes
	if in.Rules.LegacyCompat() {
		ptkpTable = t.PTKPCompany // v1 (D-37a): legacy P5 query has no TK1–TK3 rows
	}
	if len(ptkpTable) == 0 || t.JobCost.Rate.IsZero() || t.JobCost.MaxMonthly.IsZero() || len(t.Progressive) == 0 {
		return nil, "", ErrNoAnnualTaxTable
	}
	ptkp, ok := ptkpTable[in.Emp.TaxStatus]
	if !ok { // legacy inner join on PTKP finds no row: both taxes NO_DATA_FOUND → 0
		p := decimal.Zero
		return &p, "", nil
	}
	twelve := decimal.NewFromInt(12)
	gross := money.Sum(y.Gross, cur, y.ThrBonus)
	paid := y.CompanyPaid.Add(curPaid)
	deduct := y.Deduct.Add(curDeduct)
	ptkp = ptkp.Mul(twelve)
	jobCost := func(g decimal.Decimal) decimal.Decimal {
		return decimal.Min(g.Mul(t.JobCost.Rate).Shift(-2), t.JobCost.MaxMonthly.Mul(twelve))
	}
	layers := sortedLayers(t.Progressive)
	var taxAll, taxOwn decimal.Decimal
	p := decimal.Zero
	if in.Rules.LegacyCompat() {
		nett := gross.Sub(deduct).Sub(jobCost(gross)).Sub(ptkp)
		taxAll = progressiveLegacy(layers, nett)
		taxOwn = progressiveLegacy(layers, nett.Sub(paid))
		p = taxOwn.Sub(taxAll).Sub(y.P5Paid) // D-31b: YTD P5 is subtracted in v1 too
	} else {
		pkp := func(g decimal.Decimal) decimal.Decimal {
			return decimal.Max(money.Trunc(g.Sub(deduct).Sub(jobCost(g)).Sub(ptkp), -3), decimal.Zero)
		}
		taxAll = progressive(layers, pkp(gross))
		taxOwn = progressive(layers, pkp(gross.Sub(paid)))
		p = taxOwn.Sub(taxAll).Sub(y.P5Paid)
	}
	remark := fmt.Sprintf("Bruto : %s | Ditanggung Perusahaan : %s | Pengurang : %s | PTKP : %s | PPh Setahun : %s | PPh Ditanggung Karyawan : %s | P5 Jan-Nov : %s",
		thousands(gross), thousands(paid), thousands(deduct), thousands(ptkp), thousands(taxAll), thousands(taxOwn), thousands(y.P5Paid))
	return &p, remark, nil
}
