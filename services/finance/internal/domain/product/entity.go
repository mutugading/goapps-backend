// Package product contains the Product aggregate and its supporting types.
package product

import (
	"time"

	"github.com/google/uuid"
)

// Product is the aggregate root for a costing product.
// It is per-period stateless at this layer — period-specific values live
// in the cst_product_detail / cst_product_cost children handled elsewhere.
//
// All fields are private; use getters to read state.
// Pointer getters (UpdatedAt, DeletedAt, CopiedWithOptions, etc.) return the
// stored pointer directly — callers must not mutate the pointed-to value.
//
//nolint:revive // Wide struct mirrors the persistence row one-for-one.
type Product struct {
	id                    uuid.UUID
	code                  Code
	name                  Name
	itemCode              ItemCode
	shadeCode             ShadeCode
	shadeName             ShadeName
	productStatus         Status
	workflowStatus        WorkflowStatus
	createdByDeptID       uuid.UUID
	createdByDeptCode     string
	purpose               Purpose
	duplicatedFromID      uuid.UUID    // zero UUID = not duplicated
	duplicationNote       string
	copiedWithOptions     *CopyOptions // nil = not duplicated
	templateID            uuid.UUID    // zero UUID = no template
	templateVersionPinned int
	currentRequestID      uuid.UUID // zero UUID = none
	lockedAt              *time.Time
	lockedBy              string
	lockedPeriod          string
	unlockCount           int
	createdAt             time.Time
	createdBy             string
	updatedAt             *time.Time
	updatedBy             string
	deletedAt             *time.Time
	deletedBy             string
}

// NewProduct constructs a fresh Product with workflowStatus DRAFT and productStatus DRAFT.
// Use ReconstructProduct when loading from persistence (skips validation).
func NewProduct(
	code, name, itemCode string,
	shadeCode, shadeName string,
	deptID uuid.UUID,
	deptCode string,
	purpose string,
	currentRequestID uuid.UUID,
	createdBy string,
) (*Product, error) {
	codeVO, err := NewCode(code)
	if err != nil {
		return nil, err
	}

	nameVO, err := NewName(name)
	if err != nil {
		return nil, err
	}

	itemCodeVO, err := NewItemCode(itemCode)
	if err != nil {
		return nil, err
	}

	shadeCodeVO, err := NewShadeCode(shadeCode)
	if err != nil {
		return nil, err
	}

	shadeNameVO, err := NewShadeName(shadeName)
	if err != nil {
		return nil, err
	}

	purposeVO, err := NewPurpose(purpose)
	if err != nil {
		return nil, err
	}

	return &Product{
		id:               uuid.New(),
		code:             codeVO,
		name:             nameVO,
		itemCode:         itemCodeVO,
		shadeCode:        shadeCodeVO,
		shadeName:        shadeNameVO,
		productStatus:    StatusDraft,
		workflowStatus:   WorkflowDraft,
		createdByDeptID:  deptID,
		createdByDeptCode: deptCode,
		purpose:          purposeVO,
		currentRequestID: currentRequestID,
		unlockCount:      0,
		createdAt:        time.Now().UTC(),
		createdBy:        createdBy,
	}, nil
}

// ReconstructProduct rebuilds an entity from raw persisted values.
// It does NOT re-run validation — the persistence layer is the source of truth.
//
//nolint:revive // Persistence reconstitution takes many fields by design.
func ReconstructProduct(
	id uuid.UUID,
	code, name, itemCode, shadeCode, shadeName string,
	productStatus, workflowStatus string,
	deptID uuid.UUID,
	deptCode string,
	purpose string,
	duplicatedFromID uuid.UUID,
	duplicationNote string,
	copiedWithOptions *CopyOptions,
	templateID uuid.UUID,
	templateVersionPinned int,
	currentRequestID uuid.UUID,
	lockedAt *time.Time,
	lockedBy, lockedPeriod string,
	unlockCount int,
	createdAt time.Time,
	createdBy string,
	updatedAt *time.Time,
	updatedBy string,
	deletedAt *time.Time,
	deletedBy string,
) *Product {
	return &Product{
		id:                    id,
		code:                  Code{value: code},
		name:                  Name{value: name},
		itemCode:              ItemCode{value: itemCode},
		shadeCode:             ShadeCode{value: shadeCode},
		shadeName:             ShadeName{value: shadeName},
		productStatus:         Status(productStatus),
		workflowStatus:        WorkflowStatus(workflowStatus),
		createdByDeptID:       deptID,
		createdByDeptCode:     deptCode,
		purpose:               Purpose(purpose),
		duplicatedFromID:      duplicatedFromID,
		duplicationNote:       duplicationNote,
		copiedWithOptions:     copiedWithOptions,
		templateID:            templateID,
		templateVersionPinned: templateVersionPinned,
		currentRequestID:      currentRequestID,
		lockedAt:              lockedAt,
		lockedBy:              lockedBy,
		lockedPeriod:          lockedPeriod,
		unlockCount:           unlockCount,
		createdAt:             createdAt,
		createdBy:             createdBy,
		updatedAt:             updatedAt,
		updatedBy:             updatedBy,
		deletedAt:             deletedAt,
		deletedBy:             deletedBy,
	}
}

// Update mutates editable fields on the product.
// Only allowed when WorkflowStatus is DRAFT (Phase 1 rule); returns ErrLocked otherwise.
// Updates updatedAt and updatedBy on success.
func (p *Product) Update(
	name string,
	shadeCode, shadeName string,
	purpose string,
	updatedBy string,
) error {
	if !p.workflowStatus.IsEditable() {
		return ErrLocked
	}

	if err := p.applyName(name); err != nil {
		return err
	}

	if err := p.applyShadeCode(shadeCode); err != nil {
		return err
	}

	if err := p.applyShadeName(shadeName); err != nil {
		return err
	}

	if err := p.applyPurpose(purpose); err != nil {
		return err
	}

	now := time.Now().UTC()
	p.updatedAt = &now
	p.updatedBy = updatedBy

	return nil
}

// applyName validates and assigns the name field.
func (p *Product) applyName(name string) error {
	nameVO, err := NewName(name)
	if err != nil {
		return err
	}
	p.name = nameVO
	return nil
}

// applyShadeCode validates and assigns the shade code field.
func (p *Product) applyShadeCode(shadeCode string) error {
	scVO, err := NewShadeCode(shadeCode)
	if err != nil {
		return err
	}
	p.shadeCode = scVO
	return nil
}

// applyShadeName validates and assigns the shade name field.
func (p *Product) applyShadeName(shadeName string) error {
	snVO, err := NewShadeName(shadeName)
	if err != nil {
		return err
	}
	p.shadeName = snVO
	return nil
}

// applyPurpose validates and assigns the purpose field.
func (p *Product) applyPurpose(purpose string) error {
	purposeVO, err := NewPurpose(purpose)
	if err != nil {
		return err
	}
	p.purpose = purposeVO
	return nil
}

// Duplicate produces a fresh Product based on this one.
// The returned product has DuplicatedFromID set to the source's ID, a fresh UUID,
// fresh audit fields, and the supplied CopyOptions stored.
// NOTE: the actual cloning of routing/params/RM rows happens in later phases —
// this method only handles the master fields.
func (p *Product) Duplicate(
	newCode, newName string,
	duplicationNote string,
	options CopyOptions,
	currentRequestID uuid.UUID,
	createdBy string,
) (*Product, error) {
	if p.deletedAt != nil {
		return nil, ErrSourceDeleted
	}

	if newCode == p.code.value {
		return nil, ErrSelfDuplication
	}

	if len(duplicationNote) > 500 {
		return nil, ErrInvalidDuplicationNote
	}

	newCodeVO, err := NewCode(newCode)
	if err != nil {
		return nil, err
	}

	newNameVO, err := NewName(newName)
	if err != nil {
		return nil, err
	}

	opts := options
	return &Product{
		id:                uuid.New(),
		code:              newCodeVO,
		name:              newNameVO,
		itemCode:          p.itemCode,
		shadeCode:         p.shadeCode,
		shadeName:         p.shadeName,
		productStatus:     StatusDraft,
		workflowStatus:    WorkflowDraft,
		createdByDeptID:   p.createdByDeptID,
		createdByDeptCode: p.createdByDeptCode,
		purpose:           p.purpose,
		duplicatedFromID:  p.id,
		duplicationNote:   duplicationNote,
		copiedWithOptions: &opts,
		currentRequestID:  currentRequestID,
		unlockCount:       0,
		createdAt:         time.Now().UTC(),
		createdBy:         createdBy,
	}, nil
}

// SoftDelete marks the product deleted. Returns ErrNotFound if already deleted.
func (p *Product) SoftDelete(deletedBy string) error {
	if p.deletedAt != nil {
		return ErrNotFound
	}

	now := time.Now().UTC()
	p.deletedAt = &now
	p.deletedBy = deletedBy

	return nil
}

// =============================================================================
// Read-only getters
// =============================================================================

// ID returns the product's unique identifier.
func (p *Product) ID() uuid.UUID { return p.id }

// Code returns the product code value object.
func (p *Product) Code() Code { return p.code }

// Name returns the product name value object.
func (p *Product) Name() Name { return p.name }

// ItemCode returns the product item code value object.
func (p *Product) ItemCode() ItemCode { return p.itemCode }

// ShadeCode returns the product shade code value object.
func (p *Product) ShadeCode() ShadeCode { return p.shadeCode }

// ShadeName returns the product shade name value object.
func (p *Product) ShadeName() ShadeName { return p.shadeName }

// ProductStatus returns the product lifecycle status.
func (p *Product) ProductStatus() Status { return p.productStatus }

// WorkflowStatus returns the approval-workflow state.
func (p *Product) WorkflowStatus() WorkflowStatus { return p.workflowStatus }

// CreatedByDeptID returns the UUID of the department that created the product.
func (p *Product) CreatedByDeptID() uuid.UUID { return p.createdByDeptID }

// CreatedByDeptCode returns the department code of the creator.
func (p *Product) CreatedByDeptCode() string { return p.createdByDeptCode }

// Purpose returns the product's intended-use classification.
func (p *Product) Purpose() Purpose { return p.purpose }

// DuplicatedFromID returns the source product UUID when this product was duplicated.
// A zero UUID (uuid.Nil) means the product was not duplicated.
func (p *Product) DuplicatedFromID() uuid.UUID { return p.duplicatedFromID }

// DuplicationNote returns the note supplied during duplication, if any.
func (p *Product) DuplicationNote() string { return p.duplicationNote }

// CopiedWithOptions returns the duplication options used when this product was created
// by duplication. Returns nil if the product was not duplicated.
// Callers must not mutate the pointed-to value.
func (p *Product) CopiedWithOptions() *CopyOptions { return p.copiedWithOptions }

// TemplateID returns the template UUID this product was derived from.
// A zero UUID (uuid.Nil) means no template was used.
func (p *Product) TemplateID() uuid.UUID { return p.templateID }

// TemplateVersionPinned returns the specific template version number pinned to this product.
func (p *Product) TemplateVersionPinned() int { return p.templateVersionPinned }

// CurrentRequestID returns the active request UUID linked to this product.
// A zero UUID (uuid.Nil) means no active request is linked.
func (p *Product) CurrentRequestID() uuid.UUID { return p.currentRequestID }

// LockedAt returns the timestamp when the product was locked, or nil if not locked.
// Callers must not mutate the pointed-to value.
func (p *Product) LockedAt() *time.Time { return p.lockedAt }

// LockedBy returns the identity that locked the product.
func (p *Product) LockedBy() string { return p.lockedBy }

// LockedPeriod returns the period (YYYYMM) for which the product was locked.
func (p *Product) LockedPeriod() string { return p.lockedPeriod }

// UnlockCount returns the number of times the product has been unlocked.
func (p *Product) UnlockCount() int { return p.unlockCount }

// CreatedAt returns the creation timestamp.
func (p *Product) CreatedAt() time.Time { return p.createdAt }

// CreatedBy returns the identity that created the product.
func (p *Product) CreatedBy() string { return p.createdBy }

// UpdatedAt returns the last-update timestamp, or nil if never updated.
// Callers must not mutate the pointed-to value.
func (p *Product) UpdatedAt() *time.Time { return p.updatedAt }

// UpdatedBy returns the identity that last updated the product.
func (p *Product) UpdatedBy() string { return p.updatedBy }

// DeletedAt returns the soft-delete timestamp, or nil if not deleted.
// Callers must not mutate the pointed-to value.
func (p *Product) DeletedAt() *time.Time { return p.deletedAt }

// DeletedBy returns the identity that soft-deleted the product.
func (p *Product) DeletedBy() string { return p.deletedBy }

// IsDeleted reports whether the product has been soft-deleted.
func (p *Product) IsDeleted() bool { return p.deletedAt != nil }
