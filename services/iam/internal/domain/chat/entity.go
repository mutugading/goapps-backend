package chat

import (
	"time"

	"github.com/google/uuid"
)

// Participant is a member of a conversation.
type Participant struct {
	conversationID uuid.UUID
	userID         uuid.UUID
	role           Role
	joinedAt       time.Time
	leftAt         *time.Time
	lastReadAt     *time.Time
}

// ReconstructParticipant rebuilds a Participant from persistence.
func ReconstructParticipant(conversationID, userID uuid.UUID, role Role, joinedAt time.Time, leftAt, lastReadAt *time.Time) *Participant {
	return &Participant{
		conversationID: conversationID, userID: userID, role: role,
		joinedAt: joinedAt, leftAt: leftAt, lastReadAt: lastReadAt,
	}
}

// UserID returns the participant user ID.
func (p *Participant) UserID() uuid.UUID { return p.userID }

// Role returns the participant role.
func (p *Participant) Role() Role { return p.role }

// JoinedAt returns when the participant joined.
func (p *Participant) JoinedAt() time.Time { return p.joinedAt }

// LeftAt returns when the participant left, or nil.
func (p *Participant) LeftAt() *time.Time { return p.leftAt }

// LastReadAt returns the last read timestamp, or nil.
func (p *Participant) LastReadAt() *time.Time { return p.lastReadAt }

// IsActive returns true if the participant has not left.
func (p *Participant) IsActive() bool { return p.leftAt == nil }

// Conversation is the aggregate root for a chat conversation.
type Conversation struct {
	id            uuid.UUID
	convType      Type
	name          string
	avatarURL     string
	encryptionKey []byte
	convKeyPlain  []byte
	participants  []*Participant
	createdBy     string
	createdAt     time.Time
	updatedAt     time.Time
	deletedAt     *time.Time
}

// NewDirectConversation creates a 1:1 conversation.
func NewDirectConversation(ownerID, peerID uuid.UUID, convKeyPlain, encryptionKey []byte) (*Conversation, error) {
	now := time.Now().UTC()
	conv := &Conversation{
		id:            uuid.New(),
		convType:      TypeDirect,
		encryptionKey: encryptionKey,
		convKeyPlain:  convKeyPlain,
		createdBy:     ownerID.String(),
		createdAt:     now,
		updatedAt:     now,
	}
	conv.participants = []*Participant{
		{conversationID: conv.id, userID: ownerID, role: RoleOwner, joinedAt: now},
		{conversationID: conv.id, userID: peerID, role: RoleMember, joinedAt: now},
	}
	return conv, nil
}

// NewGroupConversation creates a group conversation.
func NewGroupConversation(ownerID uuid.UUID, name string, memberIDs []uuid.UUID, convKeyPlain, encryptionKey []byte) (*Conversation, error) {
	now := time.Now().UTC()
	conv := &Conversation{
		id:            uuid.New(),
		convType:      TypeGroup,
		name:          name,
		encryptionKey: encryptionKey,
		convKeyPlain:  convKeyPlain,
		createdBy:     ownerID.String(),
		createdAt:     now,
		updatedAt:     now,
	}
	conv.participants = append(conv.participants,
		&Participant{conversationID: conv.id, userID: ownerID, role: RoleOwner, joinedAt: now},
	)
	for _, mid := range memberIDs {
		conv.participants = append(conv.participants,
			&Participant{conversationID: conv.id, userID: mid, role: RoleMember, joinedAt: now},
		)
	}
	return conv, nil
}

// Reconstruct rebuilds a Conversation from persistence.
func Reconstruct(
	id uuid.UUID, convType Type, name, avatarURL string,
	encryptionKey []byte, createdBy string,
	createdAt, updatedAt time.Time, deletedAt *time.Time,
	participants []*Participant,
) *Conversation {
	return &Conversation{
		id: id, convType: convType, name: name, avatarURL: avatarURL,
		encryptionKey: encryptionKey, createdBy: createdBy,
		createdAt: createdAt, updatedAt: updatedAt, deletedAt: deletedAt,
		participants: participants,
	}
}

// ID returns the conversation ID.
func (c *Conversation) ID() uuid.UUID { return c.id }

// Type returns the conversation type.
func (c *Conversation) Type() Type { return c.convType }

// Name returns the conversation name.
func (c *Conversation) Name() string { return c.name }

// AvatarURL returns the conversation avatar URL.
func (c *Conversation) AvatarURL() string { return c.avatarURL }

// EncryptionKey returns the encrypted conversation key bytes.
func (c *Conversation) EncryptionKey() []byte { return c.encryptionKey }

// ConvKeyPlain returns the decrypted conversation key.
func (c *Conversation) ConvKeyPlain() []byte { return c.convKeyPlain }

// CreatedBy returns the creator identifier.
func (c *Conversation) CreatedBy() string { return c.createdBy }

// CreatedAt returns the creation timestamp.
func (c *Conversation) CreatedAt() time.Time { return c.createdAt }

// UpdatedAt returns the last update timestamp.
func (c *Conversation) UpdatedAt() time.Time { return c.updatedAt }

// DeletedAt returns the deletion timestamp, or nil.
func (c *Conversation) DeletedAt() *time.Time { return c.deletedAt }

// Participants returns all participants.
func (c *Conversation) Participants() []*Participant { return c.participants }

// SetConvKeyPlain sets the decrypted conversation key.
func (c *Conversation) SetConvKeyPlain(key []byte) { c.convKeyPlain = key }

// ActiveParticipantCount returns the number of non-left participants.
func (c *Conversation) ActiveParticipantCount() int {
	count := 0
	for _, p := range c.participants {
		if p.IsActive() {
			count++
		}
	}
	return count
}

// FindParticipant returns the participant with the given userID, or nil.
func (c *Conversation) FindParticipant(userID uuid.UUID) *Participant {
	for _, p := range c.participants {
		if p.userID == userID {
			return p
		}
	}
	return nil
}

// AddParticipant adds a new active participant.
func (c *Conversation) AddParticipant(userID uuid.UUID, role Role) error {
	if c.convType == TypeDirect && c.ActiveParticipantCount() >= 2 {
		return ErrDirectConversationFull
	}
	for _, p := range c.participants {
		if p.userID == userID && p.IsActive() {
			return ErrAlreadyParticipant
		}
	}
	now := time.Now().UTC()
	c.participants = append(c.participants, &Participant{
		conversationID: c.id, userID: userID, role: role, joinedAt: now,
	})
	c.updatedAt = now
	return nil
}

// UpdateGroup updates mutable group fields.
func (c *Conversation) UpdateGroup(name, avatarURL string) {
	c.name = name
	c.avatarURL = avatarURL
	c.updatedAt = time.Now().UTC()
}

// EditHistoryEntry is an immutable snapshot of a message before an edit.
type EditHistoryEntry struct {
	historyID     int64
	messageID     uuid.UUID
	bodyEncrypted []byte
	editedBy      uuid.UUID
	editedAt      time.Time
}

// ReconstructEditHistory rebuilds an edit history entry from persistence.
func ReconstructEditHistory(historyID int64, messageID uuid.UUID, bodyEncrypted []byte, editedBy uuid.UUID, editedAt time.Time) *EditHistoryEntry {
	return &EditHistoryEntry{historyID: historyID, messageID: messageID, bodyEncrypted: bodyEncrypted, editedBy: editedBy, editedAt: editedAt}
}

// HistoryID returns the history entry ID.
func (e *EditHistoryEntry) HistoryID() int64 { return e.historyID }

// MessageID returns the message this entry belongs to.
func (e *EditHistoryEntry) MessageID() uuid.UUID { return e.messageID }

// BodyEncrypted returns the encrypted pre-edit body.
func (e *EditHistoryEntry) BodyEncrypted() []byte { return e.bodyEncrypted }

// EditedBy returns the user who made the edit.
func (e *EditHistoryEntry) EditedBy() uuid.UUID { return e.editedBy }

// EditedAt returns when the edit was made.
func (e *EditHistoryEntry) EditedAt() time.Time { return e.editedAt }

// ReadReceipt records that a user has read a message.
type ReadReceipt struct {
	messageID uuid.UUID
	userID    uuid.UUID
	readAt    time.Time
}

// MessageID returns the message ID.
func (r *ReadReceipt) MessageID() uuid.UUID { return r.messageID }

// UserID returns the user ID.
func (r *ReadReceipt) UserID() uuid.UUID { return r.userID }

// ReadAt returns the read timestamp.
func (r *ReadReceipt) ReadAt() time.Time { return r.readAt }

// NewReadReceipt constructs a ReadReceipt from persistence data.
func NewReadReceipt(messageID, userID uuid.UUID, readAt time.Time) *ReadReceipt {
	return &ReadReceipt{messageID: messageID, userID: userID, readAt: readAt}
}

// Message is the aggregate root for a chat message.
type Message struct {
	messageID          uuid.UUID
	conversationID     uuid.UUID
	senderUserID       uuid.UUID
	bodyEncrypted      []byte
	bodyPlainEncrypted []byte
	isEdited           bool
	isDeleted          bool
	replyToID          uuid.UUID
	readReceipts       []*ReadReceipt
	createdAt          time.Time
	updatedAt          time.Time
}

// NewMessage creates a new message with encrypted bodies.
func NewMessage(conversationID, senderID uuid.UUID, bodyEncrypted, bodyPlainEncrypted []byte, replyToID uuid.UUID) *Message {
	now := time.Now().UTC()
	return &Message{
		messageID: uuid.New(), conversationID: conversationID, senderUserID: senderID,
		bodyEncrypted: bodyEncrypted, bodyPlainEncrypted: bodyPlainEncrypted,
		replyToID: replyToID, createdAt: now, updatedAt: now,
	}
}

// ReconstructMessage rebuilds a Message from persistence.
func ReconstructMessage(
	messageID, conversationID, senderID uuid.UUID,
	bodyEncrypted, bodyPlainEncrypted []byte,
	isEdited, isDeleted bool, replyToID uuid.UUID,
	receipts []*ReadReceipt,
	createdAt, updatedAt time.Time,
) *Message {
	return &Message{
		messageID: messageID, conversationID: conversationID, senderUserID: senderID,
		bodyEncrypted: bodyEncrypted, bodyPlainEncrypted: bodyPlainEncrypted,
		isEdited: isEdited, isDeleted: isDeleted, replyToID: replyToID,
		readReceipts: receipts, createdAt: createdAt, updatedAt: updatedAt,
	}
}

// MessageID returns the message ID.
func (m *Message) MessageID() uuid.UUID { return m.messageID }

// ConversationID returns the conversation ID.
func (m *Message) ConversationID() uuid.UUID { return m.conversationID }

// SenderUserID returns the sender user ID.
func (m *Message) SenderUserID() uuid.UUID { return m.senderUserID }

// BodyEncrypted returns the encrypted message body.
func (m *Message) BodyEncrypted() []byte { return m.bodyEncrypted }

// BodyPlainEncrypted returns the encrypted plaintext body.
func (m *Message) BodyPlainEncrypted() []byte { return m.bodyPlainEncrypted }

// IsEdited returns true if the message has been edited.
func (m *Message) IsEdited() bool { return m.isEdited }

// IsDeleted returns true if the message has been soft-deleted.
func (m *Message) IsDeleted() bool { return m.isDeleted }

// ReplyToID returns the reply-to message ID, or uuid.Nil if none.
func (m *Message) ReplyToID() uuid.UUID { return m.replyToID }

// ReadReceipts returns all read receipts for this message.
func (m *Message) ReadReceipts() []*ReadReceipt { return m.readReceipts }

// CreatedAt returns the creation timestamp.
func (m *Message) CreatedAt() time.Time { return m.createdAt }

// UpdatedAt returns the last update timestamp.
func (m *Message) UpdatedAt() time.Time { return m.updatedAt }

// Edit replaces the body and returns the OLD bodyEncrypted for history.
func (m *Message) Edit(newBodyEncrypted, newBodyPlainEncrypted []byte) []byte {
	old := m.bodyEncrypted
	m.bodyEncrypted = newBodyEncrypted
	m.bodyPlainEncrypted = newBodyPlainEncrypted
	m.isEdited = true
	m.updatedAt = time.Now().UTC()
	return old
}

// SoftDelete marks the message as deleted.
func (m *Message) SoftDelete() {
	m.isDeleted = true
	m.updatedAt = time.Now().UTC()
}
