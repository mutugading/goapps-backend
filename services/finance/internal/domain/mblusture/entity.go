// Package mblusture provides domain logic for MB (Master Batch) lusture master data.
package mblusture

// Entity is a single lusture master-data row.
type Entity struct {
	id              string
	code            string
	displayName     string
	fullDescription string
	category        string
	isActive        bool
	displayOrder    int32
	createdAt       string
	createdBy       string
	updatedAt       string
	updatedBy       string
	deletedAt       string
	deletedBy       string
}

// NewEntity constructs a new lusture row, validating code and createdBy are present and
// defaulting isActive to true.
func NewEntity(code, displayName, fullDescription, category string, displayOrder int32, createdBy string) (*Entity, error) {
	if code == "" {
		return nil, ErrCodeRequired
	}
	if createdBy == "" {
		return nil, ErrCreatedByRequired
	}
	return &Entity{
		code:            code,
		displayName:     displayName,
		fullDescription: fullDescription,
		category:        category,
		displayOrder:    displayOrder,
		isActive:        true,
		createdBy:       createdBy,
	}, nil
}

// Reconstruct rebuilds an Entity from storage without re-running construction validation.
//
//nolint:revive // Many parameters required for hydration from storage.
func Reconstruct(id, code, displayName, fullDescription, category string, displayOrder int32, isActive bool, createdAt, createdBy, updatedAt, updatedBy, deletedAt, deletedBy string) *Entity {
	return &Entity{
		id:              id,
		code:            code,
		displayName:     displayName,
		fullDescription: fullDescription,
		category:        category,
		displayOrder:    displayOrder,
		isActive:        isActive,
		createdAt:       createdAt,
		createdBy:       createdBy,
		updatedAt:       updatedAt,
		updatedBy:       updatedBy,
		deletedAt:       deletedAt,
		deletedBy:       deletedBy,
	}
}

// ID returns the lusture row's UUID.
func (e *Entity) ID() string { return e.id }

// Code returns the lusture's business code.
func (e *Entity) Code() string { return e.code }

// DisplayName returns the lusture's display name.
func (e *Entity) DisplayName() string { return e.displayName }

// FullDescription returns the lusture's full description.
func (e *Entity) FullDescription() string { return e.fullDescription }

// Category returns the lusture's category.
func (e *Entity) Category() string { return e.category }

// IsActive returns whether the lusture is active.
func (e *Entity) IsActive() bool { return e.isActive }

// DisplayOrder returns the lusture's display order.
func (e *Entity) DisplayOrder() int32 { return e.displayOrder }

// CreatedAt returns the creation timestamp.
func (e *Entity) CreatedAt() string { return e.createdAt }

// CreatedBy returns the creator's identifier.
func (e *Entity) CreatedBy() string { return e.createdBy }

// UpdatedAt returns the last update timestamp.
func (e *Entity) UpdatedAt() string { return e.updatedAt }

// UpdatedBy returns the last updater's identifier.
func (e *Entity) UpdatedBy() string { return e.updatedBy }

// DeletedAt returns the soft-delete timestamp.
func (e *Entity) DeletedAt() string { return e.deletedAt }

// DeletedBy returns the soft-deleter's identifier.
func (e *Entity) DeletedBy() string { return e.deletedBy }

// IsDeleted returns whether the lusture row has been soft-deleted.
func (e *Entity) IsDeleted() bool { return e.deletedAt != "" }

// Deactivate marks the lusture inactive.
func (e *Entity) Deactivate() { e.isActive = false }
