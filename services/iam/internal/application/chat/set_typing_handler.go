package chat

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/chat"
	chatinfra "github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/chat"
)

// SetTypingHandler publishes typing indicator events.
type SetTypingHandler struct {
	convRepo    chat.ConversationRepository
	presence    *chatinfra.PresenceService
	broadcaster *chatinfra.Broadcaster
}

// NewSetTypingHandler constructs the handler.
func NewSetTypingHandler(convRepo chat.ConversationRepository, presence *chatinfra.PresenceService, broadcaster *chatinfra.Broadcaster) *SetTypingHandler {
	return &SetTypingHandler{convRepo: convRepo, presence: presence, broadcaster: broadcaster}
}

// Handle sets/clears typing indicator and broadcasts to other participants.
func (h *SetTypingHandler) Handle(ctx context.Context, callerID, convID uuid.UUID, isTyping bool, callerName string) error {
	conv, err := h.convRepo.GetByID(ctx, convID)
	if err != nil {
		return err
	}
	p := conv.FindParticipant(callerID)
	if p == nil || !p.IsActive() {
		return fmt.Errorf("set typing: %w", chat.ErrNotParticipant)
	}
	if err := h.presence.SetTyping(ctx, convID, callerID, isTyping); err != nil {
		return fmt.Errorf("set typing: %w", err)
	}

	payload, err := json.Marshal(map[string]any{
		"type":            "typing",
		"conversation_id": convID.String(),
		"user_id":         callerID.String(),
		"user_name":       callerName,
		"is_typing":       isTyping,
	})
	if err != nil {
		log.Warn().Err(err).Msg("set typing: marshal broadcast")
		return nil
	}
	eventID := fmt.Sprintf("typing-%s-%s", convID, callerID)
	for _, part := range conv.Participants() {
		if !part.IsActive() || part.UserID() == callerID {
			continue // don't echo back to sender
		}
		h.broadcaster.Publish(&chatinfra.Event{
			EventID: fmt.Sprintf("%s-%s", eventID, part.UserID()),
			UserID:  part.UserID(),
			Payload: payload,
		})
	}
	return nil
}
