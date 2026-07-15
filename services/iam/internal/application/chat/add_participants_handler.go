package chat

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/chat"
)

// AddParticipantsHandler adds participants to a group conversation.
type AddParticipantsHandler struct {
	convRepo chat.ConversationRepository
}

// NewAddParticipantsHandler constructs the handler.
func NewAddParticipantsHandler(convRepo chat.ConversationRepository) *AddParticipantsHandler {
	return &AddParticipantsHandler{convRepo: convRepo}
}

// Handle adds userIDs to the conversation. Caller must be ADMIN or OWNER.
func (h *AddParticipantsHandler) Handle(ctx context.Context, callerID, convID uuid.UUID, userIDs []uuid.UUID) error {
	conv, err := h.convRepo.GetByID(ctx, convID)
	if err != nil {
		return err
	}
	p := conv.FindParticipant(callerID)
	if p == nil || !p.IsActive() {
		return fmt.Errorf("add participants: %w", chat.ErrNotParticipant)
	}
	if !p.Role().IsAdminOrOwner() {
		return fmt.Errorf("add participants: %w", chat.ErrNotAdmin)
	}
	newParts := make([]*chat.Participant, 0, len(userIDs))
	for _, uid := range userIDs {
		if err := conv.AddParticipant(uid, chat.RoleMember); err != nil {
			return fmt.Errorf("add participants: %w", err)
		}
		newParts = append(newParts, conv.FindParticipant(uid))
	}
	return h.convRepo.AddParticipants(ctx, convID, newParts)
}
