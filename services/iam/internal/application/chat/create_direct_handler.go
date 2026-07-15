package chat

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/chat"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/crypto"
)

// CreateDirectHandler creates a 1:1 conversation (idempotent — returns existing if present).
type CreateDirectHandler struct {
	convRepo chat.ConversationRepository
	enc      *crypto.Encryptor
}

// NewCreateDirectHandler constructs the handler.
func NewCreateDirectHandler(convRepo chat.ConversationRepository, enc *crypto.Encryptor) *CreateDirectHandler {
	return &CreateDirectHandler{convRepo: convRepo, enc: enc}
}

// Handle creates or returns an existing DIRECT conversation between callerID and peerID.
func (h *CreateDirectHandler) Handle(ctx context.Context, callerID, peerID uuid.UUID) (*chat.Conversation, error) {
	existing, err := h.convRepo.FindDirect(ctx, callerID, peerID)
	if err != nil {
		return nil, fmt.Errorf("create direct: find existing: %w", err)
	}
	if existing != nil {
		return existing, nil
	}

	convKeyPlain, err := h.enc.GenerateConversationKey()
	if err != nil {
		return nil, fmt.Errorf("create direct: generate key: %w", err)
	}
	encKey, err := h.enc.EncryptConversationKey(convKeyPlain)
	if err != nil {
		return nil, fmt.Errorf("create direct: encrypt key: %w", err)
	}

	conv, err := chat.NewDirectConversation(callerID, peerID, convKeyPlain, encKey)
	if err != nil {
		return nil, fmt.Errorf("create direct: build: %w", err)
	}
	if err := h.convRepo.Create(ctx, conv); err != nil {
		return nil, fmt.Errorf("create direct: persist: %w", err)
	}
	return conv, nil
}
