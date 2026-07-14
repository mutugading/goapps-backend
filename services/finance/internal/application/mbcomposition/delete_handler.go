package mbcomposition

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcomposition"
)

// DeleteCommand represents the delete MB composition command.
type DeleteCommand struct {
	ID string
}

// DeleteHandler handles the DeleteMbComposition command.
type DeleteHandler struct {
	repo mbcomposition.Repository
}

// NewDeleteHandler creates a new DeleteHandler.
func NewDeleteHandler(repo mbcomposition.Repository) *DeleteHandler {
	return &DeleteHandler{repo: repo}
}

// Handle executes the delete MB composition command (soft delete).
func (h *DeleteHandler) Handle(ctx context.Context, cmd DeleteCommand) error {
	return h.repo.Delete(ctx, cmd.ID)
}
