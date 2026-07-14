package mblusture

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mblusture"
)

// UpdateCommand represents the update MB lusture command.
type UpdateCommand struct {
	ID              string
	DisplayName     string
	FullDescription string
	Category        string
	DisplayOrder    int32
	IsActive        bool
	UpdatedBy       string
}

// UpdateHandler handles the UpdateMbLusture command.
type UpdateHandler struct {
	repo mblusture.Repository
}

// NewUpdateHandler creates a new UpdateHandler.
func NewUpdateHandler(repo mblusture.Repository) *UpdateHandler {
	return &UpdateHandler{repo: repo}
}

// Handle executes the update MB lusture command.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*mblusture.Entity, error) {
	existing, err := h.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}

	entity := mblusture.Reconstruct(
		existing.ID(), existing.Code(), cmd.DisplayName, cmd.FullDescription, cmd.Category,
		cmd.DisplayOrder, cmd.IsActive, existing.CreatedAt(), existing.CreatedBy(),
		existing.UpdatedAt(), cmd.UpdatedBy, existing.DeletedAt(), existing.DeletedBy(),
	)

	if err := h.repo.Update(ctx, entity); err != nil {
		return nil, err
	}

	return entity, nil
}
