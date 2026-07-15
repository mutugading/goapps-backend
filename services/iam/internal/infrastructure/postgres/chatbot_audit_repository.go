package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// ChatbotAuditEntry represents one chatbot interaction.
type ChatbotAuditEntry struct {
	UserID         uuid.UUID
	SessionID      string
	RequestTokens  int
	ResponseTokens int
	ToolsCalled    []string
	WasBlocked     bool
	BlockReason    string
}

// ChatbotAuditRepository persists chatbot audit logs.
type ChatbotAuditRepository struct {
	db *DB
}

// NewChatbotAuditRepository constructs the repo.
func NewChatbotAuditRepository(db *DB) *ChatbotAuditRepository {
	return &ChatbotAuditRepository{db: db}
}

// Create inserts a new audit log entry.
func (r *ChatbotAuditRepository) Create(ctx context.Context, entry *ChatbotAuditEntry) error {
	const q = `
		INSERT INTO chatbot_audit_log
		  (user_id, session_id, request_tokens, response_tokens, tools_called, was_blocked, block_reason)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`
	toolsStr := "{" + strings.Join(entry.ToolsCalled, ",") + "}"
	if _, err := r.db.ExecContext(ctx, q,
		entry.UserID, entry.SessionID, entry.RequestTokens, entry.ResponseTokens,
		toolsStr, entry.WasBlocked, entry.BlockReason,
	); err != nil {
		return fmt.Errorf("chatbot audit repo: create: %w", err)
	}
	return nil
}
