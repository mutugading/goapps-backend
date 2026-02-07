// Package user provides application layer handlers for User operations.
package user

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/user"
)

// UpdateCommand represents the update user command.
type UpdateCommand struct {
	UserID    string
	Email     *string
	IsActive  *bool
	UpdatedBy string
}

// UpdateHandler handles the update user command.
type UpdateHandler struct {
	repo user.Repository
}

// NewUpdateHandler creates a new UpdateHandler.
func NewUpdateHandler(repo user.Repository) *UpdateHandler {
	return &UpdateHandler{repo: repo}
}

// Handle executes the update user command.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*user.User, error) {
	// 1. Parse ID.
	id, err := uuid.Parse(cmd.UserID)
	if err != nil {
		return nil, shared.ErrNotFound
	}

	// 2. Get existing entity.
	entity, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 3. Check for duplicate email if email is being updated.
	if cmd.Email != nil {
		existingByEmail, err := h.repo.GetByEmail(ctx, *cmd.Email)
		if err == nil && existingByEmail.ID() != entity.ID() {
			return nil, shared.ErrAlreadyExists
		}
	}

	// 4. Update domain entity.
	if err := entity.Update(cmd.Email, cmd.IsActive, cmd.UpdatedBy); err != nil {
		return nil, err
	}

	// 5. Persist.
	if err := h.repo.Update(ctx, entity); err != nil {
		return nil, err
	}

	return entity, nil
}
