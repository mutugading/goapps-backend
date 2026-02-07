// Package session provides application layer handlers for session management.
package session

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/session"
)

// RevokeCommand contains the session ID to revoke.
type RevokeCommand struct {
	SessionID string
}

// RevokeHandler handles revoking a single session.
type RevokeHandler struct {
	repo session.Repository
}

// NewRevokeHandler creates a new RevokeHandler.
func NewRevokeHandler(repo session.Repository) *RevokeHandler {
	return &RevokeHandler{repo: repo}
}

// Handle executes the revoke session command.
func (h *RevokeHandler) Handle(ctx context.Context, cmd RevokeCommand) error {
	id, err := uuid.Parse(cmd.SessionID)
	if err != nil {
		return fmt.Errorf("invalid session ID: %w", err)
	}

	return h.repo.Revoke(ctx, id)
}

// RevokeAllCommand contains the user ID whose sessions should be revoked.
type RevokeAllCommand struct {
	UserID string
}

// RevokeAllHandler handles revoking all sessions for a user.
type RevokeAllHandler struct {
	repo session.Repository
}

// NewRevokeAllHandler creates a new RevokeAllHandler.
func NewRevokeAllHandler(repo session.Repository) *RevokeAllHandler {
	return &RevokeAllHandler{repo: repo}
}

// Handle executes the revoke all sessions command.
func (h *RevokeAllHandler) Handle(ctx context.Context, cmd RevokeAllCommand) error {
	id, err := uuid.Parse(cmd.UserID)
	if err != nil {
		return fmt.Errorf("invalid user ID: %w", err)
	}

	return h.repo.RevokeAllForUser(ctx, id)
}
