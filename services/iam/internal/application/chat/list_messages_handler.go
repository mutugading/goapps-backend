package chat

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/chat"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/crypto"
)

// ListMessagesHandler lists messages for a conversation, decrypting bodies.
type ListMessagesHandler struct {
	convRepo chat.ConversationRepository
	msgRepo  chat.MessageRepository
	enc      *crypto.Encryptor
}

// NewListMessagesHandler constructs the handler.
func NewListMessagesHandler(convRepo chat.ConversationRepository, msgRepo chat.MessageRepository, enc *crypto.Encryptor) *ListMessagesHandler {
	return &ListMessagesHandler{convRepo: convRepo, msgRepo: msgRepo, enc: enc}
}

// DecryptedMessage pairs a domain message with its decrypted body.
type DecryptedMessage struct {
	*chat.Message
	PlainBody string
}

// MessagesResult holds the paginated message list.
type MessagesResult struct {
	Messages   []*DecryptedMessage
	NextCursor string
	HasMore    bool
}

// Handle fetches and decrypts messages. callerID must be an active participant.
func (h *ListMessagesHandler) Handle(ctx context.Context, callerID, convID uuid.UUID, pageSize int, beforeCursor string) (*MessagesResult, error) {
	conv, err := h.convRepo.GetByID(ctx, convID)
	if err != nil {
		return nil, err
	}
	p := conv.FindParticipant(callerID)
	if p == nil || !p.IsActive() {
		return nil, fmt.Errorf("list messages: %w", chat.ErrNotParticipant)
	}

	convKeyPlain, err := h.enc.DecryptConversationKey(conv.EncryptionKey())
	if err != nil {
		log.Warn().Err(err).Str("conv", convID.String()).Msg("list messages: decrypt conv key failed (master key may have changed)")
		return &MessagesResult{Messages: []*DecryptedMessage{}, NextCursor: "", HasMore: false}, nil
	}

	msgs, nextCursor, hasMore, err := h.msgRepo.ListByConversation(ctx, convID, pageSize, beforeCursor, p.HistoryClearedAt())
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}

	decrypted := make([]*DecryptedMessage, 0, len(msgs))
	for _, msg := range msgs {
		plain := "[deleted]"
		if !msg.IsDeleted() {
			var decErr error
			plain, decErr = h.enc.DecryptMessage(convKeyPlain, msg.BodyEncrypted())
			if decErr != nil {
				plain = decryptionErrorBody
			}
		}
		decrypted = append(decrypted, &DecryptedMessage{Message: msg, PlainBody: plain})
	}
	return &MessagesResult{Messages: decrypted, NextCursor: nextCursor, HasMore: hasMore}, nil
}
