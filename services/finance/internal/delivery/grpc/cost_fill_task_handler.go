package grpc

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	app "github.com/mutugading/goapps-backend/services/finance/internal/application/costfillassignment"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costfillassignment"
)

// fillCompletionGate is a minimal completion gate that checks whether all fill tasks
// for a request are approved. State-machine integration is deferred.
type fillCompletionGate struct {
	taskRepo domain.TaskRepository
}

func (g *fillCompletionGate) CheckAndAdvance(ctx context.Context, requestID int64) error {
	n, err := g.taskRepo.CountNonApproved(ctx, requestID)
	if err != nil {
		return fmt.Errorf("count non-approved: %w", err)
	}
	if n == 0 {
		log.Info().Int64("request_id", requestID).Msg("all fill tasks approved")
	}
	return nil
}

// CostFillTaskHandler implements financev1.CostFillTaskServiceServer.
type CostFillTaskHandler struct {
	financev1.UnimplementedCostFillTaskServiceServer
	listTasks   *app.ListTasksHandler
	claimTask   *app.ClaimTaskHandler
	submitFill  *app.SubmitFillHandler
	approveTask *app.ApproveTaskHandler
	rejectTask  *app.RejectTaskHandler
}

// NewCostFillTaskHandler constructs the handler.
func NewCostFillTaskHandler(taskRepo domain.TaskRepository) *CostFillTaskHandler {
	gate := &fillCompletionGate{taskRepo: taskRepo}
	return &CostFillTaskHandler{
		listTasks:   app.NewListTasksHandler(taskRepo),
		claimTask:   app.NewClaimTaskHandler(taskRepo),
		submitFill:  app.NewSubmitFillHandler(taskRepo, gate),
		approveTask: app.NewApproveTaskHandler(taskRepo, gate),
		rejectTask:  app.NewRejectTaskHandler(taskRepo),
	}
}

// ListFillTasks returns all fill tasks for a cost product request.
func (h *CostFillTaskHandler) ListFillTasks(ctx context.Context, req *financev1.ListFillTasksRequest) (*financev1.ListFillTasksResponse, error) {
	result, err := h.listTasks.Handle(ctx, app.ListTasksQuery{RequestID: req.GetRequestId()})
	if err != nil {
		return &financev1.ListFillTasksResponse{Base: fillTaskErrToBase(err)}, nil
	}
	data := make([]*financev1.FillTask, 0, len(result.Tasks))
	for _, t := range result.Tasks {
		pt := taskToProto(t)
		if approvals, ok := result.Approvals[t.TaskID]; ok {
			pt.Approvals = approvalsToProto(approvals)
		}
		data = append(data, pt)
	}
	return &financev1.ListFillTasksResponse{
		Base: successResponse("OK"),
		Data: data,
	}, nil
}

// ClaimFillTask marks the authenticated user as the active filler for a task.
func (h *CostFillTaskHandler) ClaimFillTask(ctx context.Context, req *financev1.ClaimFillTaskRequest) (*financev1.ClaimFillTaskResponse, error) {
	userID := actorFromCtx(ctx)
	err := h.claimTask.Handle(ctx, app.ClaimTaskCommand{
		TaskID: req.GetTaskId(),
		UserID: userID,
	})
	if err != nil {
		return &financev1.ClaimFillTaskResponse{Base: fillTaskErrToBase(err)}, nil
	}
	return &financev1.ClaimFillTaskResponse{Base: successResponse("Task claimed")}, nil
}

// SubmitFillTask moves a FILLING task to APPROVAL_PENDING (or APPROVED if no approver).
func (h *CostFillTaskHandler) SubmitFillTask(ctx context.Context, req *financev1.SubmitFillTaskRequest) (*financev1.SubmitFillTaskResponse, error) {
	userID := actorFromCtx(ctx)
	err := h.submitFill.Handle(ctx, app.SubmitFillCommand{
		TaskID:    req.GetTaskId(),
		RequestID: req.GetRequestId(),
		UserID:    userID,
	})
	if err != nil {
		return &financev1.SubmitFillTaskResponse{Base: fillTaskErrToBase(err)}, nil
	}
	return &financev1.SubmitFillTaskResponse{Base: successResponse("Task submitted")}, nil
}

// ApproveFillTask approves a fill task and checks the completion gate.
func (h *CostFillTaskHandler) ApproveFillTask(ctx context.Context, req *financev1.ApproveFillTaskRequest) (*financev1.ApproveFillTaskResponse, error) {
	approverID := actorFromCtx(ctx)
	err := h.approveTask.Handle(ctx, app.ApproveTaskCommand{
		TaskID:     req.GetTaskId(),
		RequestID:  req.GetRequestId(),
		ApproverID: approverID,
		Note:       req.GetNote(),
	})
	if err != nil {
		return &financev1.ApproveFillTaskResponse{Base: fillTaskErrToBase(err)}, nil
	}
	return &financev1.ApproveFillTaskResponse{Base: successResponse("Task approved")}, nil
}

// RejectFillTask rejects a fill task and records the rejection event.
func (h *CostFillTaskHandler) RejectFillTask(ctx context.Context, req *financev1.RejectFillTaskRequest) (*financev1.RejectFillTaskResponse, error) {
	approverID := actorFromCtx(ctx)
	err := h.rejectTask.Handle(ctx, app.RejectTaskCommand{
		TaskID:     req.GetTaskId(),
		ApproverID: approverID,
		Reason:     req.GetReason(),
	})
	if err != nil {
		return &financev1.RejectFillTaskResponse{Base: fillTaskErrToBase(err)}, nil
	}
	return &financev1.RejectFillTaskResponse{Base: successResponse("Task rejected")}, nil
}

// =============================================================================
// proto <-> domain mappers
// =============================================================================

func taskToProto(t *domain.Task) *financev1.FillTask {
	if t == nil {
		return nil
	}
	p := &financev1.FillTask{
		TaskId:            t.TaskID,
		RequestId:         t.RequestID,
		RouteHeadId:       t.RouteHeadID,
		RouteLevel:        t.RouteLevel,
		FillerType:        t.FillerType,
		FillerValue:       t.FillerValue,
		ApproverType:      t.ApproverType,
		ApproverValue:     t.ApproverValue,
		Status:            t.Status(),
		ClaimedBy:         t.ClaimedBy,
		ReapproveOnChange: t.ReapproveOnChange,
		SlaFillHours:      t.SLAFillHours,
		SlaApproveHours:   t.SLAApproveHours,
		TotalParams:       t.TotalParams,
		FilledParams:      t.FilledParams,
		ActivatedAt:       t.ActivatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
	return p
}

func approvalsToProto(approvals []*domain.Approval) []*financev1.FillApproval {
	out := make([]*financev1.FillApproval, 0, len(approvals))
	for _, a := range approvals {
		if a == nil {
			continue
		}
		out = append(out, &financev1.FillApproval{
			ApprovalId: a.ApprovalID,
			TaskId:     a.TaskID,
			Decision:   a.Decision,
			DecidedBy:  a.DecidedBy,
			DecidedAt:  a.DecidedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
			Note:       a.Note,
			Trigger:    a.Trigger,
		})
	}
	return out
}

// =============================================================================
// error mapping
// =============================================================================

func fillTaskErrToBase(err error) *commonv1.BaseResponse {
	switch {
	case errors.Is(err, domain.ErrTaskNotFound):
		return ErrorResponse("404", "fill task not found")
	case errors.Is(err, domain.ErrConfigNotFound):
		return ErrorResponse("404", "config not found for route level")
	case errors.Is(err, domain.ErrAlreadyClaimed):
		return ErrorResponse("409", "fill task already claimed by another user")
	case errors.Is(err, domain.ErrInvalidTransition):
		return ErrorResponse("400", "invalid fill task state transition")
	case errors.Is(err, domain.ErrNoApprover):
		return ErrorResponse("400", "fill task has no approver configured")
	}
	return ErrorResponse("500", err.Error())
}
