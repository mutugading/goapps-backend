package chat

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
	domainChat "github.com/mutugading/goapps-backend/services/iam/internal/domain/chat"
	chatinfra "github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/chat"
)

// broadcastMessageEvent publishes a message-related event (created, edited) to
// every active participant of conv via the given broadcaster.
func broadcastMessageEvent(broadcaster *chatinfra.Broadcaster, conv *domainChat.Conversation, msg *domainChat.Message, plainBody, eventType string) {
	msgProto := domainMsgToProto(msg, plainBody)
	eventID := fmt.Sprintf("msg-%s", msg.MessageID())

	var resp iamv1.StreamChatEventsResponse
	resp.EventId = eventID
	switch eventType {
	case "message_received":
		resp.Payload = &iamv1.StreamChatEventsResponse_MessageReceived{
			MessageReceived: &iamv1.MessageEvent{ConversationId: conv.ID().String(), Message: msgProto},
		}
	case "message_edited":
		resp.Payload = &iamv1.StreamChatEventsResponse_MessageEdited{
			MessageEdited: &iamv1.MessageEvent{ConversationId: conv.ID().String(), Message: msgProto},
		}
	}

	for _, part := range conv.Participants() {
		if !part.IsActive() {
			continue
		}
		broadcaster.Publish(&chatinfra.Event{
			EventID:  fmt.Sprintf("%s-%s", eventID, part.UserID()),
			UserID:   part.UserID(),
			Response: &resp,
		})
	}
}

func domainMsgToProto(msg *domainChat.Message, plainBody string) *iamv1.MessageProto {
	replyTo := ""
	if msg.ReplyToID() != uuid.Nil {
		replyTo = msg.ReplyToID().String()
	}
	return &iamv1.MessageProto{
		MessageId:      msg.MessageID().String(),
		ConversationId: msg.ConversationID().String(),
		SenderUserId:   msg.SenderUserID().String(),
		Body:           plainBody,
		IsEdited:       msg.IsEdited(),
		IsDeleted:      msg.IsDeleted(),
		ReplyToId:      replyTo,
		CreatedAt:      msg.CreatedAt().UTC().Format(time.RFC3339Nano),
		UpdatedAt:      msg.UpdatedAt().UTC().Format(time.RFC3339Nano),
	}
}
