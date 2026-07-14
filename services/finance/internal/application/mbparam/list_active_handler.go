package mbparam

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbparam"
)

// ListActiveQuery represents the list active MB params query.
type ListActiveQuery struct{}

// ListActiveHandler handles the ListActiveMbParams query.
type ListActiveHandler struct {
	repo mbparam.Repository
}

// NewListActiveHandler creates a new ListActiveHandler.
func NewListActiveHandler(repo mbparam.Repository) *ListActiveHandler {
	return &ListActiveHandler{repo: repo}
}

// Handle executes the list active MB params query.
func (h *ListActiveHandler) Handle(ctx context.Context, _ ListActiveQuery) ([]*mbparam.Entity, error) {
	return h.repo.ListActive(ctx)
}
