package chat

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ConversationRepository handles persistence for Conversation aggregates.
type ConversationRepository interface {
	// Create inserts a new conversation with its initial participants.
	Create(ctx context.Context, conv *Conversation) error

	// GetByID returns a conversation with all active participants.
	GetByID(ctx context.Context, id uuid.UUID) (*Conversation, error)

	// FindDirect finds an existing DIRECT conversation between two users.
	FindDirect(ctx context.Context, userA, userB uuid.UUID) (*Conversation, error)

	// ListByUserID returns conversations the user participates in, ordered by last message.
	ListByUserID(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]*Conversation, int64, error)

	// UpdateGroup persists name and avatar changes.
	UpdateGroup(ctx context.Context, conv *Conversation) error

	// AddParticipants inserts new participant rows.
	AddParticipants(ctx context.Context, conversationID uuid.UUID, participants []*Participant) error

	// RemoveParticipant sets left_at for a participant.
	RemoveParticipant(ctx context.Context, conversationID, userID uuid.UUID) error

	// UpdateLastReadAt updates chat_participant.last_read_at for a user.
	UpdateLastReadAt(ctx context.Context, conversationID, userID uuid.UUID, at time.Time) error

	// GetUnreadCounts returns, for each conversation ID, the count of messages
	// created after userID's last_read_at (excluding userID's own messages).
	GetUnreadCounts(ctx context.Context, conversationIDs []uuid.UUID, userID uuid.UUID) (map[uuid.UUID]int32, error)

	// ClearHistory sets history_cleared_at = NOW() for userID's participant row,
	// hiding all prior messages from that user's view only. Other participants
	// are unaffected.
	ClearHistory(ctx context.Context, conversationID, userID uuid.UUID) error
}

// MessageRepository handles persistence for Message aggregates.
type MessageRepository interface {
	// Create inserts a new message.
	Create(ctx context.Context, msg *Message) error

	// GetByID returns a message with its read receipts.
	GetByID(ctx context.Context, id uuid.UUID) (*Message, error)

	// ListByConversation returns messages using cursor-based pagination (newest first).
	// If afterTime is non-nil, only messages created strictly after it are returned —
	// used to hide history a participant has cleared from their own view.
	ListByConversation(ctx context.Context, conversationID uuid.UUID, pageSize int, beforeCursor string, afterTime *time.Time) ([]*Message, string, bool, error)

	// UpdateBody persists body changes after an edit.
	UpdateBody(ctx context.Context, msg *Message) error

	// MarkDeleted sets is_deleted to true.
	MarkDeleted(ctx context.Context, id uuid.UUID) error

	// SaveEditHistory inserts a pre-edit snapshot.
	SaveEditHistory(ctx context.Context, entry *EditHistoryEntry) error

	// GetEditHistory returns all edit history for a message, newest first.
	GetEditHistory(ctx context.Context, messageID uuid.UUID) ([]*EditHistoryEntry, error)

	// GetLastMessages returns, for each conversation ID, viewerID's most recent
	// visible non-deleted message (one query, not N+1). Messages the viewer has
	// cleared from their own history (created at or before their
	// history_cleared_at) are excluded so the conversation preview matches the
	// thread view.
	GetLastMessages(ctx context.Context, conversationIDs []uuid.UUID, viewerID uuid.UUID) (map[uuid.UUID]*Message, error)
}

// AttachmentRepository handles persistence for chat attachments.
type AttachmentRepository interface {
	// Create inserts a new attachment.
	Create(ctx context.Context, a *Attachment) error

	// GetByIDs returns attachments matching the given IDs (order not guaranteed).
	GetByIDs(ctx context.Context, ids []uuid.UUID) ([]*Attachment, error)

	// LinkToMessage sets message_id for the given attachment IDs.
	LinkToMessage(ctx context.Context, messageID uuid.UUID, attachmentIDs []uuid.UUID) error

	// ListByMessageIDs returns, for each message ID, its attachments (one query, not N+1).
	ListByMessageIDs(ctx context.Context, messageIDs []uuid.UUID) (map[uuid.UUID][]*Attachment, error)
}

// ReadReceiptRepository handles read receipts.
type ReadReceiptRepository interface {
	// Upsert inserts or ignores a read receipt (idempotent).
	Upsert(ctx context.Context, messageID, userID uuid.UUID) error

	// ListByMessage returns all receipts for a message.
	ListByMessage(ctx context.Context, messageID uuid.UUID) ([]*ReadReceipt, error)

	// UpsertBulk marks multiple messages as read for a user.
	UpsertBulk(ctx context.Context, messageIDs []uuid.UUID, userID uuid.UUID) error
}
