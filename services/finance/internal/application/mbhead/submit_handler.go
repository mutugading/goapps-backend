// Package mbhead provides application layer handlers for MB Head operations.
package mbhead

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// SubmitCommand represents the DRAFT → SUBMITTED transition command.
type SubmitCommand struct {
	MbhID       uuid.UUID
	ActorUserID string
}

// SubmitHandler handles the SubmitMBHead command.
type SubmitHandler struct {
	repo mbhead.Repository
}

// NewSubmitHandler creates a new SubmitHandler.
func NewSubmitHandler(repo mbhead.Repository) *SubmitHandler {
	return &SubmitHandler{repo: repo}
}

// Handle executes the submit MB Head transition.
func (h *SubmitHandler) Handle(ctx context.Context, cmd SubmitCommand) (*mbhead.Entity, error) {
	entity, err := h.repo.GetByID(ctx, cmd.MbhID)
	if err != nil {
		return nil, err
	}

	fromState := entity.EntryStatus()
	if err := entity.Submit(); err != nil {
		return nil, err
	}

	if err := h.repo.Transition(ctx, entity.ID(), fromState, entity.EntryStatus(), entity.CurrentVersion(), "", cmd.ActorUserID, nil); err != nil {
		return nil, err
	}

	return entity, nil
}
