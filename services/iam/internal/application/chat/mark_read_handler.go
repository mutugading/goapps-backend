package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/chat"
	chatinfra "github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/chat"
)

// MarkReadHandler marks all unread messages in a conversation as read.
type MarkReadHandler struct {
	convRepo    chat.ConversationRepository
	msgRepo     chat.MessageRepository
	receiptRepo chat.ReadReceiptRepository
	broadcaster *chatinfra.Broadcaster
}

// NewMarkReadHandler constructs the handler.
func NewMarkReadHandler(
	convRepo chat.ConversationRepository,
	msgRepo chat.MessageRepository,
	receiptRepo chat.ReadReceiptRepository,
	broadcaster *chatinfra.Broadcaster,
) *MarkReadHandler {
	return &MarkReadHandler{convRepo: convRepo, msgRepo: msgRepo, receiptRepo: receiptRepo, broadcaster: broadcaster}
}

// Handle upserts read receipts for recent messages and updates last_read_at.
func (h *MarkReadHandler) Handle(ctx context.Context, callerID, convID uuid.UUID) error {
	conv, err := h.convRepo.GetByID(ctx, convID)
	if err != nil {
		return err
	}
	p := conv.FindParticipant(callerID)
	if p == nil || !p.IsActive() {
		return fmt.Errorf("mark read: %w", chat.ErrNotParticipant)
	}

	// Fetch recent messages (up to last 100) and upsert receipts.
	msgs, _, _, err := h.msgRepo.ListByConversation(ctx, convID, 100, "")
	if err != nil {
		return fmt.Errorf("mark read: list: %w", err)
	}

	ids := make([]uuid.UUID, 0, len(msgs))
	for _, m := range msgs {
		if !m.IsDeleted() {
			ids = append(ids, m.MessageID())
		}
	}
	if err := h.receiptRepo.UpsertBulk(ctx, ids, callerID); err != nil {
		return fmt.Errorf("mark read: upsert receipts: %w", err)
	}

	now := time.Now().UTC()
	if err := h.convRepo.UpdateLastReadAt(ctx, convID, callerID, now); err != nil {
		log.Warn().Err(err).Msg("mark read: update last_read_at")
	}

	// Broadcast read receipt event to conversation participants.
	h.broadcastRead(conv, callerID, now)
	return nil
}

func (h *MarkReadHandler) broadcastRead(conv *chat.Conversation, readerID uuid.UUID, readAt time.Time) {
	payload, err := json.Marshal(map[string]string{
		"type":            "read_receipt",
		"conversation_id": conv.ID().String(),
		"user_id":         readerID.String(),
		"read_at":         readAt.Format(time.RFC3339Nano),
	})
	if err != nil {
		log.Warn().Err(err).Msg("mark read: marshal broadcast")
		return
	}
	eventID := fmt.Sprintf("read-%s-%s", conv.ID(), readerID)
	for _, p := range conv.Participants() {
		if !p.IsActive() {
			continue
		}
		h.broadcaster.Publish(&chatinfra.Event{
			EventID: fmt.Sprintf("%s-%s", eventID, p.UserID()),
			UserID:  p.UserID(),
			Payload: payload,
		})
	}
}
