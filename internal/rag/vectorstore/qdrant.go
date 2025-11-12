package vectorstore

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/qdrant/go-client/qdrant"
	"yuon/configuration"
	"yuon/internal/rag"
)

type QdrantClient struct {
	client     *qdrant.Client
	collection string
}

func NewQdrantClient(cfg *configuration.QdrantConfig) (*QdrantClient, error) {
	client, err := qdrant.NewClient(&qdrant.Config{
		Host:   cfg.URL,
		APIKey: cfg.APIKey,
	})
	if err != nil {
		return nil, fmt.Errorf("Qdrant 클라이언트 생성 실패: %w", err)
	}

	qc := &QdrantClient{
		client:     client,
		collection: cfg.Collection,
	}

	if err := qc.ensureCollection(cfg.VectorSize); err != nil {
		return nil, fmt.Errorf("컬렉션 초기화 실패: %w", err)
	}

	return qc, nil
}

func (q *QdrantClient) ensureCollection(vectorSize int) error {
	ctx := context.Background()

	// 컬렉션 생성 시도 (이미 존재하면 무시)
	err := q.client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: q.collection,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     uint64(vectorSize),
			Distance: qdrant.Distance_Cosine,
		}),
	})

	// 이미 존재하는 경우 에러 무시
	if err != nil && !isAlreadyExistsError(err) {
		return fmt.Errorf("컬렉션 생성 실패: %w", err)
	}

	return nil
}

func (q *QdrantClient) AddDocument(ctx context.Context, doc rag.Document, vector []float32) error {
	if doc.ID == "" {
		doc.ID = uuid.New().String()
	}

	payload := map[string]interface{}{
		"content": doc.Content,
		"id":      doc.ID,
	}
	for k, v := range doc.Metadata {
		payload[k] = v
	}

	pointID := hashString(doc.ID)

	_, err := q.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: q.collection,
		Points: []*qdrant.PointStruct{
			{
				Id:      qdrant.NewIDNum(pointID),
				Vectors: qdrant.NewVectors(vector...),
				Payload: qdrant.NewValueMap(payload),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("문서 추가 실패: %w", err)
	}

	return nil
}

func (q *QdrantClient) Search(ctx context.Context, vector []float32, limit int) ([]rag.Document, error) {
	resp, err := q.client.Query(ctx, &qdrant.QueryPoints{
		CollectionName: q.collection,
		Query:          qdrant.NewQuery(vector...),
		Limit:          qdrant.PtrOf(uint64(limit)),
		WithPayload:    qdrant.NewWithPayload(true),
	})
	if err != nil {
		return nil, fmt.Errorf("검색 실패: %w", err)
	}

	var documents []rag.Document
	for _, point := range resp {
		payload := point.GetPayload()

		doc := rag.Document{
			ID:       getStringFromValue(payload["id"]),
			Content:  getStringFromValue(payload["content"]),
			Metadata: make(map[string]interface{}),
			Score:    float64(point.GetScore()),
		}

		// ID가 없으면 point ID 사용
		if doc.ID == "" {
			doc.ID = fmt.Sprintf("%v", point.GetId())
		}

		for key, value := range payload {
			if key != "content" && key != "id" {
				doc.Metadata[key] = extractValue(value)
			}
		}

		documents = append(documents, doc)
	}

	return documents, nil
}

func (q *QdrantClient) Close() error {
	if q.client != nil {
		return q.client.Close()
	}
	return nil
}

func hashString(s string) uint64 {
	var hash uint64 = 5381
	for i := 0; i < len(s); i++ {
		hash = ((hash << 5) + hash) + uint64(s[i])
	}
	return hash
}

func getStringFromValue(value *qdrant.Value) string {
	if value == nil {
		return ""
	}
	if strVal := value.GetStringValue(); strVal != "" {
		return strVal
	}
	return ""
}

func extractValue(value *qdrant.Value) interface{} {
	if value == nil {
		return nil
	}

	switch {
	case value.GetStringValue() != "":
		return value.GetStringValue()
	case value.GetIntegerValue() != 0:
		return value.GetIntegerValue()
	case value.GetDoubleValue() != 0:
		return value.GetDoubleValue()
	case value.GetBoolValue():
		return value.GetBoolValue()
	default:
		return nil
	}
}

func isAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}
	return contains(err.Error(), "already exists") || contains(err.Error(), "AlreadyExists")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && hasSubstring(s, substr))
}

func hasSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
