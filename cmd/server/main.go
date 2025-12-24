package main

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"

	"anti2api-golang/internal/config"
	"anti2api-golang/internal/server"
)

func main() {
	// 加载 .env 文件（可选）
	godotenv.Load()

	// 加载配置
	cfg := config.Load()

	// 验证必要配置
	if cfg.PanelPassword == "" {
		fmt.Println("Error: PANEL_PASSWORD is required")
		os.Exit(1)
	}

	// 创建并启动服务器
	srv := server.New()
	if err := srv.Start(); err != nil {
		fmt.Printf("Server error: %v\n", err)
		os.Exit(1)
	}
}
