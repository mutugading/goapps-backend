package costbulkimport

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/xuri/excelize/v2"
)

// unknownParamPrefix is the message prefix written by processCPP, processCAP,
// and preflightParamSheet when a param_code is not found in the parameter master.
// Used to de-duplicate these errors into the dedicated "missing_param_codes" sheet.
const unknownParamPrefix = "unknown param code: "

// SheetResult summarizes processing outcomes for one Excel sheet.
type SheetResult struct {
	SheetName string
	TotalRows int
	Inserted  int
	Updated   int
	Skipped   int
	Errors    []SheetError
}

// GenerateErrorReport builds a multi-sheet Excel error report in memory.
//
// Sheets produced:
//   - "summary"             — per-sheet totals (all error types counted)
//   - "<sheet>_errors"      — row-level errors, excluding unknown-param-code rows
//   - "missing_param_codes" — unique unknown param codes with skipped-row counts
//
// Unknown param code errors are separated into the dedicated sheet so the
// operator sees a clean, de-duplicated list rather than potentially millions
// of individual row entries for the same handful of missing codes.
func GenerateErrorReport(results []SheetResult) ([]byte, error) {
	f := excelize.NewFile()
	defer func() {
		if err := f.Close(); err != nil {
			_ = err
		}
	}()

	const summarySheet = "summary"
	if setErr := f.SetSheetName("Sheet1", summarySheet); setErr != nil {
		return nil, fmt.Errorf("set summary sheet: %w", setErr)
	}
	if err := writeSummarySheet(f, summarySheet, results); err != nil {
		return nil, err
	}

	// Per-sheet error tabs — unknown-param-code rows excluded (see missing_param_codes).
	for _, r := range results {
		filtered := filterOutUnknownParamErrors(r.Errors)
		if len(filtered) == 0 {
			continue
		}
		copy := r
		copy.Errors = filtered
		if err := writeErrorSheet(f, copy); err != nil {
			return nil, err
		}
	}

	// Dedicated sheet for unique unknown param codes.
	if codes := collectUnknownParamCodes(results); len(codes) > 0 {
		if err := writeMissingParamCodesSheet(f, codes); err != nil {
			return nil, err
		}
	}

	var buf bytes.Buffer
	if writeErr := f.Write(&buf); writeErr != nil {
		return nil, fmt.Errorf("write error report: %w", writeErr)
	}
	return buf.Bytes(), nil
}

func writeSummarySheet(f *excelize.File, sheetName string, results []SheetResult) error {
	headers := []string{"sheet_name", "total_rows", "inserted", "updated", "skipped", "errors"}
	for col, h := range headers {
		cell, cellErr := excelize.CoordinatesToCellName(col+1, 1)
		if cellErr != nil {
			return fmt.Errorf("coordinates to cell name: %w", cellErr)
		}
		if err := f.SetCellValue(sheetName, cell, h); err != nil {
			return fmt.Errorf("set cell %s: %w", cell, err)
		}
	}
	for row, r := range results {
		rowIdx := row + 2
		vals := []any{r.SheetName, r.TotalRows, r.Inserted, r.Updated, r.Skipped, len(r.Errors)}
		for col, v := range vals {
			cell, cellErr := excelize.CoordinatesToCellName(col+1, rowIdx)
			if cellErr != nil {
				return fmt.Errorf("coordinates to cell name: %w", cellErr)
			}
			if err := f.SetCellValue(sheetName, cell, v); err != nil {
				return fmt.Errorf("set cell %s: %w", cell, err)
			}
		}
	}
	return nil
}

func writeErrorSheet(f *excelize.File, r SheetResult) error {
	errSheetName := r.SheetName + "_errors"
	if _, createErr := f.NewSheet(errSheetName); createErr != nil {
		return fmt.Errorf("create sheet %q: %w", errSheetName, createErr)
	}
	errHeaders := []string{"row_number", "field", "message"}
	for col, h := range errHeaders {
		cell, cellErr := excelize.CoordinatesToCellName(col+1, 1)
		if cellErr != nil {
			return fmt.Errorf("coordinates to cell name: %w", cellErr)
		}
		if err := f.SetCellValue(errSheetName, cell, h); err != nil {
			return fmt.Errorf("set cell %s: %w", cell, err)
		}
	}
	for rowIdx, e := range r.Errors {
		vals := []any{e.RowNumber, e.Field, e.Message}
		for col, v := range vals {
			cell, cellErr := excelize.CoordinatesToCellName(col+1, rowIdx+2)
			if cellErr != nil {
				return fmt.Errorf("coordinates to cell name: %w", cellErr)
			}
			if err := f.SetCellValue(errSheetName, cell, v); err != nil {
				return fmt.Errorf("set cell %s: %w", cell, err)
			}
		}
	}
	return nil
}

// writeMissingParamCodesSheet writes a dedicated summary sheet listing every
// unique unknown param code and how many rows were skipped because of it.
func writeMissingParamCodesSheet(f *excelize.File, codes map[string]int) error {
	const sheetName = "missing_param_codes"
	if _, createErr := f.NewSheet(sheetName); createErr != nil {
		return fmt.Errorf("create sheet %q: %w", sheetName, createErr)
	}
	headers := []string{"param_code", "skipped_rows", "action_required"}
	for col, h := range headers {
		cell, cellErr := excelize.CoordinatesToCellName(col+1, 1)
		if cellErr != nil {
			return fmt.Errorf("coordinates to cell name: %w", cellErr)
		}
		if err := f.SetCellValue(sheetName, cell, h); err != nil {
			return fmt.Errorf("set header %s: %w", cell, err)
		}
	}
	sorted := make([]string, 0, len(codes))
	for c := range codes {
		sorted = append(sorted, c)
	}
	sort.Strings(sorted)
	for rowIdx, code := range sorted {
		rowNum := rowIdx + 2
		vals := []any{code, codes[code], "Create or map in Finance > Master > Parameter, then re-import"}
		for col, v := range vals {
			cell, cellErr := excelize.CoordinatesToCellName(col+1, rowNum)
			if cellErr != nil {
				return fmt.Errorf("coordinates to cell name: %w", cellErr)
			}
			if err := f.SetCellValue(sheetName, cell, v); err != nil {
				return fmt.Errorf("set cell %s: %w", cell, err)
			}
		}
	}
	return nil
}

func collectUnknownParamCodes(results []SheetResult) map[string]int {
	codes := make(map[string]int)
	for _, r := range results {
		for _, e := range r.Errors {
			if strings.HasPrefix(e.Message, unknownParamPrefix) {
				code := strings.TrimPrefix(e.Message, unknownParamPrefix)
				codes[code]++
			}
		}
	}
	return codes
}

func filterOutUnknownParamErrors(errs []SheetError) []SheetError {
	result := make([]SheetError, 0, len(errs))
	for _, e := range errs {
		if !strings.HasPrefix(e.Message, unknownParamPrefix) {
			result = append(result, e)
		}
	}
	return result
}
