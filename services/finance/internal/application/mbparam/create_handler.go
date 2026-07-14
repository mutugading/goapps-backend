// Package mbparam provides application layer handlers for MB param master data operations.
package mbparam

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbparam"
)

// CreateCommand represents the create MB param command.
type CreateCommand struct {
	Code          string
	Name          string
	Description   string
	Type          string
	DefaultValue  string
	DefaultOption string
	Unit          string
	DisplayOrder  int32
	CreatedBy     string
}

// CreateHandler handles the CreateMbParam command.
type CreateHandler struct {
	repo mbparam.Repository
}

// NewCreateHandler creates a new CreateHandler.
func NewCreateHandler(repo mbparam.Repository) *CreateHandler {
	return &CreateHandler{repo: repo}
}

// Handle executes the create MB param command.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*mbparam.Entity, error) {
	entity, err := mbparam.NewEntity(cmd.Code, cmd.Name, cmd.Type, cmd.Description, cmd.DefaultValue,
		cmd.DefaultOption, cmd.Unit, cmd.DisplayOrder, cmd.CreatedBy)
	if err != nil {
		return nil, err
	}

	if err := h.repo.Create(ctx, entity); err != nil {
		return nil, err
	}

	return entity, nil
}
