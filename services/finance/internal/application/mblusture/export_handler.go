package mblusture

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mblusture"
)

// ExportQuery represents the export MB lustures query.
type ExportQuery struct {
	IsActive *bool
}

// ExportResult represents the export MB lustures result.
type ExportResult struct {
	FileContent []byte
	FileName    string
}

// ExportHandler handles the ExportMbLusture query.
type ExportHandler struct {
	repo mblusture.Repository
}

// NewExportHandler creates a new ExportHandler.
func NewExportHandler(repo mblusture.Repository) *ExportHandler {
	return &ExportHandler{repo: repo}
}

// excelWriter wraps excelize file with error collection for non-critical operations.
type excelWriter struct {
	f         *excelize.File
	sheetName string
	errs      []error
}

func (ew *excelWriter) setCellValue(cell string, value any) {
	if err := ew.f.SetCellValue(ew.sheetName, cell, value); err != nil {
		ew.errs = append(ew.errs, fmt.Errorf("cell %s: %w", cell, err))
	}
}

func (ew *excelWriter) setColWidth(startCol, endCol string, width float64) {
	if err := ew.f.SetColWidth(ew.sheetName, startCol, endCol, width); err != nil {
		ew.errs = append(ew.errs, fmt.Errorf("column %s-%s: %w", startCol, endCol, err))
	}
}

func (ew *excelWriter) hasErrors() bool {
	return len(ew.errs) > 0
}

func (ew *excelWriter) error() error {
	if len(ew.errs) == 0 {
		return nil
	}
	return errors.Join(ew.errs...)
}

// mbLustureExportHeaders lists the export column headers in column order.
var mbLustureExportHeaders = []string{
	"No", "Code", "Display Name", "Full Description", "Category",
	"Display Order", "Active", "Created At", "Created By",
}

func setupLustureExcelSheet(f *excelize.File, sheetName string) error {
	index, err := f.NewSheet(sheetName)
	if err != nil {
		return fmt.Errorf("failed to create sheet: %w", err)
	}
	f.SetActiveSheet(index)

	if deleteErr := f.DeleteSheet("Sheet1"); deleteErr != nil {
		log.Debug().Err(deleteErr).Msg("Could not delete default Sheet1")
	}

	for col, header := range mbLustureExportHeaders {
		cell, err := excelize.CoordinatesToCellName(col+1, 1)
		if err != nil {
			return fmt.Errorf("failed to get cell name: %w", err)
		}
		if err := f.SetCellValue(sheetName, cell, header); err != nil {
			return fmt.Errorf("failed to set header %s: %w", header, err)
		}
	}

	lastCol, err := excelize.CoordinatesToCellName(len(mbLustureExportHeaders), 1)
	if err != nil {
		return fmt.Errorf("failed to get last header cell name: %w", err)
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

// writeMbLustureRow writes a single lusture entity's fields into the given Excel row.
func writeMbLustureRow(writer *excelWriter, row int, idx int, e *mblusture.Entity) {
	writer.setCellValue(fmt.Sprintf("A%d", row), idx+1)
	writer.setCellValue(fmt.Sprintf("B%d", row), e.Code())
	writer.setCellValue(fmt.Sprintf("C%d", row), e.DisplayName())
	writer.setCellValue(fmt.Sprintf("D%d", row), e.FullDescription())
	writer.setCellValue(fmt.Sprintf("E%d", row), e.Category())
	writer.setCellValue(fmt.Sprintf("F%d", row), e.DisplayOrder())
	writer.setCellValue(fmt.Sprintf("G%d", row), e.IsActive())
	writer.setCellValue(fmt.Sprintf("H%d", row), e.CreatedAt())
	writer.setCellValue(fmt.Sprintf("I%d", row), e.CreatedBy())
}

// Handle executes the export MB lustures query.
func (h *ExportHandler) Handle(ctx context.Context, query ExportQuery) (result *ExportResult, err error) {
	lustures, err := h.repo.ListAll(ctx, mblusture.ExportFilter{IsActive: query.IsActive})
	if err != nil {
		return nil, fmt.Errorf("failed to get mb lustures for export: %w", err)
	}

	f := excelize.NewFile()
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("Failed to close Excel file")
			if err == nil {
				err = fmt.Errorf("failed to close file: %w", closeErr)
			}
		}
	}()

	sheetName := "MB Lustures"
	if err := setupLustureExcelSheet(f, sheetName); err != nil {
		return nil, err
	}

	writer := &excelWriter{f: f, sheetName: sheetName}

	for i, e := range lustures {
		writeMbLustureRow(writer, i+2, i, e)
	}

	writer.setColWidth("A", "A", 5)
	writer.setColWidth("B", "B", 15)
	writer.setColWidth("C", "D", 25)
	writer.setColWidth("E", "E", 15)
	writer.setColWidth("F", "G", 12)
	writer.setColWidth("H", "I", 20)

	if writer.hasErrors() {
		log.Warn().Err(writer.error()).Msg("Some Excel formatting operations failed")
	}

	buffer, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("failed to write excel to buffer: %w", err)
	}

	return &ExportResult{
		FileContent: buffer.Bytes(),
		FileName:    "mb_lusture_export.xlsx",
	}, nil
}
