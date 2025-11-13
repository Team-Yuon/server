package http

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"yuon/internal/rag"
	"yuon/internal/rag/search"
	"yuon/internal/rag/service"
)

type DocumentHandler struct {
	service *service.ChatbotService
}

func NewDocumentHandler(service *service.ChatbotService) *DocumentHandler {
	return &DocumentHandler{
		service: service,
	}
}

func (h *DocumentHandler) ListDocuments(c *gin.Context) {
	page := parseQueryInt(c, "page", 1)
	pageSize := parseQueryInt(c, "pageSize", 20)

	params := &rag.DocumentListParams{
		Page:     page,
		PageSize: pageSize,
		Query:    c.Query("q"),
		Category: c.Query("category"),
	}

	result, err := h.service.ListDocuments(c.Request.Context(), params)
	if err != nil {
		InternalServerErrorResponse(c, "문서 목록 조회에 실패했습니다")
		return
	}

	SuccessResponse(c, result)
}

func (h *DocumentHandler) CreateDocument(c *gin.Context) {
	var doc rag.Document
	if err := c.ShouldBindJSON(&doc); err != nil {
		BadRequestResponse(c, "잘못된 문서 형식입니다")
		return
	}

	if doc.ID == "" {
		doc.ID = uuid.New().String()
	}
	ensureMetadata(&doc)

	if err := h.service.AddDocument(c.Request.Context(), doc); err != nil {
		InternalServerErrorResponse(c, "문서 생성에 실패했습니다")
		return
	}

	SuccessResponse(c, gin.H{
		"id":      doc.ID,
		"message": "문서가 성공적으로 추가되었습니다",
	})
}

func (h *DocumentHandler) BulkIngestDocuments(c *gin.Context) {
	var docs []rag.Document
	if err := c.ShouldBindJSON(&docs); err != nil {
		BadRequestResponse(c, "잘못된 문서 형식입니다")
		return
	}

	if len(docs) == 0 {
		BadRequestResponse(c, "문서 목록이 비어 있습니다")
		return
	}

	for i := range docs {
		if docs[i].ID == "" {
			docs[i].ID = uuid.New().String()
		}
		ensureMetadata(&docs[i])
	}

	if err := h.service.BulkAddDocuments(c.Request.Context(), docs); err != nil {
		InternalServerErrorResponse(c, "벌크 문서 추가에 실패했습니다")
		return
	}

	SuccessResponse(c, gin.H{
		"message": "문서가 성공적으로 추가되었습니다",
		"count":   len(docs),
	})
}

func (h *DocumentHandler) GetDocument(c *gin.Context) {
	id := c.Param("id")
	doc, err := h.service.GetDocument(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, search.ErrDocumentNotFound) {
			NotFoundResponse(c, "문서를 찾을 수 없습니다")
			return
		}
		InternalServerErrorResponse(c, "문서 조회에 실패했습니다")
		return
	}

	SuccessResponse(c, doc)
}

func (h *DocumentHandler) UpdateDocument(c *gin.Context) {
	id := c.Param("id")

	var doc rag.Document
	if err := c.ShouldBindJSON(&doc); err != nil {
		BadRequestResponse(c, "잘못된 문서 형식입니다")
		return
	}

	if doc.ID == "" {
		doc.ID = id
	}

	if doc.ID != id {
		BadRequestResponse(c, "요청 경로와 문서 ID가 일치하지 않습니다")
		return
	}

	ensureMetadata(&doc)

	if err := h.service.UpdateDocument(c.Request.Context(), doc); err != nil {
		InternalServerErrorResponse(c, "문서 업데이트에 실패했습니다")
		return
	}

	SuccessResponse(c, gin.H{
		"id":      doc.ID,
		"message": "문서가 성공적으로 업데이트되었습니다",
	})
}

func (h *DocumentHandler) DeleteDocument(c *gin.Context) {
	id := c.Param("id")
	if err := h.service.DeleteDocument(c.Request.Context(), id); err != nil {
		if errors.Is(err, search.ErrDocumentNotFound) {
			NotFoundResponse(c, "문서를 찾을 수 없습니다")
			return
		}
		InternalServerErrorResponse(c, "문서 삭제에 실패했습니다")
		return
	}

	SuccessResponse(c, gin.H{
		"id":      id,
		"message": "문서가 성공적으로 삭제되었습니다",
	})
}

func (h *DocumentHandler) ReindexDocuments(c *gin.Context) {
	var req rag.ReindexRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequestResponse(c, "잘못된 요청 형식입니다")
		return
	}

	if len(req.DocumentIDs) == 0 {
		BadRequestResponse(c, "재색인할 문서 ID를 입력하세요")
		return
	}

	result, err := h.service.ReindexDocuments(c.Request.Context(), req.DocumentIDs)
	if err != nil {
		InternalServerErrorResponse(c, "재색인 작업에 실패했습니다")
		return
	}

	SuccessResponse(c, result)
}

func (h *DocumentHandler) GetStats(c *gin.Context) {
	stats, err := h.service.GetDocumentStats(c.Request.Context())
	if err != nil {
		InternalServerErrorResponse(c, "문서 통계 조회에 실패했습니다")
		return
	}

	SuccessResponse(c, stats)
}

func (h *DocumentHandler) FetchDocumentVector(c *gin.Context) {
	id := c.Param("id")
	withPayload := c.DefaultQuery("withPayload", "true") == "true"

	vector, err := h.service.FetchDocumentVector(c.Request.Context(), id, withPayload)
	if err != nil {
		InternalServerErrorResponse(c, "벡터 조회에 실패했습니다")
		return
	}

	SuccessResponse(c, vector)
}

func (h *DocumentHandler) QueryDocumentVectors(c *gin.Context) {
	req := rag.VectorQueryRequest{
		WithPayload: true,
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequestResponse(c, "잘못된 요청 형식입니다")
		return
	}

	if req.Limit == 0 {
		req.Limit = 50
	}
	if req.Limit > 512 {
		req.Limit = 512
	}

	result, err := h.service.QueryDocumentVectors(c.Request.Context(), &req)
	if err != nil {
		InternalServerErrorResponse(c, "벡터 조회에 실패했습니다")
		return
	}

	SuccessResponse(c, result)
}

func ensureMetadata(doc *rag.Document) {
	if doc.Metadata == nil {
		doc.Metadata = map[string]interface{}{}
	}
}

func parseQueryInt(c *gin.Context, key string, defaultValue int) int {
	val := c.Query(key)
	if val == "" {
		return defaultValue
	}

	if parsed, err := strconv.Atoi(val); err == nil {
		return parsed
	}
	return defaultValue
}
