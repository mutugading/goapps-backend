package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/chat"
)

// ChatAttachmentRepository implements chat.AttachmentRepository.
type ChatAttachmentRepository struct {
	db *DB
}

// NewChatAttachmentRepository constructs the repo.
func NewChatAttachmentRepository(db *DB) *ChatAttachmentRepository {
	return &ChatAttachmentRepository{db: db}
}

// Create inserts a new attachment.
func (r *ChatAttachmentRepository) Create(ctx context.Context, a *chat.Attachment) error {
	const q = `
		INSERT INTO chat_attachment
		  (attachment_id, conversation_id, message_id, uploader_user_id,
		   file_name, file_url, content_type, file_size, thumbnail_url, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`
	if _, err := r.db.ExecContext(ctx, q,
		a.AttachmentID(), a.ConversationID(), a.MessageID(), a.UploaderUserID(),
		a.FileName(), a.FileURL(), a.ContentType(), a.FileSize(), a.ThumbnailURL(), a.CreatedAt(),
	); err != nil {
		return fmt.Errorf("chat attachment repo: create: %w", err)
	}
	return nil
}

// GetByIDs returns attachments matching the given IDs.
func (r *ChatAttachmentRepository) GetByIDs(ctx context.Context, ids []uuid.UUID) ([]*chat.Attachment, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	q := fmt.Sprintf(`
		SELECT attachment_id, conversation_id, message_id, uploader_user_id,
		       file_name, file_url, content_type, file_size, thumbnail_url, created_at
		FROM chat_attachment
		WHERE attachment_id IN (%s)`, strings.Join(placeholders, ","))

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("chat attachment repo: get by ids: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("chat attachment repo: close get-by-ids rows")
		}
	}()

	var attachments []*chat.Attachment
	for rows.Next() {
		att, scanErr := scanAttachment(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("chat attachment repo: scan: %w", scanErr)
		}
		attachments = append(attachments, att)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("chat attachment repo: get by ids rows err: %w", err)
	}
	return attachments, nil
}

// LinkToMessage sets message_id for the given attachment IDs.
func (r *ChatAttachmentRepository) LinkToMessage(ctx context.Context, messageID uuid.UUID, attachmentIDs []uuid.UUID) error {
	if len(attachmentIDs) == 0 {
		return nil
	}
	placeholders := make([]string, len(attachmentIDs))
	args := make([]any, 0, len(attachmentIDs)+1)
	args = append(args, messageID)
	for i, id := range attachmentIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args = append(args, id)
	}
	q := fmt.Sprintf(
		`UPDATE chat_attachment SET message_id=$1 WHERE attachment_id IN (%s)`,
		strings.Join(placeholders, ","),
	)
	if _, err := r.db.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("chat attachment repo: link to message: %w", err)
	}
	return nil
}

// ListByMessageIDs returns, for each message ID, its attachments.
func (r *ChatAttachmentRepository) ListByMessageIDs(ctx context.Context, messageIDs []uuid.UUID) (map[uuid.UUID][]*chat.Attachment, error) {
	if len(messageIDs) == 0 {
		return map[uuid.UUID][]*chat.Attachment{}, nil
	}
	placeholders := make([]string, len(messageIDs))
	args := make([]any, len(messageIDs))
	for i, id := range messageIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	q := fmt.Sprintf(`
		SELECT attachment_id, conversation_id, message_id, uploader_user_id,
		       file_name, file_url, content_type, file_size, thumbnail_url, created_at
		FROM chat_attachment
		WHERE message_id IN (%s)
		ORDER BY created_at ASC`, strings.Join(placeholders, ","))

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("chat attachment repo: list by message ids: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("chat attachment repo: close list-by-message rows")
		}
	}()

	result := make(map[uuid.UUID][]*chat.Attachment, len(messageIDs))
	for rows.Next() {
		att, scanErr := scanAttachment(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("chat attachment repo: scan: %w", scanErr)
		}
		if att.MessageID() != nil {
			result[*att.MessageID()] = append(result[*att.MessageID()], att)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("chat attachment repo: list by message ids rows err: %w", err)
	}
	return result, nil
}

func scanAttachment(s scanner) (*chat.Attachment, error) {
	var (
		attachmentID, convID, uploaderID uuid.UUID
		messageID                        *uuid.UUID
		fileName, fileURL, contentType   string
		thumbnailURL                     sql.NullString
		fileSize                         int64
		createdAt                        time.Time
	)
	if err := s.Scan(&attachmentID, &convID, &messageID, &uploaderID,
		&fileName, &fileURL, &contentType, &fileSize, &thumbnailURL, &createdAt); err != nil {
		return nil, err
	}
	return chat.ReconstructAttachment(
		attachmentID, convID, messageID, uploaderID,
		fileName, fileURL, contentType, fileSize, thumbnailURL.String, createdAt,
	), nil
}
