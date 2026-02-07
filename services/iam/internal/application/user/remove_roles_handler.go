// Package user provides application layer handlers for User operations.
package user

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/role"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/user"
)

// RemoveRolesCommand represents the remove roles from user command.
type RemoveRolesCommand struct {
	UserID  string
	RoleIDs []string
}

// RemoveRolesHandler handles the remove roles command.
type RemoveRolesHandler struct {
	userRepo     user.Repository
	userRoleRepo role.UserRoleRepository
}

// NewRemoveRolesHandler creates a new RemoveRolesHandler.
func NewRemoveRolesHandler(userRepo user.Repository, userRoleRepo role.UserRoleRepository) *RemoveRolesHandler {
	return &RemoveRolesHandler{
		userRepo:     userRepo,
		userRoleRepo: userRoleRepo,
	}
}

// Handle executes the remove roles command.
func (h *RemoveRolesHandler) Handle(ctx context.Context, cmd RemoveRolesCommand) error {
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

	// 4. Remove roles.
	return h.userRoleRepo.RemoveRoles(ctx, userID, roleIDs)
}
