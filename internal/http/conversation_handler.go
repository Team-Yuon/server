package http

import (
	"github.com/gin-gonic/gin"
	"yuon/internal/rag/service"
)

type ConversationHandler struct {
	service *service.ChatbotService
}

func NewConversationHandler(svc *service.ChatbotService) *ConversationHandler {
	return &ConversationHandler{service: svc}
}

func (h *ConversationHandler) List(c *gin.Context) {
	if h.service == nil {
		InternalServerErrorResponse(c, "대화 서비스가 구성되지 않았습니다")
		return
	}
	items, err := h.service.ListConversationSummaries(c.Request.Context(), 100)
	if err != nil {
		InternalServerErrorResponse(c, "대화 목록을 불러오지 못했습니다")
		return
	}

	var resp []gin.H
	for _, item := range items {
		resp = append(resp, gin.H{
			"id":           item.ID,
			"preview":      item.Preview,
			"messageCount": item.MessageCount,
			"createdAt":    item.CreatedAt,
			"tokenUsage":   item.TokenUsage,
		})
	}

	SuccessResponse(c, gin.H{
		"conversations": resp,
	})
}

func (h *ConversationHandler) Detail(c *gin.Context) {
	if h.service == nil {
		InternalServerErrorResponse(c, "대화 서비스가 구성되지 않았습니다")
		return
	}

	id := c.Param("id")
	messages, err := h.service.GetConversationMessages(c.Request.Context(), id)
	if err != nil {
		InternalServerErrorResponse(c, "대화 상세를 불러오지 못했습니다")
		return
	}

	var resp []gin.H
	for _, m := range messages {
		resp = append(resp, gin.H{
			"role":      m.Role,
			"content":   m.Content,
			"timestamp": m.Timestamp,
		})
	}

	SuccessResponse(c, gin.H{
		"id":       id,
		"messages": resp,
	})
}

func (h *ConversationHandler) Delete(c *gin.Context) {
	if h.service == nil {
		InternalServerErrorResponse(c, "대화 서비스가 구성되지 않았습니다")
		return
	}

	id := c.Param("id")
	if id == "" {
		BadRequestResponse(c, "대화 ID가 필요합니다")
		return
	}

	if err := h.service.DeleteConversation(c.Request.Context(), id); err != nil {
		InternalServerErrorResponse(c, err.Error())
		return
	}

	SuccessResponse(c, gin.H{
		"message": "대화가 삭제되었습니다",
	})
}
