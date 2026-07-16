package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// ChatUserInfo holds lightweight user data for chat participant display.
type ChatUserInfo struct {
	UserID    uuid.UUID
	Username  string
	FullName  string
	AvatarURL string
}

// ChatUserResolver resolves user IDs to display info for chat participants.
type ChatUserResolver struct {
	db *DB
}

// NewChatUserResolver constructs the resolver.
func NewChatUserResolver(db *DB) *ChatUserResolver {
	return &ChatUserResolver{db: db}
}

// ResolveUsers batch-resolves user IDs to display info.
func (r *ChatUserResolver) ResolveUsers(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID]*ChatUserInfo, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(userIDs))
	args := make([]any, len(userIDs))
	for i, id := range userIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	q := fmt.Sprintf(`
		SELECT u.user_id, u.username,
		       COALESCE(d.full_name, ''),
		       COALESCE(d.profile_picture_url, '')
		FROM mst_user u
		LEFT JOIN mst_user_detail d ON d.user_id = u.user_id
		WHERE u.user_id IN (%s)`, strings.Join(placeholders, ","))

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("chat user resolver: query: %w", err)
	}
	defer rows.Close()

	result := make(map[uuid.UUID]*ChatUserInfo, len(userIDs))
	for rows.Next() {
		var info ChatUserInfo
		var fullName, avatarURL sql.NullString
		if err := rows.Scan(&info.UserID, &info.Username, &fullName, &avatarURL); err != nil {
			return nil, fmt.Errorf("chat user resolver: scan: %w", err)
		}
		info.FullName = fullName.String
		info.AvatarURL = avatarURL.String
		result[info.UserID] = &info
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("chat user resolver: rows err: %w", err)
	}
	return result, nil
}
