// Package product contains the Product aggregate and its supporting types.
package product

import (
	"context"

	"github.com/google/uuid"
)

// ListFilter narrows results returned by Repository.List.
type ListFilter struct {
	// Search is a free-text query applied against the FTS index.
	Search string

	// WorkflowStatus filters by workflow state. Empty means no filter.
	WorkflowStatus string

	// ProductStatus filters by product lifecycle status. Empty means no filter.
	ProductStatus string

	// Purpose filters by intended-use classification. Empty means no filter.
	Purpose string

	// CreatedByDeptID filters by the creating department UUID. Nil means no filter.
	CreatedByDeptID *uuid.UUID

	// SortField is the field name to sort by; mapped via the repo's sortColumnMap.
	SortField string

	// SortDesc controls sort direction; true = descending.
	SortDesc bool

	// Page is 1-based page number.
	Page int

	// PageSize is the number of results per page.
	PageSize int
}

// SearchOptions is used by Repository.SearchByText for FTS-backed product lookups.
type SearchOptions struct {
	// Query is the free-text search query.
	Query string

	// ShadeCode restricts results to a specific shade code. Empty means any shade.
	ShadeCode string

	// Limit is the maximum number of results to return (1–50; the repo enforces the upper bound).
	Limit int
}

// Repository is the persistence contract for the Product aggregate.
type Repository interface {
	// Create persists a new Product.
	Create(ctx context.Context, p *Product) error

	// GetByID retrieves a Product by its UUID.
	GetByID(ctx context.Context, id uuid.UUID) (*Product, error)

	// GetByCode retrieves a non-deleted Product by its product code.
	GetByCode(ctx context.Context, code string) (*Product, error)

	// List retrieves Products matching the filter with pagination.
	// Returns the matching items, the total count across all pages, and any error.
	List(ctx context.Context, f ListFilter) (items []*Product, total int, err error)

	// Update persists mutations to an existing Product.
	Update(ctx context.Context, p *Product) error

	// Delete soft-deletes a Product by its UUID.
	Delete(ctx context.Context, id uuid.UUID, deletedBy string) error

	// SearchByText performs a full-text search for use in "match existing product" flows.
	SearchByText(ctx context.Context, opts SearchOptions) ([]*Product, error)

	// ListByRequestID retrieves Products linked to a specific request UUID, with pagination.
	ListByRequestID(ctx context.Context, requestID uuid.UUID, page, pageSize int) (items []*Product, total int, err error)
}
