package mbcomposition

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcomposition"
)

// ListVersionsQuery represents the list MB composition versions query.
type ListVersionsQuery struct {
	MbhID   string
	Version int32
}

// ListVersionsHandler handles the ListMbCompositionVersions query.
type ListVersionsHandler struct {
	repo mbcomposition.Repository
}

// NewListVersionsHandler creates a new ListVersionsHandler.
func NewListVersionsHandler(repo mbcomposition.Repository) *ListVersionsHandler {
	return &ListVersionsHandler{repo: repo}
}

// Handle executes the list MB composition versions query. Version == 0 resolves to the
// latest version available for the given MB head.
func (h *ListVersionsHandler) Handle(ctx context.Context, query ListVersionsQuery) ([]mbcomposition.VersionRow, error) {
	return h.repo.ListVersionsByMbhID(ctx, query.MbhID, query.Version)
}
