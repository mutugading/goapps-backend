// Package user provides application layer handlers for User operations.
package user

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/role"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/user"
)

// AssignRolesCommand represents the assign roles to user command.
type AssignRolesCommand struct {
	UserID     string
	RoleIDs    []string
	AssignedBy string
}

// AssignRolesHandler handles the assign roles command.
type AssignRolesHandler struct {
	userRepo     user.Repository
	userRoleRepo role.UserRoleRepository
}

// NewAssignRolesHandler creates a new AssignRolesHandler.
func NewAssignRolesHandler(userRepo user.Repository, userRoleRepo role.UserRoleRepository) *AssignRolesHandler {
	return &AssignRolesHandler{
		userRepo:     userRepo,
		userRoleRepo: userRoleRepo,
	}
}

// Handle executes the assign roles command.
func (h *AssignRolesHandler) Handle(ctx context.Context, cmd AssignRolesCommand) error {
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

	// 3. Parse role IDs.
	roleIDs := make([]uuid.UUID, 0, len(cmd.RoleIDs))
	for _, idStr := range cmd.RoleIDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			return shared.ErrNotFound
		}
		roleIDs = append(roleIDs, id)
	}

	// 4. Assign roles.
	return h.userRoleRepo.AssignRoles(ctx, userID, roleIDs, cmd.AssignedBy)
}
