package mbparam

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbparam"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// ListQuery represents the list MB param query.
type ListQuery struct {
	Page      int32
	PageSize  int32
	Search    string
	SortBy    string
	SortOrder string
	IsActive  *bool
}

// ListResult represents the list MB param result.
type ListResult struct {
	Items       []*mbparam.Entity
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// ListHandler handles the ListMbParams query.
type ListHandler struct {
	repo mbparam.Repository
}

// NewListHandler creates a new ListHandler.
func NewListHandler(repo mbparam.Repository) *ListHandler {
	return &ListHandler{repo: repo}
}

// Handle executes the list MB param query.
func (h *ListHandler) Handle(ctx context.Context, query ListQuery) (*ListResult, error) {
	filter := mbparam.ListFilter{
		Search:    query.Search,
		IsActive:  query.IsActive,
		Page:      query.Page,
		PageSize:  query.PageSize,
		SortBy:    query.SortBy,
		SortOrder: query.SortOrder,
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
		CurrentPage: filter.Page,
		PageSize:    filter.PageSize,
	}, nil
}
