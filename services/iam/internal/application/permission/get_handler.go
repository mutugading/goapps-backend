// Package permission provides application layer handlers for Permission operations.
package permission

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/role"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// GetQuery represents the get permission query.
type GetQuery struct {
	PermissionID string
}

// GetHandler handles the GetPermission query.
type GetHandler struct {
	repo role.PermissionRepository
}

// NewGetHandler creates a new GetHandler.
func NewGetHandler(repo role.PermissionRepository) *GetHandler {
	return &GetHandler{repo: repo}
}

// Handle executes the get permission query.
func (h *GetHandler) Handle(ctx context.Context, query GetQuery) (*role.Permission, error) {
	id, err := uuid.Parse(query.PermissionID)
	if err != nil {
		return nil, shared.ErrNotFound
	}

	return h.repo.GetByID(ctx, id)
}
