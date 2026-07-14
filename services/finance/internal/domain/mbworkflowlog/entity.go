// Package mbworkflowlog provides domain logic for MB (Master Batch) head workflow-transition audit logging.
package mbworkflowlog

// Entity is a single MB head workflow-transition audit log row.
type Entity struct {
	id          string
	mbhID       string
	fromState   string
	toState     string
	actorUserID string
	actorAt     string
	reason      string
	version     int32
}

// NewEntity constructs a new workflow-log row, validating mbh_id and to_state are present.
func NewEntity(mbhID, fromState, toState, actorUserID string) (*Entity, error) {
	if mbhID == "" {
		return nil, ErrMbhIDRequired
	}
	if toState == "" {
		return nil, ErrToStateRequired
	}
	return &Entity{mbhID: mbhID, fromState: fromState, toState: toState, actorUserID: actorUserID}, nil
}

// Reconstruct rebuilds a workflow-log Entity from persisted values, bypassing NewEntity's
// validation since the row already exists in storage.
//
//nolint:revive // positional params mirror the hydration DTO's column order
func Reconstruct(id, mbhID, fromState, toState, actorUserID, actorAt, reason string, version int32) *Entity {
	return &Entity{
		id:          id,
		mbhID:       mbhID,
		fromState:   fromState,
		toState:     toState,
		actorUserID: actorUserID,
		actorAt:     actorAt,
		reason:      reason,
		version:     version,
	}
}

// ID returns the log row's UUID.
func (e *Entity) ID() string { return e.id }

// MbhID returns the MB head this transition belongs to.
func (e *Entity) MbhID() string { return e.mbhID }

// FromState returns the state transitioned out of, empty if this is the initial transition.
func (e *Entity) FromState() string { return e.fromState }

// ToState returns the state transitioned into.
func (e *Entity) ToState() string { return e.toState }

// ActorUserID returns the user ID who performed this transition.
func (e *Entity) ActorUserID() string { return e.actorUserID }

// ActorAt returns the timestamp this transition occurred at.
func (e *Entity) ActorAt() string { return e.actorAt }

// Reason returns the free-form reason attached to this transition, if any.
func (e *Entity) Reason() string { return e.reason }

// Version returns the mb_head composition version this transition was recorded against.
func (e *Entity) Version() int32 { return e.version }
