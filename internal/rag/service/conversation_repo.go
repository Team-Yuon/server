package service

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type ConversationSummary struct {
	ID           string
	Preview      string
	MessageCount int
	CreatedAt    time.Time
	TokenUsage   int
	UpdatedAt    time.Time
}

type ConversationMessage struct {
	Role      string
	Content   string
	Timestamp time.Time
}

type ConversationRepository interface {
	EnsureConversation(ctx context.Context, id string) error
	AddMessage(ctx context.Context, id, role, content string, ts time.Time) error
	UpdateTokenUsage(ctx context.Context, id string, tokens int) error
	List(ctx context.Context, limit int) ([]ConversationSummary, error)
	Messages(ctx context.Context, id string) ([]ConversationMessage, error)
}

type PostgresConversationStore struct {
	db *sql.DB
}

func NewPostgresConversationStore(db *sql.DB) *PostgresConversationStore {
	return &PostgresConversationStore{db: db}
}

func (s *PostgresConversationStore) EnsureConversation(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO conversations (id)
		VALUES ($1)
		ON CONFLICT (id) DO UPDATE SET updated_at = NOW()
	`, id)
	if err != nil {
		return fmt.Errorf("ensure conversation failed: %w", err)
	}
	return nil
}

func (s *PostgresConversationStore) AddMessage(ctx context.Context, id, role, content string, ts time.Time) error {
	if err := s.EnsureConversation(ctx, id); err != nil {
		return err
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO conversation_messages (conversation_id, role, content, ts)
		VALUES ($1, $2, $3, $4)`, id, role, content, ts)
	if err != nil {
		return fmt.Errorf("insert conversation message failed: %w", err)
	}

	// Update summary fields
	_, err = s.db.ExecContext(ctx, `
		UPDATE conversations
		SET
			message_count = message_count + 1,
			preview = COALESCE(preview, CASE WHEN $2 = 'user' THEN $3 ELSE preview END),
			updated_at = NOW()
		WHERE id = $1
	`, id, role, content)
	if err != nil {
		return fmt.Errorf("update conversation summary failed: %w", err)
	}
	return nil
}

func (s *PostgresConversationStore) UpdateTokenUsage(ctx context.Context, id string, tokens int) error {
	if tokens <= 0 {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE conversations
		SET token_usage = token_usage + $2,
		    updated_at = NOW()
		WHERE id = $1
	`, id, tokens)
	if err != nil {
		return fmt.Errorf("update token usage failed: %w", err)
	}
	return nil
}

func (s *PostgresConversationStore) List(ctx context.Context, limit int) ([]ConversationSummary, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, preview, message_count, token_usage, created_at, updated_at
		FROM conversations
		ORDER BY updated_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list conversations failed: %w", err)
	}
	defer rows.Close()

	var result []ConversationSummary
	for rows.Next() {
		var item ConversationSummary
		if err := rows.Scan(&item.ID, &item.Preview, &item.MessageCount, &item.TokenUsage, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

func (s *PostgresConversationStore) Messages(ctx context.Context, id string) ([]ConversationMessage, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT role, content, ts
		FROM conversation_messages
		WHERE conversation_id = $1
		ORDER BY ts ASC
	`, id)
	if err != nil {
		return nil, fmt.Errorf("list conversation messages failed: %w", err)
	}
	defer rows.Close()

	var msgs []ConversationMessage
	for rows.Next() {
		var msg ConversationMessage
		if err := rows.Scan(&msg.Role, &msg.Content, &msg.Timestamp); err != nil {
			return nil, err
		}
		msgs = append(msgs, msg)
	}
	return msgs, nil
}
