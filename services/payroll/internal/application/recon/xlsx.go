package recon

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"
)

// Sheet names of the recon workbook (09§4 step 4).
const (
	SheetSummary = "Ringkasan"
	SheetTop50   = "Top50"
	SheetPerNIK  = "PerNIK"
	SheetDerived = "AttDerived"
)

// topN is the number of lines in the Top50 sheet.
const topN = 50

// maxNumericDigits is the largest significant-digit count that survives an xlsx (IEEE double) cell.
const maxNumericDigits = 15

// WriteXLSX writes the diff report as a four-sheet workbook. Amounts come from decimal: a numeric cell
// when the value is exact in a double (≤ 15 significant digits), otherwise a text cell.
// The workbook holds real NIKs and salaries: never commit it.
func WriteXLSX(rep DiffReport, w io.Writer) error {
	f := excelize.NewFile()
	defer f.Close() //nolint:errcheck // in-memory workbook
	if err := f.SetSheetName("Sheet1", SheetSummary); err != nil {
		return fmt.Errorf("rename sheet: %w", err)
	}
	for _, name := range []string{SheetTop50, SheetPerNIK, SheetDerived} {
		if _, err := f.NewSheet(name); err != nil {
			return fmt.Errorf("new sheet %s: %w", name, err)
		}
	}
	sw := &sheetWriter{f: f}
	sw.summary(rep)
	sw.top50(rep)
	sw.perNIK(rep)
	sw.derived(rep)
	if sw.err != nil {
		return sw.err
	}
	return f.Write(w)
}

// sheetWriter collects the first error so the sheet builders stay linear.
type sheetWriter struct {
	f     *excelize.File
	sheet string
	row   int
	err   error
}

func (s *sheetWriter) start(sheet string) { s.sheet, s.row = sheet, 0 }

// put writes one row; value types: string, int, bool, decimal.Decimal, *decimal.Decimal (nil = empty).
func (s *sheetWriter) put(vals ...any) {
	s.row++
	for i, v := range vals {
		if s.err != nil {
			return
		}
		cell, err := excelize.CoordinatesToCellName(i+1, s.row)
		if err != nil {
			s.err = err
			return
		}
		s.err = s.f.SetCellValue(s.sheet, cell, cellValue(v))
	}
}

func cellValue(v any) any {
	switch x := v.(type) {
	case decimal.Decimal:
		return decimalCell(x)
	case *decimal.Decimal:
		if x == nil {
			return nil
		}
		return decimalCell(*x)
	default:
		return v
	}
}

// decimalCell is the one accepted exception to "money is never float" (T023d): the recon workbook is a report
// for Excel numeric cells, never read back by the engine. Exactness is kept by the 15-digit rule below.
//
// decimalCell returns a float64 only when its shortest representation equals the decimal string
// (≤ 15 significant digits); otherwise the exact text.
func decimalCell(d decimal.Decimal) any {
	s := d.String()
	digits := strings.TrimLeft(strings.NewReplacer("-", "", ".", "").Replace(s), "0")
	if len(digits) > maxNumericDigits {
		return s
	}
	fl, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return s
	}
	return fl
}

func (s *sheetWriter) summary(rep DiffReport) {
	s.start(SheetSummary)
	sm := rep.Summary
	s.put("Recon", rep.Division, rep.YYMM, rep.Target)
	s.put("Legacy run", rep.LegacyRunID)
	s.put("NIK legacy", sm.NIKLegacy)
	s.put("NIK target", sm.NIKTarget)
	s.put("Verdict paritas", map[bool]string{true: "PARITAS", false: "TIDAK PARITAS"}[sm.Parity])
	s.put("Σ take_home legacy", sm.TakeHomeLegacy)
	s.put("Σ take_home target", sm.TakeHomeTarget)
	s.put("Σ take_home Δ", sm.TakeHomeDelta)
	s.put("Header berbeda", sm.HeadMismatch)
	s.put("ONLY_LEGACY / ONLY_TARGET (D-49, menggagalkan paritas)", sm.Categories[CatOnlyLegacy], sm.Categories[CatOnlyTarget])
	s.put("ATT_DERIVED (05§5, info; snapshot sync, 0 = tidak dicek)", sm.Categories[CatAttDerived], rep.DerivedSnapshotID)
	s.put()
	s.put("Dikecualikan (D-47)", "NIK", "Baris Δ≠0", "Baris komponen lain", "Header berbeda", "Σ take_home legacy", "Σ take_home target", "Σ take_home Δ")
	for _, e := range Exclusions {
		x := sm.Excluded[e]
		s.put(e, x.NIK, x.Lines, x.OtherLines, x.HeadMismatch, x.TakeHomeLegacy, x.TakeHomeTarget, x.TakeHomeDelta)
	}
	s.put()
	s.put("Kategori", "Jumlah baris")
	for _, c := range Categories {
		s.put(c, sm.Categories[c])
	}
	s.put()
	header := append([]any{"Komponen"}, stringsToAny(Categories[1:])...)
	s.put(header...)
	comps := make([]string, 0, len(sm.PerComponent))
	for c := range sm.PerComponent {
		comps = append(comps, c)
	}
	sort.Strings(comps)
	for _, c := range comps {
		row := make([]any, 0, 1+len(Categories)-1)
		row = append(row, c)
		for _, cat := range Categories[1:] {
			row = append(row, sm.PerComponent[c][cat])
		}
		s.put(row...)
	}
}

func stringsToAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, v := range ss {
		out[i] = v
	}
	return out
}

// TopLines returns the n lines with the largest |Δ|, ordered |Δ| desc, nik, component.
func TopLines(lines []LineDiff, n int) []LineDiff {
	out := append([]LineDiff(nil), lines...)
	sort.SliceStable(out, func(i, j int) bool {
		if c := out[i].Delta.Abs().Cmp(out[j].Delta.Abs()); c != 0 {
			return c > 0
		}
		if out[i].NIK != out[j].NIK {
			return out[i].NIK < out[j].NIK
		}
		return out[i].Component < out[j].Component
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

func (s *sheetWriter) top50(rep DiffReport) {
	s.start(SheetTop50)
	s.put("NIK", "Komponen", "Legacy", "Target", "Δ", "Kategori")
	for _, l := range TopLines(rep.Lines, topN) {
		s.put(l.NIK, l.Component, l.Legacy, l.Target, l.Delta, l.Category)
	}
}

func headTake(h *HeadTotals) *decimal.Decimal {
	if h == nil {
		return nil
	}
	return &h.TakeHome
}

// perNIKRow is one PerNIK row; Comp "" = header-only NIK.
func (s *sheetWriter) perNIK(rep DiffReport) {
	s.start(SheetPerNIK)
	s.put("NIK", "Komponen", "Legacy", "Target", "Δ", "Kategori",
		"Take home legacy", "Take home target", "Δ take home")
	heads := make(map[string]HeadDiff, len(rep.Heads))
	for _, h := range rep.Heads {
		heads[h.NIK] = h
	}
	lines := append([]LineDiff(nil), rep.Lines...)
	sort.SliceStable(lines, func(i, j int) bool {
		if lines[i].NIK != lines[j].NIK {
			return lines[i].NIK < lines[j].NIK
		}
		return lines[i].Component < lines[j].Component
	})
	seen := map[string]bool{}
	for _, l := range lines {
		seen[l.NIK] = true
		s.put(append([]any{l.NIK, l.Component, l.Legacy, l.Target, l.Delta, l.Category}, headCols(heads[l.NIK])...)...)
	}
	var only []string
	for nik := range heads {
		if !seen[nik] {
			only = append(only, nik)
		}
	}
	sort.Strings(only)
	for _, nik := range only {
		s.put(append([]any{nik, "", nil, nil, nil, "HEADER"}, headCols(heads[nik])...)...)
	}
}

func headCols(h HeadDiff) []any {
	if h.NIK == "" {
		return []any{nil, nil, nil}
	}
	delta := orZero(headTake(h.Target)).Sub(orZero(headTake(h.Legacy)))
	return []any{headTake(h.Legacy), headTake(h.Target), delta}
}

// derived lists the ATT_DERIVED differences (05§5): derived shift / otiw_decr vs MPAR_* of the synced snapshot.
func (s *sheetWriter) derived(rep DiffReport) {
	s.start(SheetDerived)
	s.put("NIK", "Kolom", "MPAR (view HRIS)", "Derived payroll", "Δ", "Dikecualikan (D-47)")
	for _, d := range rep.AttDerived {
		s.put(d.NIK, d.Field, d.MPAR, d.Derived, d.Delta, d.Excluded)
	}
}
