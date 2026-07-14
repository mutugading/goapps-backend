// Package grpc provides gRPC server implementation for finance service.
package grpc

import (
	"context"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	appmbworkflowlog "github.com/mutugading/goapps-backend/services/finance/internal/application/mbworkflowlog"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbworkflowlog"
)

// MBWorkflowLogHandler implements financev1.MbWorkflowLogServiceServer.
type MBWorkflowLogHandler struct {
	financev1.UnimplementedMbWorkflowLogServiceServer
	listHandler *appmbworkflowlog.ListHandler
	validation  *ValidationHelper
}

// NewMBWorkflowLogHandler constructs an MBWorkflowLogHandler.
func NewMBWorkflowLogHandler(repo mbworkflowlog.Repository) (*MBWorkflowLogHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &MBWorkflowLogHandler{
		listHandler: appmbworkflowlog.NewListHandler(repo),
		validation:  v,
	}, nil
}

// ListMbWorkflowLogs lists workflow state transitions for an MB head.
func (h *MBWorkflowLogHandler) ListMbWorkflowLogs(ctx context.Context, req *financev1.ListMbWorkflowLogsRequest) (*financev1.ListMbWorkflowLogsResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBWorkflowLogOperation("list", false)
		return &financev1.ListMbWorkflowLogsResponse{Base: baseResp}, nil
	}

	entities, err := h.listHandler.Handle(ctx, appmbworkflowlog.ListQuery{MbhID: req.MbhId})
	if err != nil {
		RecordMBWorkflowLogOperation("list", false)
		return &financev1.ListMbWorkflowLogsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBWorkflowLogOperation("list", true)

	items := make([]*financev1.MbWorkflowLog, len(entities))
	for i, e := range entities {
		items[i] = mbWorkflowLogEntityToProto(e)
	}

	return &financev1.ListMbWorkflowLogsResponse{
		Base: successResponse("MB workflow logs retrieved successfully"),
		Data: items,
	}, nil
}

// mbWorkflowLogEntityToProto converts a domain mbworkflowlog Entity to its proto representation.
func mbWorkflowLogEntityToProto(e *mbworkflowlog.Entity) *financev1.MbWorkflowLog {
	return &financev1.MbWorkflowLog{
		MbwlId:      e.ID(),
		MbhId:       e.MbhID(),
		FromState:   e.FromState(),
		ToState:     e.ToState(),
		ActorUserId: e.ActorUserID(),
		ActorAt:     e.ActorAt(),
		Reason:      e.Reason(),
		Version:     e.Version(),
	}
}
