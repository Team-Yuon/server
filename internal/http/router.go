package http

import (
	"net/http"

	"yuon/configuration"
	"yuon/internal/auth"
	"yuon/internal/rag/service"

	"github.com/gin-gonic/gin"
)

type Router struct {
	engine         *gin.Engine
	config         *configuration.Config
	chatbotService *service.ChatbotService
	authManager    *auth.Manager
}

func NewRouter(cfg *configuration.Config, authManager *auth.Manager) *Router {
	setGinMode(cfg.Server.Mode)

	engine := gin.New()
	engine.Use(slogMiddleware())
	engine.Use(recoveryMiddleware())
	engine.Use(corsMiddleware())

	return &Router{
		engine:      engine,
		config:      cfg,
		authManager: authManager,
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
	if r.authManager == nil {
		panic("auth manager is not configured")
	}

	r.registerSwaggerRoutes()

	v1 := r.engine.Group("/api/v1")
	{
		v1.GET("/health", r.healthCheck)
		v1.GET("/system/health", r.healthCheck)

		authHandler := NewAuthHandler(r.authManager)
		v1.POST("/auth/signup", authHandler.Signup)
		v1.POST("/auth/login", authHandler.Login)

		wsHandler := NewWebSocketHandler(r.chatbotService)
		v1.GET("/ws", wsHandler.Handle)

		documents := NewDocumentHandler(r.chatbotService)

		docGroup := v1.Group("/documents")
		docGroup.Use(authMiddleware(r.authManager))
		{
			docGroup.GET("", documents.ListDocuments)
			docGroup.GET("/stats", documents.GetStats)
			docGroup.POST("", documents.CreateDocument)
			docGroup.POST("/bulk-ingest", documents.BulkIngestDocuments)
			docGroup.POST("/bulk", documents.BulkIngestDocuments)
			docGroup.POST("/reindex", documents.ReindexDocuments)
			docGroup.POST("/vectors/query", documents.QueryDocumentVectors)
			docGroup.POST("/vectors/projection", documents.ProjectVectors)
			docGroup.GET("/:id/vector", documents.FetchDocumentVector)
			docGroup.GET("/:id", documents.GetDocument)
			docGroup.PUT("/:id", documents.UpdateDocument)
			docGroup.DELETE("/:id", documents.DeleteDocument)
		}
	}
}

func (r *Router) registerSwaggerRoutes() {
	r.engine.StaticFile("/docs/openapi.yaml", "docs/openapi.yaml")
	r.engine.GET("/docs", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(swaggerHTML))
	})
}

const swaggerHTML = `<!DOCTYPE html>
<html lang="ko">
  <head>
    <meta charset="UTF-8" />
    <title>YUON API Docs</title>
    <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css" />
    <style>
      body { margin: 0; background: #0f172a; }
      #swagger-ui { max-width: 960px; margin: 0 auto; }
    </style>
  </head>
  <body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js" crossorigin></script>
    <script>
      window.onload = () => {
        SwaggerUIBundle({
          url: '/docs/openapi.yaml',
          dom_id: '#swagger-ui',
        });
      };
    </script>
  </body>
</html>`

func (r *Router) Run(addr string) error {
	return r.engine.Run(addr)
}

func (r *Router) Engine() *gin.Engine {
	return r.engine
}
