package http

import (
	"github.com/gin-gonic/gin"
	"yuon/configuration"
	"yuon/internal/rag/service"
)

type Router struct {
	engine         *gin.Engine
	config         *configuration.Config
	chatbotService *service.ChatbotService
}

func NewRouter(cfg *configuration.Config) *Router {
	setGinMode(cfg.Server.Mode)

	engine := gin.New()
	engine.Use(slogMiddleware())
	engine.Use(recoveryMiddleware())
	engine.Use(corsMiddleware())

	return &Router{
		engine: engine,
		config: cfg,
	}
}

func (r *Router) SetChatbotService(service *service.ChatbotService) {
	r.chatbotService = service
}

func setGinMode(mode string) {
	if mode == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}
}

func (r *Router) SetupRoutes() {
	v1 := r.engine.Group("/api/v1")
	{
		v1.GET("/health", r.healthCheck)

		// 챗봇 관련 라우트
		if r.chatbotService != nil {
			chatbot := NewChatbotHandler(r.chatbotService)
			chatGroup := v1.Group("/chat")
			{
				chatGroup.POST("", chatbot.Chat)
				chatGroup.POST("/simple", chatbot.SimpleChat)
			}

			// 문서 관리
			docGroup := v1.Group("/documents")
			{
				docGroup.POST("", chatbot.AddDocument)
				docGroup.POST("/bulk", chatbot.BulkAddDocuments)
			}
		}
	}
}

func (r *Router) Run(addr string) error {
	return r.engine.Run(addr)
}

func (r *Router) Engine() *gin.Engine {
	return r.engine
}
