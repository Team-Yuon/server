package search

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/opensearch-project/opensearch-go/v2"
	"github.com/opensearch-project/opensearch-go/v2/opensearchapi"
	"yuon/configuration"
	"yuon/internal/rag"
)

type OpenSearchClient struct {
	client *opensearch.Client
	index  string
}

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
					"type": "text",
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
