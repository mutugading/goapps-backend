// Package parameter provides application layer handlers for Parameter operations.
package parameter

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/parameter"
)

// UpdateCommand represents the update Parameter command.
type UpdateCommand struct {
	ParamID        string
	ParamName      *string
	ParamShortName *string
	DataType       *string
	ParamCategory  *string
	UOMID          *string // nil=not set, pointer to ""=clear, pointer to uuid=set
	DefaultValue   *string // nil=not set, pointer to ""=clear, pointer to value=set
	MinValue       *string // nil=not set, pointer to ""=clear, pointer to value=set
	MaxValue       *string // nil=not set, pointer to ""=clear, pointer to value=set
	IsActive       *bool
	UpdatedBy      string
}

// UpdateHandler handles the UpdateParameter command.
type UpdateHandler struct {
	repo parameter.Repository
}

// NewUpdateHandler creates a new UpdateHandler.
func NewUpdateHandler(repo parameter.Repository) *UpdateHandler {
	return &UpdateHandler{repo: repo}
}

// Handle executes the update Parameter command.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*parameter.Parameter, error) {
	// 1. Parse ID
	id, err := uuid.Parse(cmd.ParamID)
	if err != nil {
		return nil, parameter.ErrNotFound
	}

	// 2. Get existing entity
	entity, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 3. Prepare DataType if provided
	var dataType *parameter.DataType
	if cmd.DataType != nil {
		dt, err := parameter.NewDataType(*cmd.DataType)
		if err != nil {
			return nil, err
		}
		dataType = &dt
	}

	// 4. Prepare ParamCategory if provided
	var paramCategory *parameter.ParamCategory
	if cmd.ParamCategory != nil {
		cat, err := parameter.NewParamCategory(*cmd.ParamCategory)
		if err != nil {
			return nil, err
		}
		paramCategory = &cat
	}

	// 5. Prepare UOM ID (double pointer: nil=not set, *nil=clear, *uuid=set)
	var uomID **uuid.UUID
	if cmd.UOMID != nil {
		if *cmd.UOMID == "" {
			// Clear UOM reference
			var nilUOM *uuid.UUID
			uomID = &nilUOM
		} else {
			parsed, err := uuid.Parse(*cmd.UOMID)
			if err != nil {
				return nil, parameter.ErrNotFound
			}
			parsedPtr := &parsed
			uomID = &parsedPtr
		}
	}

	// 6. Prepare optional string fields (double pointer pattern)
	var defaultValue, minValue, maxValue **string
	if cmd.DefaultValue != nil {
		if *cmd.DefaultValue == "" {
			var nilStr *string
			defaultValue = &nilStr
		} else {
			defaultValue = &cmd.DefaultValue
		}
	}
	if cmd.MinValue != nil {
		if *cmd.MinValue == "" {
			var nilStr *string
			minValue = &nilStr
		} else {
			minValue = &cmd.MinValue
		}
	}
	if cmd.MaxValue != nil {
		if *cmd.MaxValue == "" {
			var nilStr *string
			maxValue = &nilStr
		} else {
			maxValue = &cmd.MaxValue
		}
	}

	// 7. Update domain entity
	if err := entity.Update(
		cmd.ParamName, cmd.ParamShortName,
		dataType, paramCategory, uomID,
		defaultValue, minValue, maxValue,
		cmd.IsActive, cmd.UpdatedBy,
	); err != nil {
		return nil, err
	}

	// 8. Persist
	if err := h.repo.Update(ctx, entity); err != nil {
		return nil, err
	}

	return entity, nil
}
