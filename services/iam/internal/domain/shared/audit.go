// Package shared provides common domain types used across all IAM domain packages.
package shared

import (
	"time"
)

// AuditInfo contains common audit fields for all entities.
type AuditInfo struct {
	CreatedAt time.Time
	CreatedBy string
	UpdatedAt *time.Time
	UpdatedBy *string
	DeletedAt *time.Time
	DeletedBy *string
}

// NewAuditInfo creates a new AuditInfo with creation data.
func NewAuditInfo(createdBy string) AuditInfo {
	return AuditInfo{
		CreatedAt: time.Now(),
		CreatedBy: createdBy,
	}
}

// Update marks the entity as updated.
func (a *AuditInfo) Update(updatedBy string) {
	now := time.Now()
	a.UpdatedAt = &now
	a.UpdatedBy = &updatedBy
}

// SoftDelete marks the entity as deleted.
func (a *AuditInfo) SoftDelete(deletedBy string) {
	now := time.Now()
	a.DeletedAt = &now
	a.DeletedBy = &deletedBy
}

// IsDeleted returns true if the entity has been soft deleted.
func (a *AuditInfo) IsDeleted() bool {
	return a.DeletedAt != nil
}
