package chat

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	domainChat "github.com/mutugading/goapps-backend/services/iam/internal/domain/chat"
	chatinfra "github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/chat"
)

// broadcastMessageEvent publishes a message-related event (created, edited) to
// every active participant of conv via the given broadcaster.
func broadcastMessageEvent(broadcaster *chatinfra.Broadcaster, conv *domainChat.Conversation, msg *domainChat.Message, plainBody, eventType string) {
	payload := map[string]any{
		"type":            eventType,
		"conversation_id": conv.ID().String(),
		"message_id":      msg.MessageID().String(),
		"sender_user_id":  msg.SenderUserID().String(),
		"body":            plainBody,
		"is_edited":       msg.IsEdited(),
		"is_deleted":      msg.IsDeleted(),
		"created_at":      msg.CreatedAt().UTC().Format(time.RFC3339Nano),
		"updated_at":      msg.UpdatedAt().UTC().Format(time.RFC3339Nano),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		log.Warn().Err(err).Msg("broadcast message event: marshal payload")
		return
	}
	eventID := fmt.Sprintf("msg-%s", msg.MessageID())
	for _, part := range conv.Participants() {
		if !part.IsActive() {
			continue
		}
		broadcaster.Publish(&chatinfra.Event{
			EventID: fmt.Sprintf("%s-%s", eventID, part.UserID()),
			UserID:  part.UserID(),
			Payload: data,
		})
	}
}
