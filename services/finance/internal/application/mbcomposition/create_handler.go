// Package mbcomposition provides application layer handlers for MB composition operations.
package mbcomposition

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcomposition"
)

// CreateCommand represents the create MB composition command.
type CreateCommand struct {
	MbhID          string
	GroupHeadID    string
	CompositionPct string
	SourceType     string
	SeqNo          int32
	MbRefMbhID     string
	IsCarrier      bool
	CreatedBy      string
}

// CreateHandler handles the CreateMbComposition command.
type CreateHandler struct {
	repo mbcomposition.Repository
}

// NewCreateHandler creates a new CreateHandler.
func NewCreateHandler(repo mbcomposition.Repository) *CreateHandler {
	return &CreateHandler{repo: repo}
}

// Handle executes the create MB composition command.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*mbcomposition.Entity, error) {
	entity, err := mbcomposition.NewEntity(cmd.MbhID, cmd.GroupHeadID, cmd.CompositionPct, cmd.SourceType,
		cmd.SeqNo, cmd.MbRefMbhID, cmd.IsCarrier, cmd.CreatedBy)
	if err != nil {
		return nil, err
	}

	if err := h.repo.Create(ctx, entity); err != nil {
		return nil, err
	}

	return entity, nil
}
