package service

import (
	"context"
	"database/sql"
	"fmt"
)

type AnalyticsStore interface {
	Record(ctx context.Context, keywords []string, categories []string, hourKey string) error
	Snapshot(ctx context.Context) (AnalyticsStats, error)
	RecordSession(ctx context.Context, sessionID, conversationID string) error
	RecordResponseTime(ctx context.Context, conversationID string, responseTimeMs, tokenCount int) error
	GetActiveUsers(ctx context.Context, withinMinutes int) (int64, error)
	GetAvgResponseTime(ctx context.Context, withinHours int) (float64, error)
	SnapshotDailyStats(ctx context.Context) error
	GetDailyStats(ctx context.Context, daysAgo int) (*DailyStatsSnapshot, error)
}

type PostgresAnalyticsStore struct {
	db *sql.DB
}

func NewPostgresAnalyticsStore(db *sql.DB) *PostgresAnalyticsStore {
	return &PostgresAnalyticsStore{db: db}
}

func (s *PostgresAnalyticsStore) Record(ctx context.Context, keywords []string, categories []string, hourKey string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, kw := range keywords {
		if kw == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO analytics_keywords (keyword, count)
			VALUES ($1, 1)
			ON CONFLICT (keyword) DO UPDATE SET count = analytics_keywords.count + 1
		`, kw); err != nil {
			return fmt.Errorf("keyword upsert failed: %w", err)
		}
	}

	for _, cat := range categories {
		if cat == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO analytics_categories (category, count)
			VALUES ($1, 1)
			ON CONFLICT (category) DO UPDATE SET count = analytics_categories.count + 1
		`, cat); err != nil {
			return fmt.Errorf("category upsert failed: %w", err)
		}
	}

	if hourKey != "" {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO analytics_hourly (hour_key, count)
			VALUES ($1, 1)
			ON CONFLICT (hour_key) DO UPDATE SET count = analytics_hourly.count + 1
		`, hourKey); err != nil {
			return fmt.Errorf("hourly upsert failed: %w", err)
		}
	}

	return tx.Commit()
}

func (s *PostgresAnalyticsStore) Snapshot(ctx context.Context) (AnalyticsStats, error) {
	stats := AnalyticsStats{}

	type kv struct {
		key   string
		value int
	}

	read := func(query string) ([]kv, error) {
		rows, err := s.db.QueryContext(ctx, query)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var res []kv
		for rows.Next() {
			var key string
			var val int
			if err := rows.Scan(&key, &val); err != nil {
				return nil, err
			}
			res = append(res, kv{key: key, value: val})
		}
		return res, nil
	}

	if items, err := read(`SELECT keyword, count FROM analytics_keywords ORDER BY count DESC LIMIT 10`); err == nil {
		for _, it := range items {
			stats.TopKeywords = append(stats.TopKeywords, keywordStat{Keyword: it.key, Count: it.value})
		}
	}

	if items, err := read(`SELECT category, count FROM analytics_categories ORDER BY count DESC LIMIT 10`); err == nil {
		for _, it := range items {
			stats.TopCategories = append(stats.TopCategories, keywordStat{Keyword: it.key, Count: it.value})
		}
	}

	if items, err := read(`SELECT hour_key, count FROM analytics_hourly ORDER BY hour_key DESC LIMIT 24`); err == nil {
		for _, it := range items {
			stats.RequestsByHour = append(stats.RequestsByHour, keywordStat{Keyword: it.key, Count: it.value})
		}
	}

	// totalMessages는 keywords 총합을 근사 사용
	for _, k := range stats.TopKeywords {
		stats.TotalMessages += k.Count
	}
	return stats, nil
}

func (s *PostgresAnalyticsStore) RecordSession(ctx context.Context, sessionID, conversationID string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO active_sessions (session_id, conversation_id, last_activity)
		VALUES ($1, $2, NOW())
		ON CONFLICT (session_id)
		DO UPDATE SET
			conversation_id = EXCLUDED.conversation_id,
			last_activity = NOW()
	`, sessionID, conversationID)
	return err
}

func (s *PostgresAnalyticsStore) RecordResponseTime(ctx context.Context, conversationID string, responseTimeMs, tokenCount int) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO response_metrics (conversation_id, response_time_ms, token_count)
		VALUES ($1, $2, $3)
	`, conversationID, responseTimeMs, tokenCount)
	return err
}

func (s *PostgresAnalyticsStore) GetActiveUsers(ctx context.Context, withinMinutes int) (int64, error) {
	// Clean up old sessions first
	_, _ = s.db.ExecContext(ctx, `
		DELETE FROM active_sessions
		WHERE last_activity < NOW() - INTERVAL '30 minutes'
	`)

	var count int64
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT session_id)
		FROM active_sessions
		WHERE last_activity >= NOW() - $1 * INTERVAL '1 minute'
	`, withinMinutes).Scan(&count)

	return count, err
}

func (s *PostgresAnalyticsStore) GetAvgResponseTime(ctx context.Context, withinHours int) (float64, error) {
	var avg sql.NullFloat64
	err := s.db.QueryRowContext(ctx, `
		SELECT AVG(response_time_ms)::REAL / 1000.0
		FROM response_metrics
		WHERE created_at >= NOW() - $1 * INTERVAL '1 hour'
	`, withinHours).Scan(&avg)

	if err != nil || !avg.Valid {
		return 0, err
	}
	return avg.Float64, nil
}

type DailyStatsSnapshot struct {
	Date               string  `json:"date"`
	TotalDocuments     int64   `json:"total_documents"`
	TotalConversations int64   `json:"total_conversations"`
	TotalMessages      int64   `json:"total_messages"`
	ActiveUsers        int64   `json:"active_users"`
	AvgResponseTime    float64 `json:"avg_response_time"`
}

func (s *PostgresAnalyticsStore) SnapshotDailyStats(ctx context.Context) error {
	// This should be called daily by a cron job
	// For now, it's a placeholder
	return nil
}

func (s *PostgresAnalyticsStore) GetDailyStats(ctx context.Context, daysAgo int) (*DailyStatsSnapshot, error) {
	var snap DailyStatsSnapshot
	err := s.db.QueryRowContext(ctx, `
		SELECT
			date::TEXT,
			total_documents,
			total_conversations,
			total_messages,
			active_users,
			COALESCE(avg_response_time, 0)
		FROM daily_stats
		WHERE date = CURRENT_DATE - $1
	`, daysAgo).Scan(
		&snap.Date,
		&snap.TotalDocuments,
		&snap.TotalConversations,
		&snap.TotalMessages,
		&snap.ActiveUsers,
		&snap.AvgResponseTime,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &snap, err
}
