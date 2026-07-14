package mbparam

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbparam"
)

// UpdateOptionCommand represents the update MB param option command.
type UpdateOptionCommand struct {
	ID           string
	NumericValue string
	Description  string
	DisplayOrder int32
	IsActive     bool
}

// UpdateOptionHandler handles the UpdateMbParamOption command.
type UpdateOptionHandler struct {
	repo mbparam.Repository
}

// NewUpdateOptionHandler creates a new UpdateOptionHandler.
func NewUpdateOptionHandler(repo mbparam.Repository) *UpdateOptionHandler {
	return &UpdateOptionHandler{repo: repo}
}

// Handle executes the update MB param option command. ParamCode and Code are immutable and are
// not part of the underlying UPDATE statement, so they are left blank on the reconstructed value.
func (h *UpdateOptionHandler) Handle(ctx context.Context, cmd UpdateOptionCommand) (*mbparam.Option, error) {
	option := mbparam.ReconstructOption(cmd.ID, "", "", cmd.NumericValue, cmd.Description, cmd.DisplayOrder, cmd.IsActive)

	if err := h.repo.UpdateOption(ctx, option); err != nil {
		return nil, err
	}

	return option, nil
}
