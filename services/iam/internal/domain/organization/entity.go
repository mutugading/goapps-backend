// Package organization provides domain logic for organization hierarchy management.
package organization

import (
	"errors"
	"regexp"

	"github.com/google/uuid"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// Domain-specific errors for organization package.
var (
	ErrInvalidCodeFormat = errors.New("code must start with a letter and contain only uppercase letters, numbers, and underscores")
)

// Code validation regex.
var codeRegex = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// =============================================================================
// COMPANY
// =============================================================================

// Company represents a company/organization entity.
type Company struct {
	id          uuid.UUID
	code        string
	name        string
	description string
	isActive    bool
	audit       shared.AuditInfo
}

// NewCompany creates a new Company entity.
func NewCompany(code, name, description, createdBy string) (*Company, error) {
	if code == "" {
		return nil, shared.ErrEmptyCode
	}
	if !codeRegex.MatchString(code) {
		return nil, ErrInvalidCodeFormat
	}
	if name == "" {
		return nil, shared.ErrEmptyName
	}
	if len(code) > 20 {
		return nil, shared.ErrCodeTooLong
	}
	if len(name) > 100 {
		return nil, shared.ErrNameTooLong
	}

	return &Company{
		id:          uuid.New(),
		code:        code,
		name:        name,
		description: description,
		isActive:    true,
		audit:       shared.NewAuditInfo(createdBy),
	}, nil
}

// ReconstructCompany reconstructs a Company from persistence.
func ReconstructCompany(id uuid.UUID, code, name, description string, isActive bool, audit shared.AuditInfo) *Company {
	return &Company{
		id:          id,
		code:        code,
		name:        name,
		description: description,
		isActive:    isActive,
		audit:       audit,
	}
}

func (c *Company) ID() uuid.UUID           { return c.id }
func (c *Company) Code() string            { return c.code }
func (c *Company) Name() string            { return c.name }
func (c *Company) Description() string     { return c.description }
func (c *Company) IsActive() bool          { return c.isActive }
func (c *Company) Audit() shared.AuditInfo { return c.audit }
func (c *Company) IsDeleted() bool         { return c.audit.IsDeleted() }

func (c *Company) Update(name, description *string, isActive *bool, updatedBy string) error {
	if c.IsDeleted() {
		return shared.ErrAlreadyDeleted
	}
	if name != nil {
		if *name == "" {
			return shared.ErrEmptyName
		}
		c.name = *name
	}
	if description != nil {
		c.description = *description
	}
	if isActive != nil {
		c.isActive = *isActive
	}
	c.audit.Update(updatedBy)
	return nil
}

func (c *Company) SoftDelete(deletedBy string) error {
	if c.IsDeleted() {
		return shared.ErrAlreadyDeleted
	}
	c.isActive = false
	c.audit.SoftDelete(deletedBy)
	return nil
}

// =============================================================================
// DIVISION
// =============================================================================

// Division represents a division entity under a company.
type Division struct {
	id          uuid.UUID
	companyID   uuid.UUID
	code        string
	name        string
	description string
	isActive    bool
	audit       shared.AuditInfo
}

// NewDivision creates a new Division entity.
func NewDivision(companyID uuid.UUID, code, name, description, createdBy string) (*Division, error) {
	if code == "" {
		return nil, shared.ErrEmptyCode
	}
	if !codeRegex.MatchString(code) {
		return nil, ErrInvalidCodeFormat
	}
	if name == "" {
		return nil, shared.ErrEmptyName
	}

	return &Division{
		id:          uuid.New(),
		companyID:   companyID,
		code:        code,
		name:        name,
		description: description,
		isActive:    true,
		audit:       shared.NewAuditInfo(createdBy),
	}, nil
}

// ReconstructDivision reconstructs a Division from persistence.
func ReconstructDivision(id, companyID uuid.UUID, code, name, description string, isActive bool, audit shared.AuditInfo) *Division {
	return &Division{
		id:          id,
		companyID:   companyID,
		code:        code,
		name:        name,
		description: description,
		isActive:    isActive,
		audit:       audit,
	}
}

func (d *Division) ID() uuid.UUID           { return d.id }
func (d *Division) CompanyID() uuid.UUID    { return d.companyID }
func (d *Division) Code() string            { return d.code }
func (d *Division) Name() string            { return d.name }
func (d *Division) Description() string     { return d.description }
func (d *Division) IsActive() bool          { return d.isActive }
func (d *Division) Audit() shared.AuditInfo { return d.audit }
func (d *Division) IsDeleted() bool         { return d.audit.IsDeleted() }

func (d *Division) Update(name, description *string, isActive *bool, updatedBy string) error {
	if d.IsDeleted() {
		return shared.ErrAlreadyDeleted
	}
	if name != nil {
		if *name == "" {
			return shared.ErrEmptyName
		}
		d.name = *name
	}
	if description != nil {
		d.description = *description
	}
	if isActive != nil {
		d.isActive = *isActive
	}
	d.audit.Update(updatedBy)
	return nil
}

func (d *Division) SoftDelete(deletedBy string) error {
	if d.IsDeleted() {
		return shared.ErrAlreadyDeleted
	}
	d.isActive = false
	d.audit.SoftDelete(deletedBy)
	return nil
}

// =============================================================================
// DEPARTMENT
// =============================================================================

// Department represents a department entity under a division.
type Department struct {
	id          uuid.UUID
	divisionID  uuid.UUID
	code        string
	name        string
	description string
	isActive    bool
	audit       shared.AuditInfo
}

// NewDepartment creates a new Department entity.
func NewDepartment(divisionID uuid.UUID, code, name, description, createdBy string) (*Department, error) {
	if code == "" {
		return nil, shared.ErrEmptyCode
	}
	if !codeRegex.MatchString(code) {
		return nil, ErrInvalidCodeFormat
	}
	if name == "" {
		return nil, shared.ErrEmptyName
	}

	return &Department{
		id:          uuid.New(),
		divisionID:  divisionID,
		code:        code,
		name:        name,
		description: description,
		isActive:    true,
		audit:       shared.NewAuditInfo(createdBy),
	}, nil
}

// ReconstructDepartment reconstructs a Department from persistence.
func ReconstructDepartment(id, divisionID uuid.UUID, code, name, description string, isActive bool, audit shared.AuditInfo) *Department {
	return &Department{
		id:          id,
		divisionID:  divisionID,
		code:        code,
		name:        name,
		description: description,
		isActive:    isActive,
		audit:       audit,
	}
}

func (d *Department) ID() uuid.UUID           { return d.id }
func (d *Department) DivisionID() uuid.UUID   { return d.divisionID }
func (d *Department) Code() string            { return d.code }
func (d *Department) Name() string            { return d.name }
func (d *Department) Description() string     { return d.description }
func (d *Department) IsActive() bool          { return d.isActive }
func (d *Department) Audit() shared.AuditInfo { return d.audit }
func (d *Department) IsDeleted() bool         { return d.audit.IsDeleted() }

func (d *Department) Update(name, description *string, isActive *bool, updatedBy string) error {
	if d.IsDeleted() {
		return shared.ErrAlreadyDeleted
	}
	if name != nil {
		if *name == "" {
			return shared.ErrEmptyName
		}
		d.name = *name
	}
	if description != nil {
		d.description = *description
	}
	if isActive != nil {
		d.isActive = *isActive
	}
	d.audit.Update(updatedBy)
	return nil
}

func (d *Department) SoftDelete(deletedBy string) error {
	if d.IsDeleted() {
		return shared.ErrAlreadyDeleted
	}
	d.isActive = false
	d.audit.SoftDelete(deletedBy)
	return nil
}

// =============================================================================
// SECTION
// =============================================================================

// Section represents a section entity under a department.
type Section struct {
	id           uuid.UUID
	departmentID uuid.UUID
	code         string
	name         string
	description  string
	isActive     bool
	audit        shared.AuditInfo
}

// NewSection creates a new Section entity.
func NewSection(departmentID uuid.UUID, code, name, description, createdBy string) (*Section, error) {
	if code == "" {
		return nil, shared.ErrEmptyCode
	}
	if !codeRegex.MatchString(code) {
		return nil, ErrInvalidCodeFormat
	}
	if name == "" {
		return nil, shared.ErrEmptyName
	}

	return &Section{
		id:           uuid.New(),
		departmentID: departmentID,
		code:         code,
		name:         name,
		description:  description,
		isActive:     true,
		audit:        shared.NewAuditInfo(createdBy),
	}, nil
}

// ReconstructSection reconstructs a Section from persistence.
func ReconstructSection(id, departmentID uuid.UUID, code, name, description string, isActive bool, audit shared.AuditInfo) *Section {
	return &Section{
		id:           id,
		departmentID: departmentID,
		code:         code,
		name:         name,
		description:  description,
		isActive:     isActive,
		audit:        audit,
	}
}

func (s *Section) ID() uuid.UUID           { return s.id }
func (s *Section) DepartmentID() uuid.UUID { return s.departmentID }
func (s *Section) Code() string            { return s.code }
func (s *Section) Name() string            { return s.name }
func (s *Section) Description() string     { return s.description }
func (s *Section) IsActive() bool          { return s.isActive }
func (s *Section) Audit() shared.AuditInfo { return s.audit }
func (s *Section) IsDeleted() bool         { return s.audit.IsDeleted() }

func (s *Section) Update(name, description *string, isActive *bool, updatedBy string) error {
	if s.IsDeleted() {
		return shared.ErrAlreadyDeleted
	}
	if name != nil {
		if *name == "" {
			return shared.ErrEmptyName
		}
		s.name = *name
	}
	if description != nil {
		s.description = *description
	}
	if isActive != nil {
		s.isActive = *isActive
	}
	s.audit.Update(updatedBy)
	return nil
}

func (s *Section) SoftDelete(deletedBy string) error {
	if s.IsDeleted() {
		return shared.ErrAlreadyDeleted
	}
	s.isActive = false
	s.audit.SoftDelete(deletedBy)
	return nil
}
