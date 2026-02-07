// Package grpc provides gRPC handlers for the IAM service.
package grpc

import (
	"context"

	"github.com/google/uuid"
	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/organization"
)

// DepartmentHandler handles department-related gRPC requests.
type DepartmentHandler struct {
	iamv1.UnimplementedDepartmentServiceServer
	repo             organization.DepartmentRepository
	validationHelper *ValidationHelper
}

// NewDepartmentHandler creates a new DepartmentHandler.
func NewDepartmentHandler(repo organization.DepartmentRepository, validationHelper *ValidationHelper) *DepartmentHandler {
	return &DepartmentHandler{repo: repo, validationHelper: validationHelper}
}

func (h *DepartmentHandler) CreateDepartment(ctx context.Context, req *iamv1.CreateDepartmentRequest) (*iamv1.CreateDepartmentResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.CreateDepartmentResponse{Base: baseResp}, nil
	}

	divisionID, err := uuid.Parse(req.GetDivisionId())
	if err != nil {
		return &iamv1.CreateDepartmentResponse{
			Base: ErrorResponse("400", "Invalid division ID"),
		}, nil
	}

	exists, err := h.repo.ExistsByCode(ctx, req.GetDepartmentCode())
	if err != nil {
		return &iamv1.CreateDepartmentResponse{
			Base: InternalErrorResponse("Failed to check existing department"),
		}, nil
	}
	if exists {
		return &iamv1.CreateDepartmentResponse{
			Base: ConflictResponse("Department code already exists"),
		}, nil
	}

	userID := getUserFromCtx(ctx)
	department, err := organization.NewDepartment(divisionID, req.GetDepartmentCode(), req.GetDepartmentName(), req.GetDescription(), userID)
	if err != nil {
		return &iamv1.CreateDepartmentResponse{
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	if err := h.repo.Create(ctx, department); err != nil {
		return &iamv1.CreateDepartmentResponse{
			Base: InternalErrorResponse("Failed to create department"),
		}, nil
	}

	return &iamv1.CreateDepartmentResponse{
		Base: &commonv1.BaseResponse{IsSuccess: true, StatusCode: "201", Message: "Department created successfully"},
		Data: toDepartmentProto(department),
	}, nil
}

func (h *DepartmentHandler) GetDepartment(ctx context.Context, req *iamv1.GetDepartmentRequest) (*iamv1.GetDepartmentResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.GetDepartmentResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.GetDepartmentId())
	if err != nil {
		return &iamv1.GetDepartmentResponse{
			Base: ErrorResponse("400", "Invalid department ID"),
		}, nil
	}

	department, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return &iamv1.GetDepartmentResponse{
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	return &iamv1.GetDepartmentResponse{
		Base: SuccessResponse("Department retrieved successfully"),
		Data: toDepartmentProto(department),
	}, nil
}

func (h *DepartmentHandler) UpdateDepartment(ctx context.Context, req *iamv1.UpdateDepartmentRequest) (*iamv1.UpdateDepartmentResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.UpdateDepartmentResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.GetDepartmentId())
	if err != nil {
		return &iamv1.UpdateDepartmentResponse{
			Base: ErrorResponse("400", "Invalid department ID"),
		}, nil
	}

	department, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return &iamv1.UpdateDepartmentResponse{
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	userID := getUserFromCtx(ctx)
	if err := department.Update(req.DepartmentName, req.Description, req.IsActive, userID); err != nil {
		return &iamv1.UpdateDepartmentResponse{
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	if err := h.repo.Update(ctx, department); err != nil {
		return &iamv1.UpdateDepartmentResponse{
			Base: InternalErrorResponse("Failed to update department"),
		}, nil
	}

	return &iamv1.UpdateDepartmentResponse{
		Base: SuccessResponse("Department updated successfully"),
		Data: toDepartmentProto(department),
	}, nil
}

func (h *DepartmentHandler) DeleteDepartment(ctx context.Context, req *iamv1.DeleteDepartmentRequest) (*iamv1.DeleteDepartmentResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.DeleteDepartmentResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.GetDepartmentId())
	if err != nil {
		return &iamv1.DeleteDepartmentResponse{
			Base: ErrorResponse("400", "Invalid department ID"),
		}, nil
	}

	userID := getUserFromCtx(ctx)
	if err := h.repo.Delete(ctx, id, userID); err != nil {
		return &iamv1.DeleteDepartmentResponse{
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	return &iamv1.DeleteDepartmentResponse{
		Base: SuccessResponse("Department deleted successfully"),
	}, nil
}

func (h *DepartmentHandler) ListDepartments(ctx context.Context, req *iamv1.ListDepartmentsRequest) (*iamv1.ListDepartmentsResponse, error) {
	page := int(req.GetPage())
	pageSize := int(req.GetPageSize())
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 10
	}

	var isActive *bool
	switch req.GetActiveFilter() {
	case iamv1.ActiveFilter_ACTIVE_FILTER_ACTIVE:
		active := true
		isActive = &active
	case iamv1.ActiveFilter_ACTIVE_FILTER_INACTIVE:
		inactive := false
		isActive = &inactive
	}

	var divisionID, companyID *uuid.UUID
	if req.GetDivisionId() != "" {
		did, err := uuid.Parse(req.GetDivisionId())
		if err == nil {
			divisionID = &did
		}
	}
	if req.GetCompanyId() != "" {
		cid, err := uuid.Parse(req.GetCompanyId())
		if err == nil {
			companyID = &cid
		}
	}

	params := organization.DepartmentListParams{
		ListParams: organization.ListParams{
			Page:      page,
			PageSize:  pageSize,
			Search:    req.GetSearch(),
			IsActive:  isActive,
			SortBy:    req.GetSortBy(),
			SortOrder: req.GetSortOrder(),
		},
		DivisionID: divisionID,
		CompanyID:  companyID,
	}

	departments, total, err := h.repo.List(ctx, params)
	if err != nil {
		return &iamv1.ListDepartmentsResponse{
			Base: InternalErrorResponse("Failed to list departments"),
		}, nil
	}

	totalPages := int32(0)
	if total > 0 {
		totalPages = int32((total + int64(pageSize) - 1) / int64(pageSize))
	}

	return &iamv1.ListDepartmentsResponse{
		Base:       SuccessResponse("Departments listed successfully"),
		Data:       toDepartmentProtos(departments),
		Pagination: &commonv1.PaginationResponse{CurrentPage: int32(page), PageSize: int32(pageSize), TotalItems: total, TotalPages: totalPages},
	}, nil
}

func (h *DepartmentHandler) ExportDepartments(ctx context.Context, req *iamv1.ExportDepartmentsRequest) (*iamv1.ExportDepartmentsResponse, error) {
	return &iamv1.ExportDepartmentsResponse{
		Base: ErrorResponse("501", "Export not implemented"),
	}, nil
}

func (h *DepartmentHandler) ImportDepartments(ctx context.Context, req *iamv1.ImportDepartmentsRequest) (*iamv1.ImportDepartmentsResponse, error) {
	return &iamv1.ImportDepartmentsResponse{
		Base: ErrorResponse("501", "Import not implemented"),
	}, nil
}

func (h *DepartmentHandler) DownloadDepartmentTemplate(ctx context.Context, req *iamv1.DownloadDepartmentTemplateRequest) (*iamv1.DownloadDepartmentTemplateResponse, error) {
	return &iamv1.DownloadDepartmentTemplateResponse{
		Base: ErrorResponse("501", "Template download not implemented"),
	}, nil
}

func toDepartmentProto(d *organization.Department) *iamv1.Department {
	if d == nil {
		return nil
	}
	return &iamv1.Department{
		DepartmentId:   d.ID().String(),
		DivisionId:     d.DivisionID().String(),
		DepartmentCode: d.Code(),
		DepartmentName: d.Name(),
		Description:    d.Description(),
		IsActive:       d.IsActive(),
		Audit:          toAuditProto(d.Audit()),
	}
}

func toDepartmentProtos(departments []*organization.Department) []*iamv1.Department {
	result := make([]*iamv1.Department, len(departments))
	for i, d := range departments {
		result[i] = toDepartmentProto(d)
	}
	return result
}
