package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"yuon/internal/rag"
	"yuon/internal/rag/llm"
)

type keywordStat struct {
	Keyword string `json:"keyword"`
	Count   int    `json:"count"`
}

type AnalyticsStats struct {
	TotalMessages  int           `json:"totalMessages"`
	TopKeywords    []keywordStat `json:"topKeywords"`
	TopCategories  []keywordStat `json:"topCategories"`
	RequestsByHour []keywordStat `json:"requestsByHour"`
}

type analyticsTracker struct {
	llm            *llm.OpenAIClient
	store          AnalyticsStore
	mu             sync.RWMutex
	totalMessages  int
	keywordCounts  map[string]int
	categoryCounts map[string]int
	hourlyCounts   map[string]int
}

func newAnalyticsTracker(llmClient *llm.OpenAIClient, store AnalyticsStore) *analyticsTracker {
	return &analyticsTracker{
		llm:            llmClient,
		store:          store,
		keywordCounts:  make(map[string]int),
		categoryCounts: make(map[string]int),
		hourlyCounts:   make(map[string]int),
	}
}

func (a *analyticsTracker) Record(ctx context.Context, message string, docs []rag.Document) {
	var tokens []string

	// LLM 기반 키워드 추출만 사용
	if a.llm != nil {
		if llmKeywords, err := a.llm.ExtractKeywords(ctx, message, 8); err == nil && len(llmKeywords) > 0 {
			tokens = llmKeywords
		}
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	a.totalMessages++
	for _, t := range tokens {
		a.keywordCounts[t]++
	}

	for _, doc := range docs {
		if doc.Metadata == nil {
			continue
		}
		if category, ok := doc.Metadata["category"].(string); ok && category != "" {
			a.categoryCounts[strings.ToLower(category)]++
		}
	}

	hourKey := time.Now().UTC().Format("15:00")
	a.hourlyCounts[hourKey]++

	// Persist to store if available
	if a.store != nil {
		cats := make([]string, 0)
		for _, doc := range docs {
			if doc.Metadata == nil {
				continue
			}
			if c, ok := doc.Metadata["category"].(string); ok && c != "" {
				cats = append(cats, c)
			}
		}
		_ = a.store.Record(ctx, tokens, cats, hourKey)
	}
}

func (a *analyticsTracker) Snapshot() AnalyticsStats {
	if a.store != nil {
		if snap, err := a.store.Snapshot(context.Background()); err == nil {
			return snap
		}
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	stats := AnalyticsStats{
		TotalMessages:  a.totalMessages,
		TopKeywords:    topN(a.keywordCounts, 10),
		TopCategories:  topN(a.categoryCounts, 10),
		RequestsByHour: topN(a.hourlyCounts, 24),
	}
	return stats
}

func topN(m map[string]int, n int) []keywordStat {
	items := make([]keywordStat, 0, len(m))
	for k, v := range m {
		items = append(items, keywordStat{Keyword: k, Count: v})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			return items[i].Keyword < items[j].Keyword
		}
		return items[i].Count > items[j].Count
	})
	if len(items) > n {
		items = items[:n]
	}
	return items
}


func (a *analyticsTracker) StatsJSON() string {
	stats := a.Snapshot()
	data, _ := json.Marshal(stats)
	return string(data)
}

func (s *ChatbotService) GetAnalyticsStats() AnalyticsStats {
	if s.analytics == nil {
		return AnalyticsStats{}
	}
	return s.analytics.Snapshot()
}

func (s *ChatbotService) GenerateKnowledgeNeedAnalysis(ctx context.Context) (string, error) {
	if s.analytics == nil {
		return "", fmt.Errorf("analytics tracker not configured")
	}
	stats := s.analytics.Snapshot()
	payload, _ := json.Marshal(stats)

	prompt := fmt.Sprintf("다음은 최근 사용자 질문 통계입니다. 부족한 자료 영역을 간결하게 제안해 주세요.\n\n통계 데이터:\n%s", string(payload))

	return s.llm.GenerateText(ctx, "당신은 데이터 분석가입니다. 한국어로 3줄 이내로 부족한 지식 영역을 제안하세요.", prompt, 200)
}
