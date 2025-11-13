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
		"answer":  resp.Answer,
		"sources": resp.Sources,
	})
}
