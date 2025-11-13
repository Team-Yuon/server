package search

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/opensearch-project/opensearch-go/v2"
	"github.com/opensearch-project/opensearch-go/v2/opensearchapi"
	"yuon/configuration"
	"yuon/internal/rag"
)

type OpenSearchClient struct {
	client *opensearch.Client
	index  string
}

var ErrDocumentNotFound = errors.New("document not found")

func NewOpenSearchClient(cfg *configuration.OpenSearchConfig) (*OpenSearchClient, error) {
	client, err := opensearch.NewClient(opensearch.Config{
		Addresses: []string{cfg.URL},
		Username:  cfg.Username,
		Password:  cfg.Password,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("OpenSearch 클라이언트 생성 실패: %w", err)
	}

	osc := &OpenSearchClient{
		client: client,
		index:  cfg.Index,
	}

	if err := osc.ensureIndex(); err != nil {
		return nil, fmt.Errorf("인덱스 초기화 실패: %w", err)
	}

	return osc, nil
}

func (o *OpenSearchClient) ensureIndex() error {
	ctx := context.Background()

	exists := opensearchapi.IndicesExistsRequest{
		Index: []string{o.index},
	}

	res, err := exists.Do(ctx, o.client)
	if err != nil {
		return fmt.Errorf("인덱스 확인 실패: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode == 200 {
		return nil
	}

	mapping := map[string]interface{}{
		"mappings": map[string]interface{}{
			"properties": map[string]interface{}{
				"content": map[string]interface{}{
					"type":     "text",
					"analyzer": "standard",
				},
				"metadata": map[string]interface{}{
					"type": "object",
				},
			},
		},
	}

	body, _ := json.Marshal(mapping)
	create := opensearchapi.IndicesCreateRequest{
		Index: o.index,
		Body:  bytes.NewReader(body),
	}

	res, err = create.Do(ctx, o.client)
	if err != nil {
		return fmt.Errorf("인덱스 생성 실패: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("인덱스 생성 오류: %s", res.String())
	}

	return nil
}

func (o *OpenSearchClient) AddDocument(ctx context.Context, doc rag.Document) error {
	body := map[string]interface{}{
		"content":  doc.Content,
		"metadata": doc.Metadata,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("문서 직렬화 실패: %w", err)
	}

	req := opensearchapi.IndexRequest{
		Index:      o.index,
		DocumentID: doc.ID,
		Body:       bytes.NewReader(data),
		Refresh:    "true",
	}

	res, err := req.Do(ctx, o.client)
	if err != nil {
		return fmt.Errorf("문서 추가 실패: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("문서 추가 오류: %s", res.String())
	}

	return nil
}

func (o *OpenSearchClient) Search(ctx context.Context, query string, limit int) ([]rag.Document, error) {
	searchQuery := map[string]interface{}{
		"query": map[string]interface{}{
			"match": map[string]interface{}{
				"content": query,
			},
		},
		"size": limit,
	}

	body, err := json.Marshal(searchQuery)
	if err != nil {
		return nil, fmt.Errorf("쿼리 직렬화 실패: %w", err)
	}

	req := opensearchapi.SearchRequest{
		Index: []string{o.index},
		Body:  bytes.NewReader(body),
	}

	res, err := req.Do(ctx, o.client)
	if err != nil {
		return nil, fmt.Errorf("검색 실패: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("검색 오류: %s", res.String())
	}

	var result map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("응답 파싱 실패: %w", err)
	}

	hits := result["hits"].(map[string]interface{})["hits"].([]interface{})

	var documents []rag.Document
	for _, hit := range hits {
		h := hit.(map[string]interface{})
		source := h["_source"].(map[string]interface{})

		doc := rag.Document{
			ID:      h["_id"].(string),
			Content: source["content"].(string),
			Score:   h["_score"].(float64),
		}

		if meta, ok := source["metadata"].(map[string]interface{}); ok {
			doc.Metadata = meta
		}

		documents = append(documents, doc)
	}

	return documents, nil
}

func (o *OpenSearchClient) BulkIndex(ctx context.Context, documents []rag.Document) error {
	var buf bytes.Buffer

	for _, doc := range documents {
		meta := map[string]interface{}{
			"index": map[string]interface{}{
				"_index": o.index,
				"_id":    doc.ID,
			},
		}
		metaJSON, _ := json.Marshal(meta)
		buf.Write(metaJSON)
		buf.WriteByte('\n')

		body := map[string]interface{}{
			"content":  doc.Content,
			"metadata": doc.Metadata,
		}
		bodyJSON, _ := json.Marshal(body)
		buf.Write(bodyJSON)
		buf.WriteByte('\n')
	}

	req := opensearchapi.BulkRequest{
		Body:    strings.NewReader(buf.String()),
		Refresh: "true",
	}

	res, err := req.Do(ctx, o.client)
	if err != nil {
		return fmt.Errorf("벌크 인덱싱 실패: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("벌크 인덱싱 오류: %s", res.String())
	}

	return nil
}

func (o *OpenSearchClient) ListDocuments(ctx context.Context, params *rag.DocumentListParams) (*rag.DocumentListResult, error) {
	page := 1
	pageSize := 20
	if params != nil {
		if params.Page > 0 {
			page = params.Page
		}
		if params.PageSize > 0 && params.PageSize <= 100 {
			pageSize = params.PageSize
		} else if params.PageSize > 100 {
			pageSize = 100
		}
	}

	from := (page - 1) * pageSize

	query := map[string]interface{}{
		"from": from,
		"size": pageSize,
		"sort": []interface{}{
			map[string]interface{}{
				"_score": map[string]interface{}{
					"order": "desc",
				},
			},
		},
		"query": map[string]interface{}{
			"match_all": map[string]interface{}{},
		},
	}

	if params != nil {
		var must []map[string]interface{}
		if params.Query != "" {
			must = append(must, map[string]interface{}{
				"match": map[string]interface{}{
					"content": params.Query,
				},
			})
		}
		if params.Category != "" {
			must = append(must, map[string]interface{}{
				"match": map[string]interface{}{
					"metadata.category": params.Category,
				},
			})
		}

		if len(must) > 0 {
			query["query"] = map[string]interface{}{
				"bool": map[string]interface{}{
					"must": must,
				},
			}
		}
	}

	body, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("문서 목록 쿼리 직렬화 실패: %w", err)
	}

	req := opensearchapi.SearchRequest{
		Index: []string{o.index},
		Body:  bytes.NewReader(body),
	}

	res, err := req.Do(ctx, o.client)
	if err != nil {
		return nil, fmt.Errorf("문서 목록 조회 실패: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("문서 목록 조회 오류: %s", res.String())
	}

	var result map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("문서 목록 응답 파싱 실패: %w", err)
	}

	hitsData, ok := result["hits"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("문서 목록 응답 형식이 잘못되었습니다")
	}

	totalVal := int64(0)
	if totalInfo, ok := hitsData["total"].(map[string]interface{}); ok {
		switch v := totalInfo["value"].(type) {
		case float64:
			totalVal = int64(v)
		case int64:
			totalVal = v
		}
	}

	documents := extractDocumentsFromHits(hitsData)
	hasNext := int64(from+pageSize) < totalVal

	return &rag.DocumentListResult{
		Documents: documents,
		Total:     totalVal,
		Page:      page,
		PageSize:  pageSize,
		HasNext:   hasNext,
	}, nil
}

func (o *OpenSearchClient) GetDocument(ctx context.Context, id string) (*rag.Document, error) {
	req := opensearchapi.GetRequest{
		Index:      o.index,
		DocumentID: id,
	}

	res, err := req.Do(ctx, o.client)
	if err != nil {
		return nil, fmt.Errorf("문서 조회 실패: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusNotFound {
		return nil, ErrDocumentNotFound
	}

	if res.IsError() {
		return nil, fmt.Errorf("문서 조회 오류: %s", res.String())
	}

	var result map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("문서 조회 응답 파싱 실패: %w", err)
	}

	source, ok := result["_source"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("문서 응답 형식이 잘못되었습니다")
	}

	doc := rag.Document{
		ID:      result["_id"].(string),
		Content: getStringValue(source["content"]),
	}

	if metadata, ok := source["metadata"].(map[string]interface{}); ok {
		doc.Metadata = metadata
	}

	return &doc, nil
}

func (o *OpenSearchClient) UpdateDocument(ctx context.Context, doc rag.Document) error {
	return o.AddDocument(ctx, doc)
}

func (o *OpenSearchClient) DeleteDocument(ctx context.Context, id string) error {
	req := opensearchapi.DeleteRequest{
		Index:      o.index,
		DocumentID: id,
		Refresh:    "true",
	}

	res, err := req.Do(ctx, o.client)
	if err != nil {
		return fmt.Errorf("문서 삭제 실패: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusNotFound {
		return ErrDocumentNotFound
	}

	if res.IsError() {
		return fmt.Errorf("문서 삭제 오류: %s", res.String())
	}

	return nil
}

func (o *OpenSearchClient) FetchDocuments(ctx context.Context, ids []string) ([]rag.Document, error) {
	if len(ids) == 0 {
		return []rag.Document{}, nil
	}

	payload := map[string]interface{}{
		"ids": ids,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("문서 Fetch 직렬화 실패: %w", err)
	}

	req := opensearchapi.MgetRequest{
		Index: o.index,
		Body:  bytes.NewReader(body),
	}

	res, err := req.Do(ctx, o.client)
	if err != nil {
		return nil, fmt.Errorf("문서 Fetch 실패: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("문서 Fetch 오류: %s", res.String())
	}

	var result struct {
		Docs []struct {
			ID      string                 `json:"_id"`
			Found   bool                   `json:"found"`
			Source  map[string]interface{} `json:"_source"`
			Error   interface{}            `json:"error"`
			Status  int                    `json:"status"`
			Index   string                 `json:"_index"`
			Version int64                  `json:"_version"`
		} `json:"docs"`
	}

	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("문서 Fetch 응답 파싱 실패: %w", err)
	}

	var documents []rag.Document
	for _, doc := range result.Docs {
		if !doc.Found || doc.Source == nil {
			continue
		}
		item := rag.Document{
			ID:      doc.ID,
			Content: getStringValue(doc.Source["content"]),
		}
		if metadata, ok := doc.Source["metadata"].(map[string]interface{}); ok {
			item.Metadata = metadata
		}
		documents = append(documents, item)
	}

	return documents, nil
}

func (o *OpenSearchClient) GetStats(ctx context.Context) (*rag.DocumentStats, error) {
	req := opensearchapi.CountRequest{
		Index: []string{o.index},
	}

	res, err := req.Do(ctx, o.client)
	if err != nil {
		return nil, fmt.Errorf("문서 통계 조회 실패: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("문서 통계 조회 오류: %s", res.String())
	}

	var result struct {
		Count int64 `json:"count"`
	}

	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("문서 통계 응답 파싱 실패: %w", err)
	}

	return &rag.DocumentStats{
		TotalDocuments: result.Count,
		Index:          o.index,
		LastUpdatedAt:  time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func extractDocumentsFromHits(hits map[string]interface{}) []rag.Document {
	itemsRaw, ok := hits["hits"].([]interface{})
	if !ok {
		return nil
	}

	var documents []rag.Document
	for _, hit := range itemsRaw {
		h, ok := hit.(map[string]interface{})
		if !ok {
			continue
		}

		source, ok := h["_source"].(map[string]interface{})
		if !ok {
			continue
		}

		doc := rag.Document{
			ID:      getStringValue(h["_id"]),
			Content: getStringValue(source["content"]),
			Score:   getFloatValue(h["_score"]),
		}

		if metadata, ok := source["metadata"].(map[string]interface{}); ok {
			doc.Metadata = metadata
		}

		documents = append(documents, doc)
	}

	return documents
}

func getStringValue(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return ""
	}
}

func getFloatValue(value interface{}) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int64:
		return float64(v)
	case int:
		return float64(v)
	default:
		return 0
	}
}
