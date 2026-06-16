package costproductrequest

import (
	"context"
	"fmt"
)

// ParamValueRow is one parameter cell for a product at a route level.
type ParamValueRow struct {
	ParamID      string
	ParamCode    string
	ParamName    string
	DataType     string
	HasValue     bool
	ValueNumeric string
	ValueText    string
	ValueFlag    bool
	UOMCode      string
	IsRequired   bool
}

// LevelSummaryRow groups params for one fill task level.
type LevelSummaryRow struct {
	RouteLevel     int32
	TaskStatus     string
	FilledByUserID string
	FilledAt       string
	FilledParams   int32
	TotalParams    int32
	Params         []ParamValueRow
	LastEditedBy   string
	LastEditedAt   string
}

// ProductSummaryRow is one product in the param summary.
type ProductSummaryRow struct {
	ProductSysID int64
	ProductCode  string
	ProductName  string
	Levels       []LevelSummaryRow
}

// ParamSummaryRepository fetches the full param summary for a request.
type ParamSummaryRepository interface {
	GetParamSummary(ctx context.Context, requestID int64) ([]ProductSummaryRow, error)
}

// LevelEditInfo holds the last-override metadata for a route level.
type LevelEditInfo struct {
	ChangedBy string
	ChangedAt string
}

// ParamEditLogLoader returns the most recent edit log entry per route level for a request.
// The map key is route_level. Returns nil, nil when no overrides have been recorded.
type ParamEditLogLoader interface {
	GetLastEditInfoPerLevel(ctx context.Context, requestID int64) (map[int]LevelEditInfo, error)
}

// ParamEditLogRow is a single audit record returned by ListParamEditLog.
type ParamEditLogRow struct {
	ParamCode string
	OldValue  string
	NewValue  string
	ChangedBy string
	ChangedAt string // RFC-3339
}

// ParamEditLogByLevelReader returns the full override audit history for one fill level.
type ParamEditLogByLevelReader interface {
	ListByRequestLevel(ctx context.Context, requestID int64, routeLevel int) ([]ParamEditLogRow, error)
}

// GetParamSummaryHandler fetches the param summary for a CPR.
type GetParamSummaryHandler struct {
	repo     ParamSummaryRepository
	editLogs ParamEditLogLoader
}

// NewGetParamSummaryHandler constructs the handler.
func NewGetParamSummaryHandler(repo ParamSummaryRepository) *GetParamSummaryHandler {
	return &GetParamSummaryHandler{repo: repo}
}

// WithEditLogLoader attaches an optional edit log loader for last-edited-by metadata.
func (h *GetParamSummaryHandler) WithEditLogLoader(l ParamEditLogLoader) *GetParamSummaryHandler {
	h.editLogs = l
	return h
}

// GetParamSummaryQuery is the input.
type GetParamSummaryQuery struct {
	RequestID int64
}

// Handle executes the query and returns products, totalParams, filledParams.
func (h *GetParamSummaryHandler) Handle(ctx context.Context, q GetParamSummaryQuery) ([]ProductSummaryRow, int32, int32, error) {
	if q.RequestID <= 0 {
		return nil, 0, 0, fmt.Errorf("invalid request ID")
	}
	products, err := h.repo.GetParamSummary(ctx, q.RequestID)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("get param summary: %w", err)
	}

	// Merge last-edit metadata when the loader is available.
	if h.editLogs != nil {
		edits, editErr := h.editLogs.GetLastEditInfoPerLevel(ctx, q.RequestID)
		if editErr != nil {
			return nil, 0, 0, fmt.Errorf("get edit log: %w", editErr)
		}
		for pi := range products {
			for li := range products[pi].Levels {
				level := int(products[pi].Levels[li].RouteLevel)
				if e, ok := edits[level]; ok {
					products[pi].Levels[li].LastEditedBy = e.ChangedBy
					products[pi].Levels[li].LastEditedAt = e.ChangedAt
				}
			}
		}
	}

	var total, filled int32
	for _, p := range products {
		for _, l := range p.Levels {
			total += l.TotalParams
			filled += l.FilledParams
		}
	}
	return products, total, filled, nil
}
