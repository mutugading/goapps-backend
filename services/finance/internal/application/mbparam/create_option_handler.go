package mbparam

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbparam"
)

// CreateOptionCommand represents the create MB param option command.
type CreateOptionCommand struct {
	ParamCode    string
	Code         string
	NumericValue string
	Description  string
	DisplayOrder int32
}

// CreateOptionHandler handles the CreateMbParamOption command.
type CreateOptionHandler struct {
	repo mbparam.Repository
}

// NewCreateOptionHandler creates a new CreateOptionHandler.
func NewCreateOptionHandler(repo mbparam.Repository) *CreateOptionHandler {
	return &CreateOptionHandler{repo: repo}
}

// Handle executes the create MB param option command.
func (h *CreateOptionHandler) Handle(ctx context.Context, cmd CreateOptionCommand) (*mbparam.Option, error) {
	option, err := mbparam.NewOption(cmd.ParamCode, cmd.Code, cmd.NumericValue, cmd.Description, cmd.DisplayOrder)
	if err != nil {
		return nil, err
	}

	if err := h.repo.CreateOption(ctx, option); err != nil {
		return nil, err
	}

	return option, nil
}
