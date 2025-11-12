package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"yuon/internal/rag"
	"yuon/internal/rag/service"
)

type ChatbotHandler struct {
	service *service.ChatbotService
}

func NewChatbotHandler(service *service.ChatbotService) *ChatbotHandler {
	return &ChatbotHandler{
		service: service,
	}
}

func (h *ChatbotHandler) Chat(c *gin.Context) {
	var req rag.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequestResponse(c, "잘못된 요청 형식입니다")
		return
	}

	// 기본값 설정
	if !req.UseVectorSearch && !req.UseFullText {
		req.UseVectorSearch = true
		req.UseFullText = true
	}

	resp, err := h.service.Chat(c.Request.Context(), &req)
	if err != nil {
		InternalServerErrorResponse(c, "챗봇 응답 생성에 실패했습니다")
		return
	}

	SuccessResponse(c, resp)
}

func (h *ChatbotHandler) AddDocument(c *gin.Context) {
	var doc rag.Document
	if err := c.ShouldBindJSON(&doc); err != nil {
		BadRequestResponse(c, "잘못된 요청 형식입니다")
		return
	}

	if err := h.service.AddDocument(c.Request.Context(), doc); err != nil {
		InternalServerErrorResponse(c, "문서 추가에 실패했습니다")
		return
	}

	SuccessResponse(c, gin.H{
		"message": "문서가 성공적으로 추가되었습니다",
		"id":      doc.ID,
	})
}

func (h *ChatbotHandler) BulkAddDocuments(c *gin.Context) {
	var docs []rag.Document
	if err := c.ShouldBindJSON(&docs); err != nil {
		BadRequestResponse(c, "잘못된 요청 형식입니다")
		return
	}

	if len(docs) == 0 {
		BadRequestResponse(c, "문서가 비어있습니다")
		return
	}

	if err := h.service.BulkAddDocuments(c.Request.Context(), docs); err != nil {
		InternalServerErrorResponse(c, "문서 추가에 실패했습니다")
		return
	}

	SuccessResponse(c, gin.H{
		"message": "문서가 성공적으로 추가되었습니다",
		"count":   len(docs),
	})
}

func (h *ChatbotHandler) SimpleChat(c *gin.Context) {
	var req struct {
		Message string `json:"message" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequestResponse(c, "메시지를 입력해주세요")
		return
	}

	chatReq := &rag.ChatRequest{
		Message:         req.Message,
		UseVectorSearch: true,
		UseFullText:     true,
		TopK:            5,
	}

	resp, err := h.service.Chat(c.Request.Context(), chatReq)
	if err != nil {
		InternalServerErrorResponse(c, "챗봇 응답 생성에 실패했습니다")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"answer": resp.Answer,
		"sources": resp.Sources,
	})
}
