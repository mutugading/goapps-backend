// Package recon holds the legacy-vs-engine reconciliation tooling. T018a: extraction of golden cases
// from the LEGACY run in Postgres (D-41, 09§3). Read-only; no Oracle code.
package recon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
)

// Case is one golden file: the engine input of an employee and the legacy amounts to match (09§3).
// The input carries no PII: NIK is a pseudonym, name and birth date are dropped, remarks are blank.
// Rules and tax tables are not part of it; the golden test reloads them from RuleSetID.
type Case struct {
	Formula   string     `json:"formula"`
	YYMM      int        `json:"yymm"`
	RuleSetID int        `json:"rule_set_id"`
	Features  []string   `json:"features"`
	Input     InputDoc   `json:"input"`
	Expected  Expected   `json:"expected"`
	Source    CaseSource `json:"source"`
}

// CaseSource says where a case came from, without identifying the employee.
type CaseSource struct {
	Kind     string `json:"kind"` // LEGACY_RUN
	Division string `json:"division"`
}

// Expected is the legacy output: one amount per component and the header totals.
type Expected struct {
	Lines []ExpectedLine `json:"lines"`
	Head  *ExpectedHead  `json:"head,omitempty"`
}

// ExpectedLine is a legacy DETAIL row. Amount nil = legacy NULL (D10).
type ExpectedLine struct {
	Type   string  `json:"type"` // T / P
	ID     int     `json:"id"`
	Amount *string `json:"amount"`
}

// ExpectedHead is the legacy MAIN totals.
type ExpectedHead struct {
	TotTunjangan string `json:"tot_tunjangan"`
	TotPotongan  string `json:"tot_potongan"`
	Rounding     string `json:"rounding"`
	TakeHome     string `json:"take_home,omitempty"`
}

// InputDoc is the serializable form of engine.Input without Rules and Tax tables.
type InputDoc struct {
	Period     engine.Period                `json:"period"`
	Division   string                       `json:"division"`
	Emp        engine.Employee              `json:"emp"`
	Components []engine.EmpComponent        `json:"components"`
	Att        engine.AttRekap              `json:"att"`
	AttAdd     engine.AttRekapAdd           `json:"att_add"`
	Sch        engine.SchRekap              `json:"sch"`
	Workdays   *int                         `json:"workdays"`
	Additions  map[string][]engine.Addition `json:"additions,omitempty"`
	Loans      []engine.LoanDue             `json:"loans,omitempty"`
	Daily      engine.DailyFails            `json:"daily"`
	YTD        engine.YTDTax                `json:"ytd"`
	TaxAdj     engine.TaxAdj                `json:"tax_adj"`
	Thr        engine.ThrTax                `json:"thr"`
	YTDCompany engine.YTDCompanyTax         `json:"ytd_company"`
	Subsidy    engine.Subsidy               `json:"subsidy"`
}

// ToDoc converts an input. The employee identity is replaced by pseudo and the free text is blanked.
func ToDoc(in engine.Input, pseudo string) InputDoc {
	d := InputDoc{Period: in.Period, Division: in.Division, Emp: in.Emp, Components: in.Components,
		Att: in.Att, AttAdd: in.AttAdd, Sch: in.Sch, Workdays: in.Workdays, Daily: in.Daily, YTD: in.YTD,
		TaxAdj: in.TaxAdj, Thr: in.Thr, YTDCompany: in.YTDCompany, Subsidy: in.Subsidy}
	d.Emp.NIK, d.Emp.Name, d.Emp.BirthDate = pseudo, "", nil
	if len(in.Additions) > 0 {
		d.Additions = make(map[string][]engine.Addition, len(in.Additions))
		for k, as := range in.Additions {
			cp := make([]engine.Addition, len(as))
			for i, a := range as {
				a.Remark = ""
				cp[i] = a
			}
			d.Additions[k.String()] = cp
		}
	}
	for _, l := range in.Loans {
		l.Remark = ""
		d.Loans = append(d.Loans, l)
	}
	if d.Subsidy.Override != nil {
		o := *d.Subsidy.Override
		o.Reason = ""
		d.Subsidy.Override = &o
	}
	return d
}

var keyRe = regexp.MustCompile(`^([TP])(\d+)$`)

// ToInput rebuilds the engine input; the golden test (T018b) supplies the rule set and tax tables.
func (d InputDoc) ToInput(rules engine.RuleSet, tax engine.TaxTables) (engine.Input, error) {
	in := engine.Input{Period: d.Period, Division: d.Division, Emp: d.Emp, Components: d.Components,
		Att: d.Att, AttAdd: d.AttAdd, Sch: d.Sch, Workdays: d.Workdays, Loans: d.Loans, Daily: d.Daily,
		YTD: d.YTD, TaxAdj: d.TaxAdj, Thr: d.Thr, YTDCompany: d.YTDCompany, Subsidy: d.Subsidy,
		Tax: tax, Rules: rules}
	if len(d.Additions) > 0 {
		in.Additions = make(map[engine.CompKey][]engine.Addition, len(d.Additions))
		for k, as := range d.Additions {
			m := keyRe.FindStringSubmatch(k)
			if m == nil {
				return engine.Input{}, fmt.Errorf("recon: bad addition key %q", k)
			}
			var id int
			if _, err := fmt.Sscanf(m[2], "%d", &id); err != nil {
				return engine.Input{}, fmt.Errorf("recon: bad addition key %q: %w", k, err)
			}
			in.Additions[engine.CompKey{Type: engine.CompType(m[1]), ID: id}] = as
		}
	}
	return in, nil
}

// Candidate is an employee of the LEGACY run with everything needed for a case.
type Candidate struct {
	Division string
	Input    engine.Input
	Expected Expected
	Formulas map[string]bool // formulas of the components the employee has a legacy line for
	// Excluded is the D-47/D-50 reason ("" = usable): such a NIK is not a valid golden case (master drift, D2).
	Excluded string
}

// NIK is the real NIK (never written out).
func (c Candidate) NIK() string { return c.Input.Emp.NIK }

// Features lists the variation classes of 09§3 the employee falls into.
func Features(c Candidate) []string {
	in := c.Input
	sts := in.Emp.GapokSts
	if sts == "" {
		sts = "-"
	}
	f := []string{"div=" + c.Division, "sts=" + sts}
	if in.Att != nil && in.Att.Get("dirumahkan").IsPositive() {
		f = append(f, "dirumahkan>0")
	} else {
		f = append(f, "dirumahkan=0")
	}
	if strings.EqualFold(in.Emp.Level, "Executive") {
		f = append(f, "level=executive")
	} else {
		f = append(f, "level=non-executive")
	}
	if in.Daily.PremiFail > 0 {
		f = append(f, "premi_fail>0")
	} else {
		f = append(f, "premi_fail=0")
	}
	if j := in.Emp.JoinDate; j != nil && !j.Before(in.Period.Start) {
		f = append(f, "join=new")
	} else {
		f = append(f, "join=existing")
	}
	return f
}

// Select picks up to n candidates for a formula, greedily maximizing the variation classes covered.
// The order is deterministic (division, NIK) and ties keep that order.
func Select(cands []Candidate, formula string, n int) []Candidate {
	var pool []Candidate
	for _, c := range cands {
		if c.Formulas[formula] {
			pool = append(pool, c)
		}
	}
	sort.SliceStable(pool, func(i, j int) bool {
		if pool[i].Division != pool[j].Division {
			return pool[i].Division < pool[j].Division
		}
		return pool[i].NIK() < pool[j].NIK()
	})
	covered := map[string]bool{}
	var out []Candidate
	for len(out) < n && len(pool) > 0 {
		best := bestGain(pool, covered)
		c := pool[best]
		for _, f := range Features(c) {
			covered[f] = true
		}
		out = append(out, c)
		pool = append(pool[:best], pool[best+1:]...)
	}
	return out
}

// bestGain returns the index of the first candidate that adds the most uncovered classes.
func bestGain(pool []Candidate, covered map[string]bool) int {
	best, top := 0, -1
	for i, c := range pool {
		gain := 0
		for _, f := range Features(c) {
			if !covered[f] {
				gain++
			}
		}
		if gain > top {
			best, top = i, gain
		}
	}
	return best
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9_.-]`)

// Dir is the folder name of a formula.
func Dir(formula string) string { return unsafeName.ReplaceAllString(formula, "_") }

// WriteCase writes <out>/<formula>/<pseudo>_<yymm>.json and returns its path.
func WriteCase(out string, c Case) (string, error) {
	dir := filepath.Join(out, Dir(c.Formula))
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("recon: mkdir: %w", err)
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return "", fmt.Errorf("recon: marshal case: %w", err)
	}
	path := filepath.Join(dir, fmt.Sprintf("%s_%d.json", c.Input.Emp.NIK, c.YYMM))
	if err := os.WriteFile(path, append(b, '\n'), 0o600); err != nil {
		return "", fmt.Errorf("recon: write case: %w", err)
	}
	return path, nil
}

// ZeroIfNil is a helper for tests and reports.
func ZeroIfNil(s *string) decimal.Decimal {
	if s == nil {
		return decimal.Zero
	}
	d, err := decimal.NewFromString(*s)
	if err != nil {
		return decimal.Zero
	}
	return d
}
