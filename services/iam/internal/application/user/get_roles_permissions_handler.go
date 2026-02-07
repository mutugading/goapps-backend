// Package user provides application layer handlers for User operations.
package user

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/user"
)

// GetRolesPermissionsQuery represents the get user roles and permissions query.
type GetRolesPermissionsQuery struct {
	UserID string
}

// GetRolesPermissionsResult holds the roles and permissions for a user.
type GetRolesPermissionsResult struct {
	Roles       []user.RoleRef
	Permissions []user.PermissionRef
}

// GetRolesPermissionsHandler handles the get roles and permissions query.
type GetRolesPermissionsHandler struct {
	repo user.Repository
}

// NewGetRolesPermissionsHandler creates a new GetRolesPermissionsHandler.
func NewGetRolesPermissionsHandler(repo user.Repository) *GetRolesPermissionsHandler {
	return &GetRolesPermissionsHandler{repo: repo}
}

// Handle executes the get roles and permissions query.
func (h *GetRolesPermissionsHandler) Handle(ctx context.Context, query GetRolesPermissionsQuery) (*GetRolesPermissionsResult, error) {
	// 1. Parse user ID.
	userID, err := uuid.Parse(query.UserID)
	if err != nil {
		return nil, shared.ErrNotFound
	}

	// 2. Verify user exists.
	_, err = h.repo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// 3. Get roles and permissions.
	roles, permissions, err := h.repo.GetRolesAndPermissions(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &GetRolesPermissionsResult{
		Roles:       roles,
		Permissions: permissions,
	}, nil
}
