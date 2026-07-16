package chat

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/chat"
)

// ClearHistoryHandler clears the calling user's own view of a conversation's
// message history. Messages remain visible to other participants.
type ClearHistoryHandler struct {
	convRepo chat.ConversationRepository
}

// NewClearHistoryHandler constructs the handler.
func NewClearHistoryHandler(convRepo chat.ConversationRepository) *ClearHistoryHandler {
	return &ClearHistoryHandler{convRepo: convRepo}
}

// Handle verifies callerID is an active participant, then clears their history.
func (h *ClearHistoryHandler) Handle(ctx context.Context, callerID, convID uuid.UUID) error {
	conv, err := h.convRepo.GetByID(ctx, convID)
	if err != nil {
		return err
	}
	p := conv.FindParticipant(callerID)
	if p == nil || !p.IsActive() {
		return fmt.Errorf("clear history: %w", chat.ErrNotParticipant)
	}
	return h.convRepo.ClearHistory(ctx, convID, callerID)
}
