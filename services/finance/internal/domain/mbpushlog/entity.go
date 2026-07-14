// Package mbpushlog provides domain logic for MB (Master Batch) cost-push audit logging.
package mbpushlog

// Entity is a single MB cost-push audit log row.
type Entity struct {
	id             string
	period         string
	pushedAt       string
	pushedBy       string
	mbCount        int32
	rowCount       int32
	costTypes      string
	previousPeriod string
	notes          string
}

// NewEntity constructs a new push-log row, validating period is present.
func NewEntity(period, pushedBy string, mbCount, rowCount int32, costTypes string) (*Entity, error) {
	if period == "" {
		return nil, ErrPeriodRequired
	}
	return &Entity{period: period, pushedBy: pushedBy, mbCount: mbCount, rowCount: rowCount, costTypes: costTypes}, nil
}

// Reconstruct rebuilds a push-log Entity from persisted values, bypassing NewEntity's
// validation since the row already exists in storage.
//
//nolint:revive // positional params mirror the hydration DTO's column order
func Reconstruct(id, period, pushedAt, pushedBy string, mbCount, rowCount int32, costTypes, previousPeriod, notes string) *Entity {
	return &Entity{
		id:             id,
		period:         period,
		pushedAt:       pushedAt,
		pushedBy:       pushedBy,
		mbCount:        mbCount,
		rowCount:       rowCount,
		costTypes:      costTypes,
		previousPeriod: previousPeriod,
		notes:          notes,
	}
}

// ID returns the push log row's UUID.
func (e *Entity) ID() string { return e.id }

// Period returns the YYYYMM period this push executed for.
func (e *Entity) Period() string { return e.period }

// PushedAt returns the timestamp this push executed at.
func (e *Entity) PushedAt() string { return e.pushedAt }

// PushedBy returns the user ID who triggered the push.
func (e *Entity) PushedBy() string { return e.pushedBy }

// MBCount returns the number of MB heads included in this push.
func (e *Entity) MBCount() int32 { return e.mbCount }

// RowCount returns the number of cst_mb_cost rows written by this push.
func (e *Entity) RowCount() int32 { return e.rowCount }

// CostTypes returns the comma-separated cost types included in this push.
func (e *Entity) CostTypes() string { return e.costTypes }

// PreviousPeriod returns the prior period this push compared against, if any.
func (e *Entity) PreviousPeriod() string { return e.previousPeriod }

// Notes returns free-form notes attached to this push execution.
func (e *Entity) Notes() string { return e.notes }
