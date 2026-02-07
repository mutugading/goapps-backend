// Package user provides application layer handlers for User operations.
package user

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/role"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/user"
)

// RemovePermissionsCommand represents the remove direct permissions from user command.
type RemovePermissionsCommand struct {
	UserID        string
	PermissionIDs []string
}

// RemovePermissionsHandler handles the remove permissions command.
type RemovePermissionsHandler struct {
	userRepo           user.Repository
	userPermissionRepo role.UserPermissionRepository
}

// NewRemovePermissionsHandler creates a new RemovePermissionsHandler.
func NewRemovePermissionsHandler(userRepo user.Repository, userPermissionRepo role.UserPermissionRepository) *RemovePermissionsHandler {
	return &RemovePermissionsHandler{
		userRepo:           userRepo,
		userPermissionRepo: userPermissionRepo,
	}
}

// Handle executes the remove permissions command.
func (h *RemovePermissionsHandler) Handle(ctx context.Context, cmd RemovePermissionsCommand) error {
	// 1. Parse user ID.
	userID, err := uuid.Parse(cmd.UserID)
	if err != nil {
		return shared.ErrNotFound
	}

	// 2. Verify user exists.
	_, err = h.userRepo.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	// 3. Parse permission IDs.
	permissionIDs := make([]uuid.UUID, 0, len(cmd.PermissionIDs))
	for _, idStr := range cmd.PermissionIDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			return shared.ErrNotFound
		}
		permissionIDs = append(permissionIDs, id)
	}

	// 4. Remove permissions.
	return h.userPermissionRepo.RemovePermissions(ctx, userID, permissionIDs)
}
