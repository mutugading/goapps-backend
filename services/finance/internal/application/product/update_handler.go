// Package product holds application-layer command handlers for the Product aggregate.
package product

import (
	"context"

	"github.com/google/uuid"

	domainproduct "github.com/mutugading/goapps-backend/services/finance/internal/domain/product"
)

// UpdateCommand carries inputs to UpdateHandler.
type UpdateCommand struct {
	ID        uuid.UUID
	Name      string
	ShadeCode string
	ShadeName string
	Purpose   string
	UpdatedBy string
}

// UpdateHandler modifies editable fields on an existing Product.
// Only products in DRAFT workflow status may be updated.
type UpdateHandler struct {
	repo domainproduct.Repository
}

// NewUpdateHandler constructs an UpdateHandler.
func NewUpdateHandler(repo domainproduct.Repository) *UpdateHandler {
	return &UpdateHandler{repo: repo}
}

// Handle fetches the product, applies the update, and persists the result.
// Returns ErrNotFound if the product does not exist, ErrLocked if not in DRAFT status.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*domainproduct.Product, error) {
	existing, err := h.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}

	if err := existing.Update(cmd.Name, cmd.ShadeCode, cmd.ShadeName, cmd.Purpose, cmd.UpdatedBy); err != nil {
		return nil, err
	}

	if err := h.repo.Update(ctx, existing); err != nil {
		return nil, err
	}

	return existing, nil
}
