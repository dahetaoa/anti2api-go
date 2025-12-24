package config

import (
	"os"
	"strconv"
	"strings"
	"sync"
)

// Config 应用配置
type Config struct {
	// 服务配置
	Port int
	Host string

	// API 配置
	UserAgent string
	Timeout   int
	Proxy     string

	// 安全配置
	APIKey        string
	PanelUser     string
	PanelPassword string

	// 请求限制
	MaxRequestSize string

	// 重试配置
	RetryStatusCodes []int
	RetryMaxAttempts int

	// 日志配置
	Debug string

	// 端点模式
	EndpointMode string

	// OAuth 配置
	GoogleClientID     string
	GoogleClientSecret string

	// 数据目录
	DataDir string
}

// Endpoint API 端点
type Endpoint struct {
	Key   string
	Label string
	Host  string
}

var (
	cfg  *Config
	once sync.Once

	// APIEndpoints 可用的 API 端点
	APIEndpoints = map[string]Endpoint{
		"daily": {
			Key:   "daily",
			Label: "Daily (Sandbox)",
			Host:  "daily-cloudcode-pa.sandbox.googleapis.com",
		},
		"autopush": {
			Key:   "autopush",
			Label: "Autopush (Sandbox)",
			Host:  "autopush-cloudcode-pa.sandbox.googleapis.com",
		},
		"production": {
			Key:   "production",
			Label: "Production",
			Host:  "cloudcode-pa.googleapis.com",
		},
	}

	// RoundRobinEndpoints 轮询端点列表
	RoundRobinEndpoints = []string{"daily", "autopush", "production"}
	// RoundRobinDpEndpoints 轮询端点列表（仅 daily 和 production）
	RoundRobinDpEndpoints = []string{"daily", "production"}

	// 默认 OAuth 客户端
	DefaultClientID     = "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com"
	DefaultClientSecret = "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"
)

// Load 加载配置
func Load() *Config {
	once.Do(func() {
		cfg = &Config{
			Port:               getEnvInt("PORT", 8045),
			Host:               getEnv("HOST", "0.0.0.0"),
			UserAgent:          getEnv("API_USER_AGENT", "antigravity/1.11.3 windows/amd64"),
			Timeout:            getEnvInt("TIMEOUT", 180000),
			Proxy:              getEnv("PROXY", ""),
			APIKey:             getEnv("API_KEY", ""),
			PanelUser:          getEnv("PANEL_USER", "admin"),
			PanelPassword:      getEnv("PANEL_PASSWORD", ""),
			MaxRequestSize:     getEnv("MAX_REQUEST_SIZE", "50mb"),
			RetryStatusCodes:   getEnvIntSlice("RETRY_STATUS_CODES", []int{429, 500}),
			RetryMaxAttempts:   getEnvInt("RETRY_MAX_ATTEMPTS", 3),
			Debug:              getEnv("DEBUG", "off"),
			EndpointMode:       getEnv("ENDPOINT_MODE", "daily"),
			GoogleClientID:     getEnv("GOOGLE_CLIENT_ID", ""),
			GoogleClientSecret: getEnv("GOOGLE_CLIENT_SECRET", ""),
			DataDir:            getEnv("DATA_DIR", "./data"),
		}

		// 检查命令行参数
		for i, arg := range os.Args[1:] {
			if arg == "-debug" && i+1 < len(os.Args[1:]) {
				cfg.Debug = os.Args[i+2]
			}
		}
	})
	return cfg
}

// Get 获取配置实例
func Get() *Config {
	if cfg == nil {
		return Load()
	}
	return cfg
}

// GetClientID 获取 OAuth 客户端 ID
func GetClientID() string {
	if cfg.GoogleClientID != "" {
		return cfg.GoogleClientID
	}
	return DefaultClientID
}

// GetClientSecret 获取 OAuth 客户端密钥
func GetClientSecret() string {
	if cfg.GoogleClientSecret != "" {
		return cfg.GoogleClientSecret
	}
	return DefaultClientSecret
}

// StreamURL 获取流式请求 URL
func (e Endpoint) StreamURL() string {
	return "https://" + e.Host + "/v1internal:streamGenerateContent?alt=sse"
}

// NoStreamURL 获取非流式请求 URL
func (e Endpoint) NoStreamURL() string {
	return "https://" + e.Host + "/v1internal:generateContent"
}

// 辅助函数

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvIntSlice(key string, defaultValue []int) []int {
	if value := os.Getenv(key); value != "" {
		parts := strings.Split(value, ",")
		result := make([]int, 0, len(parts))
		for _, p := range parts {
			if i, err := strconv.Atoi(strings.TrimSpace(p)); err == nil {
				result = append(result, i)
			}
		}
		if len(result) > 0 {
			return result
		}
	}
	return defaultValue
}
