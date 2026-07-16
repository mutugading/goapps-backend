package chat

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/chat"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/crypto"
)

// GetEditHistoryHandler returns the decrypted edit history for a message.
type GetEditHistoryHandler struct {
	convRepo chat.ConversationRepository
	msgRepo  chat.MessageRepository
	enc      *crypto.Encryptor
}

// NewGetEditHistoryHandler constructs the handler.
func NewGetEditHistoryHandler(convRepo chat.ConversationRepository, msgRepo chat.MessageRepository, enc *crypto.Encryptor) *GetEditHistoryHandler {
	return &GetEditHistoryHandler{convRepo: convRepo, msgRepo: msgRepo, enc: enc}
}

// EditHistoryEntryResult pairs an edit history entry with its decrypted body.
type EditHistoryEntryResult struct {
	*chat.EditHistoryEntry
	PlainBody string
}

// Handle returns the edit history for messageID, newest first. callerID must
// be an active participant of convID.
func (h *GetEditHistoryHandler) Handle(ctx context.Context, callerID, convID, messageID uuid.UUID) ([]*EditHistoryEntryResult, error) {
	conv, err := h.convRepo.GetByID(ctx, convID)
	if err != nil {
		return nil, err
	}
	p := conv.FindParticipant(callerID)
	if p == nil || !p.IsActive() {
		return nil, fmt.Errorf("get edit history: %w", chat.ErrNotParticipant)
	}

	entries, err := h.msgRepo.GetEditHistory(ctx, messageID)
	if err != nil {
		return nil, fmt.Errorf("get edit history: %w", err)
	}

	convKeyPlain, err := h.enc.DecryptConversationKey(conv.EncryptionKey())
	if err != nil {
		log.Warn().Err(err).Str("conv", convID.String()).Msg("get edit history: decrypt conv key failed")
		return nil, fmt.Errorf("get edit history: decrypt conv key: %w", err)
	}

	results := make([]*EditHistoryEntryResult, 0, len(entries))
	for _, entry := range entries {
		plain, decErr := h.enc.DecryptMessage(convKeyPlain, entry.BodyEncrypted())
		if decErr != nil {
			plain = decryptionErrorBody
		}
		results = append(results, &EditHistoryEntryResult{EditHistoryEntry: entry, PlainBody: plain})
	}
	return results, nil
}
