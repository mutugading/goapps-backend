package postgres

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/chat"
)

// ChatMessageRepository implements chat.MessageRepository.
type ChatMessageRepository struct {
	db *DB
}

// NewChatMessageRepository constructs the repo.
func NewChatMessageRepository(db *DB) *ChatMessageRepository {
	return &ChatMessageRepository{db: db}
}

// Create inserts a new message.
func (r *ChatMessageRepository) Create(ctx context.Context, msg *chat.Message) error {
	var replyToID *uuid.UUID
	if msg.ReplyToID() != uuid.Nil {
		id := msg.ReplyToID()
		replyToID = &id
	}
	const q = `
		INSERT INTO chat_message
		  (message_id, conversation_id, sender_user_id,
		   body_encrypted, body_plain_encrypted,
		   is_edited, is_deleted, reply_to_id, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`
	if _, err := r.db.ExecContext(ctx, q,
		msg.MessageID(), msg.ConversationID(), msg.SenderUserID(),
		msg.BodyEncrypted(), msg.BodyPlainEncrypted(),
		msg.IsEdited(), msg.IsDeleted(), replyToID,
		msg.CreatedAt(), msg.UpdatedAt(),
	); err != nil {
		return fmt.Errorf("chat msg repo: create: %w", err)
	}
	return nil
}

// GetByID returns a message with its read receipts.
func (r *ChatMessageRepository) GetByID(ctx context.Context, id uuid.UUID) (*chat.Message, error) {
	const q = `
		SELECT message_id, conversation_id, sender_user_id,
		       body_encrypted, body_plain_encrypted,
		       is_edited, is_deleted, reply_to_id, created_at, updated_at
		FROM chat_message WHERE message_id = $1`

	msg, err := r.scanMessage(r.db.QueryRowContext(ctx, q, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, chat.ErrMessageNotFound
		}
		return nil, fmt.Errorf("chat msg repo: get by id: %w", err)
	}
	receipts, err := r.loadReceipts(ctx, id)
	if err != nil {
		return nil, err
	}
	return chat.ReconstructMessage(
		msg.MessageID(), msg.ConversationID(), msg.SenderUserID(),
		msg.BodyEncrypted(), msg.BodyPlainEncrypted(),
		msg.IsEdited(), msg.IsDeleted(), msg.ReplyToID(),
		receipts, msg.CreatedAt(), msg.UpdatedAt(),
	), nil
}

// ListByConversation returns messages using cursor-based pagination (newest first).
// If afterTime is non-nil, only messages created strictly after it are included —
// used to hide history the caller has cleared from their own view.
func (r *ChatMessageRepository) ListByConversation(ctx context.Context, convID uuid.UUID, pageSize int, beforeCursor string, afterTime *time.Time) ([]*chat.Message, string, bool, error) {
	fetchSize := pageSize + 1

	rows, err := r.queryMessagePage(ctx, convID, fetchSize, beforeCursor, afterTime)
	if err != nil {
		return nil, "", false, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("chat msg repo: close rows")
		}
	}()

	var msgs []*chat.Message
	for rows.Next() {
		msg, scanErr := r.scanMessageRows(rows)
		if scanErr != nil {
			return nil, "", false, fmt.Errorf("chat msg repo: scan: %w", scanErr)
		}
		msgs = append(msgs, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, "", false, fmt.Errorf("chat msg repo: rows err: %w", err)
	}

	hasMore := len(msgs) > pageSize
	if hasMore {
		msgs = msgs[:pageSize]
	}

	if err := r.attachReceipts(ctx, msgs); err != nil {
		return nil, "", false, err
	}

	nextCursor := ""
	if hasMore && len(msgs) > 0 {
		last := msgs[len(msgs)-1]
		nextCursor = encodeChatCursor(last.CreatedAt(), last.MessageID())
	}
	return msgs, nextCursor, hasMore, nil
}

const chatMessageSelectCols = `
	SELECT message_id, conversation_id, sender_user_id,
	       body_encrypted, body_plain_encrypted,
	       is_edited, is_deleted, reply_to_id, created_at, updated_at
	FROM chat_message
	WHERE conversation_id = $1 AND is_deleted = FALSE`

// queryMessagePage dispatches to the first-page or cursor-paginated query,
// each optionally bounded by afterTime (the caller's cleared-history cutoff).
func (r *ChatMessageRepository) queryMessagePage(ctx context.Context, convID uuid.UUID, fetchSize int, beforeCursor string, afterTime *time.Time) (*sql.Rows, error) {
	if beforeCursor == "" {
		return r.queryFirstPage(ctx, convID, fetchSize, afterTime)
	}
	cursorTime, cursorID, parseErr := decodeChatCursor(beforeCursor)
	if parseErr != nil {
		return nil, fmt.Errorf("chat msg repo: invalid cursor: %w", parseErr)
	}
	return r.queryCursorPage(ctx, convID, fetchSize, cursorTime, cursorID, afterTime)
}

func (r *ChatMessageRepository) queryFirstPage(ctx context.Context, convID uuid.UUID, fetchSize int, afterTime *time.Time) (*sql.Rows, error) {
	if afterTime != nil {
		return r.db.QueryContext(ctx, chatMessageSelectCols+`
		  AND created_at > $2
		ORDER BY created_at DESC, message_id DESC
		LIMIT $3`, convID, *afterTime, fetchSize)
	}
	return r.db.QueryContext(ctx, chatMessageSelectCols+`
	ORDER BY created_at DESC, message_id DESC
	LIMIT $2`, convID, fetchSize)
}

func (r *ChatMessageRepository) queryCursorPage(ctx context.Context, convID uuid.UUID, fetchSize int, cursorTime time.Time, cursorID uuid.UUID, afterTime *time.Time) (*sql.Rows, error) {
	const cursorFilter = ` AND (created_at, message_id) < ($2, $3)`
	if afterTime != nil {
		return r.db.QueryContext(ctx, chatMessageSelectCols+cursorFilter+`
		  AND created_at > $4
		ORDER BY created_at DESC, message_id DESC
		LIMIT $5`, convID, cursorTime, cursorID, *afterTime, fetchSize)
	}
	return r.db.QueryContext(ctx, chatMessageSelectCols+cursorFilter+`
	ORDER BY created_at DESC, message_id DESC
	LIMIT $4`, convID, cursorTime, cursorID, fetchSize)
}

// attachReceipts batch-loads read receipts for all given messages (one query,
// not N+1) and reconstructs each message with its receipts attached.
func (r *ChatMessageRepository) attachReceipts(ctx context.Context, msgs []*chat.Message) error {
	if len(msgs) == 0 {
		return nil
	}
	placeholders := make([]string, len(msgs))
	args := make([]any, len(msgs))
	for i, msg := range msgs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = msg.MessageID()
	}

	q := fmt.Sprintf(
		`SELECT message_id, user_id, read_at FROM chat_read_receipt WHERE message_id IN (%s)`,
		strings.Join(placeholders, ","),
	)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("chat msg repo: batch load receipts: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("chat msg repo: close batch receipt rows")
		}
	}()

	receiptsByMsg := make(map[uuid.UUID][]*chat.ReadReceipt, len(msgs))
	for rows.Next() {
		var (
			mID, uID uuid.UUID
			readAt   time.Time
		)
		if err := rows.Scan(&mID, &uID, &readAt); err != nil {
			return fmt.Errorf("chat msg repo: scan batch receipt: %w", err)
		}
		receiptsByMsg[mID] = append(receiptsByMsg[mID], chat.NewReadReceipt(mID, uID, readAt))
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("chat msg repo: batch receipt rows err: %w", err)
	}

	for i, msg := range msgs {
		msgs[i] = chat.ReconstructMessage(
			msg.MessageID(), msg.ConversationID(), msg.SenderUserID(),
			msg.BodyEncrypted(), msg.BodyPlainEncrypted(),
			msg.IsEdited(), msg.IsDeleted(), msg.ReplyToID(),
			receiptsByMsg[msg.MessageID()], msg.CreatedAt(), msg.UpdatedAt(),
		)
	}
	return nil
}

// UpdateBody persists body changes after an edit.
func (r *ChatMessageRepository) UpdateBody(ctx context.Context, msg *chat.Message) error {
	const q = `
		UPDATE chat_message
		SET body_encrypted=$1, body_plain_encrypted=$2, is_edited=TRUE, updated_at=NOW()
		WHERE message_id=$3`
	if _, err := r.db.ExecContext(ctx, q,
		msg.BodyEncrypted(), msg.BodyPlainEncrypted(), msg.MessageID(),
	); err != nil {
		return fmt.Errorf("chat msg repo: update body: %w", err)
	}
	return nil
}

// MarkDeleted sets is_deleted to true.
func (r *ChatMessageRepository) MarkDeleted(ctx context.Context, id uuid.UUID) error {
	const q = `UPDATE chat_message SET is_deleted=TRUE, updated_at=NOW() WHERE message_id=$1`
	if _, err := r.db.ExecContext(ctx, q, id); err != nil {
		return fmt.Errorf("chat msg repo: mark deleted: %w", err)
	}
	return nil
}

// SaveEditHistory inserts a pre-edit snapshot.
func (r *ChatMessageRepository) SaveEditHistory(ctx context.Context, entry *chat.EditHistoryEntry) error {
	const q = `
		INSERT INTO chat_message_edit_history (message_id, body_encrypted, edited_by, edited_at)
		VALUES ($1,$2,$3,$4)`
	if _, err := r.db.ExecContext(ctx, q,
		entry.MessageID(), entry.BodyEncrypted(), entry.EditedBy(), entry.EditedAt(),
	); err != nil {
		return fmt.Errorf("chat msg repo: save edit history: %w", err)
	}
	return nil
}

// GetEditHistory returns all edit history for a message, newest first.
func (r *ChatMessageRepository) GetEditHistory(ctx context.Context, messageID uuid.UUID) ([]*chat.EditHistoryEntry, error) {
	const q = `
		SELECT history_id, message_id, body_encrypted, edited_by, edited_at
		FROM chat_message_edit_history
		WHERE message_id=$1 ORDER BY edited_at DESC`

	rows, err := r.db.QueryContext(ctx, q, messageID)
	if err != nil {
		return nil, fmt.Errorf("chat msg repo: get edit history: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("chat msg repo: close rows")
		}
	}()

	var entries []*chat.EditHistoryEntry
	for rows.Next() {
		var (
			histID          int64
			msgID, editedBy uuid.UUID
			bodyEnc         []byte
			editedAt        time.Time
		)
		if err := rows.Scan(&histID, &msgID, &bodyEnc, &editedBy, &editedAt); err != nil {
			return nil, fmt.Errorf("chat msg repo: scan edit history: %w", err)
		}
		entries = append(entries, chat.ReconstructEditHistory(histID, msgID, bodyEnc, editedBy, editedAt))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("chat msg repo: edit history rows err: %w", err)
	}
	return entries, nil
}

// GetLastMessages returns, for each conversation ID, its most recent
// non-deleted message. Single query via DISTINCT ON — avoids N+1.
func (r *ChatMessageRepository) GetLastMessages(ctx context.Context, convIDs []uuid.UUID, viewerID uuid.UUID) (map[uuid.UUID]*chat.Message, error) {
	if len(convIDs) == 0 {
		return map[uuid.UUID]*chat.Message{}, nil
	}
	placeholders := make([]string, len(convIDs))
	args := make([]any, 0, len(convIDs)+1)
	for i, id := range convIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args = append(args, id)
	}
	viewerParam := len(convIDs) + 1
	args = append(args, viewerID)

	// Join the viewer's participant row so a cleared history hides messages
	// created at or before their history_cleared_at, matching the thread view.
	q := fmt.Sprintf(`
		SELECT DISTINCT ON (cm.conversation_id)
		       cm.message_id, cm.conversation_id, cm.sender_user_id,
		       cm.body_encrypted, cm.body_plain_encrypted,
		       cm.is_edited, cm.is_deleted, cm.reply_to_id, cm.created_at, cm.updated_at
		FROM chat_message cm
		JOIN chat_participant cp
		  ON cp.conversation_id = cm.conversation_id AND cp.user_id = $%d
		WHERE cm.conversation_id IN (%s) AND cm.is_deleted = FALSE
		  AND (cp.history_cleared_at IS NULL OR cm.created_at > cp.history_cleared_at)
		ORDER BY cm.conversation_id, cm.created_at DESC, cm.message_id DESC`,
		viewerParam, strings.Join(placeholders, ","))

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("chat msg repo: get last messages: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("chat msg repo: close last message rows")
		}
	}()

	result := make(map[uuid.UUID]*chat.Message, len(convIDs))
	for rows.Next() {
		msg, scanErr := r.scanMessageRows(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("chat msg repo: scan last message: %w", scanErr)
		}
		result[msg.ConversationID()] = msg
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("chat msg repo: last message rows err: %w", err)
	}
	return result, nil
}

func (r *ChatMessageRepository) loadReceipts(ctx context.Context, msgID uuid.UUID) ([]*chat.ReadReceipt, error) {
	const q = `SELECT message_id, user_id, read_at FROM chat_read_receipt WHERE message_id=$1`
	rows, err := r.db.QueryContext(ctx, q, msgID)
	if err != nil {
		return nil, fmt.Errorf("chat msg repo: load receipts: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("chat msg repo: close rows")
		}
	}()
	var receipts []*chat.ReadReceipt
	for rows.Next() {
		var (
			mID, uID uuid.UUID
			readAt   time.Time
		)
		if err := rows.Scan(&mID, &uID, &readAt); err != nil {
			return nil, fmt.Errorf("chat msg repo: scan receipt: %w", err)
		}
		receipts = append(receipts, chat.NewReadReceipt(mID, uID, readAt))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("chat msg repo: receipt rows err: %w", err)
	}
	return receipts, nil
}

func (r *ChatMessageRepository) scanMessage(row *sql.Row) (*chat.Message, error) {
	var (
		msgID, convID, senderID uuid.UUID
		bodyEnc, bodyPlainEnc   []byte
		isEdited, isDeleted     bool
		replyToID               *uuid.UUID
		createdAt, updatedAt    time.Time
	)
	if err := row.Scan(&msgID, &convID, &senderID, &bodyEnc, &bodyPlainEnc,
		&isEdited, &isDeleted, &replyToID, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	replyTo := uuid.Nil
	if replyToID != nil {
		replyTo = *replyToID
	}
	return chat.ReconstructMessage(msgID, convID, senderID, bodyEnc, bodyPlainEnc,
		isEdited, isDeleted, replyTo, nil, createdAt, updatedAt), nil
}

func (r *ChatMessageRepository) scanMessageRows(rows *sql.Rows) (*chat.Message, error) {
	var (
		msgID, convID, senderID uuid.UUID
		bodyEnc, bodyPlainEnc   []byte
		isEdited, isDeleted     bool
		replyToID               *uuid.UUID
		createdAt, updatedAt    time.Time
	)
	if err := rows.Scan(&msgID, &convID, &senderID, &bodyEnc, &bodyPlainEnc,
		&isEdited, &isDeleted, &replyToID, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	replyTo := uuid.Nil
	if replyToID != nil {
		replyTo = *replyToID
	}
	return chat.ReconstructMessage(msgID, convID, senderID, bodyEnc, bodyPlainEnc,
		isEdited, isDeleted, replyTo, nil, createdAt, updatedAt), nil
}

// ChatReadReceiptRepository implements chat.ReadReceiptRepository.
type ChatReadReceiptRepository struct {
	db *DB
}

// NewChatReadReceiptRepository constructs the repo.
func NewChatReadReceiptRepository(db *DB) *ChatReadReceiptRepository {
	return &ChatReadReceiptRepository{db: db}
}

// Upsert inserts or ignores a read receipt (idempotent).
func (r *ChatReadReceiptRepository) Upsert(ctx context.Context, messageID, userID uuid.UUID) error {
	const q = `INSERT INTO chat_read_receipt (message_id, user_id, read_at) VALUES ($1,$2,NOW()) ON CONFLICT DO NOTHING`
	if _, err := r.db.ExecContext(ctx, q, messageID, userID); err != nil {
		return fmt.Errorf("chat receipt repo: upsert: %w", err)
	}
	return nil
}

// ListByMessage returns all receipts for a message.
func (r *ChatReadReceiptRepository) ListByMessage(ctx context.Context, msgID uuid.UUID) ([]*chat.ReadReceipt, error) {
	const q = `SELECT message_id, user_id, read_at FROM chat_read_receipt WHERE message_id=$1`
	rows, err := r.db.QueryContext(ctx, q, msgID)
	if err != nil {
		return nil, fmt.Errorf("chat receipt repo: list: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("chat msg repo: close rows")
		}
	}()
	var receipts []*chat.ReadReceipt
	for rows.Next() {
		var (
			mID, uID uuid.UUID
			readAt   time.Time
		)
		if err := rows.Scan(&mID, &uID, &readAt); err != nil {
			return nil, fmt.Errorf("chat receipt repo: scan: %w", err)
		}
		receipts = append(receipts, chat.NewReadReceipt(mID, uID, readAt))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("chat receipt repo: rows err: %w", err)
	}
	return receipts, nil
}

// UpsertBulk marks multiple messages as read for a user.
func (r *ChatReadReceiptRepository) UpsertBulk(ctx context.Context, messageIDs []uuid.UUID, userID uuid.UUID) error {
	for _, msgID := range messageIDs {
		if err := r.Upsert(ctx, msgID, userID); err != nil {
			return err
		}
	}
	return nil
}

func encodeChatCursor(createdAt time.Time, messageID uuid.UUID) string {
	raw := createdAt.UTC().Format(time.RFC3339Nano) + "|" + messageID.String()
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

func decodeChatCursor(cursor string) (time.Time, uuid.UUID, error) {
	b, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, uuid.Nil, fmt.Errorf("decode base64: %w", err)
	}
	parts := strings.SplitN(string(b), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, uuid.Nil, fmt.Errorf("invalid cursor format")
	}
	t, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, uuid.Nil, fmt.Errorf("parse time: %w", err)
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return time.Time{}, uuid.Nil, fmt.Errorf("parse uuid: %w", err)
	}
	return t, id, nil
}
