// Package product holds application-layer command handlers for the Product aggregate.
package product

import (
	"context"

	"github.com/google/uuid"

	domainproduct "github.com/mutugading/goapps-backend/services/finance/internal/domain/product"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// ListQuery carries filter and pagination inputs to ListHandler.
type ListQuery struct {
	Search          string
	WorkflowStatus  string
	ProductStatus   string
	Purpose         string
	CreatedByDeptID *uuid.UUID
	SortField       string
	SortDesc        bool
	Page            int
	PageSize        int
}

// ListResult holds the paginated products and metadata returned by ListHandler.
type ListResult struct {
	Products    []*domainproduct.Product
	TotalItems  int
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// ListHandler retrieves a paginated list of products.
type ListHandler struct {
	repo domainproduct.Repository
}

// NewListHandler constructs a ListHandler.
func NewListHandler(repo domainproduct.Repository) *ListHandler {
	return &ListHandler{repo: repo}
}

// Handle executes a paginated product list query.
func (h *ListHandler) Handle(ctx context.Context, q ListQuery) (*ListResult, error) {
	filter := domainproduct.ListFilter(q)

	items, total, err := h.repo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	var totalPages int32
	if filter.PageSize > 0 && total > 0 {
		computed := (total + filter.PageSize - 1) / filter.PageSize
		totalPages = safeconv.IntToInt32(computed)
	}

	return &ListResult{
		Products:    items,
		TotalItems:  total,
		TotalPages:  totalPages,
		CurrentPage: safeconv.IntToInt32(filter.Page),
		PageSize:    safeconv.IntToInt32(filter.PageSize),
	}, nil
}
