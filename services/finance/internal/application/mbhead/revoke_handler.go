// Package mbhead provides application layer handlers for MB Head operations.
package mbhead

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// RevokeCommand represents the transition to REVOKED (terminal) command.
type RevokeCommand struct {
	MbhID       uuid.UUID
	Reason      string
	ActorUserID string
}

// RevokeHandler handles the RevokeMBHead command.
type RevokeHandler struct {
	repo mbhead.Repository
}

// NewRevokeHandler creates a new RevokeHandler.
func NewRevokeHandler(repo mbhead.Repository) *RevokeHandler {
	return &RevokeHandler{repo: repo}
}

// Handle executes the revoke MB Head transition.
func (h *RevokeHandler) Handle(ctx context.Context, cmd RevokeCommand) (*mbhead.Entity, error) {
	entity, err := h.repo.GetByID(ctx, cmd.MbhID)
	if err != nil {
		return nil, err
	}

	fromState := entity.EntryStatus()
	if err := entity.Revoke(cmd.Reason); err != nil {
		return nil, err
	}

	if err := h.repo.Transition(ctx, entity.ID(), fromState, entity.EntryStatus(), entity.CurrentVersion(), entity.StateReason(), cmd.ActorUserID, nil); err != nil {
		return nil, err
	}

	return entity, nil
}
