package chat

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/chat"
	chatinfra "github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/chat"
)

// DeleteMessageHandler soft-deletes a message.
type DeleteMessageHandler struct {
	convRepo    chat.ConversationRepository
	msgRepo     chat.MessageRepository
	broadcaster *chatinfra.Broadcaster
}

// NewDeleteMessageHandler constructs the handler.
func NewDeleteMessageHandler(convRepo chat.ConversationRepository, msgRepo chat.MessageRepository, broadcaster *chatinfra.Broadcaster) *DeleteMessageHandler {
	return &DeleteMessageHandler{convRepo: convRepo, msgRepo: msgRepo, broadcaster: broadcaster}
}

// Handle soft-deletes the message. Only author or conversation ADMIN/OWNER.
func (h *DeleteMessageHandler) Handle(ctx context.Context, callerID, convID, msgID uuid.UUID) error {
	conv, err := h.convRepo.GetByID(ctx, convID)
	if err != nil {
		return err
	}
	msg, err := h.msgRepo.GetByID(ctx, msgID)
	if err != nil {
		return err
	}
	if msg.SenderUserID() != callerID {
		p := conv.FindParticipant(callerID)
		if p == nil || !p.Role().IsAdminOrOwner() {
			return fmt.Errorf("delete message: %w", chat.ErrNotAuthor)
		}
	}
	if err := h.msgRepo.MarkDeleted(ctx, msgID); err != nil {
		return fmt.Errorf("delete message: %w", err)
	}
	h.broadcastDelete(conv, convID, msgID)
	return nil
}

func (h *DeleteMessageHandler) broadcastDelete(conv *chat.Conversation, convID, msgID uuid.UUID) {
	eventID := fmt.Sprintf("del-%s", msgID)
	resp := &iamv1.StreamChatEventsResponse{
		EventId: eventID,
		Payload: &iamv1.StreamChatEventsResponse_MessageDeleted{
			MessageDeleted: &iamv1.DeleteEvent{
				ConversationId: convID.String(),
				MessageId:      msgID.String(),
			},
		},
	}
	for _, p := range conv.Participants() {
		if !p.IsActive() {
			continue
		}
		h.broadcaster.Publish(&chatinfra.Event{
			EventID:  fmt.Sprintf("%s-%s", eventID, p.UserID()),
			UserID:   p.UserID(),
			Response: resp,
		})
	}
}
