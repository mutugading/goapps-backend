package chat

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/chat"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/crypto"
)

// CreateGroupHandler creates a new group conversation.
type CreateGroupHandler struct {
	convRepo chat.ConversationRepository
	enc      *crypto.Encryptor
}

// NewCreateGroupHandler constructs the handler.
func NewCreateGroupHandler(convRepo chat.ConversationRepository, enc *crypto.Encryptor) *CreateGroupHandler {
	return &CreateGroupHandler{convRepo: convRepo, enc: enc}
}

// Handle creates a group conversation with the caller as OWNER.
func (h *CreateGroupHandler) Handle(ctx context.Context, ownerID uuid.UUID, name string, memberIDs []uuid.UUID) (*chat.Conversation, error) {
	convKeyPlain, err := h.enc.GenerateConversationKey()
	if err != nil {
		return nil, fmt.Errorf("create group: generate key: %w", err)
	}
	encKey, err := h.enc.EncryptConversationKey(convKeyPlain)
	if err != nil {
		return nil, fmt.Errorf("create group: encrypt key: %w", err)
	}
	conv, err := chat.NewGroupConversation(ownerID, name, memberIDs, convKeyPlain, encKey)
	if err != nil {
		return nil, fmt.Errorf("create group: build: %w", err)
	}
	if err := h.convRepo.Create(ctx, conv); err != nil {
		return nil, fmt.Errorf("create group: persist: %w", err)
	}
	return conv, nil
}
