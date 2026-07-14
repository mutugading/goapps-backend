package mblusture

import (
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"
)

// TemplateResult represents the download template result.
type TemplateResult struct {
	FileContent []byte
	FileName    string
}

// TemplateHandler handles the DownloadMbLustureTemplate query.
type TemplateHandler struct{}

// NewTemplateHandler creates a new TemplateHandler.
func NewTemplateHandler() *TemplateHandler {
	return &TemplateHandler{}
}

// templateExcelWriter wraps excelize file with error collection for non-critical operations.
type templateExcelWriter struct {
	f         *excelize.File
	sheetName string
	errs      []error
}

func (tw *templateExcelWriter) setCellValue(cell string, value any) {
	if err := tw.f.SetCellValue(tw.sheetName, cell, value); err != nil {
		tw.errs = append(tw.errs, fmt.Errorf("cell %s: %w", cell, err))
	}
}

func (tw *templateExcelWriter) setColWidth(startCol, endCol string, width float64) {
	if err := tw.f.SetColWidth(tw.sheetName, startCol, endCol, width); err != nil {
		tw.errs = append(tw.errs, fmt.Errorf("column %s-%s: %w", startCol, endCol, err))
	}
}

func (tw *templateExcelWriter) hasErrors() bool {
	return len(tw.errs) > 0
}

// mbLustureTemplateHeaders lists the import template column headers in column order.
var mbLustureTemplateHeaders = []string{
	"Code", "Display Name", "Full Description", "Category", "Display Order", "Active",
}

// mbLustureTemplateSampleData holds sample rows matching mbLustureTemplateHeaders column order.
var mbLustureTemplateSampleData = [][]string{
	{"LC-01", "Bright Round", "Bright round cross-section lusture", "ROUND", "1", "TRUE"},
	{"LC-02", "Semi Dull", "Semi-dull finish lusture", "ROUND", "2", "TRUE"},
}

// writeMbLustureTemplateHeaders writes and styles the header row.
func writeMbLustureTemplateHeaders(f *excelize.File, sheetName string) error {
	lastCol, err := excelize.CoordinatesToCellName(len(mbLustureTemplateHeaders), 1)
	if err != nil {
		return fmt.Errorf("failed to get last header cell name: %w", err)
	}

	for col, header := range mbLustureTemplateHeaders {
		cell, err := excelize.CoordinatesToCellName(col+1, 1)
		if err != nil {
			return fmt.Errorf("failed to get cell name: %w", err)
		}
		if err := f.SetCellValue(sheetName, cell, header); err != nil {
			return fmt.Errorf("failed to set header %s: %w", header, err)
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
	if err := f.SetCellStyle(sheetName, "A1", lastCol+"1", headerStyle); err != nil {
		return fmt.Errorf("failed to set header style: %w", err)
	}

	return nil
}

// writeMbLustureTemplateSampleRows writes the sample data rows into the writer's sheet.
func writeMbLustureTemplateSampleRows(writer *templateExcelWriter) {
	for i, sample := range mbLustureTemplateSampleData {
		row := i + 2
		for col, value := range sample {
			cell, cellErr := excelize.CoordinatesToCellName(col+1, row)
			if cellErr != nil {
				writer.errs = append(writer.errs, fmt.Errorf("row %d col %d: %w", row, col, cellErr))
				continue
			}
			writer.setCellValue(cell, value)
		}
	}
}

// Handle builds the MB lusture import template Excel file.
func (h *TemplateHandler) Handle() (result *TemplateResult, err error) {
	f := excelize.NewFile()
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("Failed to close Excel file")
			if err == nil {
				err = fmt.Errorf("failed to close file: %w", closeErr)
			}
		}
	}()

	sheetName := "MB Lusture Import Template"
	index, err := f.NewSheet(sheetName)
	if err != nil {
		return nil, fmt.Errorf("failed to create sheet: %w", err)
	}
	f.SetActiveSheet(index)

	if deleteErr := f.DeleteSheet("Sheet1"); deleteErr != nil {
		log.Debug().Err(deleteErr).Msg("Could not delete default Sheet1")
	}

	if err := writeMbLustureTemplateHeaders(f, sheetName); err != nil {
		return nil, err
	}

	writer := &templateExcelWriter{f: f, sheetName: sheetName}
	writeMbLustureTemplateSampleRows(writer)

	writer.setColWidth("A", "A", 15)
	writer.setColWidth("B", "C", 25)
	writer.setColWidth("D", "D", 15)
	writer.setColWidth("E", "F", 12)

	if writer.hasErrors() {
		log.Warn().Errs("errors", writer.errs).Msg("Some Excel formatting operations failed")
	}

	if instrErr := addMbLustureInstructionsSheet(f); instrErr != nil {
		log.Debug().Err(instrErr).Msg("Could not add instructions sheet")
	}

	buffer, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("failed to write excel to buffer: %w", err)
	}

	return &TemplateResult{
		FileContent: buffer.Bytes(),
		FileName:    "mb_lusture_import_template.xlsx",
	}, nil
}

// addMbLustureInstructionsSheet adds a non-critical instructions sheet to the template.
func addMbLustureInstructionsSheet(f *excelize.File) error {
	sheetName := "Instructions"
	if _, err := f.NewSheet(sheetName); err != nil {
		return fmt.Errorf("failed to create instructions sheet: %w", err)
	}

	instructions := []string{
		"MB Lusture Import Instructions",
		"",
		"1. Code is required and must be unique.",
		"2. Display Name, Full Description, Category are plain text fields.",
		"3. Display Order is numeric — leave blank to default to 0.",
		"4. Active must be TRUE or FALSE.",
		"5. Duplicate codes are handled per the duplicate action selected on import (skip, update, or error).",
	}

	for i, line := range instructions {
		cell, err := excelize.CoordinatesToCellName(1, i+1)
		if err != nil {
			return fmt.Errorf("failed to get instructions cell name: %w", err)
		}
		if err := f.SetCellValue(sheetName, cell, line); err != nil {
			return fmt.Errorf("failed to set instructions cell: %w", err)
		}
	}

	if err := f.SetColWidth(sheetName, "A", "A", 80); err != nil {
		return fmt.Errorf("failed to set instructions column width: %w", err)
	}

	return nil
}
