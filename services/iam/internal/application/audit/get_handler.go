// Package audit provides application layer handlers for audit log management.
package audit

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/audit"
)

// GetQuery contains the audit log ID to retrieve.
type GetQuery struct {
	LogID string
}

// GetHandler handles retrieving a single audit log.
type GetHandler struct {
	repo audit.Repository
}

// NewGetHandler creates a new GetHandler.
func NewGetHandler(repo audit.Repository) *GetHandler {
	return &GetHandler{repo: repo}
}

// Handle executes the get audit log query.
func (h *GetHandler) Handle(ctx context.Context, query GetQuery) (*audit.Log, error) {
	id, err := uuid.Parse(query.LogID)
	if err != nil {
		return nil, fmt.Errorf("invalid log ID: %w", err)
	}

	return h.repo.GetByID(ctx, id)
}
