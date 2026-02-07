// Package organization provides application layer services for organization management.
package organization

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/organization"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// ErrConflict indicates a duplicate entity.
var ErrConflict = errors.New("entity already exists")

// ErrHasChildren indicates the entity has child entities.
var ErrHasChildren = errors.New("entity has child entities")

// Service provides organization management operations.
type Service struct {
	companyRepo    organization.CompanyRepository
	divisionRepo   organization.DivisionRepository
	departmentRepo organization.DepartmentRepository
	sectionRepo    organization.SectionRepository
}

// NewService creates a new organization service.
func NewService(
	companyRepo organization.CompanyRepository,
	divisionRepo organization.DivisionRepository,
	departmentRepo organization.DepartmentRepository,
	sectionRepo organization.SectionRepository,
) *Service {
	return &Service{
		companyRepo:    companyRepo,
		divisionRepo:   divisionRepo,
		departmentRepo: departmentRepo,
		sectionRepo:    sectionRepo,
	}
}

// =============================================================================
// COMPANY OPERATIONS
// =============================================================================

// CreateCompanyInput represents input for creating a company.
type CreateCompanyInput struct {
	Code        string
	Name        string
	Description string
	CreatedBy   string
}

// CreateCompany creates a new company.
func (s *Service) CreateCompany(ctx context.Context, input CreateCompanyInput) (*organization.Company, error) {
	// Check if code already exists
	exists, err := s.companyRepo.ExistsByCode(ctx, input.Code)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing company: %w", err)
	}
	if exists {
		return nil, ErrConflict
	}

	company, err := organization.NewCompany(input.Code, input.Name, input.Description, input.CreatedBy)
	if err != nil {
		return nil, fmt.Errorf("failed to create company entity: %w", err)
	}

	if err := s.companyRepo.Create(ctx, company); err != nil {
		return nil, fmt.Errorf("failed to save company: %w", err)
	}

	return company, nil
}

// GetCompanyByID retrieves a company by ID.
func (s *Service) GetCompanyByID(ctx context.Context, id uuid.UUID) (*organization.Company, error) {
	company, err := s.companyRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return nil, shared.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get company: %w", err)
	}
	return company, nil
}

// GetCompanyByCode retrieves a company by code.
func (s *Service) GetCompanyByCode(ctx context.Context, code string) (*organization.Company, error) {
	company, err := s.companyRepo.GetByCode(ctx, code)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return nil, shared.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get company: %w", err)
	}
	return company, nil
}

// UpdateCompanyInput represents input for updating a company.
type UpdateCompanyInput struct {
	ID          uuid.UUID
	Name        *string
	Description *string
	IsActive    *bool
	UpdatedBy   string
}

// UpdateCompany updates an existing company.
func (s *Service) UpdateCompany(ctx context.Context, input UpdateCompanyInput) (*organization.Company, error) {
	company, err := s.companyRepo.GetByID(ctx, input.ID)
	if err != nil {
		return nil, err
	}

	// Use the entity's Update method
	if err := company.Update(input.Name, input.Description, input.IsActive, input.UpdatedBy); err != nil {
		return nil, fmt.Errorf("failed to update company entity: %w", err)
	}

	if err := s.companyRepo.Update(ctx, company); err != nil {
		return nil, fmt.Errorf("failed to save company: %w", err)
	}

	return company, nil
}

// DeleteCompany soft deletes a company.
func (s *Service) DeleteCompany(ctx context.Context, id uuid.UUID, deletedBy string) error {
	// Check if company has divisions
	divisions, _, err := s.divisionRepo.List(ctx, organization.DivisionListParams{
		CompanyID: &id,
	})
	if err != nil {
		return fmt.Errorf("failed to check divisions: %w", err)
	}
	if len(divisions) > 0 {
		return ErrHasChildren
	}

	return s.companyRepo.Delete(ctx, id, deletedBy)
}

// ListCompanies lists companies with pagination.
func (s *Service) ListCompanies(ctx context.Context, params organization.ListParams) ([]*organization.Company, int64, error) {
	return s.companyRepo.List(ctx, params)
}

// =============================================================================
// DIVISION OPERATIONS
// =============================================================================

// CreateDivisionInput represents input for creating a division.
type CreateDivisionInput struct {
	CompanyID   uuid.UUID
	Code        string
	Name        string
	Description string
	CreatedBy   string
}

// CreateDivision creates a new division.
func (s *Service) CreateDivision(ctx context.Context, input CreateDivisionInput) (*organization.Division, error) {
	// Verify company exists
	_, err := s.companyRepo.GetByID(ctx, input.CompanyID)
	if err != nil {
		return nil, fmt.Errorf("invalid company: %w", err)
	}

	// Check if code already exists
	exists, err := s.divisionRepo.ExistsByCode(ctx, input.Code)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing division: %w", err)
	}
	if exists {
		return nil, ErrConflict
	}

	division, err := organization.NewDivision(input.CompanyID, input.Code, input.Name, input.Description, input.CreatedBy)
	if err != nil {
		return nil, fmt.Errorf("failed to create division entity: %w", err)
	}

	if err := s.divisionRepo.Create(ctx, division); err != nil {
		return nil, fmt.Errorf("failed to save division: %w", err)
	}

	return division, nil
}

// GetDivisionByID retrieves a division by ID.
func (s *Service) GetDivisionByID(ctx context.Context, id uuid.UUID) (*organization.Division, error) {
	return s.divisionRepo.GetByID(ctx, id)
}

// UpdateDivisionInput represents input for updating a division.
type UpdateDivisionInput struct {
	ID          uuid.UUID
	Name        *string
	Description *string
	IsActive    *bool
	UpdatedBy   string
}

// UpdateDivision updates an existing division.
func (s *Service) UpdateDivision(ctx context.Context, input UpdateDivisionInput) (*organization.Division, error) {
	division, err := s.divisionRepo.GetByID(ctx, input.ID)
	if err != nil {
		return nil, err
	}

	if err := division.Update(input.Name, input.Description, input.IsActive, input.UpdatedBy); err != nil {
		return nil, fmt.Errorf("failed to update division entity: %w", err)
	}

	if err := s.divisionRepo.Update(ctx, division); err != nil {
		return nil, fmt.Errorf("failed to save division: %w", err)
	}

	return division, nil
}

// DeleteDivision soft deletes a division.
func (s *Service) DeleteDivision(ctx context.Context, id uuid.UUID, deletedBy string) error {
	// Check if division has departments
	departments, _, err := s.departmentRepo.List(ctx, organization.DepartmentListParams{
		DivisionID: &id,
	})
	if err != nil {
		return fmt.Errorf("failed to check departments: %w", err)
	}
	if len(departments) > 0 {
		return ErrHasChildren
	}

	return s.divisionRepo.Delete(ctx, id, deletedBy)
}

// ListDivisions lists divisions with pagination.
func (s *Service) ListDivisions(ctx context.Context, params organization.DivisionListParams) ([]*organization.Division, int64, error) {
	return s.divisionRepo.List(ctx, params)
}

// =============================================================================
// DEPARTMENT OPERATIONS
// =============================================================================

// CreateDepartmentInput represents input for creating a department.
type CreateDepartmentInput struct {
	DivisionID  uuid.UUID
	Code        string
	Name        string
	Description string
	CreatedBy   string
}

// CreateDepartment creates a new department.
func (s *Service) CreateDepartment(ctx context.Context, input CreateDepartmentInput) (*organization.Department, error) {
	// Verify division exists
	_, err := s.divisionRepo.GetByID(ctx, input.DivisionID)
	if err != nil {
		return nil, fmt.Errorf("invalid division: %w", err)
	}

	// Check if code already exists
	exists, err := s.departmentRepo.ExistsByCode(ctx, input.Code)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing department: %w", err)
	}
	if exists {
		return nil, ErrConflict
	}

	department, err := organization.NewDepartment(input.DivisionID, input.Code, input.Name, input.Description, input.CreatedBy)
	if err != nil {
		return nil, fmt.Errorf("failed to create department entity: %w", err)
	}

	if err := s.departmentRepo.Create(ctx, department); err != nil {
		return nil, fmt.Errorf("failed to save department: %w", err)
	}

	return department, nil
}

// GetDepartmentByID retrieves a department by ID.
func (s *Service) GetDepartmentByID(ctx context.Context, id uuid.UUID) (*organization.Department, error) {
	return s.departmentRepo.GetByID(ctx, id)
}

// UpdateDepartmentInput represents input for updating a department.
type UpdateDepartmentInput struct {
	ID          uuid.UUID
	Name        *string
	Description *string
	IsActive    *bool
	UpdatedBy   string
}

// UpdateDepartment updates an existing department.
func (s *Service) UpdateDepartment(ctx context.Context, input UpdateDepartmentInput) (*organization.Department, error) {
	department, err := s.departmentRepo.GetByID(ctx, input.ID)
	if err != nil {
		return nil, err
	}

	if err := department.Update(input.Name, input.Description, input.IsActive, input.UpdatedBy); err != nil {
		return nil, fmt.Errorf("failed to update department entity: %w", err)
	}

	if err := s.departmentRepo.Update(ctx, department); err != nil {
		return nil, fmt.Errorf("failed to save department: %w", err)
	}

	return department, nil
}

// DeleteDepartment soft deletes a department.
func (s *Service) DeleteDepartment(ctx context.Context, id uuid.UUID, deletedBy string) error {
	// Check if department has sections
	sections, _, err := s.sectionRepo.List(ctx, organization.SectionListParams{
		DepartmentID: &id,
	})
	if err != nil {
		return fmt.Errorf("failed to check sections: %w", err)
	}
	if len(sections) > 0 {
		return ErrHasChildren
	}

	return s.departmentRepo.Delete(ctx, id, deletedBy)
}

// ListDepartments lists departments with pagination.
func (s *Service) ListDepartments(ctx context.Context, params organization.DepartmentListParams) ([]*organization.Department, int64, error) {
	return s.departmentRepo.List(ctx, params)
}

// =============================================================================
// SECTION OPERATIONS
// =============================================================================

// CreateSectionInput represents input for creating a section.
type CreateSectionInput struct {
	DepartmentID uuid.UUID
	Code         string
	Name         string
	Description  string
	CreatedBy    string
}

// CreateSection creates a new section.
func (s *Service) CreateSection(ctx context.Context, input CreateSectionInput) (*organization.Section, error) {
	// Verify department exists
	_, err := s.departmentRepo.GetByID(ctx, input.DepartmentID)
	if err != nil {
		return nil, fmt.Errorf("invalid department: %w", err)
	}

	// Check if code already exists
	exists, err := s.sectionRepo.ExistsByCode(ctx, input.Code)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing section: %w", err)
	}
	if exists {
		return nil, ErrConflict
	}

	section, err := organization.NewSection(input.DepartmentID, input.Code, input.Name, input.Description, input.CreatedBy)
	if err != nil {
		return nil, fmt.Errorf("failed to create section entity: %w", err)
	}

	if err := s.sectionRepo.Create(ctx, section); err != nil {
		return nil, fmt.Errorf("failed to save section: %w", err)
	}

	return section, nil
}

// GetSectionByID retrieves a section by ID.
func (s *Service) GetSectionByID(ctx context.Context, id uuid.UUID) (*organization.Section, error) {
	return s.sectionRepo.GetByID(ctx, id)
}

// UpdateSectionInput represents input for updating a section.
type UpdateSectionInput struct {
	ID          uuid.UUID
	Name        *string
	Description *string
	IsActive    *bool
	UpdatedBy   string
}

// UpdateSection updates an existing section.
func (s *Service) UpdateSection(ctx context.Context, input UpdateSectionInput) (*organization.Section, error) {
	section, err := s.sectionRepo.GetByID(ctx, input.ID)
	if err != nil {
		return nil, err
	}

	if err := section.Update(input.Name, input.Description, input.IsActive, input.UpdatedBy); err != nil {
		return nil, fmt.Errorf("failed to update section entity: %w", err)
	}

	if err := s.sectionRepo.Update(ctx, section); err != nil {
		return nil, fmt.Errorf("failed to save section: %w", err)
	}

	return section, nil
}

// DeleteSection soft deletes a section.
func (s *Service) DeleteSection(ctx context.Context, id uuid.UUID, deletedBy string) error {
	return s.sectionRepo.Delete(ctx, id, deletedBy)
}

// ListSections lists sections with pagination.
func (s *Service) ListSections(ctx context.Context, params organization.SectionListParams) ([]*organization.Section, int64, error) {
	return s.sectionRepo.List(ctx, params)
}
