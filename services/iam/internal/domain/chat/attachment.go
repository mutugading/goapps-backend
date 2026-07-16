// Package chat contains the domain model for the chat feature: conversations,
// messages, read receipts, and attachments.
package chat

import (
	"time"

	"github.com/google/uuid"
)

// Attachment is the aggregate root for a chat file or image attachment.
type Attachment struct {
	attachmentID   uuid.UUID
	conversationID uuid.UUID
	messageID      *uuid.UUID
	uploaderUserID uuid.UUID
	fileName       string
	fileURL        string
	contentType    string
	fileSize       int64
	thumbnailURL   string
	createdAt      time.Time
}

// NewAttachment creates a new attachment not yet linked to a message.
func NewAttachment(convID, uploaderID uuid.UUID, fileName, fileURL, contentType string, fileSize int64, thumbnailURL string) *Attachment {
	return &Attachment{
		attachmentID:   uuid.New(),
		conversationID: convID,
		messageID:      nil,
		uploaderUserID: uploaderID,
		fileName:       fileName,
		fileURL:        fileURL,
		contentType:    contentType,
		fileSize:       fileSize,
		thumbnailURL:   thumbnailURL,
		createdAt:      time.Now().UTC(),
	}
}

// ReconstructAttachment rebuilds an Attachment from persistence.
func ReconstructAttachment(
	attachmentID, conversationID uuid.UUID, messageID *uuid.UUID, uploaderUserID uuid.UUID,
	fileName, fileURL, contentType string, fileSize int64, thumbnailURL string, createdAt time.Time,
) *Attachment {
	return &Attachment{
		attachmentID:   attachmentID,
		conversationID: conversationID,
		messageID:      messageID,
		uploaderUserID: uploaderUserID,
		fileName:       fileName,
		fileURL:        fileURL,
		contentType:    contentType,
		fileSize:       fileSize,
		thumbnailURL:   thumbnailURL,
		createdAt:      createdAt,
	}
}

// AttachmentID returns the attachment ID.
func (a *Attachment) AttachmentID() uuid.UUID { return a.attachmentID }

// ConversationID returns the conversation ID this attachment belongs to.
func (a *Attachment) ConversationID() uuid.UUID { return a.conversationID }

// MessageID returns the linked message ID, or nil if not yet linked.
func (a *Attachment) MessageID() *uuid.UUID { return a.messageID }

// UploaderUserID returns the ID of the user who uploaded the attachment.
func (a *Attachment) UploaderUserID() uuid.UUID { return a.uploaderUserID }

// FileName returns the original file name.
func (a *Attachment) FileName() string { return a.fileName }

// FileURL returns the stored file URL.
func (a *Attachment) FileURL() string { return a.fileURL }

// ContentType returns the MIME content type.
func (a *Attachment) ContentType() string { return a.contentType }

// FileSize returns the file size in bytes.
func (a *Attachment) FileSize() int64 { return a.fileSize }

// ThumbnailURL returns the thumbnail URL, or an empty string if none.
func (a *Attachment) ThumbnailURL() string { return a.thumbnailURL }

// CreatedAt returns the creation timestamp.
func (a *Attachment) CreatedAt() time.Time { return a.createdAt }

// LinkToMessage links the attachment to a persisted message.
func (a *Attachment) LinkToMessage(msgID uuid.UUID) {
	a.messageID = &msgID
}
