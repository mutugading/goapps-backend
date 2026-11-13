package report

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"
)

// Sheet names of the per-component reconciliation workbook (format awal; final menyusul T028d).
const (
	SheetDetail  = "Detail"
	SheetSummary = "Ringkasan"
)

// maxNumericDigits is the largest significant-digit count exact in an xlsx (double) cell.
const maxNumericDigits = 15

// Summary is the Ringkasan content, exposed for tests.
type Summary struct {
	PerComp      map[string]decimal.Decimal // Comp.Key → Σ amount
	CountPerComp map[string]int
	NIKCount     int
	SumT, SumP   decimal.Decimal // Σ component lines by type
	HeadTunj     decimal.Decimal // Σ head tot_tunjangan
	HeadPot      decimal.Decimal
	HeadNet      decimal.Decimal
	Rounding     decimal.Decimal
}

// Summarize totals a run.
func Summarize(d RunData) Summary {
	s := Summary{PerComp: map[string]decimal.Decimal{}, CountPerComp: map[string]int{}, NIKCount: len(d.Emps)}
	for _, e := range d.Emps {
		s.HeadTunj = s.HeadTunj.Add(e.TotTunjangan)
		s.HeadPot = s.HeadPot.Add(e.TotPotongan)
		s.HeadNet = s.HeadNet.Add(e.NetPay)
		s.Rounding = s.Rounding.Add(e.Rounding)
		for k, v := range e.Amounts {
			s.PerComp[k] = s.PerComp[k].Add(v)
			s.CountPerComp[k]++
			if strings.HasPrefix(k, "T") {
				s.SumT = s.SumT.Add(v)
			} else {
				s.SumP = s.SumP.Add(v)
			}
		}
	}
	return s
}

// WriteReconKomponen writes the workbook and returns the Detail row count (one per NIK).
// The workbook holds real NIKs and salaries: never commit it.
func WriteReconKomponen(d RunData, w io.Writer) (int, error) {
	f := excelize.NewFile()
	defer f.Close() //nolint:errcheck // in-memory workbook
	if err := f.SetSheetName("Sheet1", SheetDetail); err != nil {
		return 0, fmt.Errorf("rename sheet: %w", err)
	}
	if _, err := f.NewSheet(SheetSummary); err != nil {
		return 0, fmt.Errorf("new sheet: %w", err)
	}
	sw := &sheetWriter{f: f}
	sw.detail(d)
	sw.summary(d)
	if sw.err != nil {
		return 0, sw.err
	}
	if err := f.Write(w); err != nil {
		return 0, err
	}
	return len(d.Emps), nil
}

type sheetWriter struct {
	f     *excelize.File
	sheet string
	row   int
	err   error
}

func (s *sheetWriter) start(name string) { s.sheet, s.row = name, 0 }

// put writes one row; string, int, decimal.Decimal; a nil value leaves the cell empty.
func (s *sheetWriter) put(vals ...any) {
	s.row++
	for i, v := range vals {
		if s.err != nil || v == nil {
			continue
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
	dv, ok := v.(decimal.Decimal)
	if !ok {
		return v
	}
	str := dv.String()
	if len(strings.NewReplacer("-", "", ".", "").Replace(str)) > maxNumericDigits {
		return str
	}
	n, err := strconv.ParseFloat(str, 64)
	if err != nil {
		return str
	}
	return n
}

func nz(v decimal.Decimal) any {
	if v.IsZero() {
		return nil
	}
	return v
}

func (s *sheetWriter) detail(d RunData) {
	s.start(SheetDetail)
	hdr := []any{"NIK", "Nama", "Dept"}
	for _, c := range d.Comps {
		hdr = append(hdr, c.Key()+" "+c.Name)
	}
	hdr = append(hdr, "Tot Tunjangan", "Tot Potongan", "Rounding", "Net Pay")
	s.put(hdr...)
	for _, e := range d.Emps {
		row := []any{e.NIK, e.Nama, e.Dept}
		for _, c := range d.Comps {
			row = append(row, nz(e.Amounts[c.Key()]))
		}
		row = append(row, e.TotTunjangan, e.TotPotongan, nz(e.Rounding), e.NetPay)
		s.put(row...)
	}
}

func (s *sheetWriter) summary(d RunData) {
	sum := Summarize(d)
	s.start(SheetSummary)
	s.put("Komponen", "Nama", "Jumlah NIK", "Total")
	for _, c := range d.Comps {
		s.put(c.Key(), c.Name, sum.CountPerComp[c.Key()], sum.PerComp[c.Key()])
	}
	s.put()
	s.put("Jumlah NIK", sum.NIKCount)
	s.put("Σ Tunjangan (komponen T)", sum.SumT)
	s.put("Σ Tunjangan (head)", sum.HeadTunj)
	s.put("Selisih tunjangan", sum.SumT.Sub(sum.HeadTunj))
	s.put("Σ Potongan (komponen P)", sum.SumP)
	s.put("Σ Potongan (head)", sum.HeadPot)
	s.put("Selisih potongan", sum.SumP.Sub(sum.HeadPot))
	s.put("Σ Rounding", sum.Rounding)
	s.put("Σ Net Pay (head)", sum.HeadNet)
	s.put("Net Pay = T − P − Rounding", sum.HeadTunj.Sub(sum.HeadPot).Sub(sum.Rounding))
}
