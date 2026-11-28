package report

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/engine/money"
)

// GL journal (F-2 sheet `upload`, 11§4, D-63). BuildGL is pure: it aggregates the run lines per group
// dept × status × level × MGT/MB/Contract × account × D/C × description, adds the Salary Payables lines, puts the
// rounding difference on the 'Gaji <divisi>' line of the same group (SALARY001 "DIFF SALARY PAYABLES"), and fails
// hard on any unmapped line or when Σ D ≠ Σ C.

// GL error codes (11§4.2). Each problem found is listed in GLError.Problems.
const (
	GLUnmapped         = "GL_UNMAPPED"          // line amount ≠ 0 without a pyr_gl_setup row
	GLGroupUnmapped    = "GL_GROUP_UNMAPPED"    // employee level without pyr_gl_group_map row
	GLCostGroupMissing = "GL_COSTGROUP_MISSING" // pe_cost_group (MGT/MB/CONTRACT) empty
	GLSubacntMissing   = "GL_SUBACNT_MISSING"   // sub_from_employee and pe_orion_sub_acnt empty (no NIK fallback)
	GLPayableUnmapped  = "GL_PAYABLE_UNMAPPED"  // no PAYABLE setup for the group
	GLRoundingUnplaced = "GL_ROUNDING_UNPLACED" // rounding ≠ 0 but not exactly one 'Gaji <divisi>' line in the group
	GLLoanUnsupported  = "GL_LOAN_UNSUPPORTED"  // P32/P33 present: no loan module yet (D-63 butir 5)
	GLUnbalanced       = "GL_UNBALANCED"        // Σ D ≠ Σ C
	GLEmpty            = "GL_EMPTY"             // no head or no journal line
	GLRateInvalid      = "GL_RATE_INVALID"      // BOOK rate ≤ 0
	GLNetPayMismatch   = "GL_NETPAY_MISMATCH"   // psh_net_pay ≠ tunj − pot − rounding
)

// ErrGL wraps every GL validation failure.
var ErrGL = errors.New("report: jurnal GL tidak valid")

// GLProblem is one validation finding. NIK may be empty (group-level problems).
type GLProblem struct{ Code, NIK, Detail string }

// GLError lists the problems found; the export is rejected as a whole. Error() only counts per code so that no NIK
// reaches stderr or a log; Details() has the full list for a local 0600 file.
type GLError struct{ Problems []GLProblem }

func (e *GLError) Error() string {
	n := map[string]int{}
	for _, p := range e.Problems {
		n[p.Code]++
	}
	codes := make([]string, 0, len(n))
	for c := range n {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	parts := make([]string, len(codes))
	for i, c := range codes {
		parts[i] = fmt.Sprintf("%s=%d", c, n[c])
	}
	return fmt.Sprintf("%v: %d masalah: %s", ErrGL, len(e.Problems), strings.Join(parts, ", "))
}

// Details is one line per problem, with the NIK: write it only to a local file readable by the operator.
func (e *GLError) Details() []string {
	out := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		out[i] = strings.TrimSpace(p.Code + " " + p.NIK + " " + p.Detail)
	}
	return out
}

func (e *GLError) Unwrap() error { return ErrGL }

// gajiDescriptions are the setup descriptions of the base-salary line that also takes −Σ rounding (SALARY001 part A+B).
var gajiDescriptions = map[string]bool{"Gaji Solo": true, "Gaji Jakarta": true, "Gaji Bandung": true}

// loanNos are the legacy Potongan numbers of the loan installment (P32 principal/interest SOLO, P33 interest JAKARTA).
var loanNos = map[int]bool{32: true, 33: true}

// GLHead is one employee of the run with the attributes that pick the group.
type GLHead struct {
	NIK, Dept, Group, CostGroup, SubAcnt string
	Tunj, Pot, Rounding                  decimal.Decimal
	NetPay                               *decimal.Decimal // psh_net_pay; nil when NULL
}

// GLLine is one non-zero detail line with its GL setup (HasSetup=false when none matched).
type GLLine struct {
	NIK, Type       string // Type = "Tunjangan" | "Potongan"
	No              int
	Amount          decimal.Decimal
	HasSetup        bool
	Main, Sub       string
	DrCr, Desc      string
	SubFromEmployee bool
}

// GLAccount is a PAYABLE setup row.
type GLAccount struct{ Main, Sub, DrCr, Desc string }

// GLInput is everything BuildGL needs; Payables is keyed by gl_group.
type GLInput struct {
	YYMM     int
	Rate     decimal.Decimal // IDR→USD BOOK rate of the period
	Heads    []GLHead
	Lines    []GLLine
	Payables map[string]GLAccount
}

// GLRow is one journal line of the upload sheet.
type GLRow struct {
	Dept, Group, CostGroup string
	Main, Sub, DrCr, Desc  string
	FC, LC                 decimal.Decimal
}

// MGTMB is the "MGT MB Contract" column (SALARY001: REPLACE(..,'CONTRACT','Contract')).
func (r GLRow) MGTMB() string { return strings.ReplaceAll(r.CostGroup, "CONTRACT", "Contract") }

// GLJournal is the built journal with its control totals.
type GLJournal struct {
	YYMM          int
	Rate          decimal.Decimal
	Rows          []GLRow
	Debit, Credit decimal.Decimal
	NIKCount      int
	NetPay        decimal.Decimal // Σ psh_net_pay (checked = T − P − R per head)
	Payables      decimal.Decimal // Σ (T − P − R) = Σ Salary Payables before D/C reversal
	LCDebit       decimal.Decimal
	LCCredit      decimal.Decimal
}

type glKey struct{ dept, group, cost, main, sub, drcr, desc string }
type glGroup struct{ dept, group, cost string }

// glBuilder accumulates the journal sums in first-seen order and the problems found.
type glBuilder struct {
	in    GLInput
	heads map[string]GLHead
	sums  map[glKey]decimal.Decimal
	order []glKey
	probs []GLProblem
}

func (b *glBuilder) add(code, nik, f string, a ...any) {
	b.probs = append(b.probs, GLProblem{Code: code, NIK: nik, Detail: fmt.Sprintf(f, a...)})
}

func glFail(code, f string, a ...any) error {
	return &GLError{Problems: []GLProblem{{Code: code, Detail: fmt.Sprintf(f, a...)}}}
}

func (b *glBuilder) acc(k glKey, v decimal.Decimal) {
	if _, ok := b.sums[k]; !ok {
		b.order = append(b.order, k)
	}
	b.sums[k] = b.sums[k].Add(v)
}

// BuildGL aggregates the run into the journal. It never drops a line silently: every problem is collected and
// returned as *GLError.
func BuildGL(in GLInput) (GLJournal, error) {
	if !in.Rate.IsPositive() {
		return GLJournal{}, glFail(GLRateInvalid, "kurs BOOK IDR→USD periode tidak valid")
	}
	if len(in.Heads) == 0 {
		return GLJournal{}, glFail(GLEmpty, "run tanpa head")
	}
	b := &glBuilder{in: in, heads: make(map[string]GLHead, len(in.Heads)), sums: map[glKey]decimal.Decimal{}}
	b.checkHeads()
	for _, l := range in.Lines {
		b.line(l)
	}
	pay := b.payables()
	if len(b.probs) > 0 {
		return GLJournal{}, &GLError{Problems: b.probs}
	}
	j := b.journal(pay)
	if len(j.Rows) == 0 {
		return GLJournal{}, glFail(GLEmpty, "jurnal tanpa baris")
	}
	if !j.Debit.Equal(j.Credit) {
		return GLJournal{}, glFail(GLUnbalanced, "Σ D %s ≠ Σ C %s", j.Debit, j.Credit)
	}
	return j, nil
}

func usable(h GLHead) bool { return h.Group != "" && h.CostGroup != "" }

func (b *glBuilder) checkHeads() {
	for _, h := range b.in.Heads {
		b.heads[h.NIK] = h
		if h.NetPay == nil || !h.NetPay.Equal(h.Tunj.Sub(h.Pot).Sub(h.Rounding)) {
			b.add(GLNetPayMismatch, h.NIK, "")
		}
		switch {
		case h.Group == "":
			b.add(GLGroupUnmapped, h.NIK, "")
		case h.CostGroup == "":
			b.add(GLCostGroupMissing, h.NIK, "")
		}
	}
}

// line adds one detail line to its group × account sum, or records why it cannot be journaled.
func (b *glBuilder) line(l GLLine) {
	if l.Amount.IsZero() {
		return
	}
	h, ok := b.heads[l.NIK]
	switch {
	case !ok:
		b.add(GLUnmapped, l.NIK, "tanpa head")
		return
	case !usable(h):
		return // already reported once per NIK
	case l.Type == typePotongan && loanNos[l.No]:
		b.add(GLLoanUnsupported, l.NIK, "Potongan %d", l.No)
		return
	case !l.HasSetup:
		b.add(GLUnmapped, l.NIK, "%s %d (grup %s %s)", l.Type, l.No, h.Group, h.CostGroup)
		return
	case l.SubFromEmployee && h.SubAcnt == "":
		b.add(GLSubacntMissing, l.NIK, "akun %s", l.Main)
		return
	}
	sub := l.Sub
	if l.SubFromEmployee {
		sub = h.SubAcnt
	}
	b.acc(glKey{h.Dept, h.Group, h.CostGroup, l.Main, sub, l.DrCr, l.Desc}, l.Amount)
}

// payables adds the Salary Payables line per group (Σ T − P − R) and moves −Σ rounding onto the group's single
// 'Gaji <divisi>' line (SALARY001 part B "DIFF SALARY PAYABLES"). It returns Σ net_pay.
func (b *glBuilder) payables() decimal.Decimal {
	payable := map[glGroup]decimal.Decimal{}
	rounding := map[glGroup]decimal.Decimal{}
	var groups []glGroup
	net := decimal.Zero
	for _, h := range b.in.Heads {
		if !usable(h) {
			continue
		}
		g := glGroup{h.Dept, h.Group, h.CostGroup}
		if _, ok := payable[g]; !ok {
			groups = append(groups, g)
		}
		n := h.Tunj.Sub(h.Pot).Sub(h.Rounding)
		payable[g] = payable[g].Add(n)
		rounding[g] = rounding[g].Add(h.Rounding)
		net = net.Add(n)
	}
	for _, g := range groups {
		pa, ok := b.in.Payables[g.group]
		if !ok {
			b.add(GLPayableUnmapped, "", "grup %s", g.group)
			continue
		}
		b.acc(glKey{g.dept, g.group, g.cost, pa.Main, pa.Sub, pa.DrCr, pa.Desc}, payable[g])
		if r := rounding[g]; !r.IsZero() {
			b.placeRounding(g, r)
		}
	}
	return net
}

func (b *glBuilder) placeRounding(g glGroup, r decimal.Decimal) {
	var target []glKey
	for _, k := range b.order {
		if k.dept == g.dept && k.group == g.group && k.cost == g.cost && gajiDescriptions[k.desc] {
			target = append(target, k)
		}
	}
	if len(target) != 1 {
		b.add(GLRoundingUnplaced, "", "dept %s grup %s %s: %d baris Gaji", g.dept, g.group, g.cost, len(target))
		return
	}
	b.sums[target[0]] = b.sums[target[0]].Sub(r)
}

// journal turns the sums into rows: zero sums dropped, negative sums reversed (11§4.1), LC = ROUND(FC × rate, 2).
func (b *glBuilder) journal(net decimal.Decimal) GLJournal {
	in := b.in
	j := GLJournal{YYMM: in.YYMM, Rate: in.Rate, NIKCount: len(in.Heads), Payables: net}
	for _, h := range in.Heads {
		if h.NetPay != nil {
			j.NetPay = j.NetPay.Add(*h.NetPay)
		}
	}
	for _, k := range b.order {
		v := b.sums[k]
		if v.IsZero() {
			continue
		}
		dc := k.drcr
		if v.IsNegative() {
			dc = flipDC(dc)
			v = v.Neg()
		}
		lc := money.Round(v.Mul(in.Rate), 2)
		j.Rows = append(j.Rows, GLRow{Dept: k.dept, Group: k.group, CostGroup: k.cost, Main: k.main, Sub: k.sub, DrCr: dc,
			Desc: k.desc + " " + monthYear(in.YYMM), FC: v, LC: lc})
		if dc == "D" {
			j.Debit, j.LCDebit = j.Debit.Add(v), j.LCDebit.Add(lc)
		} else {
			j.Credit, j.LCCredit = j.Credit.Add(v), j.LCCredit.Add(lc)
		}
	}
	sort.SliceStable(j.Rows, func(a, c int) bool {
		x, y := j.Rows[a], j.Rows[c]
		if x.Dept != y.Dept {
			return x.Dept < y.Dept
		}
		if x.Group != y.Group {
			return x.Group < y.Group
		}
		return x.CostGroup < y.CostGroup
	})
	return j
}

func flipDC(dc string) string {
	if dc == "D" {
		return "C"
	}
	return "D"
}

// GLAccountTotal is one line of the Check sheet: Σ per (main, sub, D/C).
type GLAccountTotal struct {
	Main, Sub, DrCr string
	FC              decimal.Decimal
	Rows            int
}

// AccountTotals sums the journal per (main account, sub account, D/C), sorted.
func (j GLJournal) AccountTotals() []GLAccountTotal {
	idx := map[[3]string]int{}
	var out []GLAccountTotal
	for _, r := range j.Rows {
		k := [3]string{r.Main, r.Sub, r.DrCr}
		i, ok := idx[k]
		if !ok {
			i = len(out)
			idx[k] = i
			out = append(out, GLAccountTotal{Main: r.Main, Sub: r.Sub, DrCr: r.DrCr})
		}
		out[i].FC = out[i].FC.Add(r.FC)
		out[i].Rows++
	}
	sort.Slice(out, func(a, b int) bool {
		x, y := out[a], out[b]
		if x.Main != y.Main {
			return x.Main < y.Main
		}
		if x.Sub != y.Sub {
			return x.Sub < y.Sub
		}
		return x.DrCr < y.DrCr
	})
	return out
}
