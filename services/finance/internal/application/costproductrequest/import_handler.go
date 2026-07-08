// Package costproductrequest holds application use cases for the Phase A request aggregate.
package costproductrequest

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"

	pmDomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductrequest"
	rtDomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costrequesttype"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// ImportCommand represents the import cost product requests command.
type ImportCommand struct {
	FileContent     []byte
	FileName        string
	DuplicateAction string // "skip", "update", "error" — accepted for shape parity only; import is create-only.
	CreatedBy       string
}

// ImportResult represents the import result. Create-only: SkippedCount and
// UpdatedCount are always 0 (kept for shape consistency with other imports).
type ImportResult struct {
	SuccessCount int32
	SkippedCount int32
	UpdatedCount int32
	FailedCount  int32
	Errors       []ImportError
}

// ImportError represents a single row-level import error.
type ImportError struct {
	RowNumber int32
	Field     string
	Message   string
}

// ImportHandler handles the ImportCostProductRequests command. Every row
// creates a new DRAFT request (create-only, design.md §4 Area D6) — a
// row-level failure (unresolvable code, validation error) is recorded as an
// ImportError and processing continues with the remaining rows.
//
// requestTypeRepo and productMasterRepo resolve the import's human-readable
// "Request type" / "Reference product" code columns into the internal IDs
// CreateCommand expects; createHandler performs the actual domain
// construction + persistence + notification, reused as-is.
type ImportHandler struct {
	requestTypeRepo   rtDomain.Repository
	productMasterRepo pmDomain.Repository
	createHandler     *CreateHandler
}

// NewImportHandler creates a new ImportHandler.
func NewImportHandler(requestTypeRepo rtDomain.Repository, productMasterRepo pmDomain.Repository, createHandler *CreateHandler) *ImportHandler {
	return &ImportHandler{requestTypeRepo: requestTypeRepo, productMasterRepo: productMasterRepo, createHandler: createHandler}
}

// Handle executes the import command.
func (h *ImportHandler) Handle(ctx context.Context, cmd ImportCommand) (result *ImportResult, err error) {
	result = &ImportResult{Errors: []ImportError{}}

	rows, err := parseImportExcelFile(cmd.FileContent, cmd.FileName)
	if err != nil {
		return nil, err
	}
	if len(rows) <= 1 {
		return result, nil // header only, no data rows
	}

	for i, row := range rows[1:] {
		rowNum := safeconv.IntToInt32(i + 2) // 1-indexed, skip header
		h.processImportRow(ctx, row, rowNum, cmd, result)
	}
	return result, nil
}

// parseImportExcelFile opens and validates the Excel file, returning rows.
func parseImportExcelFile(content []byte, fileName string) ([][]string, error) {
	ext := strings.ToLower(filepath.Ext(fileName))
	if ext != ".xlsx" && ext != ".xls" {
		return nil, fmt.Errorf("unsupported file format: %s", ext)
	}
	f, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("failed to open excel file: %w", err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("Failed to close Excel file")
		}
	}()
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("no sheets found in file")
	}
	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, fmt.Errorf("failed to get rows: %w", err)
	}
	return rows, nil
}

// importRowData holds the raw cell values for one D6 import row, in
// exportHeaders column order.
type importRowData struct {
	requestTypeCode      string
	title                string
	description          string
	customerName         string
	customerCode         string
	urgencyLevel         string
	neededByDate         string
	productDescription   string
	shadeCode            string
	shadeName            string
	tubeType             string
	referenceProductCode string
	targetVolume         string
	targetPriceRange     string
}

// parseImportRow extracts cell values from a row (exportHeaders column order).
func parseImportRow(row []string) importRowData {
	return importRowData{
		requestTypeCode:      getImportCell(row, 0),
		title:                getImportCell(row, 1),
		description:          getImportCell(row, 2),
		customerName:         getImportCell(row, 3),
		customerCode:         getImportCell(row, 4),
		urgencyLevel:         strings.ToLower(getImportCell(row, 5)),
		neededByDate:         getImportCell(row, 6),
		productDescription:   getImportCell(row, 7),
		shadeCode:            getImportCell(row, 8),
		shadeName:            getImportCell(row, 9),
		tubeType:             strings.ToUpper(getImportCell(row, 10)),
		referenceProductCode: getImportCell(row, 11),
		targetVolume:         getImportCell(row, 12),
		targetPriceRange:     getImportCell(row, 13),
	}
}

// getImportCell safely gets a cell value from a row.
func getImportCell(row []string, index int) string {
	if index < len(row) {
		return strings.TrimSpace(row[index])
	}
	return ""
}

// addImportError appends a row-level error and increments FailedCount.
func addImportRowError(result *ImportResult, rowNum int32, field, message string) {
	result.FailedCount++
	result.Errors = append(result.Errors, ImportError{RowNumber: rowNum, Field: field, Message: message})
}

// processImportRow resolves codes, validates, and creates one row. Any
// failure is recorded in result as a row-level ImportError; it never returns
// an error that would abort the whole batch.
func (h *ImportHandler) processImportRow(ctx context.Context, row []string, rowNum int32, cmd ImportCommand, result *ImportResult) {
	data := parseImportRow(row)

	requestTypeID, ok := h.resolveRequestType(ctx, data, rowNum, result)
	if !ok {
		return
	}
	referenceProductSysID, ok := h.resolveReferenceProduct(ctx, data, rowNum, result)
	if !ok {
		return
	}

	spec := domain.SpecInput{
		ProductDescription: data.productDescription,
		ShadeCode:          data.shadeCode,
		ShadeName:          data.shadeName,
		TubeType:           data.tubeType,
	}
	if err := spec.Validate(); err != nil {
		addImportRowError(result, rowNum, "spec", err.Error())
		return
	}

	cmdCreate := CreateCommand{
		RequestTypeID:         requestTypeID,
		Title:                 data.title,
		Description:           data.description,
		CustomerName:          data.customerName,
		CustomerCode:          data.customerCode,
		ProductClassification: domain.ClassNew,
		TargetVolume:          data.targetVolume,
		TargetPriceRange:      data.targetPriceRange,
		UrgencyLevel:          data.urgencyLevel,
		NeededByDate:          data.neededByDate,
		RequesterUserID:       cmd.CreatedBy,
		Spec:                  &spec,
		ReferenceProductSysID: referenceProductSysID,
	}
	if _, err := h.createHandler.Handle(ctx, cmdCreate); err != nil {
		addImportRowError(result, rowNum, "create", err.Error())
		return
	}
	result.SuccessCount++
}

// resolveRequestType resolves the row's request type code to an ID. Returns
// ok=false (with an ImportError already recorded) if the code is blank or unresolvable.
func (h *ImportHandler) resolveRequestType(ctx context.Context, data importRowData, rowNum int32, result *ImportResult) (int32, bool) {
	if data.requestTypeCode == "" {
		addImportRowError(result, rowNum, "request_type", "request type code cannot be empty")
		return 0, false
	}
	id, err := h.requestTypeRepo.GetIDByCode(ctx, data.requestTypeCode)
	if err != nil {
		addImportRowError(result, rowNum, "request_type", fmt.Sprintf("invalid request type code %q: %v", data.requestTypeCode, err))
		return 0, false
	}
	return id, true
}

// resolveReferenceProduct resolves the row's optional reference product code
// to a sys ID. An empty code is valid (0 = unset); an unresolvable non-empty
// code records an ImportError and returns ok=false.
func (h *ImportHandler) resolveReferenceProduct(ctx context.Context, data importRowData, rowNum int32, result *ImportResult) (int64, bool) {
	if data.referenceProductCode == "" {
		return 0, true
	}
	product, err := h.productMasterRepo.GetByCode(ctx, data.referenceProductCode)
	if err != nil {
		addImportRowError(result, rowNum, "reference_product", fmt.Sprintf("invalid reference product code %q: %v", data.referenceProductCode, err))
		return 0, false
	}
	return product.ProductSysID(), true
}
