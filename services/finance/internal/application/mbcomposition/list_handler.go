package mbcomposition

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcomposition"
)

// ListQuery represents the list MB composition query.
type ListQuery struct {
	MbhID string
}

// ListHandler handles the ListMbCompositions query.
type ListHandler struct {
	repo mbcomposition.Repository
}

// NewListHandler creates a new ListHandler.
func NewListHandler(repo mbcomposition.Repository) *ListHandler {
	return &ListHandler{repo: repo}
}

// Handle executes the list MB composition query.
func (h *ListHandler) Handle(ctx context.Context, query ListQuery) ([]*mbcomposition.Entity, error) {
	return h.repo.ListByMbhID(ctx, query.MbhID)
}
