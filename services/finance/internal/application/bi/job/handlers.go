// Package job provides application-layer handlers for BI ETL job listing + manual triggers.
//
// In MVP the Trigger handler records a placeholder log entry (RUNNING → SUCCESS) without
// actually executing the Oracle procedure. Spec 1D wires the real RabbitMQ → finance-bi-worker
// bridge that turns this trigger into a real job run.
package job

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	jobdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/job"
)

// ListHandler returns the job registry with last-run summary.
type ListHandler struct{ repo jobdomain.Repository }

// NewListHandler constructs a ListHandler.
func NewListHandler(r jobdomain.Repository) *ListHandler { return &ListHandler{repo: r} }

// Handle returns jobs.
func (h *ListHandler) Handle(ctx context.Context, includeInactive bool) ([]*jobdomain.Job, error) {
	return h.repo.List(ctx, includeInactive)
}

// ListLogsQuery is the payload for ListLogsHandler.
type ListLogsQuery struct {
	JobID    uuid.UUID
	Page     int
	PageSize int
}

// ListLogsResult bundles the page + total count.
type ListLogsResult struct {
	Items []*jobdomain.Log
	Total int64
}

// ListLogsHandler returns paginated logs for a single job.
type ListLogsHandler struct{ repo jobdomain.Repository }

// NewListLogsHandler constructs a ListLogsHandler.
func NewListLogsHandler(r jobdomain.Repository) *ListLogsHandler { return &ListLogsHandler{repo: r} }

// Handle executes the list-logs query.
func (h *ListLogsHandler) Handle(ctx context.Context, q ListLogsQuery) (ListLogsResult, error) {
	items, total, err := h.repo.ListLogs(ctx, q.JobID, q.Page, q.PageSize)
	if err != nil {
		return ListLogsResult{}, fmt.Errorf("list job logs: %w", err)
	}
	return ListLogsResult{Items: items, Total: total}, nil
}

// MVRefresher refreshes BI materialized views.
// Injected into TriggerHandler so the mv_refresh job kind can trigger a view
// refresh without executing an Oracle fetch.
type MVRefresher interface {
	RefreshMVs(ctx context.Context) error
}

// TriggerCommand is the payload for TriggerHandler.
type TriggerCommand struct {
	JobID       uuid.UUID
	TriggeredBy uuid.UUID
}

// TriggerHandler records a manual job trigger.
//
// In MVP (spec 1A+1B): inserts a RUNNING log row, then immediately marks it SUCCESS with
// rows_affected=0 — a placeholder that lets the admin UI verify the trigger button works
// without actually running Oracle procedures. Spec 1D replaces this body with a RabbitMQ
// publish to the finance-bi-worker queue.
//
// Special case: jobs with config["kind"]=="mv_refresh" skip the Oracle path and only
// call MVRefresher.RefreshMVs, then mark SUCCESS with rows_affected=0.
type TriggerHandler struct {
	repo        jobdomain.Repository
	mvRefresher MVRefresher // optional — nil when not needed
}

// NewTriggerHandler constructs a TriggerHandler.
func NewTriggerHandler(r jobdomain.Repository, mv MVRefresher) *TriggerHandler {
	return &TriggerHandler{repo: r, mvRefresher: mv}
}

// Handle executes the manual trigger.
func (h *TriggerHandler) Handle(ctx context.Context, cmd TriggerCommand) (*jobdomain.Log, error) {
	theJob, err := h.repo.GetByID(ctx, cmd.JobID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	entry := &jobdomain.Log{
		JobID:       cmd.JobID,
		StartedAt:   now,
		Status:      jobdomain.StatusRunning,
		TriggeredBy: "MANUAL:" + cmd.TriggeredBy.String(),
	}
	if err = h.repo.InsertLog(ctx, entry); err != nil {
		return nil, fmt.Errorf("insert running log: %w", err)
	}

	// mv_refresh job kind: refresh materialized views and mark SUCCESS immediately.
	if theJob.Config["kind"] == "mv_refresh" {
		return h.handleMVRefresh(ctx, entry, now)
	}

	// MVP: immediately resolve to SUCCESS placeholder. Real work happens in spec 1D.
	ended := time.Now().UTC()
	entry.EndedAt = ended
	entry.Status = jobdomain.StatusSuccess
	entry.DurationMs = int(ended.Sub(now).Milliseconds())
	if err = h.repo.UpdateLog(ctx, entry); err != nil {
		return nil, fmt.Errorf("update completion log: %w", err)
	}
	return entry, nil
}

// handleMVRefresh calls RefreshMVs and writes the result back to the log.
func (h *TriggerHandler) handleMVRefresh(ctx context.Context, entry *jobdomain.Log, started time.Time) (*jobdomain.Log, error) {
	var refreshErr error
	if h.mvRefresher != nil {
		refreshErr = h.mvRefresher.RefreshMVs(ctx)
	}
	ended := time.Now().UTC()
	entry.EndedAt = ended
	entry.DurationMs = int(ended.Sub(started).Milliseconds())
	if refreshErr != nil {
		entry.Status = jobdomain.StatusFailed
		entry.ErrorMessage = refreshErr.Error()
	} else {
		entry.Status = jobdomain.StatusSuccess
	}
	if err := h.repo.UpdateLog(ctx, entry); err != nil {
		return nil, fmt.Errorf("update mv_refresh completion log: %w", err)
	}
	return entry, nil
}
