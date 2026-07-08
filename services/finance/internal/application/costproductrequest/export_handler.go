// Package costproductrequest holds application use cases for the Phase A request aggregate.
package costproductrequest

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"

	pmDomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductrequest"
)

// exportHeaders is the D6 export/import column set (design.md §4 Area D6).
// Human-readable only — no sysIDs. Excludes the D1-removed fields (raw
// material type, box type, weight per bobbin).
var exportHeaders = []string{
	"Request type", "Title", "Description", "Customer name", "Customer code",
	"Urgency", "Needed by (YYYY-MM-DD)", "Product description", "Shade code",
	"Shade name", "Tube (Paper/Plastic)", "Reference product", "Target volume",
	"Target price range",
}

// ExportQuery represents the export cost product requests query. Filters
// mirror ListCostProductRequestsRequest's subset relevant to export.
type ExportQuery struct {
	Search        string
	Status        string
	RequestTypeID int32
}

// ExportResult represents the export result.
type ExportResult struct {
	FileContent []byte
	FileName    string
}

// ExportHandler handles the ExportCostProductRequests query. productMasterRepo
// resolves each request's reference_product_sys_id back to its human-readable
// product code (the request type code is already available on the entity via
// the cost_request_type join, so no separate lookup is needed for it).
type ExportHandler struct {
	repo              domain.Repository
	productMasterRepo pmDomain.Repository
}

// NewExportHandler creates a new ExportHandler.
func NewExportHandler(repo domain.Repository, productMasterRepo pmDomain.Repository) *ExportHandler {
	return &ExportHandler{repo: repo, productMasterRepo: productMasterRepo}
}

// exportExcelWriter wraps excelize file with error collection for non-critical operations.
type exportExcelWriter struct {
	f         *excelize.File
	sheetName string
	errs      []error
}

func (ew *exportExcelWriter) setCellValue(cell string, value interface{}) {
	if err := ew.f.SetCellValue(ew.sheetName, cell, value); err != nil {
		ew.errs = append(ew.errs, fmt.Errorf("cell %s: %w", cell, err))
	}
}

func (ew *exportExcelWriter) hasErrors() bool { return len(ew.errs) > 0 }

func (ew *exportExcelWriter) error() error {
	if len(ew.errs) == 0 {
		return nil
	}
	return errors.Join(ew.errs...)
}

// setupExportSheet creates and configures the export sheet with the D6 header row.
func setupExportSheet(f *excelize.File, sheetName string) error {
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

// writeHeaderRow writes headers into row 1 of sheetName.
func writeHeaderRow(f *excelize.File, sheetName string, headers []string) error {
	for col, header := range headers {
		cell, err := excelize.CoordinatesToCellName(col+1, 1)
		if err != nil {
			return fmt.Errorf("failed to get cell name: %w", err)
		}
		if err := f.SetCellValue(sheetName, cell, header); err != nil {
			return fmt.Errorf("failed to set header %s: %w", header, err)
		}
	}
	return nil
}

// resolveReferenceProductCode resolves req's reference product sys ID to its
// product code (empty if unset or unresolvable — export never fails on a
// dangling reference).
func (h *ExportHandler) resolveReferenceProductCode(ctx context.Context, req *domain.Request) string {
	sysID := req.ReferenceProductSysID()
	if sysID <= 0 || h.productMasterRepo == nil {
		return ""
	}
	product, err := h.productMasterRepo.GetBySysID(ctx, sysID)
	if err != nil {
		log.Warn().Err(err).Int64("reference_product_sys_id", sysID).Msg("failed to resolve reference product code for export")
		return ""
	}
	return product.ProductCode()
}

// writeExportRow writes one request's row to the sheet.
func (h *ExportHandler) writeExportRow(ctx context.Context, writer *exportExcelWriter, row int, req *domain.Request) {
	var productDescription, shadeCode, shadeName, tubeType string
	if s := req.Spec(); s != nil {
		productDescription = s.ProductDescription
		shadeCode = s.ShadeCode
		shadeName = s.ShadeName
		tubeType = s.TubeType
	}
	values := []any{
		req.RequestTypeCode(),
		req.Title(),
		req.Description(),
		req.CustomerName(),
		req.CustomerCode(),
		req.UrgencyLevel(),
		req.NeededByDate(),
		productDescription,
		shadeCode,
		shadeName,
		tubeType,
		h.resolveReferenceProductCode(ctx, req),
		req.TargetVolume(),
		req.TargetPriceRange(),
	}
	for col, v := range values {
		cell, err := excelize.CoordinatesToCellName(col+1, row)
		if err != nil {
			writer.errs = append(writer.errs, fmt.Errorf("cell coord row %d col %d: %w", row, col+1, err))
			continue
		}
		writer.setCellValue(cell, v)
	}
}

// Handle executes the export query.
func (h *ExportHandler) Handle(ctx context.Context, query ExportQuery) (result *ExportResult, err error) {
	requests, err := h.repo.ListAll(ctx, domain.Filter{
		Search:        query.Search,
		Status:        query.Status,
		RequestTypeID: query.RequestTypeID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get cost product requests for export: %w", err)
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

	sheetName := "Cost Product Requests"
	if err := setupExportSheet(f, sheetName); err != nil {
		return nil, err
	}

	writer := &exportExcelWriter{f: f, sheetName: sheetName}
	for i, req := range requests {
		h.writeExportRow(ctx, writer, i+2, req)
	}
	if writer.hasErrors() {
		log.Warn().Err(writer.error()).Msg("Some Excel formatting operations failed")
	}

	buffer, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("failed to write excel to buffer: %w", err)
	}

	return &ExportResult{
		FileContent: buffer.Bytes(),
		FileName:    "cost_product_requests_export.xlsx",
	}, nil
}
