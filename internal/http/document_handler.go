package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"yuon/internal/rag"
	"yuon/internal/rag/search"
	"yuon/internal/rag/service"
	"yuon/internal/storage"
	"yuon/internal/textextract"
)

type DocumentHandler struct {
	service *service.ChatbotService
	storage storage.FileStorage
}

func NewDocumentHandler(service *service.ChatbotService, storage storage.FileStorage) *DocumentHandler {
	return &DocumentHandler{
		service: service,
		storage: storage,
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

	for i := range result.Documents {
		populateFileFields(&result.Documents[i])
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
		c.Error(err) // Log the actual error
		InternalServerErrorResponse(c, fmt.Sprintf("문서 생성에 실패했습니다: %v", err))
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

	populateFileFields(doc)
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

func (h *DocumentHandler) ProjectVectors(c *gin.Context) {
	var req rag.VectorProjectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequestResponse(c, "잘못된 요청 형식입니다")
		return
	}

	if req.Limit == 0 {
		req.Limit = 200
	}

	result, err := h.service.ProjectVectors(c.Request.Context(), &req)
	if err != nil {
		InternalServerErrorResponse(c, "벡터 프로젝션에 실패했습니다")
		return
	}

	SuccessResponse(c, result)
}

func (h *DocumentHandler) DownloadDocumentFile(c *gin.Context) {
	if h.storage == nil {
		InternalServerErrorResponse(c, "파일 저장소가 구성되지 않았습니다")
		return
	}

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

	fileKey, _ := doc.Metadata["fileKey"].(string)
	if fileKey == "" {
		NotFoundResponse(c, "해당 문서에는 원본 파일이 없습니다")
		return
	}

	data, contentType, err := h.storage.Download(c.Request.Context(), fileKey)
	if err != nil {
		InternalServerErrorResponse(c, "파일 다운로드에 실패했습니다")
		return
	}

	filename := "download"
	if name, ok := doc.Metadata["filename"].(string); ok && name != "" {
		filename = name
	}

	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	c.Data(http.StatusOK, contentType, data)
}

const maxUploadSize = 20 * 1024 * 1024

func (h *DocumentHandler) UploadDocument(c *gin.Context) {
	if h.storage == nil {
		InternalServerErrorResponse(c, "파일 저장소가 구성되지 않았습니다")
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		BadRequestResponse(c, "file 필드를 포함한 multipart/form-data 요청이 필요합니다")
		return
	}
	defer file.Close()

	data, err := readFileWithLimit(file, maxUploadSize)
	if err != nil {
		BadRequestResponse(c, err.Error())
		return
	}

	filename := header.Filename
	if filename == "" {
		filename = fmt.Sprintf("upload-%s", uuid.New().String())
	}

	text, err := textextract.ExtractText(filename, data)
	if err != nil {
		BadRequestResponse(c, err.Error())
		return
	}

	metadata := make(map[string]interface{})
	if raw := c.PostForm("metadata"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
			BadRequestResponse(c, "metadata 필드는 올바른 JSON 이어야 합니다")
			return
		}
	}

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}

	key := fmt.Sprintf("documents/%s/%s", time.Now().UTC().Format("20060102"), uuid.New().String()+strings.ToLower(filepath.Ext(filename)))
	url, err := h.storage.Upload(c.Request.Context(), key, data, contentType)
	if err != nil {
		InternalServerErrorResponse(c, fmt.Sprintf("파일 업로드 실패: %v", err))
		return
	}

	metadata["fileKey"] = key
	metadata["fileUrl"] = url
	metadata["filename"] = filename
	metadata["contentType"] = contentType
	metadata["uploadedAt"] = time.Now().UTC().Format(time.RFC3339)

	docID := c.PostForm("documentId")
	if docID == "" {
		docID = uuid.New().String()
	}

	doc := rag.Document{
		ID:       docID,
		Content:  text,
		Metadata: metadata,
	}

	if err := h.service.AddDocument(c.Request.Context(), doc); err != nil {
		c.Error(err) // Log the actual error
		InternalServerErrorResponse(c, fmt.Sprintf("문서 생성에 실패했습니다: %v", err))
		return
	}

	SuccessResponse(c, gin.H{
		"message":  "파일이 업로드되고 문서가 생성되었습니다",
		"id":       doc.ID,
		"fileUrl":  url,
		"fileKey":  key,
		"fileName": filename,
	})
}

func readFileWithLimit(file multipart.File, limit int) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	if _, err := io.CopyN(buf, file, int64(limit)+1); err != nil && err != io.EOF {
		return nil, fmt.Errorf("파일을 읽는 중 오류가 발생했습니다: %w", err)
	}
	if buf.Len() > limit {
		return nil, fmt.Errorf("파일 크기가 %dMB를 초과합니다", limit/1024/1024)
	}
	return buf.Bytes(), nil
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

func populateFileFields(doc *rag.Document) {
	if doc == nil || doc.Metadata == nil {
		return
	}
	if v, ok := doc.Metadata["fileKey"].(string); ok {
		doc.FileKey = v
	}
	if v, ok := doc.Metadata["fileUrl"].(string); ok {
		doc.FileURL = v
	}
}
