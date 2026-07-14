// Package grpc provides gRPC server implementation for finance service.
package grpc

import (
	"context"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	appmbparam "github.com/mutugading/goapps-backend/services/finance/internal/application/mbparam"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbparam"
)

// MBParamHandler implements financev1.MbParamServiceServer.
type MBParamHandler struct {
	financev1.UnimplementedMbParamServiceServer
	createHandler       *appmbparam.CreateHandler
	updateHandler       *appmbparam.UpdateHandler
	deleteHandler       *appmbparam.DeleteHandler
	listHandler         *appmbparam.ListHandler
	createOptionHandler *appmbparam.CreateOptionHandler
	updateOptionHandler *appmbparam.UpdateOptionHandler
	deleteOptionHandler *appmbparam.DeleteOptionHandler
	exportHandler       *appmbparam.ExportHandler
	importHandler       *appmbparam.ImportHandler
	templateHandler     *appmbparam.TemplateHandler
	validation          *ValidationHelper
}

// NewMBParamHandler constructs an MBParamHandler.
func NewMBParamHandler(repo mbparam.Repository) (*MBParamHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &MBParamHandler{
		createHandler:       appmbparam.NewCreateHandler(repo),
		updateHandler:       appmbparam.NewUpdateHandler(repo),
		deleteHandler:       appmbparam.NewDeleteHandler(repo),
		listHandler:         appmbparam.NewListHandler(repo),
		createOptionHandler: appmbparam.NewCreateOptionHandler(repo),
		updateOptionHandler: appmbparam.NewUpdateOptionHandler(repo),
		deleteOptionHandler: appmbparam.NewDeleteOptionHandler(repo),
		exportHandler:       appmbparam.NewExportHandler(repo),
		importHandler:       appmbparam.NewImportHandler(repo),
		templateHandler:     appmbparam.NewTemplateHandler(),
		validation:          v,
	}, nil
}

// CreateMbParam creates a new MB param master record.
func (h *MBParamHandler) CreateMbParam(ctx context.Context, req *financev1.CreateMbParamRequest) (*financev1.CreateMbParamResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBParamOperation("create", false)
		return &financev1.CreateMbParamResponse{Base: baseResp}, nil
	}

	entity, err := h.createHandler.Handle(ctx, appmbparam.CreateCommand{
		Code:          req.Code,
		Name:          req.Name,
		Description:   req.Description,
		Type:          req.Type,
		DefaultValue:  req.DefaultValue,
		DefaultOption: req.DefaultOption,
		Unit:          req.Unit,
		DisplayOrder:  req.DisplayOrder,
		CreatedBy:     getUserFromContext(ctx),
	})
	if err != nil {
		RecordMBParamOperation("create", false)
		return &financev1.CreateMbParamResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBParamOperation("create", true)
	return &financev1.CreateMbParamResponse{
		Base: successResponse("MB param created successfully"),
		Data: mbParamEntityToProto(entity),
	}, nil
}

// UpdateMbParam updates an existing MB param master record.
func (h *MBParamHandler) UpdateMbParam(ctx context.Context, req *financev1.UpdateMbParamRequest) (*financev1.UpdateMbParamResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBParamOperation("update", false)
		return &financev1.UpdateMbParamResponse{Base: baseResp}, nil
	}

	entity, err := h.updateHandler.Handle(ctx, appmbparam.UpdateCommand{
		ID:            req.MbpId,
		Name:          req.Name,
		Description:   req.Description,
		DefaultValue:  req.DefaultValue,
		DefaultOption: req.DefaultOption,
		Unit:          req.Unit,
		DisplayOrder:  req.DisplayOrder,
		IsActive:      req.IsActive,
		UpdatedBy:     getUserFromContext(ctx),
	})
	if err != nil {
		RecordMBParamOperation("update", false)
		return &financev1.UpdateMbParamResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBParamOperation("update", true)
	return &financev1.UpdateMbParamResponse{
		Base: successResponse("MB param updated successfully"),
		Data: mbParamEntityToProto(entity),
	}, nil
}

// DeleteMbParam soft-deletes an MB param master record.
func (h *MBParamHandler) DeleteMbParam(ctx context.Context, req *financev1.DeleteMbParamRequest) (*financev1.DeleteMbParamResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBParamOperation("delete", false)
		return &financev1.DeleteMbParamResponse{Base: baseResp}, nil
	}

	if err := h.deleteHandler.Handle(ctx, appmbparam.DeleteCommand{ID: req.MbpId}); err != nil {
		RecordMBParamOperation("delete", false)
		return &financev1.DeleteMbParamResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBParamOperation("delete", true)
	return &financev1.DeleteMbParamResponse{Base: successResponse("MB param deleted successfully")}, nil
}

// ListMbParams lists MB param master records with search, sort, filter and pagination.
func (h *MBParamHandler) ListMbParams(ctx context.Context, req *financev1.ListMbParamsRequest) (*financev1.ListMbParamsResponse, error) {
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

	result, err := h.listHandler.Handle(ctx, appmbparam.ListQuery{
		Page:      req.Page,
		PageSize:  req.PageSize,
		Search:    req.Search,
		SortBy:    req.SortBy,
		SortOrder: req.SortDir,
		IsActive:  isActive,
	})
	if err != nil {
		RecordMBParamOperation("list", false)
		return &financev1.ListMbParamsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBParamOperation("list", true)

	items := make([]*financev1.MbParam, len(result.Items))
	for i, e := range result.Items {
		items[i] = mbParamEntityToProto(e)
	}

	return &financev1.ListMbParamsResponse{
		Base: successResponse("MB params retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

// CreateMbParamOption creates a new MB param picklist option.
func (h *MBParamHandler) CreateMbParamOption(ctx context.Context, req *financev1.CreateMbParamOptionRequest) (*financev1.CreateMbParamOptionResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBParamOperation("create_option", false)
		return &financev1.CreateMbParamOptionResponse{Base: baseResp}, nil
	}

	option, err := h.createOptionHandler.Handle(ctx, appmbparam.CreateOptionCommand{
		ParamCode:    req.MbpCode,
		Code:         req.Code,
		NumericValue: req.NumericValue,
		Description:  req.Description,
		DisplayOrder: req.DisplayOrder,
	})
	if err != nil {
		RecordMBParamOperation("create_option", false)
		return &financev1.CreateMbParamOptionResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBParamOperation("create_option", true)
	return &financev1.CreateMbParamOptionResponse{
		Base: successResponse("MB param option created successfully"),
		Data: mbParamOptionToProto(option),
	}, nil
}

// UpdateMbParamOption updates an existing MB param picklist option.
func (h *MBParamHandler) UpdateMbParamOption(ctx context.Context, req *financev1.UpdateMbParamOptionRequest) (*financev1.UpdateMbParamOptionResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBParamOperation("update_option", false)
		return &financev1.UpdateMbParamOptionResponse{Base: baseResp}, nil
	}

	option, err := h.updateOptionHandler.Handle(ctx, appmbparam.UpdateOptionCommand{
		ID:           req.MbpoId,
		NumericValue: req.NumericValue,
		Description:  req.Description,
		DisplayOrder: req.DisplayOrder,
		IsActive:     req.IsActive,
	})
	if err != nil {
		RecordMBParamOperation("update_option", false)
		return &financev1.UpdateMbParamOptionResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBParamOperation("update_option", true)
	return &financev1.UpdateMbParamOptionResponse{
		Base: successResponse("MB param option updated successfully"),
		Data: mbParamOptionToProto(option),
	}, nil
}

// DeleteMbParamOption soft-deletes an MB param picklist option.
func (h *MBParamHandler) DeleteMbParamOption(ctx context.Context, req *financev1.DeleteMbParamOptionRequest) (*financev1.DeleteMbParamOptionResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBParamOperation("delete_option", false)
		return &financev1.DeleteMbParamOptionResponse{Base: baseResp}, nil
	}

	if err := h.deleteOptionHandler.Handle(ctx, appmbparam.DeleteOptionCommand{ID: req.MbpoId}); err != nil {
		RecordMBParamOperation("delete_option", false)
		return &financev1.DeleteMbParamOptionResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBParamOperation("delete_option", true)
	return &financev1.DeleteMbParamOptionResponse{Base: successResponse("MB param option deleted successfully")}, nil
}

// ExportMbParams exports MB params to an Excel file.
func (h *MBParamHandler) ExportMbParams(ctx context.Context, req *financev1.ExportMbParamsRequest) (*financev1.ExportMbParamsResponse, error) {
	query := appmbparam.ExportQuery{}

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
		RecordMBParamOperation("export", false)
		return &financev1.ExportMbParamsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBParamOperation("export", true)

	return &financev1.ExportMbParamsResponse{
		Base:        successResponse("MB params exported successfully"),
		FileContent: result.FileContent,
		FileName:    result.FileName,
	}, nil
}

// ImportMbParams imports MB params from an Excel file.
func (h *MBParamHandler) ImportMbParams(ctx context.Context, req *financev1.ImportMbParamsRequest) (*financev1.ImportMbParamsResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBParamOperation("import", false)
		return &financev1.ImportMbParamsResponse{Base: baseResp}, nil
	}

	cmd := appmbparam.ImportCommand{
		FileContent:     req.FileContent,
		FileName:        req.FileName,
		DuplicateAction: req.DuplicateAction,
		CreatedBy:       getUserFromContext(ctx),
	}

	result, err := h.importHandler.Handle(ctx, cmd)
	if err != nil {
		RecordMBParamOperation("import", false)
		return &financev1.ImportMbParamsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBParamOperation("import", true)

	importErrors := make([]*financev1.ImportError, len(result.Errors))
	for i, e := range result.Errors {
		importErrors[i] = &financev1.ImportError{
			RowNumber: e.RowNumber,
			Field:     e.Field,
			Message:   e.Message,
		}
	}

	return &financev1.ImportMbParamsResponse{
		Base:         successResponse("Import completed"),
		SuccessCount: result.SuccessCount,
		SkippedCount: result.SkippedCount,
		FailedCount:  result.FailedCount,
		Errors:       importErrors,
	}, nil
}

// DownloadMbParamTemplate downloads the Excel import template for MB params.
func (h *MBParamHandler) DownloadMbParamTemplate(_ context.Context, _ *financev1.DownloadMbParamTemplateRequest) (*financev1.DownloadMbParamTemplateResponse, error) {
	result, err := h.templateHandler.Handle()
	if err != nil {
		return &financev1.DownloadMbParamTemplateResponse{Base: InternalErrorResponse(err.Error())}, nil
	}

	return &financev1.DownloadMbParamTemplateResponse{
		Base:        successResponse("Template generated successfully"),
		FileContent: result.FileContent,
		FileName:    result.FileName,
	}, nil
}

// mbParamEntityToProto converts a domain mbparam Entity to its proto representation.
func mbParamEntityToProto(e *mbparam.Entity) *financev1.MbParam {
	options := e.Options()
	protoOptions := make([]*financev1.MbParamOption, len(options))
	for i, o := range options {
		protoOptions[i] = mbParamOptionToProto(o)
	}

	return &financev1.MbParam{
		MbpId:         e.ID(),
		Code:          e.Code(),
		Name:          e.Name(),
		Description:   e.Description(),
		Type:          e.Type(),
		DefaultValue:  e.DefaultValue(),
		DefaultOption: e.DefaultOption(),
		Unit:          e.Unit(),
		DisplayOrder:  e.DisplayOrder(),
		IsActive:      e.IsActive(),
		Audit: &commonv1.AuditInfo{
			CreatedAt: e.CreatedAt(),
			CreatedBy: e.CreatedBy(),
			UpdatedAt: e.UpdatedAt(),
			UpdatedBy: e.UpdatedBy(),
		},
		Options: protoOptions,
	}
}

// mbParamOptionToProto converts a domain mbparam Option to its proto representation.
func mbParamOptionToProto(o *mbparam.Option) *financev1.MbParamOption {
	return &financev1.MbParamOption{
		MbpoId:       o.ID(),
		MbpCode:      o.ParamCode(),
		Code:         o.Code(),
		NumericValue: o.NumericValue(),
		Description:  o.Description(),
		DisplayOrder: o.DisplayOrder(),
		IsActive:     o.IsActive(),
	}
}
