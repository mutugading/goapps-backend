// Package audit provides application layer handlers for audit log management.
package audit

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/audit"
	"github.com/mutugading/goapps-backend/services/iam/pkg/safeconv"
)

// ListQuery contains parameters for listing audit logs.
type ListQuery struct {
	Page        int
	PageSize    int
	Search      string
	EventType   string
	UserID      *uuid.UUID
	TableName   string
	ServiceName string
	DateFrom    *time.Time
	DateTo      *time.Time
	SortBy      string
	SortOrder   string
}

// ListResult contains the result of listing audit logs.
type ListResult struct {
	Logs        []*audit.Log
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// ListHandler handles listing audit logs.
type ListHandler struct {
	repo audit.Repository
}

// NewListHandler creates a new ListHandler.
func NewListHandler(repo audit.Repository) *ListHandler {
	return &ListHandler{repo: repo}
}

// Handle executes the list audit logs query.
func (h *ListHandler) Handle(ctx context.Context, query ListQuery) (*ListResult, error) {
	page := query.Page
	if page < 1 {
		page = 1
	}
	pageSize := query.PageSize
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}

	params := audit.ListParams{
		Page:        page,
		PageSize:    pageSize,
		Search:      query.Search,
		EventType:   audit.EventType(query.EventType),
		UserID:      query.UserID,
		TableName:   query.TableName,
		ServiceName: query.ServiceName,
		DateFrom:    query.DateFrom,
		DateTo:      query.DateTo,
		SortBy:      query.SortBy,
		SortOrder:   query.SortOrder,
	}

	logs, total, err := h.repo.List(ctx, params)
	if err != nil {
		return nil, err
	}

	totalPages := int32(0)
	if total > 0 {
		totalPages = safeconv.Int64ToInt32((total + int64(pageSize) - 1) / int64(pageSize))
	}

	return &ListResult{
		Logs:        logs,
		TotalItems:  total,
		TotalPages:  totalPages,
		CurrentPage: safeconv.IntToInt32(page),
		PageSize:    safeconv.IntToInt32(pageSize),
	}, nil
}
