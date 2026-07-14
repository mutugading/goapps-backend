package mbparam

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbparam"
)

// DeleteCommand represents the delete MB param command.
type DeleteCommand struct {
	ID string
}

// DeleteHandler handles the DeleteMbParam command.
type DeleteHandler struct {
	repo mbparam.Repository
}

// NewDeleteHandler creates a new DeleteHandler.
func NewDeleteHandler(repo mbparam.Repository) *DeleteHandler {
	return &DeleteHandler{repo: repo}
}

// Handle executes the delete MB param command (soft delete).
func (h *DeleteHandler) Handle(ctx context.Context, cmd DeleteCommand) error {
	return h.repo.Delete(ctx, cmd.ID)
}
