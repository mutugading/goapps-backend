// Package audit provides application layer handlers for audit log management.
package audit

import (
	"context"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/audit"
)

// SummaryQuery contains parameters for retrieving audit summary.
type SummaryQuery struct {
	TimeRange   string
	ServiceName string
}

// SummaryHandler handles retrieving audit summary statistics.
type SummaryHandler struct {
	repo audit.Repository
}

// NewSummaryHandler creates a new SummaryHandler.
func NewSummaryHandler(repo audit.Repository) *SummaryHandler {
	return &SummaryHandler{repo: repo}
}

// Handle executes the get audit summary query.
func (h *SummaryHandler) Handle(ctx context.Context, query SummaryQuery) (*audit.Summary, error) {
	timeRange := query.TimeRange
	if timeRange == "" {
		timeRange = "24h"
	}

	return h.repo.GetSummary(ctx, timeRange, query.ServiceName)
}
