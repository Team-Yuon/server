package database

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
	"yuon/configuration"
)

// Connect opens a PostgreSQL connection using configuration.DatabaseConfig.
func Connect(cfg *configuration.DatabaseConfig) (*sql.DB, error) {
	if cfg == nil {
		return nil, fmt.Errorf("database config is nil")
	}

	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host,
		cfg.Port,
		cfg.User,
		cfg.Password,
		cfg.Name,
		cfg.SSLMode,
	)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Quick validation
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("database ping failed: %w", err)
	}

	return db, nil
}

// EnsureSchemas creates required tables if they do not exist.
func EnsureSchemas(db *sql.DB) error {
	statements := []string{
		// Users
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			email TEXT UNIQUE NOT NULL,
			password_hash BYTEA NOT NULL,
			role TEXT NOT NULL DEFAULT 'user',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`,
		// Conversations
		`CREATE TABLE IF NOT EXISTS conversations (
			id TEXT PRIMARY KEY,
			preview TEXT,
			message_count INTEGER NOT NULL DEFAULT 0,
			token_usage INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`,
		// Conversation messages
		`CREATE TABLE IF NOT EXISTS conversation_messages (
			id BIGSERIAL PRIMARY KEY,
			conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			ts TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`,
		// Analytics keyword/category/hourly counters
		`CREATE TABLE IF NOT EXISTS analytics_keywords (
			keyword TEXT PRIMARY KEY,
			count BIGINT NOT NULL DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS analytics_categories (
			category TEXT PRIMARY KEY,
			count BIGINT NOT NULL DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS analytics_hourly (
			hour_key TEXT PRIMARY KEY,
			count BIGINT NOT NULL DEFAULT 0
		);`,
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("schema init failed: %w", err)
		}
	}
	return nil
}
