package chat

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
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

	eventID := fmt.Sprintf("typing-%s-%s", convID, callerID)
	resp := &iamv1.StreamChatEventsResponse{
		EventId: eventID,
		Payload: &iamv1.StreamChatEventsResponse_Typing{
			Typing: &iamv1.TypingEvent{
				ConversationId: convID.String(),
				UserId:         callerID.String(),
				UserName:       callerName,
				IsTyping:       isTyping,
			},
		},
	}
	for _, part := range conv.Participants() {
		if !part.IsActive() || part.UserID() == callerID {
			continue
		}
		h.broadcaster.Publish(&chatinfra.Event{
			EventID:  fmt.Sprintf("%s-%s", eventID, part.UserID()),
			UserID:   part.UserID(),
			Response: resp,
		})
	}
	return nil
}
