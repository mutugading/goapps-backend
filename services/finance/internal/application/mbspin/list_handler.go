// Package mbspin provides application layer handlers for MB Spin operations.
package mbspin

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// ListQuery represents the list MB Spins query.
type ListQuery struct {
	HeadID    uuid.UUID
	Page      int
	PageSize  int
	Search    string
	IsActive  *bool
	SortBy    string
	SortOrder string
	// FG filters (design 2.2): FgProductID 0 = any; IncludeSuperseded false hides superseded spins.
	FgProductID       int64
	FgSearch          string
	IncludeSuperseded bool
}

// ListResult represents the list MB Spins result.
type ListResult struct {
	Items       []*mbspin.Entity
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// ListHandler handles the ListMBSpins query.
type ListHandler struct {
	repo mbspin.Repository
}

// NewListHandler creates a new ListHandler.
func NewListHandler(repo mbspin.Repository) *ListHandler {
	return &ListHandler{repo: repo}
}

// Handle executes the list MB Spins query.
func (h *ListHandler) Handle(ctx context.Context, query ListQuery) (*ListResult, error) {
	filter := mbspin.ListFilter{
		HeadID:    query.HeadID,
		Search:    query.Search,
		Page:      query.Page,
		PageSize:  query.PageSize,
		SortBy:    query.SortBy,
		SortOrder: query.SortOrder,
		IsActive:  query.IsActive,

		FgProductID:       query.FgProductID,
		FgSearch:          query.FgSearch,
		IncludeSuperseded: query.IncludeSuperseded,
	}
	filter.Validate()

	items, total, err := h.repo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	var totalPages int32
	if filter.PageSize > 0 && total > 0 {
		computed := (total + int64(filter.PageSize) - 1) / int64(filter.PageSize)
		totalPages = safeconv.Int64ToInt32(computed)
	}

	return &ListResult{
		Items:       items,
		TotalItems:  total,
		TotalPages:  totalPages,
		CurrentPage: safeconv.IntToInt32(filter.Page),
		PageSize:    safeconv.IntToInt32(filter.PageSize),
	}, nil
}
