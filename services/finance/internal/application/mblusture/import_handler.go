package mblusture

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

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mblusture"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

const (
	duplicateActionSkip   = "skip"
	duplicateActionUpdate = "update"
	duplicateActionError  = "error"

	fieldCode = "code"
)

// ImportCommand represents the import MB lustures command.
type ImportCommand struct {
	FileContent     []byte
	FileName        string
	DuplicateAction string // "skip", "update", "error"
	CreatedBy       string
}

// ImportResult represents the import MB lustures result.
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

// ImportHandler handles the ImportMbLusture command.
type ImportHandler struct {
	repo mblusture.Repository
}

// NewImportHandler creates a new ImportHandler.
func NewImportHandler(repo mblusture.Repository) *ImportHandler {
	return &ImportHandler{repo: repo}
}

// Handle executes the import MB lustures command.
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

// mbLustureRowData holds parsed row values, matching mbLustureTemplateHeaders column order.
type mbLustureRowData struct {
	code            string
	displayName     string
	fullDescription string
	category        string
	displayOrder    string
	isActive        string
}

func parseMbLustureRow(row []string) mbLustureRowData {
	return mbLustureRowData{
		code:            getCell(row, 0),
		displayName:     getCell(row, 1),
		fullDescription: getCell(row, 2),
		category:        getCell(row, 3),
		displayOrder:    getCell(row, 4),
		isActive:        getCell(row, 5),
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
	data := parseMbLustureRow(row)

	if data.code == "" {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     fieldCode,
			Message:   "code cannot be empty",
		})
		return
	}

	displayOrder, isActive, parseErr := parseMbLustureFields(data)
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
	if err != nil && !errors.Is(err, mblusture.ErrNotFound) {
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

	h.createMbLusture(ctx, data, displayOrder, rowNum, cmd.CreatedBy, result)
}

// parseMbLustureFields parses the optional numeric/boolean fields from a row.
func parseMbLustureFields(data mbLustureRowData) (displayOrder int32, isActive bool, err error) {
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
	ctx context.Context, existing *mblusture.Entity, data mbLustureRowData,
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

// updateExisting updates an existing MB lusture.
func (h *ImportHandler) updateExisting(
	ctx context.Context, existing *mblusture.Entity, data mbLustureRowData,
	displayOrder int32, isActive bool, rowNum int32, updatedBy string, result *ImportResult,
) {
	entity := mblusture.Reconstruct(
		existing.ID(), existing.Code(), data.displayName, data.fullDescription, data.category,
		displayOrder, isActive, existing.CreatedAt(), existing.CreatedBy(),
		existing.UpdatedAt(), updatedBy, existing.DeletedAt(), existing.DeletedBy(),
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

// createMbLusture creates a new MB lusture.
func (h *ImportHandler) createMbLusture(
	ctx context.Context, data mbLustureRowData, displayOrder int32, rowNum int32, createdBy string, result *ImportResult,
) {
	entity, err := mblusture.NewEntity(data.code, data.displayName, data.fullDescription, data.category, displayOrder, createdBy)
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
