package mblusture

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mblusture"
)

// DeleteCommand represents the delete MB lusture command.
type DeleteCommand struct {
	ID string
}

// DeleteHandler handles the DeleteMbLusture command.
type DeleteHandler struct {
	repo mblusture.Repository
}

// NewDeleteHandler creates a new DeleteHandler.
func NewDeleteHandler(repo mblusture.Repository) *DeleteHandler {
	return &DeleteHandler{repo: repo}
}

// Handle executes the delete MB lusture command (soft delete).
func (h *DeleteHandler) Handle(ctx context.Context, cmd DeleteCommand) error {
	return h.repo.Delete(ctx, cmd.ID)
}
