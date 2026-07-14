// Package mbhead provides application layer handlers for MB Head operations.
package mbhead

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// UnApproveCommand represents the APPROVED → UN_APPROVED transition command.
type UnApproveCommand struct {
	MbhID       uuid.UUID
	Reason      string
	ActorUserID string
}

// UnApproveHandler handles the UnApproveMBHead command.
type UnApproveHandler struct {
	repo mbhead.Repository
}

// NewUnApproveHandler creates a new UnApproveHandler.
func NewUnApproveHandler(repo mbhead.Repository) *UnApproveHandler {
	return &UnApproveHandler{repo: repo}
}

// Handle executes the un-approve MB Head transition.
func (h *UnApproveHandler) Handle(ctx context.Context, cmd UnApproveCommand) (*mbhead.Entity, error) {
	entity, err := h.repo.GetByID(ctx, cmd.MbhID)
	if err != nil {
		return nil, err
	}

	fromState := entity.EntryStatus()
	if err := entity.UnApprove(cmd.Reason); err != nil {
		return nil, err
	}

	if err := h.repo.Transition(ctx, entity.ID(), fromState, entity.EntryStatus(), entity.CurrentVersion(), entity.StateReason(), cmd.ActorUserID, nil); err != nil {
		return nil, err
	}

	return entity, nil
}
