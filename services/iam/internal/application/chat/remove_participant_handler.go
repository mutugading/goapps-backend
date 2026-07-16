package chat

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/chat"
)

// RemoveParticipantHandler removes a participant from a group conversation.
type RemoveParticipantHandler struct {
	convRepo chat.ConversationRepository
}

// NewRemoveParticipantHandler constructs the handler.
func NewRemoveParticipantHandler(convRepo chat.ConversationRepository) *RemoveParticipantHandler {
	return &RemoveParticipantHandler{convRepo: convRepo}
}

// Handle removes targetUserID from the conversation. Caller must be ADMIN/OWNER.
func (h *RemoveParticipantHandler) Handle(ctx context.Context, callerID, convID, targetUserID uuid.UUID) error {
	conv, err := h.convRepo.GetByID(ctx, convID)
	if err != nil {
		return err
	}
	p := conv.FindParticipant(callerID)
	if p == nil || !p.IsActive() {
		return fmt.Errorf("remove participant: %w", chat.ErrNotParticipant)
	}
	if !p.Role().IsAdminOrOwner() {
		return fmt.Errorf("remove participant: %w", chat.ErrNotAdmin)
	}
	target := conv.FindParticipant(targetUserID)
	if target == nil || !target.IsActive() {
		return fmt.Errorf("remove participant: target %w", chat.ErrNotParticipant)
	}
	return h.convRepo.RemoveParticipant(ctx, convID, targetUserID)
}
