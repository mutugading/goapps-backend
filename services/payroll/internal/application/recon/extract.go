package recon

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
)

// Errors.
var (
	ErrNoLegacyRun = errors.New("recon: no ACTIVE LEGACY run for the period")
	ErrBadOptions  = errors.New("recon: invalid options")
)

// SourceQuery selects the LEGACY run of one division.
type SourceQuery struct {
	CompanyID int16
	Division  string
	YYMM      int
	RuleSetID int
}

// Source reads the LEGACY run and the legacy snapshot of one division (D-41), read-only.
type Source interface {
	Candidates(ctx context.Context, q SourceQuery) ([]Candidate, error)
}

// Options of ExtractGolden.
type Options struct {
	CompanyID  int16
	Divisions  []string
	YYMM       int
	RuleSetID  int
	PerFormula int
	Out        string
}

// FileReport is one written case, identified by pseudonym only.
type FileReport struct {
	Formula string
	Path    string
}

// Report summarizes an extraction.
type Report struct {
	Candidates int
	// Excluded counts the candidates left out per D-47/D-50 reason (not golden cases).
	Excluded   map[string]int
	PerFormula map[string]int
	Files      []FileReport
}

// ExtractGolden writes up to PerFormula cases for every formula found in the LEGACY runs of the divisions.
func ExtractGolden(ctx context.Context, src Source, o Options) (Report, error) {
	if o.YYMM < 190001 || o.YYMM%100 < 1 || o.YYMM%100 > 12 || o.PerFormula <= 0 || o.Out == "" || len(o.Divisions) == 0 {
		return Report{}, ErrBadOptions
	}
	var all []Candidate
	for _, d := range o.Divisions {
		cs, err := src.Candidates(ctx, SourceQuery{CompanyID: o.CompanyID, Division: d, YYMM: o.YYMM, RuleSetID: o.RuleSetID})
		if err != nil {
			return Report{}, fmt.Errorf("division %s: %w", d, err)
		}
		all = append(all, cs...)
	}
	rep := Report{Candidates: len(all), Excluded: map[string]int{}, PerFormula: map[string]int{}}
	usable := all[:0:0]
	for _, c := range all {
		if c.Excluded != "" {
			rep.Excluded[c.Excluded]++
			continue
		}
		usable = append(usable, c)
	}
	all = usable
	names := formulaNames(all)

	pseudo := map[string]string{} // division/NIK -> pseudonym, assigned in output order; never persisted
	for _, f := range names {
		for _, c := range Select(all, f, o.PerFormula) {
			key := c.Division + "/" + c.NIK()
			p, ok := pseudo[key]
			if !ok {
				p = fmt.Sprintf("E%05d", len(pseudo)+1)
				pseudo[key] = p
			}
			path, err := WriteCase(o.Out, Case{Formula: f, YYMM: o.YYMM, RuleSetID: o.RuleSetID, Features: Features(c),
				Input: ToDoc(c.Input, p), Expected: c.Expected, Source: CaseSource{Kind: "LEGACY_RUN", Division: c.Division}})
			if err != nil {
				return rep, err
			}
			rep.PerFormula[f]++
			rep.Files = append(rep.Files, FileReport{Formula: f, Path: path})
		}
	}
	return rep, nil
}

// FormulaFunc names the formula of a component ("" = unknown, falls back to the component key).
type FormulaFunc func(engine.CompKey) string

// formulaNames lists the formulas present in the candidates, sorted.
func formulaNames(cands []Candidate) []string {
	set := map[string]bool{}
	for _, c := range cands {
		for f := range c.Formulas {
			set[f] = true
		}
	}
	names := make([]string, 0, len(set))
	for f := range set {
		names = append(names, f)
	}
	sort.Strings(names)
	return names
}
