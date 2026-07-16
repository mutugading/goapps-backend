package chat

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/chat"
	storageinfra "github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/storage"
)

// UploadAttachmentHandler uploads a file and records an unlinked attachment.
type UploadAttachmentHandler struct {
	convRepo chat.ConversationRepository
	attRepo  chat.AttachmentRepository
	storage  storageinfra.Service
}

// NewUploadAttachmentHandler constructs the handler.
func NewUploadAttachmentHandler(convRepo chat.ConversationRepository, attRepo chat.AttachmentRepository, storage storageinfra.Service) *UploadAttachmentHandler {
	return &UploadAttachmentHandler{convRepo: convRepo, attRepo: attRepo, storage: storage}
}

// Handle validates participation, uploads the file, and persists an attachment
// that is not yet linked to any message.
func (h *UploadAttachmentHandler) Handle(ctx context.Context, uploaderID, convID uuid.UUID, fileName, contentType string, data []byte) (*chat.Attachment, error) {
	conv, err := h.convRepo.GetByID(ctx, convID)
	if err != nil {
		return nil, err
	}
	p := conv.FindParticipant(uploaderID)
	if p == nil || !p.IsActive() {
		return nil, fmt.Errorf("upload attachment: %w", chat.ErrNotParticipant)
	}
	if h.storage == nil {
		return nil, fmt.Errorf("upload attachment: %w", chat.ErrStorageUnavailable)
	}

	fileURL, err := h.storage.UploadChatAttachment(ctx, convID.String(), fileName, bytes.NewReader(data), int64(len(data)), contentType)
	if err != nil {
		return nil, fmt.Errorf("upload attachment: store file: %w", err)
	}

	thumbnailURL := ""
	if strings.HasPrefix(contentType, "image/") {
		thumbnailURL = fileURL
	}

	att := chat.NewAttachment(convID, uploaderID, fileName, fileURL, contentType, int64(len(data)), thumbnailURL)
	if err := h.attRepo.Create(ctx, att); err != nil {
		return nil, fmt.Errorf("upload attachment: persist: %w", err)
	}
	return att, nil
}
