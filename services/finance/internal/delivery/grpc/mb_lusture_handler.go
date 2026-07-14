// Package grpc provides gRPC server implementation for finance service.
package grpc

import (
	"context"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	appmblusture "github.com/mutugading/goapps-backend/services/finance/internal/application/mblusture"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mblusture"
)

// MBLustureHandler implements financev1.MbLustureServiceServer.
type MBLustureHandler struct {
	financev1.UnimplementedMbLustureServiceServer
	createHandler   *appmblusture.CreateHandler
	getHandler      *appmblusture.GetHandler
	listHandler     *appmblusture.ListHandler
	updateHandler   *appmblusture.UpdateHandler
	deleteHandler   *appmblusture.DeleteHandler
	exportHandler   *appmblusture.ExportHandler
	importHandler   *appmblusture.ImportHandler
	templateHandler *appmblusture.TemplateHandler
	validation      *ValidationHelper
}

// NewMBLustureHandler constructs an MBLustureHandler.
func NewMBLustureHandler(repo mblusture.Repository) (*MBLustureHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &MBLustureHandler{
		createHandler:   appmblusture.NewCreateHandler(repo),
		getHandler:      appmblusture.NewGetHandler(repo),
		listHandler:     appmblusture.NewListHandler(repo),
		updateHandler:   appmblusture.NewUpdateHandler(repo),
		deleteHandler:   appmblusture.NewDeleteHandler(repo),
		exportHandler:   appmblusture.NewExportHandler(repo),
		importHandler:   appmblusture.NewImportHandler(repo),
		templateHandler: appmblusture.NewTemplateHandler(),
		validation:      v,
	}, nil
}

// CreateMbLusture creates a new MB lusture master record.
func (h *MBLustureHandler) CreateMbLusture(ctx context.Context, req *financev1.CreateMbLustureRequest) (*financev1.CreateMbLustureResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBLustureOperation("create", false)
		return &financev1.CreateMbLustureResponse{Base: baseResp}, nil
	}

	entity, err := h.createHandler.Handle(ctx, appmblusture.CreateCommand{
		Code:            req.Code,
		DisplayName:     req.DisplayName,
		FullDescription: req.FullDescription,
		Category:        req.Category,
		DisplayOrder:    req.DisplayOrder,
		CreatedBy:       getUserFromContext(ctx),
	})
	if err != nil {
		RecordMBLustureOperation("create", false)
		return &financev1.CreateMbLustureResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBLustureOperation("create", true)
	return &financev1.CreateMbLustureResponse{
		Base: successResponse("MB lusture created successfully"),
		Data: mbLustureEntityToProto(entity),
	}, nil
}

// GetMbLusture retrieves an MB lusture master record by ID.
func (h *MBLustureHandler) GetMbLusture(ctx context.Context, req *financev1.GetMbLustureRequest) (*financev1.GetMbLustureResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBLustureOperation("get", false)
		return &financev1.GetMbLustureResponse{Base: baseResp}, nil
	}

	entity, err := h.getHandler.Handle(ctx, appmblusture.GetQuery{ID: req.MblId})
	if err != nil {
		RecordMBLustureOperation("get", false)
		return &financev1.GetMbLustureResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBLustureOperation("get", true)
	return &financev1.GetMbLustureResponse{
		Base: successResponse("MB lusture retrieved successfully"),
		Data: mbLustureEntityToProto(entity),
	}, nil
}

// UpdateMbLusture updates an existing MB lusture master record.
func (h *MBLustureHandler) UpdateMbLusture(ctx context.Context, req *financev1.UpdateMbLustureRequest) (*financev1.UpdateMbLustureResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBLustureOperation("update", false)
		return &financev1.UpdateMbLustureResponse{Base: baseResp}, nil
	}

	entity, err := h.updateHandler.Handle(ctx, appmblusture.UpdateCommand{
		ID:              req.MblId,
		DisplayName:     req.DisplayName,
		FullDescription: req.FullDescription,
		Category:        req.Category,
		DisplayOrder:    req.DisplayOrder,
		IsActive:        req.IsActive,
		UpdatedBy:       getUserFromContext(ctx),
	})
	if err != nil {
		RecordMBLustureOperation("update", false)
		return &financev1.UpdateMbLustureResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBLustureOperation("update", true)
	return &financev1.UpdateMbLustureResponse{
		Base: successResponse("MB lusture updated successfully"),
		Data: mbLustureEntityToProto(entity),
	}, nil
}

// DeleteMbLusture soft-deletes an MB lusture master record.
func (h *MBLustureHandler) DeleteMbLusture(ctx context.Context, req *financev1.DeleteMbLustureRequest) (*financev1.DeleteMbLustureResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBLustureOperation("delete", false)
		return &financev1.DeleteMbLustureResponse{Base: baseResp}, nil
	}

	if err := h.deleteHandler.Handle(ctx, appmblusture.DeleteCommand{ID: req.MblId}); err != nil {
		RecordMBLustureOperation("delete", false)
		return &financev1.DeleteMbLustureResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBLustureOperation("delete", true)
	return &financev1.DeleteMbLustureResponse{Base: successResponse("MB lusture deleted successfully")}, nil
}

// ListMbLusture lists MB lusture master records with search, sort, filter and pagination.
func (h *MBLustureHandler) ListMbLusture(ctx context.Context, req *financev1.ListMbLustureRequest) (*financev1.ListMbLustureResponse, error) {
	var isActive *bool
	switch req.ActiveFilter {
	case financev1.ActiveFilter_ACTIVE_FILTER_ACTIVE:
		t := true
		isActive = &t
	case financev1.ActiveFilter_ACTIVE_FILTER_INACTIVE:
		f := false
		isActive = &f
	default:
	}

	result, err := h.listHandler.Handle(ctx, appmblusture.ListQuery{
		Page:      req.Page,
		PageSize:  req.PageSize,
		Search:    req.Search,
		SortBy:    req.SortBy,
		SortOrder: req.SortDir,
		IsActive:  isActive,
	})
	if err != nil {
		RecordMBLustureOperation("list", false)
		return &financev1.ListMbLustureResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBLustureOperation("list", true)

	items := make([]*financev1.MbLusture, len(result.Items))
	for i, e := range result.Items {
		items[i] = mbLustureEntityToProto(e)
	}

	return &financev1.ListMbLustureResponse{
		Base: successResponse("MB lustures retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

// ExportMbLusture exports MB lustures to an Excel file.
func (h *MBLustureHandler) ExportMbLusture(ctx context.Context, req *financev1.ExportMbLustureRequest) (*financev1.ExportMbLustureResponse, error) {
	query := appmblusture.ExportQuery{}

	switch req.ActiveFilter {
	case financev1.ActiveFilter_ACTIVE_FILTER_ACTIVE:
		active := true
		query.IsActive = &active
	case financev1.ActiveFilter_ACTIVE_FILTER_INACTIVE:
		active := false
		query.IsActive = &active
	case financev1.ActiveFilter_ACTIVE_FILTER_UNSPECIFIED:
		// Export all - no filter
	}

	result, err := h.exportHandler.Handle(ctx, query)
	if err != nil {
		RecordMBLustureOperation("export", false)
		return &financev1.ExportMbLustureResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBLustureOperation("export", true)

	return &financev1.ExportMbLustureResponse{
		Base:        successResponse("MB lustures exported successfully"),
		FileContent: result.FileContent,
		FileName:    result.FileName,
	}, nil
}

// ImportMbLusture imports MB lustures from an Excel file.
func (h *MBLustureHandler) ImportMbLusture(ctx context.Context, req *financev1.ImportMbLustureRequest) (*financev1.ImportMbLustureResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBLustureOperation("import", false)
		return &financev1.ImportMbLustureResponse{Base: baseResp}, nil
	}

	cmd := appmblusture.ImportCommand{
		FileContent:     req.FileContent,
		FileName:        req.FileName,
		DuplicateAction: req.DuplicateAction,
		CreatedBy:       getUserFromContext(ctx),
	}

	result, err := h.importHandler.Handle(ctx, cmd)
	if err != nil {
		RecordMBLustureOperation("import", false)
		return &financev1.ImportMbLustureResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBLustureOperation("import", true)

	importErrors := make([]*financev1.ImportError, len(result.Errors))
	for i, e := range result.Errors {
		importErrors[i] = &financev1.ImportError{
			RowNumber: e.RowNumber,
			Field:     e.Field,
			Message:   e.Message,
		}
	}

	return &financev1.ImportMbLustureResponse{
		Base:         successResponse("Import completed"),
		SuccessCount: result.SuccessCount,
		SkippedCount: result.SkippedCount,
		FailedCount:  result.FailedCount,
		Errors:       importErrors,
	}, nil
}

// DownloadMbLustureTemplate downloads the Excel import template for MB lustures.
func (h *MBLustureHandler) DownloadMbLustureTemplate(_ context.Context, _ *financev1.DownloadMbLustureTemplateRequest) (*financev1.DownloadMbLustureTemplateResponse, error) {
	result, err := h.templateHandler.Handle()
	if err != nil {
		return &financev1.DownloadMbLustureTemplateResponse{Base: InternalErrorResponse(err.Error())}, nil
	}

	return &financev1.DownloadMbLustureTemplateResponse{
		Base:        successResponse("Template generated successfully"),
		FileContent: result.FileContent,
		FileName:    result.FileName,
	}, nil
}

// mbLustureEntityToProto converts a domain mblusture Entity to its proto representation.
func mbLustureEntityToProto(e *mblusture.Entity) *financev1.MbLusture {
	p := &financev1.MbLusture{
		MblId:           e.ID(),
		Code:            e.Code(),
		DisplayName:     e.DisplayName(),
		FullDescription: e.FullDescription(),
		Category:        e.Category(),
		IsActive:        e.IsActive(),
		DisplayOrder:    e.DisplayOrder(),
		Audit: &commonv1.AuditInfo{
			CreatedAt: e.CreatedAt(),
			CreatedBy: e.CreatedBy(),
			UpdatedAt: e.UpdatedAt(),
			UpdatedBy: e.UpdatedBy(),
		},
	}
	return p
}
