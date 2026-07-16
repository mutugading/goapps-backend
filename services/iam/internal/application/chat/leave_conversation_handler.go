package chat

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/chat"
)

// LeaveConversationHandler removes the calling user from a conversation.
type LeaveConversationHandler struct {
	convRepo chat.ConversationRepository
}

// NewLeaveConversationHandler constructs the handler.
func NewLeaveConversationHandler(convRepo chat.ConversationRepository) *LeaveConversationHandler {
	return &LeaveConversationHandler{convRepo: convRepo}
}

// Handle removes callerID from the conversation.
func (h *LeaveConversationHandler) Handle(ctx context.Context, callerID, convID uuid.UUID) error {
	conv, err := h.convRepo.GetByID(ctx, convID)
	if err != nil {
		return err
	}
	p := conv.FindParticipant(callerID)
	if p == nil || !p.IsActive() {
		return fmt.Errorf("leave conversation: %w", chat.ErrNotParticipant)
	}
	return h.convRepo.RemoveParticipant(ctx, convID, callerID)
}
