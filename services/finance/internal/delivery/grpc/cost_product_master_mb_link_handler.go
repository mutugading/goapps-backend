package grpc

import (
	"context"
	"errors"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbsourceautofill"
	cpp "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductparameter"
)

// MBLinkService is the application port behind GetCostProductMbLink / RelinkCostProductMb.
type MBLinkService interface {
	Explain(ctx context.Context, productSysID int64) (mbsourceautofill.Link, error)
	Relink(ctx context.Context, productSysID int64, actor string) (mbsourceautofill.Link, error)
}

// WithMBLink enables the MB link explain / re-link RPCs.
func (h *CostProductMasterHandler) WithMBLink(svc MBLinkService) { h.mbLink = svc }

func mbLinkNotConfigured() *commonv1.BaseResponse {
	return &commonv1.BaseResponse{IsSuccess: false, StatusCode: "501", Message: "MB link is not configured"}
}

func mbLinkErrToBase(err error) *commonv1.BaseResponse {
	switch {
	case errors.Is(err, mbsourceautofill.ErrLinkProductNotFound):
		return NotFoundResponse(err.Error())
	case errors.Is(err, cpp.ErrProductLocked):
		return ConflictResponse(err.Error())
	case errors.Is(err, mbsourceautofill.ErrLinkMBType):
		return BadRequestResponse(err.Error())
	case errors.Is(err, mbsourceautofill.ErrLinkNotConfigured):
		return mbLinkNotConfigured()
	default:
		return InternalErrorResponse(err.Error())
	}
}

func mbLinkToProto(l mbsourceautofill.Link) *financev1.CostProductMbLink {
	return &financev1.CostProductMbLink{
		LinkSource:             l.LinkSource,
		SpinId:                 l.SpinID,
		SpinCode:               l.SpinCode,
		SpinShadeCode:          l.SpinShadeCode,
		FgProductId:            l.FgProductID,
		FgProductCode:          l.FgProductCode,
		CandidatePoyProductIds: l.CandidatePoyIDs,
		Reason:                 l.Reason,
		CurrentParamValue:      l.CurrentParamValue,
		ParamMatches:           l.ParamMatches,
	}
}

// GetCostProductMbLink explains (read-only) how the product's MB source resolves.
func (h *CostProductMasterHandler) GetCostProductMbLink(ctx context.Context, req *financev1.GetCostProductMbLinkRequest) (*financev1.GetCostProductMbLinkResponse, error) {
	if h.mbLink == nil {
		return &financev1.GetCostProductMbLinkResponse{Base: mbLinkNotConfigured()}, nil
	}
	l, err := h.mbLink.Explain(ctx, req.GetProductSysId())
	if err != nil {
		return &financev1.GetCostProductMbLinkResponse{Base: mbLinkErrToBase(err)}, nil
	}
	return &financev1.GetCostProductMbLinkResponse{Base: successResponse("MB link"), Data: mbLinkToProto(l)}, nil
}

// RelinkCostProductMb force-writes the resolved MB source of one product.
func (h *CostProductMasterHandler) RelinkCostProductMb(ctx context.Context, req *financev1.RelinkCostProductMbRequest) (*financev1.RelinkCostProductMbResponse, error) {
	if h.mbLink == nil {
		return &financev1.RelinkCostProductMbResponse{Base: mbLinkNotConfigured()}, nil
	}
	actor, _ := GetUserIDFromCtx(ctx)
	l, err := h.mbLink.Relink(ctx, req.GetProductSysId(), actor)
	if err != nil {
		return &financev1.RelinkCostProductMbResponse{Base: mbLinkErrToBase(err)}, nil
	}
	return &financev1.RelinkCostProductMbResponse{Base: successResponse("MB link re-linked"), Data: mbLinkToProto(l)}, nil
}
