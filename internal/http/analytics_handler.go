package http

import (
	"github.com/gin-gonic/gin"
	"yuon/internal/rag/service"
)

type AnalyticsHandler struct {
	service *service.ChatbotService
}

func NewAnalyticsHandler(service *service.ChatbotService) *AnalyticsHandler {
	return &AnalyticsHandler{service: service}
}

func (h *AnalyticsHandler) ChatStats(c *gin.Context) {
	stats := h.service.GetAnalyticsStats()
	SuccessResponse(c, stats)
}

func (h *AnalyticsHandler) KnowledgeNeed(c *gin.Context) {
	analysis, err := h.service.GenerateKnowledgeNeedAnalysis(c.Request.Context())
	if err != nil {
		InternalServerErrorResponse(c, "분석 생성에 실패했습니다")
		return
	}
	SuccessResponse(c, gin.H{
		"analysis": analysis,
	})
}
