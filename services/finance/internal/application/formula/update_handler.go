// Package formula provides application layer handlers for Formula operations.
package formula

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/formula"
)

// UpdateCommand represents the update Formula command.
type UpdateCommand struct {
	FormulaID     string
	FormulaName   *string
	FormulaType   *string
	Expression    *string
	ResultParamID *string
	InputParamIDs []string // nil=no change, non-nil=replace all
	Description   *string
	IsActive      *bool
	UpdatedBy     string
}

// UpdateHandler handles the UpdateFormula command.
type UpdateHandler struct {
	repo formula.Repository
}

// NewUpdateHandler creates a new UpdateHandler.
func NewUpdateHandler(repo formula.Repository) *UpdateHandler {
	return &UpdateHandler{repo: repo}
}

// Handle executes the update Formula command.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*formula.Formula, error) {
	id, err := uuid.Parse(cmd.FormulaID)
	if err != nil {
		return nil, formula.ErrNotFound
	}

	entity, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Parse optional FormulaType
	var formulaType *formula.FormulaType
	if cmd.FormulaType != nil {
		ft, ftErr := formula.NewFormulaType(*cmd.FormulaType)
		if ftErr != nil {
			return nil, ftErr
		}
		formulaType = &ft
	}

	// Parse optional ResultParamID
	var resultParamID *uuid.UUID
	if cmd.ResultParamID != nil {
		rpID, rpErr := uuid.Parse(*cmd.ResultParamID)
		if rpErr != nil {
			return nil, formula.ErrResultParamNotFound
		}

		// Validate param exists
		exists, existsErr := h.repo.ParamExistsByID(ctx, rpID)
		if existsErr != nil {
			return nil, existsErr
		}
		if !exists {
			return nil, formula.ErrResultParamNotFound
		}

		// Check not used by another formula
		used, usedErr := h.repo.ResultParamUsedByOther(ctx, rpID, entity.ID())
		if usedErr != nil {
			return nil, usedErr
		}
		if used {
			return nil, formula.ErrResultParamAlreadyUsed
		}

		resultParamID = &rpID
	}

	// Parse InputParamIDs
	var inputParamIDs []uuid.UUID
	if cmd.InputParamIDs != nil {
		inputParamIDs = make([]uuid.UUID, 0, len(cmd.InputParamIDs))
		for _, idStr := range cmd.InputParamIDs {
			paramID, parseErr := uuid.Parse(idStr)
			if parseErr != nil {
				return nil, formula.ErrInputParamNotFound
			}
			exists, existsErr := h.repo.ParamExistsByID(ctx, paramID)
			if existsErr != nil {
				return nil, existsErr
			}
			if !exists {
				return nil, formula.ErrInputParamNotFound
			}
			inputParamIDs = append(inputParamIDs, paramID)
		}
	}

	// Apply update
	if err := entity.Update(
		cmd.FormulaName, formulaType, cmd.Expression,
		resultParamID, inputParamIDs, cmd.Description,
		cmd.IsActive, cmd.UpdatedBy,
	); err != nil {
		return nil, err
	}

	if err := h.repo.Update(ctx, entity); err != nil {
		return nil, err
	}

	return h.repo.GetByID(ctx, entity.ID())
}
