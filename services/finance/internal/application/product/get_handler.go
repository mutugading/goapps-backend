// Package product holds application-layer command handlers for the Product aggregate.
package product

import (
	"context"

	"github.com/google/uuid"

	domainproduct "github.com/mutugading/goapps-backend/services/finance/internal/domain/product"
)

// GetCommand carries inputs to GetHandler.
type GetCommand struct {
	ID uuid.UUID
}

// GetHandler retrieves a single Product by its UUID.
type GetHandler struct {
	repo domainproduct.Repository
}

// NewGetHandler constructs a GetHandler.
func NewGetHandler(repo domainproduct.Repository) *GetHandler {
	return &GetHandler{repo: repo}
}

// Handle fetches a Product by ID, returning ErrNotFound if absent.
func (h *GetHandler) Handle(ctx context.Context, cmd GetCommand) (*domainproduct.Product, error) {
	return h.repo.GetByID(ctx, cmd.ID)
}
