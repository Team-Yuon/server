package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"yuon/configuration"
	"yuon/internal/auth"
	"yuon/internal/database"
	httpserver "yuon/internal/http"
	"yuon/internal/rag/llm"
	"yuon/internal/rag/search"
	"yuon/internal/rag/service"
	"yuon/internal/rag/vectorstore"
	"yuon/internal/storage"
	"yuon/package/logger"
	"yuon/package/validator"
)

func main() {
	banner()

	cfg, err := configuration.Load()
	if err != nil {
		slog.Error("설정 로드 실패", "error", err)
		os.Exit(1)
	}

	logger.New(cfg.App.Environment)
	validator.Init()

	logConfig(cfg)

	db, err := database.Connect(&cfg.Database)
	if err != nil {
		slog.Error("데이터베이스 연결 실패", "error", err)
		os.Exit(1)
	}
	defer safeClose(db)

	if err := database.EnsureSchemas(db); err != nil {
		slog.Error("DB 스키마 초기화 실패", "error", err)
		os.Exit(1)
	}

	if cfg.Auth.RootPassword == "" {
		slog.Error("ROOT_ADMIN_PASSWORD 환경 변수가 설정되어 있지 않습니다")
		os.Exit(1)
	}
	if cfg.Auth.JWTSecret == "" {
		slog.Error("JWT_SECRET 환경 변수가 설정되어 있지 않습니다")
		os.Exit(1)
	}

	// RAG 시스템 초기화
	chatbotSvc, cleanup, err := initializeRAG(cfg, db)
	if err != nil {
		slog.Error("RAG 시스템 초기화 실패", "error", err)
		os.Exit(1)
	}
	defer cleanup()

	storageClient, err := storage.NewS3Client(&cfg.Storage)
	if err != nil {
		slog.Error("S3 클라이언트 초기화 실패", "error", err)
		os.Exit(1)
	}

	userStore := auth.NewPostgresUserStore(db)
	authManager := auth.NewManager(cfg.Auth.JWTSecret, userStore)
	if err := authManager.EnsureRootUser("root@yuon.root", cfg.Auth.RootPassword); err != nil {
		slog.Error("루트 사용자 초기화 실패", "error", err)
		os.Exit(1)
	}

	router := httpserver.NewRouter(cfg, authManager, storageClient)
	if chatbotSvc != nil {
		router.SetChatbotService(chatbotSvc)
		slog.Info("RAG 챗봇 서비스 활성화")
	}
	router.SetupRoutes()

	srv := createServer(cfg, router)

	go startServer(srv, cfg)

	waitForShutdown(srv)
}

func safeClose(db *sql.DB) {
	if db != nil {
		_ = db.Close()
	}
}

func banner() {
	slog.Info("")
	slog.Info("████████╗███████╗ █████╗ ███╗   ███╗    ██╗   ██╗██╗   ██╗ ██████╗ ███╗   ██╗")
	slog.Info("╚══██╔══╝██╔════╝██╔══██╗████╗ ████║    ╚██╗ ██╔╝██║   ██║██╔═══██╗████╗  ██║")
	slog.Info("   ██║   █████╗  ███████║██╔████╔██║     ╚████╔╝ ██║   ██║██║   ██║██╔██╗ ██║")
	slog.Info("   ██║   ██╔══╝  ██╔══██║██║╚██╔╝██║      ╚██╔╝  ██║   ██║██║   ██║██║╚██╗██║")
	slog.Info("   ██║   ███████╗██║  ██║██║ ╚═╝ ██║       ██║   ╚██████╔╝╚██████╔╝██║ ╚████║")
	slog.Info("   ╚═╝   ╚══════╝╚═╝  ╚═╝╚═╝     ╚═╝       ╚═╝    ╚═════╝  ╚═════╝ ╚═╝  ╚═══╝")
	slog.Info("")
	slog.Info("Contribute by Daedok Software Meister High School")
	slog.Info("Contributor: @kangeunchan")
	slog.Info("")
}

func logConfig(cfg *configuration.Config) {
	slog.Info("애플리케이션 시작",
		"name", cfg.App.Name,
		"version", cfg.App.Version,
		"environment", cfg.App.Environment,
	)
}

func createServer(cfg *configuration.Config, router *httpserver.Router) *http.Server {
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)

	return &http.Server{
		Addr:         addr,
		Handler:      router.Engine(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

func startServer(srv *http.Server, cfg *configuration.Config) {
	slog.Info("서버 시작",
		"address", srv.Addr,
		"mode", cfg.Server.Mode,
		"environment", cfg.App.Environment,
	)

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("서버 실행 오류", "error", err)
		os.Exit(1)
	}
}

func initializeRAG(cfg *configuration.Config, db *sql.DB) (*service.ChatbotService, func(), error) {
	// OpenAI 클라이언트
	llmClient := llm.NewOpenAIClient(&cfg.OpenAI)
	slog.Info("OpenAI 클라이언트 초기화 완료")

	// Qdrant 클라이언트
	qdrantClient, err := vectorstore.NewQdrantClient(&cfg.Qdrant)
	if err != nil {
		return nil, nil, fmt.Errorf("Qdrant 초기화 실패: %w", err)
	}
	slog.Info("Qdrant 클라이언트 초기화 완료", "url", cfg.Qdrant.URL)

	// OpenSearch 클라이언트
	opensearchClient, err := search.NewOpenSearchClient(&cfg.OpenSearch)
	if err != nil {
		return nil, nil, fmt.Errorf("OpenSearch 초기화 실패: %w", err)
	}
	slog.Info("OpenSearch 클라이언트 초기화 완료", "url", cfg.OpenSearch.URL)

	var convStore service.ConversationRepository
	var analyticsStore service.AnalyticsStore
	if db != nil {
		convStore = service.NewPostgresConversationStore(db)
		analyticsStore = service.NewPostgresAnalyticsStore(db)
	}

	// 챗봇 서비스
	chatbotSvc := service.NewChatbotService(llmClient, qdrantClient, opensearchClient, convStore, analyticsStore)

	cleanup := func() {
		if qdrantClient != nil {
			qdrantClient.Close()
			slog.Info("Qdrant 연결 종료")
		}
	}

	return chatbotSvc, cleanup, nil
}

func waitForShutdown(srv *http.Server) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("서버 종료 시작")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("서버 강제 종료", "error", err)
		os.Exit(1)
	}

	slog.Info("서버 정상 종료")
}
