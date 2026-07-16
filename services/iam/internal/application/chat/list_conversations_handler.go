package chat

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/chat"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/crypto"
)

// ListConversationsHandler lists conversations for the calling user.
type ListConversationsHandler struct {
	convRepo chat.ConversationRepository
	msgRepo  chat.MessageRepository
	enc      *crypto.Encryptor
}

// NewListConversationsHandler constructs the handler.
func NewListConversationsHandler(convRepo chat.ConversationRepository, msgRepo chat.MessageRepository, enc *crypto.Encryptor) *ListConversationsHandler {
	return &ListConversationsHandler{convRepo: convRepo, msgRepo: msgRepo, enc: enc}
}

// ConversationSummary pairs a conversation with its decrypted last message
// and the caller's unread count.
type ConversationSummary struct {
	Conversation    *chat.Conversation
	LastMessage     *chat.Message
	LastMessageBody string
	UnreadCount     int32
}

// ListResult holds the list result with pagination.
type ListResult struct {
	Conversations []*ConversationSummary
	Total         int64
}

// Handle returns paginated conversations for callerID, enriched with last
// message + unread count. Both are batch-loaded (one query each), not N+1.
func (h *ListConversationsHandler) Handle(ctx context.Context, callerID uuid.UUID, page, pageSize int) (*ListResult, error) {
	convs, total, err := h.convRepo.ListByUserID(ctx, callerID, page, pageSize)
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}

	convIDs := make([]uuid.UUID, len(convs))
	for i, c := range convs {
		convIDs[i] = c.ID()
	}

	lastMsgs, err := h.msgRepo.GetLastMessages(ctx, convIDs)
	if err != nil {
		return nil, fmt.Errorf("list conversations: last messages: %w", err)
	}
	unreadCounts, err := h.convRepo.GetUnreadCounts(ctx, convIDs, callerID)
	if err != nil {
		return nil, fmt.Errorf("list conversations: unread counts: %w", err)
	}

	summaries := make([]*ConversationSummary, 0, len(convs))
	for _, c := range convs {
		summary := &ConversationSummary{Conversation: c, UnreadCount: unreadCounts[c.ID()]}
		if lastMsg := lastMsgs[c.ID()]; lastMsg != nil {
			summary.LastMessage = lastMsg
			summary.LastMessageBody = h.decryptLastMessage(c, lastMsg)
		}
		summaries = append(summaries, summary)
	}
	return &ListResult{Conversations: summaries, Total: total}, nil
}

func (h *ListConversationsHandler) decryptLastMessage(conv *chat.Conversation, msg *chat.Message) string {
	convKeyPlain, err := h.enc.DecryptConversationKey(conv.EncryptionKey())
	if err != nil {
		log.Warn().Err(err).Str("conv", conv.ID().String()).Msg("list conversations: decrypt conv key failed")
		return ""
	}
	plain, err := h.enc.DecryptMessage(convKeyPlain, msg.BodyEncrypted())
	if err != nil {
		return decryptionErrorBody
	}
	return plain
}
