package chat

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/chat"
)

// GetConversationHandler fetches a conversation, enforcing participant check.
type GetConversationHandler struct {
	convRepo chat.ConversationRepository
}

// NewGetConversationHandler constructs the handler.
func NewGetConversationHandler(convRepo chat.ConversationRepository) *GetConversationHandler {
	return &GetConversationHandler{convRepo: convRepo}
}

// Handle returns the conversation if callerID is an active participant.
func (h *GetConversationHandler) Handle(ctx context.Context, callerID, convID uuid.UUID) (*chat.Conversation, error) {
	conv, err := h.convRepo.GetByID(ctx, convID)
	if err != nil {
		return nil, err
	}
	p := conv.FindParticipant(callerID)
	if p == nil || !p.IsActive() {
		return nil, fmt.Errorf("get conversation: %w", chat.ErrNotParticipant)
	}
	return conv, nil
}
