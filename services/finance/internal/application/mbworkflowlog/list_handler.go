// Package mbworkflowlog provides application-layer handlers for MB workflow log operations.
package mbworkflowlog

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbworkflowlog"
)

// ListQuery represents the list MB workflow logs query.
type ListQuery struct {
	MbhID string
}

// ListHandler handles the ListMbWorkflowLogs query.
type ListHandler struct {
	repo mbworkflowlog.Repository
}

// NewListHandler creates a new ListHandler.
func NewListHandler(repo mbworkflowlog.Repository) *ListHandler {
	return &ListHandler{repo: repo}
}

// Handle executes the list MB workflow logs query.
func (h *ListHandler) Handle(ctx context.Context, query ListQuery) ([]*mbworkflowlog.Entity, error) {
	return h.repo.ListByMbhID(ctx, query.MbhID)
}
