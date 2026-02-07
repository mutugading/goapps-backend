// Package user provides domain logic for User management.
package user

import (
	"time"

	"github.com/google/uuid"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// UserDetail contains extended profile information for a user.
type UserDetail struct {
	id             uuid.UUID
	userID         uuid.UUID
	sectionID      *uuid.UUID
	employeeCode   string
	fullName       string
	firstName      string
	lastName       string
	phone          string
	profilePicture string
	position       string
	dateOfBirth    *time.Time
	address        string
	extraData      map[string]interface{}
	audit          shared.AuditInfo
}

// NewUserDetail creates a new UserDetail entity.
func NewUserDetail(
	userID uuid.UUID,
	sectionID *uuid.UUID,
	employeeCode, fullName, firstName, lastName string,
	createdBy string,
) (*UserDetail, error) {
	if employeeCode == "" {
		return nil, shared.ErrEmptyCode
	}
	if fullName == "" {
		return nil, shared.ErrEmptyName
	}

	return &UserDetail{
		id:           uuid.New(),
		userID:       userID,
		sectionID:    sectionID,
		employeeCode: employeeCode,
		fullName:     fullName,
		firstName:    firstName,
		lastName:     lastName,
		audit:        shared.NewAuditInfo(createdBy),
	}, nil
}

// ReconstructUserDetail reconstructs a UserDetail from persistence.
func ReconstructUserDetail(
	id, userID uuid.UUID,
	sectionID *uuid.UUID,
	employeeCode, fullName, firstName, lastName string,
	phone, profilePicture, position string,
	dateOfBirth *time.Time,
	address string,
	extraData map[string]interface{},
	audit shared.AuditInfo,
) *UserDetail {
	return &UserDetail{
		id:             id,
		userID:         userID,
		sectionID:      sectionID,
		employeeCode:   employeeCode,
		fullName:       fullName,
		firstName:      firstName,
		lastName:       lastName,
		phone:          phone,
		profilePicture: profilePicture,
		position:       position,
		dateOfBirth:    dateOfBirth,
		address:        address,
		extraData:      extraData,
		audit:          audit,
	}
}

// =============================================================================
// Getters
// =============================================================================

func (d *UserDetail) ID() uuid.UUID                     { return d.id }
func (d *UserDetail) UserID() uuid.UUID                 { return d.userID }
func (d *UserDetail) SectionID() *uuid.UUID             { return d.sectionID }
func (d *UserDetail) EmployeeCode() string              { return d.employeeCode }
func (d *UserDetail) FullName() string                  { return d.fullName }
func (d *UserDetail) FirstName() string                 { return d.firstName }
func (d *UserDetail) LastName() string                  { return d.lastName }
func (d *UserDetail) Phone() string                     { return d.phone }
func (d *UserDetail) ProfilePicture() string            { return d.profilePicture }
func (d *UserDetail) Position() string                  { return d.position }
func (d *UserDetail) DateOfBirth() *time.Time           { return d.dateOfBirth }
func (d *UserDetail) Address() string                   { return d.address }
func (d *UserDetail) ExtraData() map[string]interface{} { return d.extraData }
func (d *UserDetail) Audit() shared.AuditInfo           { return d.audit }

// =============================================================================
// Domain Behavior Methods
// =============================================================================

// Update updates user detail fields.
func (d *UserDetail) Update(
	sectionID *uuid.UUID,
	fullName, firstName, lastName *string,
	phone, profilePicture, position *string,
	dateOfBirth *time.Time,
	address *string,
	extraData map[string]interface{},
	updatedBy string,
) error {
	if sectionID != nil {
		d.sectionID = sectionID
	}
	if fullName != nil {
		if *fullName == "" {
			return shared.ErrEmptyName
		}
		d.fullName = *fullName
	}
	if firstName != nil {
		d.firstName = *firstName
	}
	if lastName != nil {
		d.lastName = *lastName
	}
	if phone != nil {
		d.phone = *phone
	}
	if profilePicture != nil {
		d.profilePicture = *profilePicture
	}
	if position != nil {
		d.position = *position
	}
	if dateOfBirth != nil {
		d.dateOfBirth = dateOfBirth
	}
	if address != nil {
		d.address = *address
	}
	if extraData != nil {
		d.extraData = extraData
	}

	d.audit.Update(updatedBy)
	return nil
}
