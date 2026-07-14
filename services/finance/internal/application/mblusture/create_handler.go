// Package mblusture provides application layer handlers for MB lusture master data operations.
package mblusture

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mblusture"
)

// CreateCommand represents the create MB lusture command.
type CreateCommand struct {
	Code            string
	DisplayName     string
	FullDescription string
	Category        string
	DisplayOrder    int32
	CreatedBy       string
}

// CreateHandler handles the CreateMbLusture command.
type CreateHandler struct {
	repo mblusture.Repository
}

// NewCreateHandler creates a new CreateHandler.
func NewCreateHandler(repo mblusture.Repository) *CreateHandler {
	return &CreateHandler{repo: repo}
}

// Handle executes the create MB lusture command.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*mblusture.Entity, error) {
	entity, err := mblusture.NewEntity(cmd.Code, cmd.DisplayName, cmd.FullDescription, cmd.Category, cmd.DisplayOrder, cmd.CreatedBy)
	if err != nil {
		return nil, err
	}

	if err := h.repo.Create(ctx, entity); err != nil {
		return nil, err
	}

	return entity, nil
}
