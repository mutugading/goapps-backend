// Package uom provides application layer handlers for UOM operations.
package uom

import (
	"fmt"

	"github.com/xuri/excelize/v2"
)

// TemplateResult represents the download template result.
type TemplateResult struct {
	FileContent []byte
	FileName    string
}

// TemplateHandler handles the DownloadTemplate query.
type TemplateHandler struct{}

// NewTemplateHandler creates a new TemplateHandler.
func NewTemplateHandler() *TemplateHandler {
	return &TemplateHandler{}
}

// Handle generates the import template Excel file.
func (h *TemplateHandler) Handle() (*TemplateResult, error) {
	// Create Excel file
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	sheetName := "UOM Import Template"
	index, _ := f.NewSheet(sheetName)
	f.SetActiveSheet(index)
	_ = f.DeleteSheet("Sheet1")

	// Set headers
	headers := []string{"Code", "Name", "Category", "Description"}
	for col, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(col+1, 1)
		_ = f.SetCellValue(sheetName, cell, header)
	}

	// Style headers
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"4472C4"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	_ = f.SetCellStyle(sheetName, "A1", "D1", headerStyle)

	// Add sample data
	sampleData := [][]string{
		{"KG", "Kilogram", "WEIGHT", "Weight in kilograms"},
		{"MTR", "Meter", "LENGTH", "Length in meters"},
		{"LTR", "Liter", "VOLUME", "Volume in liters"},
		{"PCS", "Pieces", "QUANTITY", "Count in pieces"},
	}

	for i, row := range sampleData {
		rowNum := i + 2
		_ = f.SetCellValue(sheetName, fmt.Sprintf("A%d", rowNum), row[0])
		_ = f.SetCellValue(sheetName, fmt.Sprintf("B%d", rowNum), row[1])
		_ = f.SetCellValue(sheetName, fmt.Sprintf("C%d", rowNum), row[2])
		_ = f.SetCellValue(sheetName, fmt.Sprintf("D%d", rowNum), row[3])
	}

	// Set column widths
	_ = f.SetColWidth(sheetName, "A", "A", 15)
	_ = f.SetColWidth(sheetName, "B", "B", 25)
	_ = f.SetColWidth(sheetName, "C", "C", 15)
	_ = f.SetColWidth(sheetName, "D", "D", 40)

	// Add validation note sheet
	notesSheet := "Instructions"
	_, _ = f.NewSheet(notesSheet)
	_ = f.SetCellValue(notesSheet, "A1", "Import Instructions")
	_ = f.SetCellValue(notesSheet, "A3", "1. Code: Unique, uppercase letters/numbers/underscores (e.g., KG, MTR_SQ)")
	_ = f.SetCellValue(notesSheet, "A4", "2. Name: Display name (required)")
	_ = f.SetCellValue(notesSheet, "A5", "3. Category: Must be one of: WEIGHT, LENGTH, VOLUME, QUANTITY")
	_ = f.SetCellValue(notesSheet, "A6", "4. Description: Optional description")
	_ = f.SetCellValue(notesSheet, "A8", "Notes:")
	_ = f.SetCellValue(notesSheet, "A9", "- Delete sample data rows before importing")
	_ = f.SetCellValue(notesSheet, "A10", "- Save file as .xlsx format")

	// Write to buffer
	buffer, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("failed to write excel to buffer: %w", err)
	}

	return &TemplateResult{
		FileContent: buffer.Bytes(),
		FileName:    "uom_import_template.xlsx",
	}, nil
}
