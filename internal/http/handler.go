package http

import (
	"github.com/gin-gonic/gin"
)

type HealthCheckResponse struct {
	Status      string `json:"status"`
	Message     string `json:"message"`
	Version     string `json:"version"`
	Environment string `json:"environment"`
}

func (r *Router) healthCheck(c *gin.Context) {
	SuccessResponse(c, HealthCheckResponse{
		Status:      "healthy",
		Message:     "서버가 정상 작동 중입니다",
		Version:     r.config.App.Version,
		Environment: r.config.App.Environment,
	})
}
