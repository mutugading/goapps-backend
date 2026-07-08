// Package costproductrequest holds application use cases for the Phase A request aggregate.
package costproductrequest

import (
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"
)

// TemplateResult represents the download template result.
type TemplateResult struct {
	FileContent []byte
	FileName    string
}

// TemplateHandler handles the GetCostProductRequestImportTemplate query.
type TemplateHandler struct{}

// NewTemplateHandler creates a new TemplateHandler.
func NewTemplateHandler() *TemplateHandler {
	return &TemplateHandler{}
}

// templateExcelWriter wraps excelize file with error collection for template generation.
type templateExcelWriter struct {
	f         *excelize.File
	sheetName string
	errs      []error
}

func (tw *templateExcelWriter) setCellValue(cell string, value interface{}) {
	if err := tw.f.SetCellValue(tw.sheetName, cell, value); err != nil {
		tw.errs = append(tw.errs, fmt.Errorf("cell %s: %w", cell, err))
	}
}

func (tw *templateExcelWriter) setColWidth(startCol, endCol string, width float64) {
	if err := tw.f.SetColWidth(tw.sheetName, startCol, endCol, width); err != nil {
		tw.errs = append(tw.errs, fmt.Errorf("column %s-%s: %w", startCol, endCol, err))
	}
}

func (tw *templateExcelWriter) hasErrors() bool { return len(tw.errs) > 0 }

func (tw *templateExcelWriter) error() error {
	if len(tw.errs) == 0 {
		return nil
	}
	return errors.Join(tw.errs...)
}

// templateSheetName is capped at 31 characters — Excel's sheet name limit —
// so it cannot spell out the full "Cost Product Request Import Template".
const templateSheetName = "CPR Import Template"

// setupTemplateSheet creates sheetName, deletes the default Sheet1, writes
// the D6 header row, and applies the bold header style.
func setupTemplateSheet(f *excelize.File, sheetName string) error {
	index, err := f.NewSheet(sheetName)
	if err != nil {
		return fmt.Errorf("failed to create sheet: %w", err)
	}
	f.SetActiveSheet(index)
	if deleteErr := f.DeleteSheet("Sheet1"); deleteErr != nil {
		log.Debug().Err(deleteErr).Msg("Could not delete default Sheet1")
	}
	if err := writeHeaderRow(f, sheetName, exportHeaders); err != nil {
		return err
	}
	headerStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"4472C4"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	if err != nil {
		return fmt.Errorf("failed to create header style: %w", err)
	}
	lastCol, err := excelize.ColumnNumberToName(len(exportHeaders))
	if err != nil {
		return fmt.Errorf("failed to compute last column: %w", err)
	}
	if err := f.SetCellStyle(sheetName, "A1", lastCol+"1", headerStyle); err != nil {
		return fmt.Errorf("failed to set header style: %w", err)
	}
	return nil
}

// writeTemplateSampleRow writes one illustrative sample row (row 2) and sets
// a readable column width, collecting any per-cell errors on writer.
func writeTemplateSampleRow(writer *templateExcelWriter) {
	sampleRow := []string{
		"STANDARD", "Sample cost request", "Optional description", "Acme Corp", "ACME001",
		"medium", "2026-08-01", "50mm x 100m PET film", "SH-001", "Sky Blue",
		"PAPER", "", "10000", "1.20-1.35 USD/unit",
	}
	for col, v := range sampleRow {
		cell, cellErr := excelize.CoordinatesToCellName(col+1, 2)
		if cellErr != nil {
			writer.errs = append(writer.errs, fmt.Errorf("cell coord col %d: %w", col+1, cellErr))
			continue
		}
		writer.setCellValue(cell, v)
	}
	for _, col := range []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M", "N"} {
		writer.setColWidth(col, col, 22)
	}
}

// writeTemplateInstructionsSheet creates the "Instructions" sheet on f,
// explaining each column's resolution rule and the create-only import
// policy. Logs (does not fail) on any error, matching the writer pattern
// used elsewhere in this handler.
func writeTemplateInstructionsSheet(f *excelize.File) {
	const notesSheet = "Instructions"
	if _, sheetErr := f.NewSheet(notesSheet); sheetErr != nil {
		log.Debug().Err(sheetErr).Msg("Could not create Instructions sheet")
		return
	}
	notesWriter := &templateExcelWriter{f: f, sheetName: notesSheet}
	notesWriter.setCellValue("A1", "Import Instructions")
	notesWriter.setCellValue("A3", "1. Request type: must match an existing Request Type code (e.g., STANDARD)")
	notesWriter.setCellValue("A4", "2. Title, Customer name: required")
	notesWriter.setCellValue("A5", "3. Urgency: low / medium / high (default: medium)")
	notesWriter.setCellValue("A6", "4. Needed by: YYYY-MM-DD, optional")
	notesWriter.setCellValue("A7", "5. Product description: required")
	notesWriter.setCellValue("A8", "6. Shade code or Shade name: at least one required")
	notesWriter.setCellValue("A9", "7. Tube: PAPER, PLASTIC, or blank")
	notesWriter.setCellValue("A10", "8. Reference product: optional, must match an existing product code if set")
	notesWriter.setCellValue("A12", "Notes:")
	notesWriter.setCellValue("A13", "- Each row creates a new draft request. There is no update/skip/dedup — importing the same row twice creates two drafts.")
	notesWriter.setCellValue("A14", "- Delete the sample data row before importing.")
	notesWriter.setCellValue("A15", "- Save file as .xlsx format.")
	if notesWriter.hasErrors() {
		log.Warn().Err(notesWriter.error()).Msg("Some Instructions sheet operations failed")
	}
}

// Handle generates the import template Excel file.
func (h *TemplateHandler) Handle() (result *TemplateResult, err error) {
	f := excelize.NewFile()
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("Failed to close Excel template file")
			if err == nil {
				err = fmt.Errorf("failed to close file: %w", closeErr)
			}
		}
	}()

	if err := setupTemplateSheet(f, templateSheetName); err != nil {
		return nil, err
	}

	writer := &templateExcelWriter{f: f, sheetName: templateSheetName}
	writeTemplateSampleRow(writer)
	writeTemplateInstructionsSheet(f)
	if writer.hasErrors() {
		log.Warn().Err(writer.error()).Msg("Some Excel formatting operations failed")
	}

	buffer, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("failed to write excel to buffer: %w", err)
	}

	return &TemplateResult{
		FileContent: buffer.Bytes(),
		FileName:    "cost_product_request_import_template.xlsx",
	}, nil
}
