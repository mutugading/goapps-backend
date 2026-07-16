package chat

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/chat"
	chatinfra "github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/chat"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/crypto"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/postgres"
)

// EditMessageHandler handles message edits.
type EditMessageHandler struct {
	convRepo     chat.ConversationRepository
	msgRepo      chat.MessageRepository
	enc          *crypto.Encryptor
	broadcaster  *chatinfra.Broadcaster
	userResolver *postgres.ChatUserResolver
}

// NewEditMessageHandler constructs the handler.
func NewEditMessageHandler(convRepo chat.ConversationRepository, msgRepo chat.MessageRepository, enc *crypto.Encryptor, broadcaster *chatinfra.Broadcaster, userResolver *postgres.ChatUserResolver) *EditMessageHandler {
	return &EditMessageHandler{convRepo: convRepo, msgRepo: msgRepo, enc: enc, broadcaster: broadcaster, userResolver: userResolver}
}

// Handle edits a message. Only the author can edit; saves edit history.
func (h *EditMessageHandler) Handle(ctx context.Context, callerID, convID, msgID uuid.UUID, newBody string) (*chat.Message, error) {
	conv, err := h.convRepo.GetByID(ctx, convID)
	if err != nil {
		return nil, err
	}
	msg, err := h.msgRepo.GetByID(ctx, msgID)
	if err != nil {
		return nil, err
	}
	if msg.IsDeleted() {
		return nil, fmt.Errorf("edit message: %w", chat.ErrMessageDeleted)
	}
	if msg.SenderUserID() != callerID {
		p := conv.FindParticipant(callerID)
		if p == nil || !p.Role().IsAdminOrOwner() {
			return nil, fmt.Errorf("edit message: %w", chat.ErrNotAuthor)
		}
	}

	convKeyPlain, err := h.enc.DecryptConversationKey(conv.EncryptionKey())
	if err != nil {
		return nil, fmt.Errorf("edit message: decrypt conv key: %w", err)
	}

	// Save edit history snapshot before mutating.
	histEntry := chat.ReconstructEditHistory(0, msgID, msg.BodyEncrypted(), callerID, time.Now().UTC())
	if saveErr := h.msgRepo.SaveEditHistory(ctx, histEntry); saveErr != nil {
		log.Warn().Err(saveErr).Msg("edit message: save history")
	}

	newBodyEnc, err := h.enc.EncryptMessage(convKeyPlain, newBody)
	if err != nil {
		return nil, fmt.Errorf("edit message: encrypt new body: %w", err)
	}
	newBodyPlainEnc, err := h.enc.EncryptMessage(convKeyPlain, newBody)
	if err != nil {
		return nil, fmt.Errorf("edit message: encrypt plain: %w", err)
	}
	msg.Edit(newBodyEnc, newBodyPlainEnc)

	if err := h.msgRepo.UpdateBody(ctx, msg); err != nil {
		return nil, fmt.Errorf("edit message: update: %w", err)
	}
	senderName := resolveSenderName(ctx, h.userResolver, msg.SenderUserID())
	broadcastMessageEvent(h.broadcaster, conv, msg, newBody, "message_edited", senderName)
	return msg, nil
}
