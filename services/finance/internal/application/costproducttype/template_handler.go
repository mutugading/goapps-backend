// Package costproducttype contains application use cases for CostProductType.
package costproducttype

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

// TemplateHandler handles the DownloadCostProductTypeTemplate query.
type TemplateHandler struct{}

// NewTemplateHandler creates a new TemplateHandler.
func NewTemplateHandler() *TemplateHandler {
	return &TemplateHandler{}
}

// cptExcelWriter wraps excelize file with error collection for non-critical operations.
type cptExcelWriter struct {
	f         *excelize.File
	sheetName string
	errs      []error
}

// setCellValue sets a cell value and collects any error.
func (ew *cptExcelWriter) setCellValue(cell string, value interface{}) {
	if err := ew.f.SetCellValue(ew.sheetName, cell, value); err != nil {
		ew.errs = append(ew.errs, fmt.Errorf("cell %s: %w", cell, err))
	}
}

// setColWidth sets column width and collects any error.
func (ew *cptExcelWriter) setColWidth(startCol, endCol string, width float64) {
	if err := ew.f.SetColWidth(ew.sheetName, startCol, endCol, width); err != nil {
		ew.errs = append(ew.errs, fmt.Errorf("column %s-%s: %w", startCol, endCol, err))
	}
}

// hasErrors returns true if any errors were collected.
func (ew *cptExcelWriter) hasErrors() bool {
	return len(ew.errs) > 0
}

// error returns combined errors or nil.
func (ew *cptExcelWriter) error() error {
	if len(ew.errs) == 0 {
		return nil
	}
	return errors.Join(ew.errs...)
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

	sheetName := "cost_product_type"
	index, err := f.NewSheet(sheetName)
	if err != nil {
		return nil, fmt.Errorf("failed to create sheet: %w", err)
	}
	f.SetActiveSheet(index)

	if deleteErr := f.DeleteSheet("Sheet1"); deleteErr != nil {
		log.Debug().Err(deleteErr).Msg("Could not delete default Sheet1")
	}

	if err := writeCPTTemplateHeaders(f, sheetName); err != nil {
		return nil, err
	}

	writer := &cptExcelWriter{f: f, sheetName: sheetName}
	writeCPTSampleData(writer)
	writeCPTColumnWidths(writer)

	if writer.hasErrors() {
		log.Warn().Err(writer.error()).Msg("Some Excel formatting operations failed")
	}

	writeCPTInstructionsSheet(f)

	buffer, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("failed to write excel to buffer: %w", err)
	}

	return &TemplateResult{
		FileContent: buffer.Bytes(),
		FileName:    "cost_product_type_import_template.xlsx",
	}, nil
}

func writeCPTTemplateHeaders(f *excelize.File, sheetName string) error {
	headers := []string{"cpt_type_code", "cpt_type_name", "cpt_is_active"}
	for col, header := range headers {
		cell, cellErr := excelize.CoordinatesToCellName(col+1, 1)
		if cellErr != nil {
			return fmt.Errorf("failed to get cell name: %w", cellErr)
		}
		if cellErr := f.SetCellValue(sheetName, cell, header); cellErr != nil {
			return fmt.Errorf("failed to set header %s: %w", header, cellErr)
		}
	}

	headerStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"4472C4"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	if err != nil {
		return fmt.Errorf("failed to create header style: %w", err)
	}
	if err := f.SetCellStyle(sheetName, "A1", "C1", headerStyle); err != nil {
		return fmt.Errorf("failed to set header style: %w", err)
	}
	return nil
}

func writeCPTSampleData(writer *cptExcelWriter) {
	sampleData := [][]interface{}{
		{"POY", "Partially Oriented Yarn", true},
		{"PTY", "Polyester Textured Yarn", true},
	}

	for i, row := range sampleData {
		rowNum := i + 2
		for j, val := range row {
			cell, cellErr := excelize.CoordinatesToCellName(j+1, rowNum)
			if cellErr != nil {
				writer.errs = append(writer.errs, fmt.Errorf("coordinates row %d col %d: %w", rowNum, j+1, cellErr))
				continue
			}
			writer.setCellValue(cell, val)
		}
	}
}

func writeCPTColumnWidths(writer *cptExcelWriter) {
	writer.setColWidth("A", "A", 15)
	writer.setColWidth("B", "B", 30)
	writer.setColWidth("C", "C", 12)
}

func writeCPTInstructionsSheet(f *excelize.File) {
	notesSheet := "Instructions"
	if _, sheetErr := f.NewSheet(notesSheet); sheetErr != nil {
		log.Debug().Err(sheetErr).Msg("Could not create Instructions sheet")
		return
	}

	notesWriter := &cptExcelWriter{f: f, sheetName: notesSheet}
	notesWriter.setCellValue("A1", "Import Instructions")
	notesWriter.setCellValue("A3", "1. cpt_type_code: Unique code, uppercase letters and digits only, starts with letter, max 5 chars (e.g., POY, PTY)")
	notesWriter.setCellValue("A4", "2. cpt_type_name: Display name (required, max 100 chars)")
	notesWriter.setCellValue("A5", "3. cpt_is_active: Optional active flag (true/false, yes/no, 1/0, active/inactive). Defaults to true on create.")
	notesWriter.setCellValue("A7", "Notes:")
	notesWriter.setCellValue("A8", "- Delete sample data rows before importing")
	notesWriter.setCellValue("A9", "- Save file as .xlsx format")
	notesWriter.setCellValue("A10", "- Duplicate codes are handled based on the selected duplicate action (skip / update / error)")

	if notesWriter.hasErrors() {
		log.Warn().Err(notesWriter.error()).Msg("Some Instructions sheet operations failed")
	}
}
