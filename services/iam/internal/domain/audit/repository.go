// Package audit provides domain logic for audit logging.
package audit

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Repository defines the interface for audit log persistence operations.
type Repository interface {
	// Create creates a new audit log entry.
	Create(ctx context.Context, log *Log) error

	// GetByID retrieves an audit log by ID.
	GetByID(ctx context.Context, id uuid.UUID) (*Log, error)

	// List lists audit logs with filters.
	List(ctx context.Context, params ListParams) ([]*Log, int64, error)

	// GetSummary retrieves audit statistics for a time range.
	GetSummary(ctx context.Context, timeRange string, serviceName string) (*Summary, error)
}

// ListParams contains parameters for listing audit logs.
type ListParams struct {
	Page        int
	PageSize    int
	Search      string
	EventType   EventType
	UserID      *uuid.UUID
	TableName   string
	ServiceName string
	DateFrom    *time.Time
	DateTo      *time.Time
	SortBy      string
	SortOrder   string
}
