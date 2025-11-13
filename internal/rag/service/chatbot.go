package service

import (
	"context"
	"fmt"
	"log/slog"
	"math"

	"gonum.org/v1/gonum/mat"
	"yuon/internal/rag"
	"yuon/internal/rag/llm"
	"yuon/internal/rag/search"
	"yuon/internal/rag/vectorstore"
)

type ChatbotService struct {
	llm           *llm.OpenAIClient
	vectorStore   *vectorstore.QdrantClient
	fullText      *search.OpenSearchClient
	conversations *ConversationStore
}

func NewChatbotService(
	llmClient *llm.OpenAIClient,
	vectorStore *vectorstore.QdrantClient,
	fullText *search.OpenSearchClient,
) *ChatbotService {
	return &ChatbotService{
		llm:           llmClient,
		vectorStore:   vectorStore,
		fullText:      fullText,
		conversations: NewConversationStore(),
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
	s.enrichDocumentMetadata(ctx, &doc)

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
	for i := range docs {
		s.enrichDocumentMetadata(ctx, &docs[i])
	}

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
	s.enrichDocumentMetadata(ctx, &doc)

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

func (s *ChatbotService) ConversationHistory(conversationID string) []rag.ChatMessage {
	if s.conversations == nil || conversationID == "" {
		return nil
	}
	return s.conversations.History(conversationID)
}

func (s *ChatbotService) AppendConversationMessage(conversationID string, msg rag.ChatMessage) {
	if s.conversations == nil || conversationID == "" {
		return
	}
	s.conversations.Append(conversationID, msg)
}

func (s *ChatbotService) CloseConversation(conversationID string) {
	if s.conversations == nil || conversationID == "" {
		return
	}
	s.conversations.End(conversationID)
}

func (s *ChatbotService) enrichDocumentMetadata(ctx context.Context, doc *rag.Document) {
	if doc.Metadata == nil {
		doc.Metadata = make(map[string]interface{})
	}

	if _, ok := doc.Metadata["category"]; ok {
		return
	}

	category, err := s.llm.ClassifyCategory(ctx, doc.Content)
	if err != nil {
		slog.Warn("문서 카테고리 분류 실패", "error", err)
		return
	}

	if category == "" {
		return
	}

	doc.Metadata["category"] = category
	slog.Info("문서 카테고리 자동 분류", "id", doc.ID, "category", category)
}

func (s *ChatbotService) ProjectVectors(ctx context.Context, req *rag.VectorProjectionRequest) (*rag.VectorProjectionResponse, error) {
	query := &rag.VectorQueryRequest{
		Limit:       req.Limit,
		Offset:      req.Offset,
		WithPayload: req.WithPayload,
	}

	vectorsResp, err := s.QueryDocumentVectors(ctx, query)
	if err != nil {
		return nil, err
	}

	if len(vectorsResp.Vectors) == 0 {
		return &rag.VectorProjectionResponse{
			Vectors:    []rag.ProjectedVector{},
			Count:      0,
			HasMore:    vectorsResp.HasMore,
			NextOffset: vectorsResp.NextOffset,
		}, nil
	}

	points := make([][]float64, 0, len(vectorsResp.Vectors))
	for _, v := range vectorsResp.Vectors {
		if len(v.Vector) == 0 {
			continue
		}
		point := make([]float64, len(v.Vector))
		for i, val := range v.Vector {
			point[i] = float64(val)
		}
		points = append(points, point)
	}

	if len(points) == 0 {
		return &rag.VectorProjectionResponse{
			Vectors:    []rag.ProjectedVector{},
			Count:      0,
			HasMore:    vectorsResp.HasMore,
			NextOffset: vectorsResp.NextOffset,
		}, nil
	}

	projection := projectTo2D(points)
	result := make([]rag.ProjectedVector, 0, len(projection))

	idx := 0
	for i, coords := range projection {
		for idx < len(vectorsResp.Vectors) && len(vectorsResp.Vectors[idx].Vector) == 0 {
			idx++
		}
		if idx >= len(vectorsResp.Vectors) {
			break
		}
		vec := vectorsResp.Vectors[idx]
		result = append(result, rag.ProjectedVector{
			ID:        vec.ID,
			X:         coords[0],
			Y:         coords[1],
			Content:   vec.Content,
			Metadata:  vec.Metadata,
			Magnitude: vectorMagnitude(vec.Vector),
		})
		idx++
		if i >= len(projection) {
			break
		}
	}

	return &rag.VectorProjectionResponse{
		Vectors:    result,
		Count:      len(result),
		HasMore:    vectorsResp.HasMore,
		NextOffset: vectorsResp.NextOffset,
	}, nil
}

func vectorMagnitude(vec []float32) float64 {
	var sum float64
	for _, v := range vec {
		sum += float64(v) * float64(v)
	}
	return math.Sqrt(sum)
}

func projectTo2D(points [][]float64) [][]float64 {
	rows := len(points)
	if rows == 0 {
		return nil
	}
	cols := len(points[0])
	if cols == 0 {
		return nil
	}

	means := make([]float64, cols)
	for _, point := range points {
		for j, val := range point {
			means[j] += val
		}
	}
	for j := range means {
		means[j] /= float64(rows)
	}

	data := make([]float64, rows*cols)
	for i, point := range points {
		for j := 0; j < cols; j++ {
			data[i*cols+j] = point[j] - means[j]
		}
	}

	matrix := mat.NewDense(rows, cols, data)
	var svd mat.SVD
	if ok := svd.Factorize(matrix, mat.SVDThin); !ok {
		result := make([][]float64, rows)
		for i := range points {
			if cols == 1 {
				result[i] = []float64{points[i][0], 0}
			} else {
				result[i] = []float64{points[i][0], points[i][1]}
			}
		}
		return result
	}

	var v mat.Dense
	svd.VTo(&v)
	targetDims := 2
	if cols < 2 {
		targetDims = 1
	}

	components := v.Slice(0, cols, 0, targetDims)
	var projected mat.Dense
	projected.Mul(matrix, components)

	projData := make([][]float64, rows)
	for i := 0; i < rows; i++ {
		projData[i] = make([]float64, 2)
		projData[i][0] = projected.At(i, 0)
		if targetDims == 2 {
			projData[i][1] = projected.At(i, 1)
		} else {
			projData[i][1] = 0
		}
	}

	return projData
}
