// Package mbhead provides application layer handlers for MB Head operations.
package mbhead

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// ApproveCommand represents the SUBMITTED → APPROVED (or UN_APPROVED → APPROVED) transition command.
type ApproveCommand struct {
	MbhID       uuid.UUID
	ActorUserID string
}

// ApproveHandler handles the ApproveMBHead command.
type ApproveHandler struct {
	repo mbhead.Repository
}

// NewApproveHandler creates a new ApproveHandler.
func NewApproveHandler(repo mbhead.Repository) *ApproveHandler {
	return &ApproveHandler{repo: repo}
}

// Handle executes the approve MB Head transition.
func (h *ApproveHandler) Handle(ctx context.Context, cmd ApproveCommand) (*mbhead.Entity, error) {
	entity, err := h.repo.GetByID(ctx, cmd.MbhID)
	if err != nil {
		return nil, err
	}

	fromState := entity.EntryStatus()
	if err := entity.Approve(); err != nil {
		return nil, err
	}

	if err := h.repo.Transition(ctx, entity.ID(), fromState, entity.EntryStatus(), entity.CurrentVersion(), "", cmd.ActorUserID, nil); err != nil {
		return nil, err
	}

	return entity, nil
}
