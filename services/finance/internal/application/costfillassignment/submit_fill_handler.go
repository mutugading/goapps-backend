package costfillassignment

import (
	"context"
	"fmt"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costfillassignment"
)

// SubmitFillCommand carries the submit request.
type SubmitFillCommand struct {
	TaskID    int64
	RequestID int64
	UserID    string
}

// SubmitFillHandler transitions a FILLING task to APPROVAL_PENDING or APPROVED.
type SubmitFillHandler struct {
	repo domain.TaskRepository
	gate CompletionGate
}

// NewSubmitFillHandler constructs the handler.
func NewSubmitFillHandler(repo domain.TaskRepository, gate CompletionGate) *SubmitFillHandler {
	return &SubmitFillHandler{repo: repo, gate: gate}
}

// Handle loads the task, calls Submit() on the domain object, saves it, and if auto-approved checks the gate.
func (h *SubmitFillHandler) Handle(ctx context.Context, cmd SubmitFillCommand) error {
	if cmd.TaskID <= 0 {
		return fmt.Errorf("task ID must be > 0")
	}
	task, err := h.repo.GetByID(ctx, cmd.TaskID)
	if err != nil {
		return fmt.Errorf("get task %d: %w", cmd.TaskID, err)
	}
	if err = task.Submit(); err != nil {
		return fmt.Errorf("submit task %d: %w", cmd.TaskID, err)
	}
	if err = h.repo.Save(ctx, task); err != nil {
		return fmt.Errorf("save task %d after submit: %w", cmd.TaskID, err)
	}
	// If the task auto-approved (no approver), check if all tasks are done.
	if task.Status() == domain.StatusApproved {
		if gateErr := h.gate.CheckAndAdvance(ctx, cmd.RequestID); gateErr != nil {
			return fmt.Errorf("completion gate after auto-approve: %w", gateErr)
		}
	}
	return nil
}
