// Package user provides application layer handlers for User operations.
package user

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/user"
)

// GetQuery represents the get user query.
type GetQuery struct {
	UserID string
}

// GetHandler handles the get user query.
type GetHandler struct {
	repo user.Repository
}

// NewGetHandler creates a new GetHandler.
func NewGetHandler(repo user.Repository) *GetHandler {
	return &GetHandler{repo: repo}
}

// Handle executes the get user query.
func (h *GetHandler) Handle(ctx context.Context, query GetQuery) (*user.User, error) {
	id, err := uuid.Parse(query.UserID)
	if err != nil {
		return nil, shared.ErrNotFound
	}

	return h.repo.GetByID(ctx, id)
}
