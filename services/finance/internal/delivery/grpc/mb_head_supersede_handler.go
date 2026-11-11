package grpc

import (
	"context"
	"errors"

	"github.com/google/uuid"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	appmbhead "github.com/mutugading/goapps-backend/services/finance/internal/application/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// WithSupersedeReader wires the supersede preview (PreviewMBHeadSupersede). The repo is the
// handler's own mbhead.Repository passed again because MBHeadHandler does not keep it.
func (h *MBHeadHandler) WithSupersedeReader(repo mbhead.Repository, reader mbspin.SupersedeReader) *MBHeadHandler {
	if repo != nil && reader != nil {
		h.previewSupersedeH = appmbhead.NewPreviewSupersedeHandler(repo, reader)
	}
	return h
}

// PreviewMBHeadSupersede lists, read-only, the live spins of other recipes that a Validate
// of this head would supersede (same shade + FG), with the repoint counts (design §2.3).
func (h *MBHeadHandler) PreviewMBHeadSupersede(ctx context.Context, req *financev1.PreviewMBHeadSupersedeRequest) (*financev1.PreviewMBHeadSupersedeResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBHeadOperation("preview_supersede", false)
		return &financev1.PreviewMBHeadSupersedeResponse{Base: baseResp}, nil
	}
	if h.previewSupersedeH == nil {
		RecordMBHeadOperation("preview_supersede", false)
		return &financev1.PreviewMBHeadSupersedeResponse{Base: InternalErrorResponse("MB head supersede preview is not configured")}, nil
	}
	id, err := uuid.Parse(req.MbhId)
	if err != nil {
		RecordMBHeadOperation("preview_supersede", false)
		return &financev1.PreviewMBHeadSupersedeResponse{Base: invalidIDResponse("mbh_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}
	preview, err := h.previewSupersedeH.Handle(ctx, appmbhead.PreviewSupersedeQuery{MbhID: id})
	if err != nil {
		RecordMBHeadOperation("preview_supersede", false)
		return &financev1.PreviewMBHeadSupersedeResponse{Base: supersedeErrToBase(err)}, nil
	}
	RecordMBHeadOperation("preview_supersede", true)
	return &financev1.PreviewMBHeadSupersedeResponse{
		Base: successResponse("MB head supersede preview retrieved successfully"),
		Data: supersedePreviewToProto(preview),
	}, nil
}

// supersedeErrToBase maps preview/supersede errors: head not found → 404, confirmation
// required → 409, everything else through the generic domain mapping.
func supersedeErrToBase(err error) *commonv1.BaseResponse {
	switch {
	case errors.Is(err, mbhead.ErrNotFound):
		return NotFoundResponse(err.Error())
	case errors.Is(err, mbspin.ErrSupersedeConfirmationRequired):
		return ConflictResponse(err.Error())
	default:
		return domainErrorToBaseResponse(err)
	}
}

// supersedePreviewToProto maps the domain preview; counts are clamped to int32.
func supersedePreviewToProto(p *mbspin.SupersedePreview) *financev1.MBHeadSupersedePreview {
	out := &financev1.MBHeadSupersedePreview{}
	if p == nil {
		return out
	}
	out.RequiresConfirmation = p.RequiresConfirmation
	out.Candidates = make([]*financev1.MBSupersedeCandidate, 0, len(p.Candidates))
	for i := range p.Candidates {
		out.Candidates = append(out.Candidates, supersedeCandidateToProto(&p.Candidates[i]))
	}
	return out
}

func supersedeCandidateToProto(c *mbspin.SupersedeCandidate) *financev1.MBSupersedeCandidate {
	pc := &financev1.MBSupersedeCandidate{
		OldSpinId:                  c.OldSpinID.String(),
		OldSpinCode:                c.OldSpinCode,
		ShadeCode:                  c.ShadeCode,
		FgProductId:                c.FgProductID,
		FgProductCode:              c.FgProductCode,
		OldHeadId:                  c.OldHeadID.String(),
		OldHeadCode:                c.OldHeadCode,
		OldHeadWillDeactivate:      c.OldHeadWillDeactivate,
		OldMbProductSysId:          c.OldMBProductSysID,
		OldMbProductWillDeactivate: c.OldMBProductWillDeactivate,
		ParamRowsPointer:           safeIntToInt32(c.ParamRowsPointer),
		ParamRowsTextOnly:          safeIntToInt32(c.ParamRowsTextOnly),
		FilledByCounts:             make([]*financev1.MBSupersedeFilledByCount, 0, len(c.FilledByCounts)),
		AffectedProducts:           make([]*financev1.MBSupersedeAffectedProduct, 0, len(c.AffectedProducts)),
		Warnings:                   append([]string{}, c.Warnings...),
	}
	for _, f := range c.FilledByCounts {
		pc.FilledByCounts = append(pc.FilledByCounts, &financev1.MBSupersedeFilledByCount{
			FilledBy: f.FilledBy, Count: safeIntToInt32(f.Count),
		})
	}
	for _, a := range c.AffectedProducts {
		pc.AffectedProducts = append(pc.AffectedProducts, &financev1.MBSupersedeAffectedProduct{
			ProductSysId: a.ProductSysID, ProductCode: a.ProductCode, ProductName: a.ProductName, FilledBy: a.FilledBy,
		})
	}
	return pc
}
