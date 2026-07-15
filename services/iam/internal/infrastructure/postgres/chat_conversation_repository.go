package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/chat"
)

// ChatConversationRepository implements chat.ConversationRepository.
type ChatConversationRepository struct {
	db *DB
}

// NewChatConversationRepository constructs the repo.
func NewChatConversationRepository(db *DB) *ChatConversationRepository {
	return &ChatConversationRepository{db: db}
}

// Create inserts a new conversation with its initial participants.
func (r *ChatConversationRepository) Create(ctx context.Context, conv *chat.Conversation) error {
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		const convQ = `
			INSERT INTO chat_conversation
			  (conversation_id, type, name, avatar_url, encryption_key, created_by, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`
		if _, err := tx.ExecContext(ctx, convQ,
			conv.ID(), conv.Type().String(), conv.Name(), conv.AvatarURL(),
			conv.EncryptionKey(), conv.CreatedBy(), conv.CreatedAt(), conv.UpdatedAt(),
		); err != nil {
			return fmt.Errorf("insert conversation: %w", err)
		}

		const partQ = `
			INSERT INTO chat_participant (conversation_id, user_id, role, joined_at)
			VALUES ($1,$2,$3,$4)`
		for _, p := range conv.Participants() {
			if _, err := tx.ExecContext(ctx, partQ,
				conv.ID(), p.UserID(), p.Role().String(), p.JoinedAt(),
			); err != nil {
				return fmt.Errorf("insert participant: %w", err)
			}
		}
		return nil
	})
}

// GetByID returns a conversation with all active participants.
func (r *ChatConversationRepository) GetByID(ctx context.Context, id uuid.UUID) (*chat.Conversation, error) {
	const q = `
		SELECT conversation_id, type, name, avatar_url, encryption_key,
		       created_by, created_at, updated_at, deleted_at
		FROM chat_conversation
		WHERE conversation_id = $1 AND deleted_at IS NULL`

	conv, err := r.scanConversation(r.db.QueryRowContext(ctx, q, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, chat.ErrConversationNotFound
		}
		return nil, fmt.Errorf("chat conv repo: get by id: %w", err)
	}

	participants, err := r.loadParticipants(ctx, id)
	if err != nil {
		return nil, err
	}
	return chat.Reconstruct(
		conv.ID(), conv.Type(), conv.Name(), conv.AvatarURL(),
		conv.EncryptionKey(), conv.CreatedBy(),
		conv.CreatedAt(), conv.UpdatedAt(), conv.DeletedAt(),
		participants,
	), nil
}

// FindDirect finds an existing DIRECT conversation between two users.
func (r *ChatConversationRepository) FindDirect(ctx context.Context, userA, userB uuid.UUID) (*chat.Conversation, error) {
	const q = `
		SELECT cc.conversation_id
		FROM chat_conversation cc
		JOIN chat_participant pa ON pa.conversation_id = cc.conversation_id AND pa.user_id = $1 AND pa.left_at IS NULL
		JOIN chat_participant pb ON pb.conversation_id = cc.conversation_id AND pb.user_id = $2 AND pb.left_at IS NULL
		WHERE cc.type = 'DIRECT' AND cc.deleted_at IS NULL
		LIMIT 1`

	var convID uuid.UUID
	err := r.db.QueryRowContext(ctx, q, userA, userB).Scan(&convID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil //nolint:nilnil // nil,nil means not found
	}
	if err != nil {
		return nil, fmt.Errorf("chat conv repo: find direct: %w", err)
	}
	return r.GetByID(ctx, convID)
}

// ListByUserID returns conversations the user participates in.
func (r *ChatConversationRepository) ListByUserID(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]*chat.Conversation, int64, error) {
	offset := (page - 1) * pageSize

	const listQ = `
		SELECT cc.conversation_id
		FROM chat_conversation cc
		JOIN chat_participant cp ON cp.conversation_id = cc.conversation_id
		  AND cp.user_id = $1 AND cp.left_at IS NULL
		WHERE cc.deleted_at IS NULL
		ORDER BY cc.updated_at DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.db.QueryContext(ctx, listQ, userID, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("chat conv repo: list: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, 0, fmt.Errorf("chat conv repo: scan id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("chat conv repo: rows err: %w", err)
	}

	const countQ = `
		SELECT COUNT(*) FROM chat_conversation cc
		JOIN chat_participant cp ON cp.conversation_id = cc.conversation_id
		  AND cp.user_id = $1 AND cp.left_at IS NULL
		WHERE cc.deleted_at IS NULL`

	var total int64
	if err := r.db.QueryRowContext(ctx, countQ, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("chat conv repo: count: %w", err)
	}

	convs := make([]*chat.Conversation, 0, len(ids))
	for _, id := range ids {
		c, err := r.GetByID(ctx, id)
		if err != nil {
			return nil, 0, err
		}
		convs = append(convs, c)
	}
	return convs, total, nil
}

// UpdateGroup persists name and avatar changes.
func (r *ChatConversationRepository) UpdateGroup(ctx context.Context, conv *chat.Conversation) error {
	const q = `
		UPDATE chat_conversation SET name=$1, avatar_url=$2, updated_at=$3
		WHERE conversation_id=$4 AND deleted_at IS NULL`
	if _, err := r.db.ExecContext(ctx, q,
		conv.Name(), conv.AvatarURL(), time.Now().UTC(), conv.ID(),
	); err != nil {
		return fmt.Errorf("chat conv repo: update group: %w", err)
	}
	return nil
}

// AddParticipants inserts new participant rows.
func (r *ChatConversationRepository) AddParticipants(ctx context.Context, convID uuid.UUID, participants []*chat.Participant) error {
	const q = `
		INSERT INTO chat_participant (conversation_id, user_id, role, joined_at)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (conversation_id, user_id) DO UPDATE SET left_at = NULL, role = $3`
	for _, p := range participants {
		if _, err := r.db.ExecContext(ctx, q,
			convID, p.UserID(), p.Role().String(), p.JoinedAt(),
		); err != nil {
			return fmt.Errorf("chat conv repo: add participant: %w", err)
		}
	}
	return nil
}

// RemoveParticipant sets left_at for a participant.
func (r *ChatConversationRepository) RemoveParticipant(ctx context.Context, convID, userID uuid.UUID) error {
	const q = `
		UPDATE chat_participant SET left_at = NOW()
		WHERE conversation_id=$1 AND user_id=$2 AND left_at IS NULL`
	if _, err := r.db.ExecContext(ctx, q, convID, userID); err != nil {
		return fmt.Errorf("chat conv repo: remove participant: %w", err)
	}
	return nil
}

// UpdateLastReadAt updates chat_participant.last_read_at for a user.
func (r *ChatConversationRepository) UpdateLastReadAt(ctx context.Context, convID, userID uuid.UUID, at time.Time) error {
	const q = `UPDATE chat_participant SET last_read_at=$1 WHERE conversation_id=$2 AND user_id=$3`
	if _, err := r.db.ExecContext(ctx, q, at, convID, userID); err != nil {
		return fmt.Errorf("chat conv repo: update last_read_at: %w", err)
	}
	return nil
}

func (r *ChatConversationRepository) loadParticipants(ctx context.Context, convID uuid.UUID) ([]*chat.Participant, error) {
	const q = `
		SELECT user_id, role, joined_at, left_at, last_read_at
		FROM chat_participant WHERE conversation_id = $1`

	rows, err := r.db.QueryContext(ctx, q, convID)
	if err != nil {
		return nil, fmt.Errorf("chat conv repo: load participants: %w", err)
	}
	defer rows.Close()

	var parts []*chat.Participant
	for rows.Next() {
		var (
			userID              uuid.UUID
			roleStr             string
			joinedAt            time.Time
			leftAt, lastReadAt  *time.Time
		)
		if err := rows.Scan(&userID, &roleStr, &joinedAt, &leftAt, &lastReadAt); err != nil {
			return nil, fmt.Errorf("chat conv repo: scan participant: %w", err)
		}
		role, err := chat.ParseRole(roleStr)
		if err != nil {
			return nil, fmt.Errorf("chat conv repo: parse role: %w", err)
		}
		parts = append(parts, chat.ReconstructParticipant(convID, userID, role, joinedAt, leftAt, lastReadAt))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("chat conv repo: participants rows err: %w", err)
	}
	return parts, nil
}

func (r *ChatConversationRepository) scanConversation(row *sql.Row) (*chat.Conversation, error) {
	var (
		id            uuid.UUID
		typeStr       string
		name          sql.NullString
		avatarURL     sql.NullString
		encryptionKey []byte
		createdBy     string
		createdAt     time.Time
		updatedAt     time.Time
		deletedAt     *time.Time
	)
	if err := row.Scan(&id, &typeStr, &name, &avatarURL, &encryptionKey,
		&createdBy, &createdAt, &updatedAt, &deletedAt); err != nil {
		return nil, err
	}
	convType, err := chat.ParseType(typeStr)
	if err != nil {
		return nil, fmt.Errorf("scan conversation: parse type: %w", err)
	}
	return chat.Reconstruct(id, convType, name.String, avatarURL.String, encryptionKey, createdBy, createdAt, updatedAt, deletedAt, nil), nil
}
