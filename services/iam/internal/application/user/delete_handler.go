// Package user provides application layer handlers for User operations.
package user

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/user"
)

// DeleteCommand represents the delete user command.
type DeleteCommand struct {
	UserID    string
	DeletedBy string
}

// DeleteHandler handles the delete user command.
type DeleteHandler struct {
	repo user.Repository
}

// NewDeleteHandler creates a new DeleteHandler.
func NewDeleteHandler(repo user.Repository) *DeleteHandler {
	return &DeleteHandler{repo: repo}
}

// Handle executes the delete user command (soft delete).
func (h *DeleteHandler) Handle(ctx context.Context, cmd DeleteCommand) error {
	// 1. Parse ID.
	id, err := uuid.Parse(cmd.UserID)
	if err != nil {
		return shared.ErrNotFound
	}

	// 2. Check existence.
	_, err = h.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// 3. Soft delete.
	return h.repo.Delete(ctx, id, cmd.DeletedBy)
}
