// Package user provides application layer handlers for User operations.
package user

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/role"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/user"
)

// AssignPermissionsCommand represents the assign direct permissions to user command.
type AssignPermissionsCommand struct {
	UserID        string
	PermissionIDs []string
	AssignedBy    string
}

// AssignPermissionsHandler handles the assign permissions command.
type AssignPermissionsHandler struct {
	userRepo           user.Repository
	userPermissionRepo role.UserPermissionRepository
}

// NewAssignPermissionsHandler creates a new AssignPermissionsHandler.
func NewAssignPermissionsHandler(userRepo user.Repository, userPermissionRepo role.UserPermissionRepository) *AssignPermissionsHandler {
	return &AssignPermissionsHandler{
		userRepo:           userRepo,
		userPermissionRepo: userPermissionRepo,
	}
}

// Handle executes the assign permissions command.
func (h *AssignPermissionsHandler) Handle(ctx context.Context, cmd AssignPermissionsCommand) error {
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

	// 4. Assign permissions.
	return h.userPermissionRepo.AssignPermissions(ctx, userID, permissionIDs, cmd.AssignedBy)
}
