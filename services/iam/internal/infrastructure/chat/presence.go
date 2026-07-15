package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	presenceKeyPrefix = "iam:presence:"
	typingKeyPrefix   = "iam:typing:"
	presenceTTL       = 90 * time.Second
	typingTTL         = 5 * time.Second
)

// PresenceService tracks user online status and per-conversation typing
// indicators using short-TTL Redis keys. Presence/typing is best-effort:
// callers refresh via periodic heartbeats, and keys expire naturally when a
// client disconnects without an explicit "offline" signal.
type PresenceService struct {
	rdb *redis.Client
}

// NewPresenceService returns a PresenceService backed by rdb.
func NewPresenceService(rdb *redis.Client) *PresenceService {
	return &PresenceService{rdb: rdb}
}

func presenceKey(userID uuid.UUID) string {
	return presenceKeyPrefix + userID.String()
}

func typingKey(convID, userID uuid.UUID) string {
	return typingKeyPrefix + convID.String() + ":" + userID.String()
}

// SetOnline marks userID as online for presenceTTL. Callers should call this
// periodically (heartbeat) while the user has an active chat connection.
func (p *PresenceService) SetOnline(ctx context.Context, userID uuid.UUID) error {
	if err := p.rdb.Set(ctx, presenceKey(userID), "1", presenceTTL).Err(); err != nil {
		return fmt.Errorf("presence: set online for user %s: %w", userID, err)
	}
	return nil
}

// IsOnline reports whether userID has an unexpired presence key.
func (p *PresenceService) IsOnline(ctx context.Context, userID uuid.UUID) (bool, error) {
	n, err := p.rdb.Exists(ctx, presenceKey(userID)).Result()
	if err != nil {
		return false, fmt.Errorf("presence: check online for user %s: %w", userID, err)
	}
	return n > 0, nil
}

// GetOnlineUsers returns the subset of userIDs that are currently online. If
// userIDs is empty, all currently online users are returned instead (found by
// scanning the presence key namespace).
func (p *PresenceService) GetOnlineUsers(ctx context.Context, userIDs []uuid.UUID) ([]uuid.UUID, error) {
	if len(userIDs) == 0 {
		return p.getAllOnlineUsers(ctx)
	}

	keys := make([]string, len(userIDs))
	for i, id := range userIDs {
		keys[i] = presenceKey(id)
	}

	vals, err := p.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("presence: mget online users: %w", err)
	}

	online := make([]uuid.UUID, 0, len(userIDs))
	for i, v := range vals {
		if v != nil {
			online = append(online, userIDs[i])
		}
	}
	return online, nil
}

func (p *PresenceService) getAllOnlineUsers(ctx context.Context) ([]uuid.UUID, error) {
	keys, err := p.rdb.Keys(ctx, presenceKeyPrefix+"*").Result()
	if err != nil {
		return nil, fmt.Errorf("presence: scan online users: %w", err)
	}

	online := make([]uuid.UUID, 0, len(keys))
	for _, k := range keys {
		idStr := strings.TrimPrefix(k, presenceKeyPrefix)
		id, err := uuid.Parse(idStr)
		if err != nil {
			continue
		}
		online = append(online, id)
	}
	return online, nil
}

// SetTyping marks userID as typing in convID for typingTTL, or immediately
// clears the typing indicator when isTyping is false.
func (p *PresenceService) SetTyping(ctx context.Context, convID, userID uuid.UUID, isTyping bool) error {
	key := typingKey(convID, userID)
	if !isTyping {
		if err := p.rdb.Del(ctx, key).Err(); err != nil && !errors.Is(err, redis.Nil) {
			return fmt.Errorf("presence: clear typing for user %s in conversation %s: %w", userID, convID, err)
		}
		return nil
	}
	if err := p.rdb.Set(ctx, key, "1", typingTTL).Err(); err != nil {
		return fmt.Errorf("presence: set typing for user %s in conversation %s: %w", userID, convID, err)
	}
	return nil
}

// GetTypingUsers returns the users currently typing in convID.
func (p *PresenceService) GetTypingUsers(ctx context.Context, convID uuid.UUID) ([]uuid.UUID, error) {
	pattern := typingKeyPrefix + convID.String() + ":*"
	keys, err := p.rdb.Keys(ctx, pattern).Result()
	if err != nil {
		return nil, fmt.Errorf("presence: scan typing users for conversation %s: %w", convID, err)
	}

	prefix := typingKeyPrefix + convID.String() + ":"
	typing := make([]uuid.UUID, 0, len(keys))
	for _, k := range keys {
		idStr := strings.TrimPrefix(k, prefix)
		id, err := uuid.Parse(idStr)
		if err != nil {
			continue
		}
		typing = append(typing, id)
	}
	return typing, nil
}
