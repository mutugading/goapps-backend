// Package organization provides application layer services for organization management.
package organization

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/organization"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// MockCompanyRepository is a mock implementation of CompanyRepository.
type MockCompanyRepository struct {
	companies map[uuid.UUID]*organization.Company
	codeIndex map[string]uuid.UUID
	createErr error
	getErr    error
	updateErr error
	deleteErr error
	existsErr error
	listErr   error
}

func NewMockCompanyRepository() *MockCompanyRepository {
	return &MockCompanyRepository{
		companies: make(map[uuid.UUID]*organization.Company),
		codeIndex: make(map[string]uuid.UUID),
	}
}

func (m *MockCompanyRepository) Create(ctx context.Context, c *organization.Company) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.companies[c.ID()] = c
	m.codeIndex[c.Code()] = c.ID()
	return nil
}

func (m *MockCompanyRepository) GetByID(ctx context.Context, id uuid.UUID) (*organization.Company, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if c, ok := m.companies[id]; ok {
		return c, nil
	}
	return nil, shared.ErrNotFound
}

func (m *MockCompanyRepository) GetByCode(ctx context.Context, code string) (*organization.Company, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if id, ok := m.codeIndex[code]; ok {
		return m.companies[id], nil
	}
	return nil, shared.ErrNotFound
}

func (m *MockCompanyRepository) Update(ctx context.Context, c *organization.Company) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.companies[c.ID()] = c
	return nil
}

func (m *MockCompanyRepository) Delete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.companies, id)
	return nil
}

func (m *MockCompanyRepository) List(ctx context.Context, params organization.ListParams) ([]*organization.Company, int64, error) {
	if m.listErr != nil {
		return nil, 0, m.listErr
	}
	result := make([]*organization.Company, 0, len(m.companies))
	for _, c := range m.companies {
		result = append(result, c)
	}
	return result, int64(len(result)), nil
}

func (m *MockCompanyRepository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	if m.existsErr != nil {
		return false, m.existsErr
	}
	_, ok := m.codeIndex[code]
	return ok, nil
}

func (m *MockCompanyRepository) BatchCreate(ctx context.Context, companies []*organization.Company) (int, error) {
	count := 0
	for _, c := range companies {
		if err := m.Create(ctx, c); err == nil {
			count++
		}
	}
	return count, nil
}

// MockDivisionRepository is a mock implementation for testing.
type MockDivisionRepository struct {
	divisions []*organization.Division
}

func (m *MockDivisionRepository) Create(ctx context.Context, d *organization.Division) error {
	m.divisions = append(m.divisions, d)
	return nil
}
func (m *MockDivisionRepository) GetByID(ctx context.Context, id uuid.UUID) (*organization.Division, error) {
	for _, d := range m.divisions {
		if d.ID() == id {
			return d, nil
		}
	}
	return nil, shared.ErrNotFound
}
func (m *MockDivisionRepository) GetByCode(ctx context.Context, code string) (*organization.Division, error) {
	return nil, shared.ErrNotFound
}
func (m *MockDivisionRepository) Update(ctx context.Context, d *organization.Division) error {
	return nil
}
func (m *MockDivisionRepository) Delete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	return nil
}
func (m *MockDivisionRepository) List(ctx context.Context, params organization.DivisionListParams) ([]*organization.Division, int64, error) {
	return m.divisions, int64(len(m.divisions)), nil
}
func (m *MockDivisionRepository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	return false, nil
}
func (m *MockDivisionRepository) BatchCreate(ctx context.Context, divisions []*organization.Division) (int, error) {
	return 0, nil
}

// MockDepartmentRepository is a mock implementation for testing.
type MockDepartmentRepository struct{}

func (m *MockDepartmentRepository) Create(ctx context.Context, d *organization.Department) error {
	return nil
}
func (m *MockDepartmentRepository) GetByID(ctx context.Context, id uuid.UUID) (*organization.Department, error) {
	return nil, shared.ErrNotFound
}
func (m *MockDepartmentRepository) GetByCode(ctx context.Context, code string) (*organization.Department, error) {
	return nil, shared.ErrNotFound
}
func (m *MockDepartmentRepository) Update(ctx context.Context, d *organization.Department) error {
	return nil
}
func (m *MockDepartmentRepository) Delete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	return nil
}
func (m *MockDepartmentRepository) List(ctx context.Context, params organization.DepartmentListParams) ([]*organization.Department, int64, error) {
	return nil, 0, nil
}
func (m *MockDepartmentRepository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	return false, nil
}
func (m *MockDepartmentRepository) BatchCreate(ctx context.Context, departments []*organization.Department) (int, error) {
	return 0, nil
}

// MockSectionRepository is a mock implementation for testing.
type MockSectionRepository struct{}

func (m *MockSectionRepository) Create(ctx context.Context, s *organization.Section) error {
	return nil
}
func (m *MockSectionRepository) GetByID(ctx context.Context, id uuid.UUID) (*organization.Section, error) {
	return nil, shared.ErrNotFound
}
func (m *MockSectionRepository) GetByCode(ctx context.Context, code string) (*organization.Section, error) {
	return nil, shared.ErrNotFound
}
func (m *MockSectionRepository) Update(ctx context.Context, s *organization.Section) error {
	return nil
}
func (m *MockSectionRepository) Delete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	return nil
}
func (m *MockSectionRepository) List(ctx context.Context, params organization.SectionListParams) ([]*organization.Section, int64, error) {
	return nil, 0, nil
}
func (m *MockSectionRepository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	return false, nil
}
func (m *MockSectionRepository) BatchCreate(ctx context.Context, sections []*organization.Section) (int, error) {
	return 0, nil
}

// Tests

func TestService_CreateCompany(t *testing.T) {
	tests := []struct {
		name    string
		input   CreateCompanyInput
		setup   func(*MockCompanyRepository)
		wantErr bool
	}{
		{
			name: "successful creation",
			input: CreateCompanyInput{
				Code:        "ACME",
				Name:        "ACME Corporation",
				Description: "Main company",
				CreatedBy:   "admin",
			},
			setup:   func(m *MockCompanyRepository) {},
			wantErr: false,
		},
		{
			name: "duplicate code",
			input: CreateCompanyInput{
				Code:        "ACME",
				Name:        "ACME Corp",
				Description: "",
				CreatedBy:   "admin",
			},
			setup: func(m *MockCompanyRepository) {
				c, _ := organization.NewCompany("ACME", "Existing", "", "admin")
				m.Create(context.Background(), c)
			},
			wantErr: true,
		},
		{
			name: "invalid code format",
			input: CreateCompanyInput{
				Code:        "acme",
				Name:        "ACME Corp",
				Description: "",
				CreatedBy:   "admin",
			},
			setup:   func(m *MockCompanyRepository) {},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			companyRepo := NewMockCompanyRepository()
			divisionRepo := &MockDivisionRepository{}
			deptRepo := &MockDepartmentRepository{}
			sectionRepo := &MockSectionRepository{}

			tt.setup(companyRepo)

			svc := NewService(companyRepo, divisionRepo, deptRepo, sectionRepo)
			company, err := svc.CreateCompany(context.Background(), tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("CreateCompany() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("CreateCompany() unexpected error: %v", err)
				}
				if company == nil {
					t.Errorf("CreateCompany() returned nil company")
				}
			}
		})
	}
}

func TestService_GetCompanyByID(t *testing.T) {
	companyRepo := NewMockCompanyRepository()
	divisionRepo := &MockDivisionRepository{}
	deptRepo := &MockDepartmentRepository{}
	sectionRepo := &MockSectionRepository{}

	// Create a company
	c, _ := organization.NewCompany("TEST", "Test Corp", "", "admin")
	companyRepo.Create(context.Background(), c)

	svc := NewService(companyRepo, divisionRepo, deptRepo, sectionRepo)

	// Test found
	result, err := svc.GetCompanyByID(context.Background(), c.ID())
	if err != nil {
		t.Errorf("GetCompanyByID() unexpected error: %v", err)
	}
	if result.ID() != c.ID() {
		t.Errorf("GetCompanyByID() ID mismatch")
	}

	// Test not found
	_, err = svc.GetCompanyByID(context.Background(), uuid.New())
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("GetCompanyByID() expected ErrNotFound, got: %v", err)
	}
}

func TestService_UpdateCompany(t *testing.T) {
	companyRepo := NewMockCompanyRepository()
	divisionRepo := &MockDivisionRepository{}
	deptRepo := &MockDepartmentRepository{}
	sectionRepo := &MockSectionRepository{}

	// Create a company
	c, _ := organization.NewCompany("TEST", "Test Corp", "Desc", "admin")
	companyRepo.Create(context.Background(), c)

	svc := NewService(companyRepo, divisionRepo, deptRepo, sectionRepo)

	newName := "Updated Corp"
	updated, err := svc.UpdateCompany(context.Background(), UpdateCompanyInput{
		ID:        c.ID(),
		Name:      &newName,
		UpdatedBy: "updater",
	})

	if err != nil {
		t.Errorf("UpdateCompany() unexpected error: %v", err)
	}
	if updated.Name() != newName {
		t.Errorf("UpdateCompany() name = %v, want %v", updated.Name(), newName)
	}
}

func TestService_DeleteCompany_WithChildren(t *testing.T) {
	companyRepo := NewMockCompanyRepository()
	divisionRepo := &MockDivisionRepository{}
	deptRepo := &MockDepartmentRepository{}
	sectionRepo := &MockSectionRepository{}

	// Create company
	c, _ := organization.NewCompany("TEST", "Test Corp", "", "admin")
	companyRepo.Create(context.Background(), c)

	// Create division under company
	d, _ := organization.NewDivision(c.ID(), "DIV", "Division", "", "admin")
	divisionRepo.divisions = append(divisionRepo.divisions, d)

	svc := NewService(companyRepo, divisionRepo, deptRepo, sectionRepo)

	// Should fail because company has divisions
	err := svc.DeleteCompany(context.Background(), c.ID(), "deleter")
	if !errors.Is(err, ErrHasChildren) {
		t.Errorf("DeleteCompany() expected ErrHasChildren, got: %v", err)
	}
}

func TestService_ListCompanies(t *testing.T) {
	companyRepo := NewMockCompanyRepository()
	divisionRepo := &MockDivisionRepository{}
	deptRepo := &MockDepartmentRepository{}
	sectionRepo := &MockSectionRepository{}

	// Create companies
	c1, _ := organization.NewCompany("COMP1", "Company 1", "", "admin")
	c2, _ := organization.NewCompany("COMP2", "Company 2", "", "admin")
	companyRepo.Create(context.Background(), c1)
	companyRepo.Create(context.Background(), c2)

	svc := NewService(companyRepo, divisionRepo, deptRepo, sectionRepo)

	companies, total, err := svc.ListCompanies(context.Background(), organization.ListParams{
		Page:     1,
		PageSize: 10,
	})

	if err != nil {
		t.Errorf("ListCompanies() unexpected error: %v", err)
	}
	if total != 2 {
		t.Errorf("ListCompanies() total = %v, want 2", total)
	}
	if len(companies) != 2 {
		t.Errorf("ListCompanies() count = %v, want 2", len(companies))
	}
}
