package service

import (
	"context"
	"fmt"
	"log/slog"

	"yuon/internal/rag"
	"yuon/internal/rag/llm"
	"yuon/internal/rag/search"
	"yuon/internal/rag/vectorstore"
)

type ChatbotService struct {
	llm         *llm.OpenAIClient
	vectorStore *vectorstore.QdrantClient
	fullText    *search.OpenSearchClient
}

func NewChatbotService(
	llmClient *llm.OpenAIClient,
	vectorStore *vectorstore.QdrantClient,
	fullText *search.OpenSearchClient,
) *ChatbotService {
	return &ChatbotService{
		llm:         llmClient,
		vectorStore: vectorStore,
		fullText:    fullText,
	}
}

func (s *ChatbotService) Chat(ctx context.Context, req *rag.ChatRequest) (*rag.ChatResponse, error) {
	var retrievedDocs []rag.Document

	if req.TopK == 0 {
		req.TopK = 5
	}

	// 벡터 검색
	if req.UseVectorSearch {
		vectorDocs, err := s.searchByVector(ctx, req.Message, req.TopK)
		if err != nil {
			slog.Error("벡터 검색 실패", "error", err)
		} else {
			retrievedDocs = append(retrievedDocs, vectorDocs...)
		}
	}

	// 전문 검색
	if req.UseFullText {
		fullTextDocs, err := s.searchByFullText(ctx, req.Message, req.TopK)
		if err != nil {
			slog.Error("전문 검색 실패", "error", err)
		} else {
			retrievedDocs = append(retrievedDocs, fullTextDocs...)
		}
	}

	// 중복 제거 및 상위 문서 선택
	retrievedDocs = s.deduplicateAndRank(retrievedDocs, req.TopK)

	// 대화 메시지 구성
	messages := append(req.History, rag.ChatMessage{
		Role:    "user",
		Content: req.Message,
	})

	// LLM 응답 생성
	answer, tokensUsed, err := s.llm.Chat(ctx, messages, retrievedDocs)
	if err != nil {
		return nil, fmt.Errorf("LLM 응답 생성 실패: %w", err)
	}

	return &rag.ChatResponse{
		Answer:         answer,
		ConversationID: req.ConversationID,
		Sources:        retrievedDocs,
		TokensUsed:     tokensUsed,
	}, nil
}

func (s *ChatbotService) searchByVector(ctx context.Context, query string, topK int) ([]rag.Document, error) {
	// 쿼리를 벡터로 변환
	vector, err := s.llm.GenerateEmbedding(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("임베딩 생성 실패: %w", err)
	}

	// 벡터 검색
	docs, err := s.vectorStore.Search(ctx, vector, topK)
	if err != nil {
		return nil, fmt.Errorf("벡터 검색 실패: %w", err)
	}

	return docs, nil
}

func (s *ChatbotService) searchByFullText(ctx context.Context, query string, topK int) ([]rag.Document, error) {
	docs, err := s.fullText.Search(ctx, query, topK)
	if err != nil {
		return nil, fmt.Errorf("전문 검색 실패: %w", err)
	}

	return docs, nil
}

func (s *ChatbotService) deduplicateAndRank(docs []rag.Document, topK int) []rag.Document {
	seen := make(map[string]bool)
	var unique []rag.Document

	for _, doc := range docs {
		if !seen[doc.ID] {
			seen[doc.ID] = true
			unique = append(unique, doc)
		}
	}

	// Score 기준 정렬 (내림차순)
	for i := 0; i < len(unique)-1; i++ {
		for j := i + 1; j < len(unique); j++ {
			if unique[i].Score < unique[j].Score {
				unique[i], unique[j] = unique[j], unique[i]
			}
		}
	}

	if len(unique) > topK {
		unique = unique[:topK]
	}

	return unique
}

func (s *ChatbotService) AddDocument(ctx context.Context, doc rag.Document) error {
	// OpenSearch에 추가
	if err := s.fullText.AddDocument(ctx, doc); err != nil {
		return fmt.Errorf("OpenSearch 문서 추가 실패: %w", err)
	}

	// 벡터 생성 및 Qdrant에 추가
	vector, err := s.llm.GenerateEmbedding(ctx, doc.Content)
	if err != nil {
		return fmt.Errorf("임베딩 생성 실패: %w", err)
	}

	if err := s.vectorStore.AddDocument(ctx, doc, vector); err != nil {
		return fmt.Errorf("Qdrant 문서 추가 실패: %w", err)
	}

	slog.Info("문서 추가 완료", "id", doc.ID)
	return nil
}

func (s *ChatbotService) BulkAddDocuments(ctx context.Context, docs []rag.Document) error {
	// OpenSearch 벌크 인덱싱
	if err := s.fullText.BulkIndex(ctx, docs); err != nil {
		return fmt.Errorf("OpenSearch 벌크 인덱싱 실패: %w", err)
	}

	// Qdrant에 개별 추가
	for _, doc := range docs {
		vector, err := s.llm.GenerateEmbedding(ctx, doc.Content)
		if err != nil {
			slog.Error("임베딩 생성 실패", "id", doc.ID, "error", err)
			continue
		}

		if err := s.vectorStore.AddDocument(ctx, doc, vector); err != nil {
			slog.Error("Qdrant 문서 추가 실패", "id", doc.ID, "error", err)
			continue
		}
	}

	slog.Info("벌크 문서 추가 완료", "count", len(docs))
	return nil
}

func (s *ChatbotService) ListDocuments(ctx context.Context, params *rag.DocumentListParams) (*rag.DocumentListResult, error) {
	return s.fullText.ListDocuments(ctx, params)
}

func (s *ChatbotService) GetDocument(ctx context.Context, id string) (*rag.Document, error) {
	return s.fullText.GetDocument(ctx, id)
}

func (s *ChatbotService) UpdateDocument(ctx context.Context, doc rag.Document) error {
	if err := s.fullText.UpdateDocument(ctx, doc); err != nil {
		return fmt.Errorf("OpenSearch 문서 업데이트 실패: %w", err)
	}

	vector, err := s.llm.GenerateEmbedding(ctx, doc.Content)
	if err != nil {
		return fmt.Errorf("임베딩 생성 실패: %w", err)
	}

	if err := s.vectorStore.AddDocument(ctx, doc, vector); err != nil {
		return fmt.Errorf("Qdrant 문서 업데이트 실패: %w", err)
	}

	return nil
}

func (s *ChatbotService) DeleteDocument(ctx context.Context, id string) error {
	if err := s.fullText.DeleteDocument(ctx, id); err != nil {
		return fmt.Errorf("OpenSearch 문서 삭제 실패: %w", err)
	}

	if err := s.vectorStore.DeleteDocument(ctx, id); err != nil {
		return fmt.Errorf("Qdrant 문서 삭제 실패: %w", err)
	}

	return nil
}

func (s *ChatbotService) ReindexDocuments(ctx context.Context, ids []string) (*rag.ReindexResult, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("재색인할 문서 ID가 없습니다")
	}

	docs, err := s.fullText.FetchDocuments(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("문서 조회 실패: %w", err)
	}

	result := &rag.ReindexResult{
		Requested: len(ids),
	}

	existing := make(map[string]rag.Document)
	for _, doc := range docs {
		existing[doc.ID] = doc
	}

	for _, id := range ids {
		doc, ok := existing[id]
		if !ok {
			result.Failed = append(result.Failed, id)
			continue
		}

		vector, err := s.llm.GenerateEmbedding(ctx, doc.Content)
		if err != nil {
			slog.Error("임베딩 생성 실패", "id", doc.ID, "error", err)
			result.Failed = append(result.Failed, doc.ID)
			continue
		}

		if err := s.vectorStore.AddDocument(ctx, doc, vector); err != nil {
			slog.Error("Qdrant 재색인 실패", "id", doc.ID, "error", err)
			result.Failed = append(result.Failed, doc.ID)
			continue
		}

		result.Reindexed++
	}

	return result, nil
}

func (s *ChatbotService) GetDocumentStats(ctx context.Context) (*rag.DocumentStats, error) {
	return s.fullText.GetStats(ctx)
}

func (s *ChatbotService) FetchDocumentVector(ctx context.Context, id string, withPayload bool) (*rag.DocumentVector, error) {
	return s.vectorStore.GetDocumentVector(ctx, id, withPayload)
}

func (s *ChatbotService) QueryDocumentVectors(ctx context.Context, req *rag.VectorQueryRequest) (*rag.VectorQueryResponse, error) {
	vectors, hasMore, nextOffset, err := s.vectorStore.QueryDocumentVectors(ctx, req.DocumentIDs, req.Limit, req.WithPayload, req.Offset)
	if err != nil {
		return nil, err
	}

	return &rag.VectorQueryResponse{
		Vectors:    vectors,
		Count:      len(vectors),
		HasMore:    hasMore,
		NextOffset: nextOffset,
	}, nil
}
