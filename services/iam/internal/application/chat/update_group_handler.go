package chat

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/chat"
)

// UpdateGroupHandler updates a group conversation's name and avatar.
type UpdateGroupHandler struct {
	convRepo chat.ConversationRepository
}

// NewUpdateGroupHandler constructs the handler.
func NewUpdateGroupHandler(convRepo chat.ConversationRepository) *UpdateGroupHandler {
	return &UpdateGroupHandler{convRepo: convRepo}
}

// Handle updates the group conversation. Caller must be ADMIN or OWNER.
func (h *UpdateGroupHandler) Handle(ctx context.Context, callerID, convID uuid.UUID, name, avatarURL string) (*chat.Conversation, error) {
	conv, err := h.convRepo.GetByID(ctx, convID)
	if err != nil {
		return nil, err
	}
	p := conv.FindParticipant(callerID)
	if p == nil || !p.IsActive() {
		return nil, fmt.Errorf("update group: %w", chat.ErrNotParticipant)
	}
	if !p.Role().IsAdminOrOwner() {
		return nil, fmt.Errorf("update group: %w", chat.ErrNotAdmin)
	}
	conv.UpdateGroup(name, avatarURL)
	if err := h.convRepo.UpdateGroup(ctx, conv); err != nil {
		return nil, fmt.Errorf("update group: persist: %w", err)
	}
	return conv, nil
}
