package grpc

import (
	"context"
	"errors"

	"github.com/google/uuid"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	cpmapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductmaster"
	cpm "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
)

// ProductEtlService is the application port behind the Oracle -> Product Master ETL RPCs.
type ProductEtlService interface {
	Preview(ctx context.Context, cmd cpmapp.PreviewEtlCommand) (cpmapp.EtlPreviewResult, error)
	Apply(ctx context.Context, cmd cpmapp.ApplyEtlCommand) (cpmapp.EtlApplyResult, error)
}

// WithProductEtl enables the Oracle product ETL RPCs.
func (h *CostProductMasterHandler) WithProductEtl(svc ProductEtlService) { h.productEtl = svc }

func etlNotConfigured() *commonv1.BaseResponse {
	return ErrorResponse("503", cpm.ErrEtlNotConfigured.Error())
}

func etlErrToBase(err error) *commonv1.BaseResponse {
	switch {
	case errors.Is(err, cpm.ErrEtlNotConfigured):
		return etlNotConfigured()
	case errors.Is(err, cpm.ErrEtlRunNotFound):
		return NotFoundResponse(err.Error())
	case errors.Is(err, cpm.ErrEtlRunNotPreview),
		errors.Is(err, cpm.ErrEtlApplyLimitExceeded),
		errors.Is(err, cpm.ErrEtlInvalidFromDate):
		return BadRequestResponse(err.Error())
	default:
		return InternalErrorResponse(err.Error())
	}
}

func etlRowsToProto(rows []cpm.EtlRunRow) []*financev1.ProductEtlRow {
	out := make([]*financev1.ProductEtlRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, &financev1.ProductEtlRow{
			RowKey: r.RowKey, MergeItemOrion: r.ItemCode, ItemName: r.ItemName,
			ShadeCode: r.ShadeCode, ShadeName: r.ShadeName, TypeCode: r.TypeCode,
			Status: r.Status, Selectable: r.Selectable, MatchedProductIds: r.MatchedIDs,
			Denier: r.Denier, Filament: r.Filament, Note: r.Note,
		})
	}
	return out
}

func etlCountsToProto(c cpm.EtlCounts) *financev1.ProductEtlCounts {
	return &financev1.ProductEtlCounts{
		Exists: c.Exists, ExistsMulti: c.ExistsMulti, NewShade: c.NewShade, NewItem: c.NewItem,
		PossibleDuplicate: c.PossibleDuplicate, InvalidType: c.InvalidType, Total: c.Total,
	}
}

func etlApplyRowsToProto(rows []cpm.EtlInsertResult) []*financev1.ProductEtlApplyRow {
	out := make([]*financev1.ProductEtlApplyRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, &financev1.ProductEtlApplyRow{
			RowKey: r.RowKey, Result: r.Result, ProductSysId: r.ProductSysID,
			ProductCode: r.ProductCode, Message: r.Message,
		})
	}
	return out
}

// PreviewProductEtlFromOracle reads Oracle (SELECT-only), classifies rows and stores a PREVIEW run.
func (h *CostProductMasterHandler) PreviewProductEtlFromOracle(ctx context.Context, req *financev1.PreviewProductEtlFromOracleRequest) (*financev1.PreviewProductEtlFromOracleResponse, error) {
	if h.productEtl == nil {
		return &financev1.PreviewProductEtlFromOracleResponse{Base: etlNotConfigured()}, nil
	}
	actor, _ := GetUserIDFromCtx(ctx)
	res, err := h.productEtl.Preview(ctx, cpmapp.PreviewEtlCommand{
		FromDate: req.GetFromDate(), Search: req.GetSearch(), IncludeExisting: req.GetIncludeExisting(), Actor: actor,
	})
	if err != nil {
		return &financev1.PreviewProductEtlFromOracleResponse{Base: etlErrToBase(err)}, nil
	}
	return &financev1.PreviewProductEtlFromOracleResponse{
		Base: successResponse("ETL preview"),
		Data: &financev1.ProductEtlPreview{
			RunId: res.RunID.String(), FromDate: res.FromDate, Rows: etlRowsToProto(res.Rows),
			Counts: etlCountsToProto(res.Counts), ApplyLimit: int32(res.ApplyLimit), //nolint:gosec // G115: limit is small config value
		},
	}, nil
}

// ApplyProductEtlFromOracle inserts the selected rows of a stored PREVIEW run (insert-only).
func (h *CostProductMasterHandler) ApplyProductEtlFromOracle(ctx context.Context, req *financev1.ApplyProductEtlFromOracleRequest) (*financev1.ApplyProductEtlFromOracleResponse, error) {
	if h.productEtl == nil {
		return &financev1.ApplyProductEtlFromOracleResponse{Base: etlNotConfigured()}, nil
	}
	runID, err := uuid.Parse(req.GetPreviewRunId())
	if err != nil {
		return &financev1.ApplyProductEtlFromOracleResponse{Base: BadRequestResponse("invalid preview_run_id")}, nil //nolint:nilerr // error is carried in BaseResponse
	}
	actor, _ := GetUserIDFromCtx(ctx)
	res, err := h.productEtl.Apply(ctx, cpmapp.ApplyEtlCommand{PreviewRunID: runID, RowKeys: req.GetRowKeys(), Actor: actor})
	if err != nil {
		return &financev1.ApplyProductEtlFromOracleResponse{Base: etlErrToBase(err)}, nil
	}
	return &financev1.ApplyProductEtlFromOracleResponse{
		Base: successResponse("ETL apply"),
		Data: &financev1.ProductEtlApplyResult{
			RunId: res.RunID.String(), Inserted: res.Inserted, Skipped: res.Skipped, Failed: res.Failed,
			Rows: etlApplyRowsToProto(res.Rows),
		},
	}, nil
}
