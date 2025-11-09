package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"yuon/configuration"
	httpserver "yuon/internal/http"
)

func main() {
	banner()

	cfg, err := configuration.Load()
	if err != nil {
		slog.Error("설정 로드 실패", "error", err)
		os.Exit(1)
	}

	logConfig(cfg)

	router := httpserver.NewRouter(cfg)
	router.SetupRoutes()

	srv := createServer(cfg, router)

	go startServer(srv, cfg)

	waitForShutdown(srv)
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
