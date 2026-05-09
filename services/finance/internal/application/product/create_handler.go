// Package product holds application-layer command handlers for the Product aggregate.
package product

import (
	"context"

	"github.com/google/uuid"

	domainproduct "github.com/mutugading/goapps-backend/services/finance/internal/domain/product"
)

// CreateCommand carries inputs to CreateHandler.
type CreateCommand struct {
	Code             string
	Name             string
	ItemCode         string
	ShadeCode        string
	ShadeName        string
	DeptID           uuid.UUID
	DeptCode         string
	Purpose          string
	CurrentRequestID uuid.UUID
	CreatedBy        string
}

// CreateHandler creates a new product in DRAFT workflow status.
type CreateHandler struct {
	repo domainproduct.Repository
}

// NewCreateHandler constructs a CreateHandler.
func NewCreateHandler(repo domainproduct.Repository) *CreateHandler {
	return &CreateHandler{repo: repo}
}

// Handle validates and persists a new Product.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*domainproduct.Product, error) {
	p, err := domainproduct.NewProduct(
		cmd.Code, cmd.Name, cmd.ItemCode,
		cmd.ShadeCode, cmd.ShadeName,
		cmd.DeptID, cmd.DeptCode,
		cmd.Purpose,
		cmd.CurrentRequestID,
		cmd.CreatedBy,
	)
	if err != nil {
		return nil, err
	}

	if err := h.repo.Create(ctx, p); err != nil {
		return nil, err
	}

	return p, nil
}
