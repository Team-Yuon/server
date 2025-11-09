package http

import (
	"github.com/gin-gonic/gin"
	"yuon/configuration"
)

type Router struct {
	engine *gin.Engine
	config *configuration.Config
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
	}
}

func (r *Router) Run(addr string) error {
	return r.engine.Run(addr)
}

func (r *Router) Engine() *gin.Engine {
	return r.engine
}
