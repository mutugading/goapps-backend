package mbcomposition

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcomposition"
)

// UpdateCommand represents the update MB composition command.
type UpdateCommand struct {
	ID             string
	GroupHeadID    string
	CompositionPct string
	SourceType     string
	MbRefMbhID     string
	IsCarrier      bool
	UpdatedBy      string
}

// UpdateHandler handles the UpdateMbComposition command.
type UpdateHandler struct {
	repo mbcomposition.Repository
}

// NewUpdateHandler creates a new UpdateHandler.
func NewUpdateHandler(repo mbcomposition.Repository) *UpdateHandler {
	return &UpdateHandler{repo: repo}
}

// Handle executes the update MB composition command.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*mbcomposition.Entity, error) {
	existing, err := h.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}

	entity := mbcomposition.Reconstruct(
		existing.ID(), existing.MbhID(), existing.SeqNo(), cmd.GroupHeadID, cmd.CompositionPct,
		cmd.SourceType, cmd.MbRefMbhID, cmd.IsCarrier, existing.LegacySysID(),
		existing.CreatedAt(), existing.CreatedBy(), existing.UpdatedAt(), cmd.UpdatedBy,
		existing.DeletedAt(), existing.DeletedBy(),
	)

	if err := h.repo.Update(ctx, entity); err != nil {
		return nil, err
	}

	return entity, nil
}
