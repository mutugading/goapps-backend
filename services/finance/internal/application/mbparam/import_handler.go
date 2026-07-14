package mbparam

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbparam"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

const (
	duplicateActionSkip   = "skip"
	duplicateActionUpdate = "update"
	duplicateActionError  = "error"

	fieldCode = "code"
)

// ImportCommand represents the import MB params command.
type ImportCommand struct {
	FileContent     []byte
	FileName        string
	DuplicateAction string // "skip", "update", "error"
	CreatedBy       string
}

// ImportResult represents the import MB params result.
type ImportResult struct {
	SuccessCount int32
	SkippedCount int32
	FailedCount  int32
	Errors       []ImportError
}

// ImportError represents a single import error.
type ImportError struct {
	RowNumber int32
	Field     string
	Message   string
}

// ImportHandler handles the ImportMbParams command.
type ImportHandler struct {
	repo mbparam.Repository
}

// NewImportHandler creates a new ImportHandler.
func NewImportHandler(repo mbparam.Repository) *ImportHandler {
	return &ImportHandler{repo: repo}
}

// Handle executes the import MB params command.
func (h *ImportHandler) Handle(ctx context.Context, cmd ImportCommand) (result *ImportResult, err error) {
	result = &ImportResult{Errors: []ImportError{}}

	rows, err := h.parseExcelFile(cmd.FileContent, cmd.FileName)
	if err != nil {
		return nil, err
	}

	if len(rows) <= 1 {
		return result, nil
	}

	for i, row := range rows[1:] {
		rowNum := safeconv.IntToInt32(i + 2)
		h.processRow(ctx, row, rowNum, cmd, result)
	}

	return result, nil
}

// parseExcelFile opens and validates the Excel file, returning rows.
func (h *ImportHandler) parseExcelFile(content []byte, fileName string) ([][]string, error) {
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

// mbParamRowData holds parsed row values, matching mbParamTemplateHeaders column order.
type mbParamRowData struct {
	code          string
	name          string
	paramType     string
	description   string
	defaultValue  string
	defaultOption string
	unit          string
	displayOrder  string
	isActive      string
}

func parseMbParamRow(row []string) mbParamRowData {
	return mbParamRowData{
		code:          getCell(row, 0),
		name:          getCell(row, 1),
		paramType:     getCell(row, 2),
		description:   getCell(row, 3),
		defaultValue:  getCell(row, 4),
		defaultOption: getCell(row, 5),
		unit:          getCell(row, 6),
		displayOrder:  getCell(row, 7),
		isActive:      getCell(row, 8),
	}
}

// getCell safely gets a cell value from a row.
func getCell(row []string, index int) string {
	if index < len(row) {
		return strings.TrimSpace(row[index])
	}
	return ""
}

// processRow handles a single row import.
func (h *ImportHandler) processRow(ctx context.Context, row []string, rowNum int32, cmd ImportCommand, result *ImportResult) {
	data := parseMbParamRow(row)

	if data.code == "" {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     fieldCode,
			Message:   "code cannot be empty",
		})
		return
	}

	displayOrder, isActive, parseErr := parseMbParamFields(data)
	if parseErr != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     "fields",
			Message:   parseErr.Error(),
		})
		return
	}

	existing, err := h.repo.GetByCode(ctx, data.code)
	if err != nil && !errors.Is(err, mbparam.ErrNotFound) {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     fieldCode,
			Message:   fmt.Sprintf("failed to check duplicate: %v", err),
		})
		return
	}

	if existing != nil {
		h.handleDuplicate(ctx, existing, data, displayOrder, isActive, rowNum, cmd, result)
		return
	}

	h.createMbParam(ctx, data, displayOrder, rowNum, cmd.CreatedBy, result)
}

// parseMbParamFields parses the optional numeric/boolean fields from a row.
func parseMbParamFields(data mbParamRowData) (displayOrder int32, isActive bool, err error) {
	isActive = true
	if data.displayOrder != "" {
		v, parseErr := strconv.Atoi(data.displayOrder)
		if parseErr != nil {
			return 0, false, fmt.Errorf("invalid display order %q: %w", data.displayOrder, parseErr)
		}
		displayOrder = safeconv.IntToInt32(v)
	}
	if data.isActive != "" {
		v, parseErr := strconv.ParseBool(data.isActive)
		if parseErr != nil {
			return 0, false, fmt.Errorf("invalid active %q: %w", data.isActive, parseErr)
		}
		isActive = v
	}
	return displayOrder, isActive, nil
}

// handleDuplicate handles a duplicate code based on the specified action.
func (h *ImportHandler) handleDuplicate(
	ctx context.Context, existing *mbparam.Entity, data mbParamRowData,
	displayOrder int32, isActive bool, rowNum int32, cmd ImportCommand, result *ImportResult,
) {
	switch cmd.DuplicateAction {
	case duplicateActionSkip:
		result.SkippedCount++
	case duplicateActionUpdate:
		h.updateExisting(ctx, existing, data, displayOrder, isActive, rowNum, cmd.CreatedBy, result)
	case duplicateActionError:
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     fieldCode,
			Message:   "duplicate code already exists",
		})
	default:
		result.SkippedCount++
	}
}

// updateExisting updates an existing MB param.
func (h *ImportHandler) updateExisting(
	ctx context.Context, existing *mbparam.Entity, data mbParamRowData,
	displayOrder int32, isActive bool, rowNum int32, updatedBy string, result *ImportResult,
) {
	entity := mbparam.Reconstruct(
		existing.ID(), existing.Code(), data.name, data.description, existing.Type(),
		data.defaultValue, data.defaultOption, data.unit, displayOrder, isActive,
		existing.CreatedAt(), existing.CreatedBy(), existing.UpdatedAt(), updatedBy,
		existing.DeletedAt(), existing.DeletedBy(),
	)

	if err := h.repo.Update(ctx, entity); err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     "update",
			Message:   fmt.Sprintf("failed to update: %v", err),
		})
		return
	}

	result.SuccessCount++
}

// createMbParam creates a new MB param.
func (h *ImportHandler) createMbParam(
	ctx context.Context, data mbParamRowData, displayOrder int32, rowNum int32, createdBy string, result *ImportResult,
) {
	paramType := strings.ToUpper(data.paramType)
	entity, err := mbparam.NewEntity(
		data.code, data.name, paramType, data.description, data.defaultValue,
		data.defaultOption, data.unit, displayOrder, createdBy,
	)
	if err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     "create",
			Message:   err.Error(),
		})
		return
	}

	if err := h.repo.Create(ctx, entity); err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     "create",
			Message:   fmt.Sprintf("failed to create: %v", err),
		})
		return
	}

	result.SuccessCount++
}
