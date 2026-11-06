package golden

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/recon"
	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
)

// goldenCase is a golden case plus what the engine needs besides the input: the rule constants and tax tables.
// Synthetic fixtures (G1–G14) share testdata/tax.json and carry the rule constants inline; extracted real cases get them from GOLDEN_RULES.
type goldenCase struct {
	recon.Case
	Rules   ruleDoc          `json:"rules"`
	Tax     engine.TaxTables `json:"tax"`
	Partial bool             `json:"partial,omitempty"` // only the listed lines are compared (extra engine lines are fine)
	Note    string           `json:"note,omitempty"`
}

// ruleDoc is a static legacy-compat (v1) rule set.
type ruleDoc struct {
	Legacy    bool              `json:"legacy"`
	Constants map[string]string `json:"constants"`
}

func (r ruleDoc) ID() int            { return 1 }
func (r ruleDoc) LegacyCompat() bool { return r.Legacy }
func (r ruleDoc) Constant(k string) (decimal.Decimal, bool) {
	s, ok := r.Constants[k]
	if !ok {
		return decimal.Zero, false
	}
	v, err := decimal.NewFromString(s)
	return v, err == nil
}

// rulesFile is the GOLDEN_RULES document for real cases: the rule set and tax tables of the extracted period.
type rulesFile struct {
	Rules ruleDoc          `json:"rules"`
	Tax   engine.TaxTables `json:"tax"`
}

// sharedTax is the tax table every synthetic fixture uses (testdata/tax.json).
func sharedTax(t *testing.T) engine.TaxTables {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "tax.json"))
	if err != nil {
		t.Fatal(err)
	}
	var tax engine.TaxTables
	if err := json.Unmarshal(b, &tax); err != nil {
		t.Fatalf("tax.json: %v", err)
	}
	return tax
}

func loadCases(t *testing.T, root string, rf *rulesFile) []goldenCase {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(root, func(p string, e os.DirEntry, err error) error {
		if err == nil && !e.IsDir() && strings.HasSuffix(p, ".json") {
			paths = append(paths, p)
		}
		return err
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	sort.Strings(paths)
	var out []goldenCase
	for _, p := range paths {
		b, err := os.ReadFile(p) //nolint:gosec // test goldenCase path from WalkDir
		if err != nil {
			t.Fatal(err)
		}
		var f goldenCase
		if err := json.Unmarshal(b, &f); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if rf != nil {
			f.Rules, f.Tax = rf.Rules, rf.Tax
		} else {
			f.Tax = sharedTax(t)
		}
		out = append(out, f)
	}
	return out
}

// compare returns one message per difference between the engine result and the legacy expectation (|Δ| = 0).
func compare(f goldenCase, res engine.Result) []string {
	var diffs []string
	got := map[string]engine.Line{}
	for _, l := range res.Lines {
		got[l.Key.String()] = l
	}
	seen := map[string]bool{}
	for _, e := range f.Expected.Lines {
		k := engine.CompKey{Type: engine.CompType(e.Type), ID: e.ID}.String()
		seen[k] = true
		l, ok := got[k]
		switch {
		case !ok:
			diffs = append(diffs, fmt.Sprintf("%s: ONLY_LEGACY (want %s)", k, show(e.Amount)))
		case (e.Amount == nil) != (l.Amount == nil):
			diffs = append(diffs, fmt.Sprintf("%s: want %s, got %s", k, show(e.Amount), showDec(l.Amount)))
		case e.Amount != nil && !decimal.RequireFromString(*e.Amount).Equal(*l.Amount):
			diffs = append(diffs, fmt.Sprintf("%s: want %s, got %s", k, *e.Amount, l.Amount))
		}
	}
	if !f.Partial {
		for k := range got {
			if !seen[k] {
				diffs = append(diffs, fmt.Sprintf("%s: ONLY_TARGET (got %s)", k, showDec(got[k].Amount)))
			}
		}
	}
	return append(diffs, compareHead(f, res)...)
}

func compareHead(f goldenCase, res engine.Result) []string {
	h := f.Expected.Head
	switch {
	case h == nil && res.Header != nil && !f.Partial:
		return []string{"header: legacy has none (D2), engine produced one"}
	case h == nil:
		return nil
	case res.Header == nil:
		return []string{"header: want one, engine produced none"}
	}
	var diffs []string
	for _, c := range []struct {
		name string
		want string
		got  decimal.Decimal
	}{
		{"tot_tunjangan", h.TotTunjangan, res.Header.TotTunjangan},
		{"tot_potongan", h.TotPotongan, res.Header.TotPotongan},
		{"rounding", h.Rounding, res.Header.Rounding},
		{"take_home", h.TakeHome, res.Header.TakeHome},
	} {
		if c.want == "" { // fixtures without take_home (real extracts) skip it
			continue
		}
		if !decimal.RequireFromString(c.want).Equal(c.got) {
			diffs = append(diffs, fmt.Sprintf("header.%s: want %s, got %s", c.name, c.want, c.got))
		}
	}
	return diffs
}

func show(s *string) string {
	if s == nil {
		return "NULL"
	}
	return *s
}

func showDec(d *decimal.Decimal) string {
	if d == nil {
		return "NULL"
	}
	return d.String()
}

func runCases(t *testing.T, fixtures []goldenCase) {
	t.Helper()
	if len(fixtures) == 0 {
		t.Fatal("no golden fixtures found")
	}
	for _, f := range fixtures {
		name := fmt.Sprintf("%s/%s_%d", recon.Dir(f.Formula), f.Input.Emp.NIK, f.YYMM)
		t.Run(name, func(t *testing.T) {
			in, err := f.Input.ToInput(f.Rules, f.Tax)
			if err != nil {
				t.Fatal(err)
			}
			res, err := engine.Calculate(in)
			if err != nil {
				t.Fatalf("Calculate: %v", err)
			}
			for _, d := range compare(f, res) {
				t.Error(d)
			}
		})
	}
}

// TestGolden runs the committed synthetic cases G1–G12 (09§3).
func TestGolden(t *testing.T) {
	runCases(t, loadCases(t, filepath.Join("testdata", "golden"), nil))
}

// TestGoldenReal runs cases extracted from a real legacy run (T018a). Set GOLDEN_DIR to the extractor output and
// GOLDEN_RULES to a JSON {"rules":{legacy,constants},"tax":{…}} of the same period. Never committed (real salaries).
func TestGoldenReal(t *testing.T) {
	dir, rules := os.Getenv("GOLDEN_DIR"), os.Getenv("GOLDEN_RULES")
	if dir == "" || rules == "" {
		t.Skip("GOLDEN_DIR and GOLDEN_RULES not set")
	}
	b, err := os.ReadFile(rules) //nolint:gosec // path from the developer's environment
	if err != nil {
		t.Fatal(err)
	}
	var rf rulesFile
	if err := json.Unmarshal(b, &rf); err != nil {
		t.Fatal(err)
	}
	runCases(t, loadCases(t, dir, &rf))
}
