package chat

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/chat"
	chatinfra "github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/chat"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/crypto"
)

// SendMessageHandler sends a new message to a conversation.
type SendMessageHandler struct {
	convRepo    chat.ConversationRepository
	msgRepo     chat.MessageRepository
	receiptRepo chat.ReadReceiptRepository
	enc         *crypto.Encryptor
	broadcaster *chatinfra.Broadcaster
}

// NewSendMessageHandler constructs the handler.
func NewSendMessageHandler(
	convRepo chat.ConversationRepository,
	msgRepo chat.MessageRepository,
	receiptRepo chat.ReadReceiptRepository,
	enc *crypto.Encryptor,
	broadcaster *chatinfra.Broadcaster,
) *SendMessageHandler {
	return &SendMessageHandler{convRepo: convRepo, msgRepo: msgRepo, receiptRepo: receiptRepo, enc: enc, broadcaster: broadcaster}
}

// Handle validates participation, encrypts body, saves, and broadcasts.
func (h *SendMessageHandler) Handle(ctx context.Context, senderID, convID uuid.UUID, body string, replyToID uuid.UUID) (*chat.Message, error) {
	conv, err := h.convRepo.GetByID(ctx, convID)
	if err != nil {
		return nil, err
	}
	p := conv.FindParticipant(senderID)
	if p == nil || !p.IsActive() {
		return nil, fmt.Errorf("send message: %w", chat.ErrNotParticipant)
	}

	convKeyPlain, err := h.enc.DecryptConversationKey(conv.EncryptionKey())
	if err != nil {
		return nil, fmt.Errorf("send message: decrypt conv key: %w", err)
	}

	bodyEnc, err := h.enc.EncryptMessage(convKeyPlain, body)
	if err != nil {
		return nil, fmt.Errorf("send message: encrypt body: %w", err)
	}
	bodyPlainEnc, err := h.enc.EncryptMessage(convKeyPlain, body) // same body, independent nonce for search index slot
	if err != nil {
		return nil, fmt.Errorf("send message: encrypt plain: %w", err)
	}

	msg := chat.NewMessage(convID, senderID, bodyEnc, bodyPlainEnc, replyToID)
	if err := h.msgRepo.Create(ctx, msg); err != nil {
		return nil, fmt.Errorf("send message: persist: %w", err)
	}

	// Auto-read for sender.
	if err := h.receiptRepo.Upsert(ctx, msg.MessageID(), senderID); err != nil {
		log.Warn().Err(err).Msg("send message: auto read receipt failed")
	}

	broadcastMessageEvent(h.broadcaster, conv, msg, body, "message_received")
	return msg, nil
}
