package mbparam

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbparam"
)

// DeleteOptionCommand represents the delete MB param option command.
type DeleteOptionCommand struct {
	ID string
}

// DeleteOptionHandler handles the DeleteMbParamOption command.
type DeleteOptionHandler struct {
	repo mbparam.Repository
}

// NewDeleteOptionHandler creates a new DeleteOptionHandler.
func NewDeleteOptionHandler(repo mbparam.Repository) *DeleteOptionHandler {
	return &DeleteOptionHandler{repo: repo}
}

// Handle executes the delete MB param option command (soft delete).
func (h *DeleteOptionHandler) Handle(ctx context.Context, cmd DeleteOptionCommand) error {
	return h.repo.DeleteOption(ctx, cmd.ID)
}
