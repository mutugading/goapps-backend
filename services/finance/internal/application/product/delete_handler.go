// Package product holds application-layer command handlers for the Product aggregate.
package product

import (
	"context"

	"github.com/google/uuid"

	domainproduct "github.com/mutugading/goapps-backend/services/finance/internal/domain/product"
)

// DeleteCommand carries inputs to DeleteHandler.
type DeleteCommand struct {
	ID        uuid.UUID
	DeletedBy string
}

// DeleteHandler soft-deletes a Product by its UUID.
type DeleteHandler struct {
	repo domainproduct.Repository
}

// NewDeleteHandler constructs a DeleteHandler.
func NewDeleteHandler(repo domainproduct.Repository) *DeleteHandler {
	return &DeleteHandler{repo: repo}
}

// Handle soft-deletes the product identified by cmd.ID.
// Returns ErrNotFound if the product does not exist.
func (h *DeleteHandler) Handle(ctx context.Context, cmd DeleteCommand) error {
	return h.repo.Delete(ctx, cmd.ID, cmd.DeletedBy)
}
