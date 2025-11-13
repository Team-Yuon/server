package http

import (
	"yuon/configuration"
	"yuon/internal/rag/service"

	"github.com/gin-gonic/gin"
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
	if r.chatbotService == nil {
		panic("chatbot service is not configured; call SetChatbotService before SetupRoutes")
	}

	v1 := r.engine.Group("/api/v1")
	{
		v1.GET("/health", r.healthCheck)
		v1.GET("/system/health", r.healthCheck)

		chatbot := NewChatbotHandler(r.chatbotService)
		documents := NewDocumentHandler(r.chatbotService)

		chatGroup := v1.Group("/chat")
		{
			chatGroup.POST("", chatbot.Chat)
			chatGroup.POST("/simple", chatbot.SimpleChat)
		}

		docGroup := v1.Group("/documents")
		{
			docGroup.GET("", documents.ListDocuments)
			docGroup.GET("/stats", documents.GetStats)
			docGroup.POST("", documents.CreateDocument)
			docGroup.POST("/bulk-ingest", documents.BulkIngestDocuments)
			docGroup.POST("/bulk", documents.BulkIngestDocuments)
			docGroup.POST("/reindex", documents.ReindexDocuments)
			docGroup.POST("/vectors/query", documents.QueryDocumentVectors)
			docGroup.GET("/:id/vector", documents.FetchDocumentVector)
			docGroup.GET("/:id", documents.GetDocument)
			docGroup.PUT("/:id", documents.UpdateDocument)
			docGroup.DELETE("/:id", documents.DeleteDocument)
		}
	}
}

func (r *Router) Run(addr string) error {
	return r.engine.Run(addr)
}

func (r *Router) Engine() *gin.Engine {
	return r.engine
}
