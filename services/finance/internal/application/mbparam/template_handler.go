package mbparam

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

// TemplateHandler handles the DownloadMbParamTemplate query.
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

// mbParamTemplateHeaders lists the import template column headers in column order.
var mbParamTemplateHeaders = []string{
	"Code", "Name", "Type", "Description", "Default Value", "Default Option", "Unit", "Display Order", "Active",
}

// mbParamTemplateSampleData holds sample rows matching mbParamTemplateHeaders column order.
var mbParamTemplateSampleData = [][]string{
	{"MC-01", "Moisture Content", "SCALAR", "Target moisture content percentage", "12.5", "", "%", "1", "TRUE"},
	{"FN-01", "Finish", "PICKLIST", "Yarn finish type", "", "BRIGHT", "", "2", "TRUE"},
}

// writeMbParamTemplateHeaders writes and styles the header row.
func writeMbParamTemplateHeaders(f *excelize.File, sheetName string) error {
	lastCol, err := excelize.CoordinatesToCellName(len(mbParamTemplateHeaders), 1)
	if err != nil {
		return fmt.Errorf("failed to get last header cell name: %w", err)
	}

	for col, header := range mbParamTemplateHeaders {
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

// writeMbParamTemplateSampleRows writes the sample data rows into the writer's sheet.
func writeMbParamTemplateSampleRows(writer *templateExcelWriter) {
	for i, sample := range mbParamTemplateSampleData {
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

// Handle builds the MB param import template Excel file.
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

	sheetName := "MB Param Import Template"
	index, err := f.NewSheet(sheetName)
	if err != nil {
		return nil, fmt.Errorf("failed to create sheet: %w", err)
	}
	f.SetActiveSheet(index)

	if deleteErr := f.DeleteSheet("Sheet1"); deleteErr != nil {
		log.Debug().Err(deleteErr).Msg("Could not delete default Sheet1")
	}

	if err := writeMbParamTemplateHeaders(f, sheetName); err != nil {
		return nil, err
	}

	writer := &templateExcelWriter{f: f, sheetName: sheetName}
	writeMbParamTemplateSampleRows(writer)

	writer.setColWidth("A", "A", 15)
	writer.setColWidth("B", "B", 25)
	writer.setColWidth("C", "C", 12)
	writer.setColWidth("D", "D", 30)
	writer.setColWidth("E", "F", 15)
	writer.setColWidth("G", "I", 12)

	if writer.hasErrors() {
		log.Warn().Errs("errors", writer.errs).Msg("Some Excel formatting operations failed")
	}

	if instrErr := addMbParamInstructionsSheet(f); instrErr != nil {
		log.Debug().Err(instrErr).Msg("Could not add instructions sheet")
	}

	buffer, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("failed to write excel to buffer: %w", err)
	}

	return &TemplateResult{
		FileContent: buffer.Bytes(),
		FileName:    "mb_param_import_template.xlsx",
	}, nil
}

// addMbParamInstructionsSheet adds a non-critical instructions sheet to the template.
func addMbParamInstructionsSheet(f *excelize.File) error {
	sheetName := "Instructions"
	if _, err := f.NewSheet(sheetName); err != nil {
		return fmt.Errorf("failed to create instructions sheet: %w", err)
	}

	instructions := []string{
		"MB Param Import Instructions",
		"",
		"1. Code is required and must be unique.",
		"2. Type must be SCALAR or PICKLIST.",
		"3. Name, Description, Unit are plain text fields.",
		"4. Default Value is numeric, used for SCALAR params — leave blank if not applicable.",
		"5. Default Option is the picklist option code, used for PICKLIST params — leave blank if not applicable.",
		"6. Display Order is numeric — leave blank to default to 0.",
		"7. Active must be TRUE or FALSE.",
		"8. Duplicate codes are handled per the duplicate action selected on import (skip, update, or error).",
		"9. Picklist options are not managed via this import — use the Options editor after creating the parameter.",
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
