package service

import (
	"context"
	"database/sql"
	"fmt"
	"yuon/internal/rag"
)

type AnalyticsStore interface {
	Record(ctx context.Context, keywords []string, categories []string, hourKey string) error
	Snapshot(ctx context.Context) (AnalyticsStats, error)
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
