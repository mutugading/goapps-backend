package grpc

import (
	"context"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/lookupmaster"
)

// LookupMasterHandler implements financev1.LookupMasterServiceServer.
type LookupMasterHandler struct {
	financev1.UnimplementedLookupMasterServiceServer
	repo lookupmaster.Repository
}

// NewLookupMasterHandler creates a new LookupMasterHandler.
func NewLookupMasterHandler(repo lookupmaster.Repository) (*LookupMasterHandler, error) {
	return &LookupMasterHandler{repo: repo}, nil
}

// ListLookupMasters returns all registered master lookup codes.
func (h *LookupMasterHandler) ListLookupMasters(ctx context.Context, req *financev1.ListLookupMastersRequest) (*financev1.ListLookupMastersResponse, error) { //nolint:nilerr // BaseResponse pattern
	masters, err := h.repo.ListMasters(ctx, req.GetActiveOnly())
	if err != nil {
		return &financev1.ListLookupMastersResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	items := make([]*financev1.LookupMaster, 0, len(masters))
	for _, m := range masters {
		items = append(items, &financev1.LookupMaster{
			LmCode:        m.Code,
			LmDisplayName: m.DisplayName,
			LmApiPath:     m.APIPath,
			LmCodeField:   m.CodeField,
			LmLabelField:  m.LabelField,
			LmIsActive:    m.IsActive,
		})
	}
	return &financev1.ListLookupMastersResponse{Base: successResponse(""), Data: items}, nil
}

// ListLookupMasterColumns returns fillable columns for a given master code.
func (h *LookupMasterHandler) ListLookupMasterColumns(ctx context.Context, req *financev1.ListLookupMasterColumnsRequest) (*financev1.ListLookupMasterColumnsResponse, error) { //nolint:nilerr // BaseResponse pattern
	cols, err := h.repo.ListColumns(ctx, req.GetMasterCode())
	if err != nil {
		return &financev1.ListLookupMasterColumnsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	items := make([]*financev1.LookupMasterColumn, 0, len(cols))
	for _, c := range cols {
		items = append(items, &financev1.LookupMasterColumn{
			LmcMasterCode:  c.MasterCode,
			LmcColumnName:  c.ColumnName,
			LmcDisplayName: c.DisplayName,
			LmcDataType:    c.DataType,
			LmcSortOrder:   int32(c.SortOrder), //nolint:gosec // sort_order is bounded (seeded data, max ~100)
		})
	}
	return &financev1.ListLookupMasterColumnsResponse{Base: successResponse(""), Data: items}, nil
}

var _ financev1.LookupMasterServiceServer = (*LookupMasterHandler)(nil)
