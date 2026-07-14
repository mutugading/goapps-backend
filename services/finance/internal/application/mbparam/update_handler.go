package mbparam

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbparam"
)

// UpdateCommand represents the update MB param command.
type UpdateCommand struct {
	ID            string
	Name          string
	Description   string
	DefaultValue  string
	DefaultOption string
	Unit          string
	DisplayOrder  int32
	IsActive      bool
	UpdatedBy     string
}

// UpdateHandler handles the UpdateMbParam command.
type UpdateHandler struct {
	repo mbparam.Repository
}

// NewUpdateHandler creates a new UpdateHandler.
func NewUpdateHandler(repo mbparam.Repository) *UpdateHandler {
	return &UpdateHandler{repo: repo}
}

// Handle executes the update MB param command.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*mbparam.Entity, error) {
	existing, err := h.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}

	entity := mbparam.Reconstruct(
		existing.ID(), existing.Code(), cmd.Name, cmd.Description, existing.Type(),
		cmd.DefaultValue, cmd.DefaultOption, cmd.Unit, cmd.DisplayOrder, cmd.IsActive,
		existing.CreatedAt(), existing.CreatedBy(), existing.UpdatedAt(), cmd.UpdatedBy,
		existing.DeletedAt(), existing.DeletedBy(),
	)

	if err := h.repo.Update(ctx, entity); err != nil {
		return nil, err
	}

	return entity, nil
}
