package server

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof" // pprof 内存分析支持
	"os"
	"os/signal"
	"syscall"
	"time"

	"anti2api-golang/internal/config"
	"anti2api-golang/internal/logger"
	"anti2api-golang/internal/store"
)

// Server HTTP 服务器
type Server struct {
	httpServer *http.Server
	config     *config.Config
}

// New 创建新服务器
func New() *Server {
	cfg := config.Get()

	mux := http.NewServeMux()
	SetupRoutes(mux)

	// 应用中间件
	handler := RequestLogger(CORS(mux))

	return &Server{
		httpServer: &http.Server{
			Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
			Handler:      handler,
			ReadTimeout:  time.Duration(cfg.Timeout) * time.Millisecond,
			WriteTimeout: time.Duration(cfg.Timeout) * time.Millisecond,
			IdleTimeout:  120 * time.Second,
		},
		config: cfg,
	}
}

// Start 启动服务器
func (s *Server) Start() error {
	// 初始化日志
	logger.Init()

	// 加载账号
	store.GetAccountStore()

	// 打印启动横幅
	logger.Banner(s.config.Port, s.config.EndpointMode)

	// 启动 pprof 服务器（用于内存分析）
	go func() {
		pprofAddr := "localhost:6060"
		logger.Info("pprof server listening on http://%s/debug/pprof/", pprofAddr)
		if err := http.ListenAndServe(pprofAddr, nil); err != nil {
			logger.Error("pprof server error: %v", err)
		}
	}()

	// 启动服务器
	go func() {
		logger.Info("Server listening on %s", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Server error: %v", err)
			os.Exit(1)
		}
	}()

	// 等待中断信号
	return s.waitForShutdown()
}

// waitForShutdown 等待关闭信号
func (s *Server) waitForShutdown() error {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.httpServer.Shutdown(ctx); err != nil {
		logger.Error("Server shutdown error: %v", err)
		return err
	}

	logger.Info("Server stopped")
	return nil
}
