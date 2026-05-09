// Package product holds application-layer command handlers for the Product aggregate.
package product

import (
	"context"

	"github.com/google/uuid"

	domainproduct "github.com/mutugading/goapps-backend/services/finance/internal/domain/product"
)

// DuplicateCommand carries inputs to DuplicateHandler.
//
// Phase 1 note: only master fields are cloned. Routing, parameters, RM associations,
// and attachments are deferred to later phases. The Options flags are stored in the
// new product's copied_with_options JSONB column so later phases can honor them.
type DuplicateCommand struct {
	SourceID         uuid.UUID
	NewCode          string
	NewName          string
	DuplicationNote  string
	Options          domainproduct.CopyOptions
	CurrentRequestID uuid.UUID
	CreatedBy        string
}

// DuplicateHandler creates a new Product by cloning master fields from an existing one.
type DuplicateHandler struct {
	repo domainproduct.Repository
}

// NewDuplicateHandler constructs a DuplicateHandler.
func NewDuplicateHandler(repo domainproduct.Repository) *DuplicateHandler {
	return &DuplicateHandler{repo: repo}
}

// Handle fetches the source product, duplicates it, and persists the new product.
// Returns ErrNotFound if the source does not exist, ErrSourceDeleted if already deleted,
// ErrSelfDuplication if the new code matches the source code.
func (h *DuplicateHandler) Handle(ctx context.Context, cmd DuplicateCommand) (*domainproduct.Product, error) {
	source, err := h.repo.GetByID(ctx, cmd.SourceID)
	if err != nil {
		return nil, err
	}

	newProduct, err := source.Duplicate(
		cmd.NewCode,
		cmd.NewName,
		cmd.DuplicationNote,
		cmd.Options,
		cmd.CurrentRequestID,
		cmd.CreatedBy,
	)
	if err != nil {
		return nil, err
	}

	if err := h.repo.Create(ctx, newProduct); err != nil {
		return nil, err
	}

	return newProduct, nil
}
