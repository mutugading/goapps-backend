// Package organization provides domain logic for organization hierarchy management.
package organization

import (
	"testing"
)

func TestNewCompany(t *testing.T) {
	tests := []struct {
		name        string
		code        string
		companyName string
		description string
		createdBy   string
		wantErr     bool
		errMsg      string
	}{
		{
			name:        "valid company",
			code:        "ACME",
			companyName: "ACME Corporation",
			description: "Main Company",
			createdBy:   "admin",
			wantErr:     false,
		},
		{
			name:        "empty code",
			code:        "",
			companyName: "Test Corp",
			description: "",
			createdBy:   "admin",
			wantErr:     true,
			errMsg:      "code",
		},
		{
			name:        "empty name",
			code:        "TEST",
			companyName: "",
			description: "",
			createdBy:   "admin",
			wantErr:     true,
			errMsg:      "name",
		},
		{
			name:        "invalid code format lowercase",
			code:        "acme",
			companyName: "Test Corp",
			description: "",
			createdBy:   "admin",
			wantErr:     true,
			errMsg:      "format",
		},
		{
			name:        "code with underscore",
			code:        "ACME_CORP",
			companyName: "ACME Corp",
			description: "",
			createdBy:   "admin",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			company, err := NewCompany(tt.code, tt.companyName, tt.description, tt.createdBy)
			if tt.wantErr {
				if err == nil {
					t.Errorf("NewCompany() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("NewCompany() unexpected error: %v", err)
				}
				if company == nil {
					t.Errorf("NewCompany() returned nil company")
					return
				}
				if company.Code() != tt.code {
					t.Errorf("Company.Code() = %v, want %v", company.Code(), tt.code)
				}
				if company.Name() != tt.companyName {
					t.Errorf("Company.Name() = %v, want %v", company.Name(), tt.companyName)
				}
				if company.Description() != tt.description {
					t.Errorf("Company.Description() = %v, want %v", company.Description(), tt.description)
				}
				if !company.IsActive() {
					t.Errorf("Company.IsActive() = false, want true")
				}
				if company.Audit().CreatedBy != tt.createdBy {
					t.Errorf("Company.Audit().CreatedBy = %v, want %v", company.Audit().CreatedBy, tt.createdBy)
				}
			}
		})
	}
}

func TestCompany_Update(t *testing.T) {
	company, err := NewCompany("TEST", "Test Corp", "Description", "admin")
	if err != nil {
		t.Fatalf("Failed to create company: %v", err)
	}

	newName := "Updated Corp"
	newDesc := "Updated Description"
	isActive := false

	err = company.Update(&newName, &newDesc, &isActive, "updater")
	if err != nil {
		t.Errorf("Company.Update() unexpected error: %v", err)
	}

	if company.Name() != newName {
		t.Errorf("Company.Name() = %v, want %v", company.Name(), newName)
	}
	if company.Description() != newDesc {
		t.Errorf("Company.Description() = %v, want %v", company.Description(), newDesc)
	}
	if company.IsActive() != isActive {
		t.Errorf("Company.IsActive() = %v, want %v", company.IsActive(), isActive)
	}
	if company.Audit().UpdatedBy == nil || *company.Audit().UpdatedBy != "updater" {
		t.Errorf("Company.Audit().UpdatedBy should be 'updater'")
	}
}

func TestCompany_SoftDelete(t *testing.T) {
	company, err := NewCompany("TEST", "Test Corp", "Description", "admin")
	if err != nil {
		t.Fatalf("Failed to create company: %v", err)
	}

	err = company.SoftDelete("deleter")
	if err != nil {
		t.Errorf("Company.SoftDelete() unexpected error: %v", err)
	}

	if !company.IsDeleted() {
		t.Errorf("Company.IsDeleted() = false, want true")
	}
	if company.IsActive() {
		t.Errorf("Company.IsActive() = true, want false")
	}

	// Double delete should fail
	err = company.SoftDelete("deleter")
	if err == nil {
		t.Errorf("Company.SoftDelete() on deleted should return error")
	}
}

func TestNewDivision(t *testing.T) {
	company, _ := NewCompany("COMP", "Company", "", "admin")

	tests := []struct {
		name         string
		code         string
		divisionName string
		description  string
		wantErr      bool
	}{
		{
			name:         "valid division",
			code:         "DIV01",
			divisionName: "Division One",
			description:  "First Division",
			wantErr:      false,
		},
		{
			name:         "empty code",
			code:         "",
			divisionName: "Division",
			description:  "",
			wantErr:      true,
		},
		{
			name:         "empty name",
			code:         "DIV02",
			divisionName: "",
			description:  "",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			division, err := NewDivision(company.ID(), tt.code, tt.divisionName, tt.description, "admin")
			if tt.wantErr {
				if err == nil {
					t.Errorf("NewDivision() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("NewDivision() unexpected error: %v", err)
				}
				if division == nil {
					t.Errorf("NewDivision() returned nil")
					return
				}
				if division.CompanyID() != company.ID() {
					t.Errorf("Division.CompanyID() mismatch")
				}
				if division.Code() != tt.code {
					t.Errorf("Division.Code() = %v, want %v", division.Code(), tt.code)
				}
			}
		})
	}
}

func TestNewDepartment(t *testing.T) {
	company, _ := NewCompany("COMP", "Company", "", "admin")
	division, _ := NewDivision(company.ID(), "DIV", "Division", "", "admin")

	dept, err := NewDepartment(division.ID(), "DEPT", "Department", "Desc", "admin")
	if err != nil {
		t.Fatalf("NewDepartment() unexpected error: %v", err)
	}

	if dept.DivisionID() != division.ID() {
		t.Errorf("Department.DivisionID() mismatch")
	}
	if dept.Code() != "DEPT" {
		t.Errorf("Department.Code() = %v, want DEPT", dept.Code())
	}
}

func TestNewSection(t *testing.T) {
	company, _ := NewCompany("COMP", "Company", "", "admin")
	division, _ := NewDivision(company.ID(), "DIV", "Division", "", "admin")
	dept, _ := NewDepartment(division.ID(), "DEPT", "Department", "", "admin")

	section, err := NewSection(dept.ID(), "SEC", "Section", "Desc", "admin")
	if err != nil {
		t.Fatalf("NewSection() unexpected error: %v", err)
	}

	if section.DepartmentID() != dept.ID() {
		t.Errorf("Section.DepartmentID() mismatch")
	}
	if section.Code() != "SEC" {
		t.Errorf("Section.Code() = %v, want SEC", section.Code())
	}
}
