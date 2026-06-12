// Package costproducttype contains application use cases for CostProductType.
package costproducttype

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproducttype"
)

// ExportQuery represents the export CostProductTypes query.
type ExportQuery struct {
	ActiveFilter string // "all" | "active" | "inactive" | ""
}

// ExportResult represents the export CostProductTypes result.
type ExportResult struct {
	FileContent []byte
	FileName    string
}

// ExportHandler handles the ExportCostProductTypes query.
type ExportHandler struct {
	repo domain.Repository
}

// NewExportHandler creates a new ExportHandler.
func NewExportHandler(repo domain.Repository) *ExportHandler {
	return &ExportHandler{repo: repo}
}

// Handle executes the export CostProductTypes query.
func (h *ExportHandler) Handle(ctx context.Context, query ExportQuery) (result *ExportResult, err error) {
	items, _, err := h.repo.List(ctx, domain.Filter{
		ActiveFilter: query.ActiveFilter,
		Page:         1,
		PageSize:     100000,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get cost product types for export: %w", err)
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

	sheetName := "cost_product_type"
	if err := setupCPTExcelSheet(f, sheetName); err != nil {
		return nil, err
	}

	writer := &cptExcelWriter{f: f, sheetName: sheetName}

	for i, item := range items {
		row := i + 2
		writer.setCellValue(fmt.Sprintf("A%d", row), i+1)
		writer.setCellValue(fmt.Sprintf("B%d", row), item.TypeCode())
		writer.setCellValue(fmt.Sprintf("C%d", row), item.TypeName())
		writer.setCellValue(fmt.Sprintf("D%d", row), item.IsActive())
		writer.setCellValue(fmt.Sprintf("E%d", row), item.CreatedAt().Format("2006-01-02 15:04:05"))
	}

	writer.setColWidth("A", "A", 5)
	writer.setColWidth("B", "B", 15)
	writer.setColWidth("C", "C", 30)
	writer.setColWidth("D", "D", 10)
	writer.setColWidth("E", "E", 20)

	if writer.hasErrors() {
		log.Warn().Err(writer.error()).Msg("Some Excel formatting operations failed")
	}

	buffer, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("failed to write excel to buffer: %w", err)
	}

	return &ExportResult{
		FileContent: buffer.Bytes(),
		FileName:    "cost_product_type_export.xlsx",
	}, nil
}

// setupCPTExcelSheet creates and configures the export sheet.
func setupCPTExcelSheet(f *excelize.File, sheetName string) error {
	index, err := f.NewSheet(sheetName)
	if err != nil {
		return fmt.Errorf("failed to create sheet: %w", err)
	}
	f.SetActiveSheet(index)

	if deleteErr := f.DeleteSheet("Sheet1"); deleteErr != nil {
		log.Debug().Err(deleteErr).Msg("Could not delete default Sheet1")
	}

	headers := []string{"No", "cpt_type_code", "cpt_type_name", "cpt_is_active", "Created At"}
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
	if err := f.SetCellStyle(sheetName, "A1", "E1", headerStyle); err != nil {
		return fmt.Errorf("failed to set header style: %w", err)
	}

	return nil
}
