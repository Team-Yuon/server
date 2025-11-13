package vectorstore

import (
	"context"
	"fmt"
	"strconv"
	"strings"

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

func (q *QdrantClient) DeleteDocument(ctx context.Context, docID string) error {
	pointID := hashString(docID)

	_, err := q.client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: q.collection,
		Points:         qdrant.NewPointsSelector(qdrant.NewIDNum(pointID)),
	})
	if err != nil {
		return fmt.Errorf("Qdrant 문서 삭제 실패: %w", err)
	}

	return nil
}

func (q *QdrantClient) GetDocumentVector(ctx context.Context, docID string, withPayload bool) (*rag.DocumentVector, error) {
	pointID := hashString(docID)

	points, err := q.client.Get(ctx, &qdrant.GetPoints{
		CollectionName: q.collection,
		Ids:            []*qdrant.PointId{qdrant.NewIDNum(pointID)},
		WithVectors:    qdrant.NewWithVectors(true),
		WithPayload:    qdrant.NewWithPayload(withPayload),
	})
	if err != nil {
		return nil, fmt.Errorf("Qdrant 벡터 조회 실패: %w", err)
	}

	if len(points) == 0 {
		return nil, fmt.Errorf("벡터를 찾을 수 없습니다")
	}

	vector := convertPointToDocumentVector(points[0], withPayload)
	return &vector, nil
}

func (q *QdrantClient) QueryDocumentVectors(ctx context.Context, docIDs []string, limit int, withPayload bool, offset string) ([]rag.DocumentVector, bool, string, error) {
	if len(docIDs) > 0 {
		return q.getVectorsByIDs(ctx, docIDs, withPayload)
	}

	if limit <= 0 {
		limit = 50
	}
	if limit > 512 {
		limit = 512
	}

	scrollReq := &qdrant.ScrollPoints{
		CollectionName: q.collection,
		Limit:          qdrant.PtrOf(uint32(limit)),
		WithVectors:    qdrant.NewWithVectors(true),
		WithPayload:    qdrant.NewWithPayload(withPayload),
	}

	if offset != "" {
		if pointID, err := parsePointID(offset); err == nil && pointID != nil {
			scrollReq.Offset = pointID
		}
	}

	points, nextOffset, err := q.client.ScrollAndOffset(ctx, scrollReq)
	if err != nil {
		return nil, false, "", fmt.Errorf("Qdrant 벡터 스크롤 실패: %w", err)
	}

	var vectors []rag.DocumentVector
	for _, point := range points {
		vectors = append(vectors, convertPointToDocumentVector(point, withPayload))
	}

	hasMore := nextOffset != nil
	nextOffsetStr := ""
	if nextOffset != nil {
		nextOffsetStr = pointIDToString(nextOffset)
	}

	return vectors, hasMore, nextOffsetStr, nil
}

func (q *QdrantClient) getVectorsByIDs(ctx context.Context, docIDs []string, withPayload bool) ([]rag.DocumentVector, bool, string, error) {
	var ids []*qdrant.PointId
	for _, id := range docIDs {
		ids = append(ids, qdrant.NewIDNum(hashString(id)))
	}

	points, err := q.client.Get(ctx, &qdrant.GetPoints{
		CollectionName: q.collection,
		Ids:            ids,
		WithVectors:    qdrant.NewWithVectors(true),
		WithPayload:    qdrant.NewWithPayload(withPayload),
	})
	if err != nil {
		return nil, false, "", fmt.Errorf("Qdrant 벡터 조회 실패: %w", err)
	}

	var vectors []rag.DocumentVector
	for _, point := range points {
		vectors = append(vectors, convertPointToDocumentVector(point, withPayload))
	}

	return vectors, false, "", nil
}

func convertPointToDocumentVector(point *qdrant.RetrievedPoint, withPayload bool) rag.DocumentVector {
	vector := rag.DocumentVector{
		ID: pointIDToString(point.GetId()),
	}

	vector.Vector = extractVector(point)

	if withPayload {
		payloadMap := make(map[string]interface{})
		for key, value := range point.GetPayload() {
			payloadMap[key] = extractValue(value)
		}

		if id, ok := payloadMap["id"].(string); ok && id != "" {
			vector.ID = id
		}
		if content, ok := payloadMap["content"].(string); ok {
			vector.Content = content
			delete(payloadMap, "content")
		}
		delete(payloadMap, "id")

		if len(payloadMap) > 0 {
			vector.Metadata = payloadMap
		}
	}

	return vector
}

func extractVector(point *qdrant.RetrievedPoint) []float32 {
	vectors := point.GetVectors()
	if vectors == nil {
		return nil
	}

	if single := vectors.GetVector(); single != nil {
		if dense := single.GetDense(); dense != nil && len(dense.GetData()) > 0 {
			data := dense.GetData()
			copied := make([]float32, len(data))
			copy(copied, data)
			return copied
		}
		if data := single.GetData(); len(data) > 0 {
			copied := make([]float32, len(data))
			copy(copied, data)
			return copied
		}
	}

	if named := vectors.GetVectors(); named != nil {
		for _, vector := range named.GetVectors() {
			if dense := vector.GetDense(); dense != nil && len(dense.GetData()) > 0 {
				data := dense.GetData()
				copied := make([]float32, len(data))
				copy(copied, data)
				return copied
			}
			if data := vector.GetData(); len(data) > 0 {
				copied := make([]float32, len(data))
				copy(copied, data)
				return copied
			}
		}
	}

	return nil
}

func pointIDToString(id *qdrant.PointId) string {
	if id == nil {
		return ""
	}
	switch v := id.PointIdOptions.(type) {
	case *qdrant.PointId_Num:
		return fmt.Sprintf("%d", v.Num)
	case *qdrant.PointId_Uuid:
		return v.Uuid
	default:
		return ""
	}
}

func parsePointID(raw string) (*qdrant.PointId, error) {
	if raw == "" {
		return nil, nil
	}

	if strings.Contains(raw, "-") {
		return qdrant.NewID(raw), nil
	}

	num, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return nil, err
	}
	return qdrant.NewIDNum(num), nil
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
