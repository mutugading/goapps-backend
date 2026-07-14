// Package mbcomposition provides domain logic for MB (Master Batch) composition rows.
package mbcomposition

// Source type constants for a composition row's origin.
const (
	SourceTypeGroup   = "GROUP"
	SourceTypeMB      = "MB"
	SourceTypeCarrier = "CARRIER"
)

// Entity is a single MB composition row (one ingredient line within an MB head).
type Entity struct {
	id             string
	mbhID          string
	seqNo          int32
	groupHeadID    string
	compositionPct string
	sourceType     string
	mbRefMbhID     string
	isCarrier      bool
	legacySysID    string
	createdAt      string
	createdBy      string
	updatedAt      string
	updatedBy      string
	deletedAt      string
	deletedBy      string
}

// NewEntity constructs a new composition row, validating mbh_id, source_type, and the
// group_head_id/source_type=GROUP invariant.
func NewEntity(mbhID, groupHeadID, compositionPct, sourceType string, seqNo int32, mbRefMbhID string, isCarrier bool, createdBy string) (*Entity, error) {
	if mbhID == "" {
		return nil, ErrMbhIDRequired
	}
	if sourceType != SourceTypeGroup && sourceType != SourceTypeMB && sourceType != SourceTypeCarrier {
		return nil, ErrInvalidSourceType
	}
	if sourceType == SourceTypeGroup && groupHeadID == "" {
		return nil, ErrGroupHeadIDRequired
	}
	if createdBy == "" {
		return nil, ErrCreatedByRequired
	}
	return &Entity{
		mbhID:          mbhID,
		seqNo:          seqNo,
		groupHeadID:    groupHeadID,
		compositionPct: compositionPct,
		sourceType:     sourceType,
		mbRefMbhID:     mbRefMbhID,
		isCarrier:      isCarrier,
		createdBy:      createdBy,
	}, nil
}

// Reconstruct rebuilds a composition row from persistence data.
//
//nolint:revive // Many parameters required for persistence reconstitution.
func Reconstruct(id, mbhID string, seqNo int32, groupHeadID, compositionPct, sourceType, mbRefMbhID string, isCarrier bool, legacySysID, createdAt, createdBy, updatedAt, updatedBy, deletedAt, deletedBy string) *Entity {
	return &Entity{
		id: id, mbhID: mbhID, seqNo: seqNo, groupHeadID: groupHeadID,
		compositionPct: compositionPct, sourceType: sourceType, mbRefMbhID: mbRefMbhID,
		isCarrier: isCarrier, legacySysID: legacySysID,
		createdAt: createdAt, createdBy: createdBy, updatedAt: updatedAt, updatedBy: updatedBy,
		deletedAt: deletedAt, deletedBy: deletedBy,
	}
}

// ID returns the composition row's UUID.
func (e *Entity) ID() string { return e.id }

// MbhID returns the parent MB head's UUID.
func (e *Entity) MbhID() string { return e.mbhID }

// SeqNo returns the row's ordering sequence number within its parent MB.
func (e *Entity) SeqNo() int32 { return e.seqNo }

// SourceType returns GROUP, MB, or CARRIER.
func (e *Entity) SourceType() string { return e.sourceType }

// GroupHeadID returns the referenced RM-group head's ID (required when SourceType is GROUP).
func (e *Entity) GroupHeadID() string { return e.groupHeadID }

// CompositionPct returns the composition percentage as a decimal string (NUMERIC(6,3), scanned
// as string to avoid float precision loss).
func (e *Entity) CompositionPct() string { return e.compositionPct }

// MbRefMbhID returns the nested MB reference's head ID, empty when SourceType is not MB.
func (e *Entity) MbRefMbhID() string { return e.mbRefMbhID }

// IsCarrier returns whether this row is a carrier-only line.
func (e *Entity) IsCarrier() bool { return e.isCarrier }

// LegacySysID returns the optional Oracle legacy system ID, empty if not imported.
func (e *Entity) LegacySysID() string { return e.legacySysID }

// CreatedAt returns the creation timestamp (RFC3339 string).
func (e *Entity) CreatedAt() string { return e.createdAt }

// CreatedBy returns the creator.
func (e *Entity) CreatedBy() string { return e.createdBy }

// UpdatedAt returns the last update timestamp (RFC3339 string), empty if never updated.
func (e *Entity) UpdatedAt() string { return e.updatedAt }

// UpdatedBy returns the last updater, empty if never updated.
func (e *Entity) UpdatedBy() string { return e.updatedBy }

// DeletedAt returns the soft-delete timestamp (RFC3339 string), empty if not deleted.
func (e *Entity) DeletedAt() string { return e.deletedAt }

// DeletedBy returns who soft-deleted the record, empty if not deleted.
func (e *Entity) DeletedBy() string { return e.deletedBy }

// IsDeleted returns true if the row has been soft-deleted.
func (e *Entity) IsDeleted() bool { return e.deletedAt != "" }
