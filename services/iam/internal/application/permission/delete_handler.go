// Package permission provides application layer handlers for Permission operations.
package permission

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/role"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// DeleteCommand represents the delete permission command.
type DeleteCommand struct {
	PermissionID string
	DeletedBy    string
}

// DeleteHandler handles the DeletePermission command.
type DeleteHandler struct {
	repo role.PermissionRepository
}

// NewDeleteHandler creates a new DeleteHandler.
func NewDeleteHandler(repo role.PermissionRepository) *DeleteHandler {
	return &DeleteHandler{repo: repo}
}

// Handle executes the delete permission command (soft delete).
func (h *DeleteHandler) Handle(ctx context.Context, cmd DeleteCommand) error {
	// 1. Parse ID
	id, err := uuid.Parse(cmd.PermissionID)
	if err != nil {
		return shared.ErrNotFound
	}

	// 2. Verify existence
	_, err = h.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// 3. Soft delete
	return h.repo.Delete(ctx, id, cmd.DeletedBy)
}
