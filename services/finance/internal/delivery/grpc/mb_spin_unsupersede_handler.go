package grpc

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	appmbspin "github.com/mutugading/goapps-backend/services/finance/internal/application/mbspin"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// WithUnsuperseder enables UnsupersedeMBSpin. sync may be nil (no after-commit recalc).
func (h *MBSpinHandler) WithUnsuperseder(repo mbspin.Unsuperseder, sync appmbspin.LookupSyncTrigger) *MBSpinHandler {
	h.unsupersedeHandler = appmbspin.NewUnsupersedeHandler(repo).WithLookupSyncTrigger(sync)
	return h
}

// UnsupersedeMBSpin restores a superseded spin (and its params, row by row) in one tx.
func (h *MBSpinHandler) UnsupersedeMBSpin(ctx context.Context, req *financev1.UnsupersedeMBSpinRequest) (*financev1.UnsupersedeMBSpinResponse, error) {
	if h.unsupersedeHandler == nil {
		RecordMBSpinOperation("unsupersede", false)
		return nil, status.Error(codes.Unimplemented, "UnsupersedeMBSpin is not enabled on this server build")
	}
	headID, err := uuid.Parse(req.GetMbhId())
	if err != nil {
		RecordMBSpinOperation("unsupersede", false)
		return &financev1.UnsupersedeMBSpinResponse{Base: invalidIDResponse("mbh_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}
	spinID, err := uuid.Parse(req.GetMbsId())
	if err != nil {
		RecordMBSpinOperation("unsupersede", false)
		return &financev1.UnsupersedeMBSpinResponse{Base: invalidIDResponse("mbs_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}
	res, err := h.unsupersedeHandler.Handle(ctx, appmbspin.UnsupersedeCommand{
		HeadID: headID, SpinID: spinID, ActorUserID: getUserFromContext(ctx),
	})
	if err != nil {
		RecordMBSpinOperation("unsupersede", false)
		return &financev1.UnsupersedeMBSpinResponse{Base: mbSpinErrorToBaseResponse(err)}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}
	RecordMBSpinOperation("unsupersede", true)
	return &financev1.UnsupersedeMBSpinResponse{
		Base:           successResponse(unsupersedeMessage(res)),
		Data:           mbSpinEntityToProto(res.Spin),
		ParamsReverted: safeconv.IntToInt32(res.ParamsReverted),
		Skipped:        unsupersedeSkippedToProto(res.Skipped),
	}, nil
}

func unsupersedeMessage(res *mbspin.UnsupersedeResult) string {
	msg := "MB spin restored"
	for _, w := range res.Warnings {
		msg += "; " + w
	}
	return msg
}

func unsupersedeSkippedToProto(in []mbspin.UnsupersedeSkipped) []*financev1.MBUnsupersedeSkippedParam {
	out := make([]*financev1.MBUnsupersedeSkippedParam, 0, len(in))
	for _, s := range in {
		out = append(out, &financev1.MBUnsupersedeSkippedParam{
			CppValueId: s.CppValueID, ProductSysId: s.ProductSysID, ProductCode: s.ProductCode,
			ParamCode: s.ParamCode, Reason: s.Reason,
		})
	}
	return out
}
