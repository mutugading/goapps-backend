// Package grpc provides gRPC server implementation for finance service.
package grpc

import (
	"context"
	"fmt"
	"strings"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbbatch"
)

// MBBatchHandler implements financev1.MbBatchServiceServer.
type MBBatchHandler struct {
	financev1.UnimplementedMbBatchServiceServer
	triggerHandler *mbbatch.TriggerHandler
	validation     *ValidationHelper
}

// NewMBBatchHandler constructs an MBBatchHandler.
func NewMBBatchHandler(triggerHandler *mbbatch.TriggerHandler) (*MBBatchHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &MBBatchHandler{
		triggerHandler: triggerHandler,
		validation:     v,
	}, nil
}

// TriggerMbBatch computes cst_product_cost rows for every VALIDATED MB Head for a period.
func (h *MBBatchHandler) TriggerMbBatch(ctx context.Context, req *financev1.TriggerMbBatchRequest) (*financev1.TriggerMbBatchResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBBatchOperation("trigger", false)
		return &financev1.TriggerMbBatchResponse{Base: baseResp}, nil
	}

	result, err := h.triggerHandler.Handle(ctx, req.Period, getUserFromContext(ctx))
	if err != nil {
		RecordMBBatchOperation("trigger", false)
		return &financev1.TriggerMbBatchResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBBatchOperation("trigger", true)

	return &financev1.TriggerMbBatchResponse{
		Base:         successResponse(triggerMbBatchMessage(result)),
		JobId:        result.JobID,
		Period:       result.Period,
		SuccessCount: result.SuccessCount,
		FailedCount:  result.FailedCount,
		RowsInserted: result.RowCount,
		DurationMs:   result.DurationMs,
		Errors:       mbBatchErrorsToProto(result.Errors),
	}, nil
}

// triggerMbBatchMessage summarizes an MB_BATCH trigger outcome, folding any per-MB partial
// failures into the base message (mirrors mb_push_handler.go's executePushMessage).
func triggerMbBatchMessage(result *mbbatch.TriggerResult) string {
	if len(result.Errors) == 0 {
		return "MB batch computed successfully"
	}
	failures := make([]string, len(result.Errors))
	for i, e := range result.Errors {
		failures[i] = fmt.Sprintf("%s: %s", e.MBHID, e.Error)
	}
	return fmt.Sprintf("MB batch completed with %d succeeded, %d failed (%s)",
		result.SuccessCount, len(result.Errors), strings.Join(failures, "; "))
}

// mbBatchErrorsToProto converts application-layer BatchError rows to their proto representation.
func mbBatchErrorsToProto(errs []mbbatch.BatchError) []*financev1.MbBatchError {
	out := make([]*financev1.MbBatchError, len(errs))
	for i, e := range errs {
		out[i] = &financev1.MbBatchError{
			MbhId: e.MBHID,
			Error: e.Error,
		}
	}
	return out
}
