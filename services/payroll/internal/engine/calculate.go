package engine

import (
	"errors"
	"fmt"
	"sort"
	"time"

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
	if err := checkLoanDouble(in); err != nil {
		return Result{}, err
	}
	basis, err := computeBasis(in)
	if err != nil {
		return Result{}, err
	}

	comps := withLoanComponents(in, append([]EmpComponent(nil), in.Components...))
	sort.SliceStable(comps, func(i, j int) bool { return comps[i].Key.Less(comps[j].Key) })

	ctx := &EvalCtx{In: in, Basis: basis, lines: make(map[CompKey]Line, len(comps))}
	res := Result{NIK: in.Emp.NIK}

	for _, phase := range splitPhases(comps) {
		for _, c := range phase.comps {
			if err := evalLine(reg, ctx, &res, c, phase.pph); err != nil {
				return Result{}, err
			}
		}
	}

	res.Warnings = append(res.Warnings, ctx.warnings...)
	SortLines(res.Lines)
	h := buildHeader(in, basis, res.Lines)
	if net := h.TotTunjangan.Sub(h.TotPotongan); len(in.Loans) > 0 && net.IsNegative() {
		res.Warnings = append(res.Warnings, fmt.Sprintf("%s: net %s setelah cicilan pinjaman", WarnLoanNetNegative, net))
	}
	if !in.Rules.LegacyCompat() || (in.HasRekap() && hasDetailRow(res.Lines)) {
		res.Header = h
	}
	return res, nil
}

type phase struct {
	comps []EmpComponent
	pph   bool
}

// splitPhases orders sorted components into T → P non-PPh → P1 → P5 → P0 (06§9).
func splitPhases(comps []EmpComponent) []phase {
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
	return []phase{{tList, false}, {pList, false}, {pphList, true}}
}

// hasDetailRow mirrors the legacy MAIN join to DETAIL: DETAIL keeps only lines with a
// non-zero amount plus P0, so an employee without such a line gets no header (D-27).
func hasDetailRow(lines []Line) bool {
	for _, l := range lines {
		if l.Key == (CompKey{CompP, 0}) || !l.AmountOrZero().IsZero() {
			return true
		}
	}
	return false
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
	if pph && errors.Is(err, errPending) {
		res.Incomplete = true
		res.Warnings = append(res.Warnings, fmt.Sprintf("%s: formula %s untuk periode %d belum tersedia, dilewati", c.Key, spec.Code, ctx.In.Period.YYMM))
		return nil
	}
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

// buildHeader derives the header of 06§7 from the final lines (legacy PRC_SALARY_MGT, D-27).
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
	return &Header{
		BasicPay:          componentAmount(in, 1),
		TunjJabatan:       componentAmount(in, 2),
		TunjRumah:         componentAmount(in, 3),
		TunjKesejahteraan: componentAmount(in, 7),
		TunjTransport:     componentAmount(in, 4),
		RatePerJam:        b.rateJam,
		TotTunjangan:      totT,
		TotPotongan:       totP,
		Rounding:          net.Sub(takeHome),
		TakeHome:          takeHome,
		JamLembur:         jamLembur(in),
		Remarks:           headerRemarks(in),
	}
}

// componentAmount is the MAIN scalar subquery on COMPNT_EMP: A(T,id) or NULL.
func componentAmount(in Input, id int) *decimal.Decimal {
	for _, c := range in.Components {
		if c.Key == (CompKey{CompT, id}) {
			a := c.Amount
			return &a
		}
	}
	return nil
}

// jamLembur is MAIN PM_JAM_LEMBUR*: recap hours × the multiplier shown on the slip. NAS 2/3
// are ×2 for Executive and ×3/×4 for others; a NULL level takes the ELSE branch. Nil
// without a recap row (no header then, D2).
func jamLembur(in Input) map[string]decimal.Decimal {
	if in.Att == nil {
		return nil
	}
	mult := map[string]string{
		"lembur_1": "1.5", "lembur_2": "2",
		"lembur_nas_1": "2", "lembur_nas_2": "3", "lembur_nas_3": "4",
		"lembur_off_1": "2", "lembur_off_2": "3", "lembur_off_3": "4",
		"lembur_nasp_1": "2", "lembur_nasp_2": "3", "lembur_nasp_3": "4",
	}
	if isExecutive(in.Emp.Level) {
		mult["lembur_nas_2"], mult["lembur_nas_3"] = "2", "2"
	}
	out := make(map[string]decimal.Decimal, len(mult))
	for col, m := range mult {
		if v, ok := in.Att[col]; ok { // NULL × k stays NULL: the key is left out
			out[col] = v.Mul(decimal.RequireFromString(m))
		}
	}
	return out
}

// monthNamesID are the Indonesian month names of legacy table MONTHS (INITCAP).
func monthNamesID() [12]string {
	return [12]string{"Januari", "Februari", "Maret", "April", "Mei", "Juni",
		"Juli", "Agustus", "September", "Oktober", "November", "Desember"}
}

// headerRemarks is MAIN PM_REMARKS: the attendance summary, then the birthday line
// (legacy PRC_SALARY_MGT cursor C1, D-27). Remarks are informational, not parity (06§8).
func headerRemarks(in Input) string {
	a := in.Att
	if a == nil {
		return ""
	}
	total := a.Get(colPayTot).Add(a.Get(colDirumahkan)).Add(a.Get(colAlpa)).Add(a.Get(colIjin))
	s := fmt.Sprintf("Working : %s Days, Off : %s Days, Leave : %s Days, Alpa : %s Days, Ijin : %s Days, Total : %s Days",
		a.Get(colDayWork), a.Get("dayoff"), a.Get("cuti"), a.Get(colAlpa), a.Get(colIjin), total)
	if bd := birthdayLine(in.Period, in.Emp.BirthDate); bd != "" {
		s += "\n\n" + bd
	}
	return s
}

// birthdayLine reproduces the legacy birthday text. Legacy checks the window 26/MM through
// 25/MM+1 of YYMM (not the payroll period 26/MM-1 through 25/MM); the year is the YYMM year,
// or the next one for a January birthday in December. 29/02 becomes 28/02 when that year is
// not divisible by 4. Empty outside the window.
func birthdayLine(p Period, bd *time.Time) string {
	if bd == nil {
		return ""
	}
	year := p.Year()
	if p.IsDecember() && bd.Month() == time.January {
		year++
	}
	day := bd.Day()
	if bd.Month() == time.February && day == 29 && year%4 != 0 {
		day = 28
	}
	date := time.Date(year, bd.Month(), day, 0, 0, 0, 0, time.UTC)
	from := time.Date(p.Year(), time.Month(p.Month()), 26, 0, 0, 0, 0, time.UTC)
	to := time.Date(p.Year(), time.Month(p.Month())+1, 25, 0, 0, 0, 0, time.UTC) // Month 13 normalizes to January
	if date.Before(from) || date.After(to) {
		return ""
	}
	return fmt.Sprintf("Selamat Ulang Tahun Ke %d Tanggal %02d %s %d",
		year-bd.Year(), bd.Day(), monthNamesID()[bd.Month()-1], year)
}
