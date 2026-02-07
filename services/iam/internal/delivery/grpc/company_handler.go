// Package grpc provides gRPC handlers for the IAM service.
package grpc

import (
	"context"

	"github.com/google/uuid"
	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/organization"
)

// getUserFromCtx extracts the user ID from context, falling back to "system".
func getUserFromCtx(ctx context.Context) string {
	if id, err := getUserIDFromContext(ctx); err == nil {
		return id.String()
	}
	return "system"
}

// CompanyHandler handles company-related gRPC requests.
type CompanyHandler struct {
	iamv1.UnimplementedCompanyServiceServer
	repo             organization.CompanyRepository
	validationHelper *ValidationHelper
}

// NewCompanyHandler creates a new CompanyHandler.
func NewCompanyHandler(repo organization.CompanyRepository, validationHelper *ValidationHelper) *CompanyHandler {
	return &CompanyHandler{repo: repo, validationHelper: validationHelper}
}

func (h *CompanyHandler) CreateCompany(ctx context.Context, req *iamv1.CreateCompanyRequest) (*iamv1.CreateCompanyResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.CreateCompanyResponse{Base: baseResp}, nil
	}

	// Check for existing code
	exists, err := h.repo.ExistsByCode(ctx, req.GetCompanyCode())
	if err != nil {
		return &iamv1.CreateCompanyResponse{
			Base: InternalErrorResponse("Failed to check existing company"),
		}, nil
	}
	if exists {
		return &iamv1.CreateCompanyResponse{
			Base: ConflictResponse("Company code already exists"),
		}, nil
	}

	userID := getUserFromCtx(ctx)
	company, err := organization.NewCompany(req.GetCompanyCode(), req.GetCompanyName(), req.GetDescription(), userID)
	if err != nil {
		return &iamv1.CreateCompanyResponse{
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	if err := h.repo.Create(ctx, company); err != nil {
		return &iamv1.CreateCompanyResponse{
			Base: InternalErrorResponse("Failed to create company"),
		}, nil
	}

	return &iamv1.CreateCompanyResponse{
		Base: &commonv1.BaseResponse{IsSuccess: true, StatusCode: "201", Message: "Company created successfully"},
		Data: toCompanyProto(company),
	}, nil
}

func (h *CompanyHandler) GetCompany(ctx context.Context, req *iamv1.GetCompanyRequest) (*iamv1.GetCompanyResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.GetCompanyResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.GetCompanyId())
	if err != nil {
		return &iamv1.GetCompanyResponse{
			Base: ErrorResponse("400", "Invalid company ID"),
		}, nil
	}

	company, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return &iamv1.GetCompanyResponse{
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	return &iamv1.GetCompanyResponse{
		Base: SuccessResponse("Company retrieved successfully"),
		Data: toCompanyProto(company),
	}, nil
}

func (h *CompanyHandler) UpdateCompany(ctx context.Context, req *iamv1.UpdateCompanyRequest) (*iamv1.UpdateCompanyResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.UpdateCompanyResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.GetCompanyId())
	if err != nil {
		return &iamv1.UpdateCompanyResponse{
			Base: ErrorResponse("400", "Invalid company ID"),
		}, nil
	}

	company, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return &iamv1.UpdateCompanyResponse{
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	userID := getUserFromCtx(ctx)
	if err := company.Update(req.CompanyName, req.Description, req.IsActive, userID); err != nil {
		return &iamv1.UpdateCompanyResponse{
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	if err := h.repo.Update(ctx, company); err != nil {
		return &iamv1.UpdateCompanyResponse{
			Base: InternalErrorResponse("Failed to update company"),
		}, nil
	}

	return &iamv1.UpdateCompanyResponse{
		Base: SuccessResponse("Company updated successfully"),
		Data: toCompanyProto(company),
	}, nil
}

func (h *CompanyHandler) DeleteCompany(ctx context.Context, req *iamv1.DeleteCompanyRequest) (*iamv1.DeleteCompanyResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.DeleteCompanyResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.GetCompanyId())
	if err != nil {
		return &iamv1.DeleteCompanyResponse{
			Base: ErrorResponse("400", "Invalid company ID"),
		}, nil
	}

	userID := getUserFromCtx(ctx)
	if err := h.repo.Delete(ctx, id, userID); err != nil {
		return &iamv1.DeleteCompanyResponse{
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	return &iamv1.DeleteCompanyResponse{
		Base: SuccessResponse("Company deleted successfully"),
	}, nil
}

func (h *CompanyHandler) ListCompanies(ctx context.Context, req *iamv1.ListCompaniesRequest) (*iamv1.ListCompaniesResponse, error) {
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

	params := organization.ListParams{
		Page:      page,
		PageSize:  pageSize,
		Search:    req.GetSearch(),
		IsActive:  isActive,
		SortBy:    req.GetSortBy(),
		SortOrder: req.GetSortOrder(),
	}

	companies, total, err := h.repo.List(ctx, params)
	if err != nil {
		return &iamv1.ListCompaniesResponse{
			Base: InternalErrorResponse("Failed to list companies"),
		}, nil
	}

	totalPages := int32(0)
	if total > 0 {
		totalPages = int32((total + int64(pageSize) - 1) / int64(pageSize))
	}

	return &iamv1.ListCompaniesResponse{
		Base:       SuccessResponse("Companies listed successfully"),
		Data:       toCompanyProtos(companies),
		Pagination: &commonv1.PaginationResponse{CurrentPage: int32(page), PageSize: int32(pageSize), TotalItems: total, TotalPages: totalPages},
	}, nil
}

func (h *CompanyHandler) ExportCompanies(ctx context.Context, req *iamv1.ExportCompaniesRequest) (*iamv1.ExportCompaniesResponse, error) {
	// TODO: Implement Excel export.
	return &iamv1.ExportCompaniesResponse{
		Base: ErrorResponse("501", "Export not implemented"),
	}, nil
}

func (h *CompanyHandler) ImportCompanies(ctx context.Context, req *iamv1.ImportCompaniesRequest) (*iamv1.ImportCompaniesResponse, error) {
	// TODO: Implement Excel import.
	return &iamv1.ImportCompaniesResponse{
		Base: ErrorResponse("501", "Import not implemented"),
	}, nil
}

func (h *CompanyHandler) DownloadCompanyTemplate(ctx context.Context, req *iamv1.DownloadCompanyTemplateRequest) (*iamv1.DownloadCompanyTemplateResponse, error) {
	// TODO: Implement template download.
	return &iamv1.DownloadCompanyTemplateResponse{
		Base: ErrorResponse("501", "Template download not implemented"),
	}, nil
}

// Helper functions

func toCompanyProto(c *organization.Company) *iamv1.Company {
	if c == nil {
		return nil
	}
	return &iamv1.Company{
		CompanyId:   c.ID().String(),
		CompanyCode: c.Code(),
		CompanyName: c.Name(),
		Description: c.Description(),
		IsActive:    c.IsActive(),
		Audit:       toAuditProto(c.Audit()),
	}
}

func toCompanyProtos(companies []*organization.Company) []*iamv1.Company {
	result := make([]*iamv1.Company, len(companies))
	for i, c := range companies {
		result[i] = toCompanyProto(c)
	}
	return result
}
