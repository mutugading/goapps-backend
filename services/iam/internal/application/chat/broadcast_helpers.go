package chat

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
	domainChat "github.com/mutugading/goapps-backend/services/iam/internal/domain/chat"
	chatinfra "github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/chat"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/postgres"
)

// decryptionErrorBody is the placeholder body shown when a message body
// fails to decrypt (e.g. master key rotated out from under an old message).
const decryptionErrorBody = "[decryption error]"

// resolveSenderName looks up a display name for senderID via userResolver,
// falling back to username, then an empty string if the resolver is nil or
// the lookup fails.
func resolveSenderName(ctx context.Context, userResolver *postgres.ChatUserResolver, senderID uuid.UUID) string {
	if userResolver == nil {
		return ""
	}
	infos, err := userResolver.ResolveUsers(ctx, []uuid.UUID{senderID})
	if err != nil {
		log.Warn().Err(err).Str("user", senderID.String()).Msg("chat: resolve sender name")
		return ""
	}
	info := infos[senderID]
	if info == nil {
		return ""
	}
	if info.FullName != "" {
		return info.FullName
	}
	return info.Username
}

// broadcastMessageEvent publishes a message-related event (created, edited) to
// every active participant of conv via the given broadcaster.
func broadcastMessageEvent(broadcaster *chatinfra.Broadcaster, conv *domainChat.Conversation, msg *domainChat.Message, plainBody, eventType, senderName string, attachments []*iamv1.AttachmentProto) {
	msgProto := domainMsgToProto(msg, plainBody)
	msgProto.SenderName = senderName
	msgProto.Attachments = attachments
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

// attachmentToProto maps a domain attachment to its proto representation.
func attachmentToProto(a *domainChat.Attachment) *iamv1.AttachmentProto {
	return &iamv1.AttachmentProto{
		AttachmentId: a.AttachmentID().String(),
		FileName:     a.FileName(),
		FileUrl:      a.FileURL(),
		ContentType:  a.ContentType(),
		FileSize:     a.FileSize(),
		ThumbnailUrl: a.ThumbnailURL(),
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
