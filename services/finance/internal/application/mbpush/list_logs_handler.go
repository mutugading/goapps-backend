package mbpush

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbpushlog"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// ListLogsQuery represents the list MB push logs query.
type ListLogsQuery struct {
	Page     int32
	PageSize int32
	Period   string
}

// ListLogsResult represents the list MB push logs result.
type ListLogsResult struct {
	Items       []*mbpushlog.Entity
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// ListLogsHandler handles the ListMbPushLogs query.
type ListLogsHandler struct {
	repo mbpushlog.Repository
}

// NewListLogsHandler creates a new ListLogsHandler.
func NewListLogsHandler(repo mbpushlog.Repository) *ListLogsHandler {
	return &ListLogsHandler{repo: repo}
}

// Handle executes the list MB push logs query.
func (h *ListLogsHandler) Handle(ctx context.Context, query ListLogsQuery) (*ListLogsResult, error) {
	page := query.Page
	if page < 1 {
		page = 1
	}
	pageSize := query.PageSize
	if pageSize < 1 {
		pageSize = 10
	}

	items, total, err := h.repo.List(ctx, page, pageSize, query.Period)
	if err != nil {
		return nil, err
	}

	var totalPages int32
	if pageSize > 0 && total > 0 {
		computed := (total + int64(pageSize) - 1) / int64(pageSize)
		totalPages = safeconv.Int64ToInt32(computed)
	}

	return &ListLogsResult{
		Items:       items,
		TotalItems:  total,
		TotalPages:  totalPages,
		CurrentPage: page,
		PageSize:    pageSize,
	}, nil
}
