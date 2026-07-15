package chat

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/chat"
)

// ListConversationsHandler lists conversations for the calling user.
type ListConversationsHandler struct {
	convRepo chat.ConversationRepository
}

// NewListConversationsHandler constructs the handler.
func NewListConversationsHandler(convRepo chat.ConversationRepository) *ListConversationsHandler {
	return &ListConversationsHandler{convRepo: convRepo}
}

// ListResult holds the list result with pagination.
type ListResult struct {
	Conversations []*chat.Conversation
	Total         int64
}

// Handle returns paginated conversations for callerID.
func (h *ListConversationsHandler) Handle(ctx context.Context, callerID uuid.UUID, page, pageSize int) (*ListResult, error) {
	convs, total, err := h.convRepo.ListByUserID(ctx, callerID, page, pageSize)
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	return &ListResult{Conversations: convs, Total: total}, nil
}
