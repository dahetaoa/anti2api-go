package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"anti2api-golang/internal/auth"
	"anti2api-golang/internal/config"
	"anti2api-golang/internal/logger"
)

// responseWriter 包装器用于捕获状态码（同时支持 Flusher 接口）
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Flush 实现 http.Flusher 接口，支持流式响应
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// RequestLogger 请求日志中间件
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 跳过静态资源
		if strings.HasPrefix(r.URL.Path, "/favicon") ||
			strings.HasPrefix(r.URL.Path, "/admin/assets") {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		wrapper := &responseWriter{ResponseWriter: w, statusCode: 200}

		next.ServeHTTP(wrapper, r)

		duration := time.Since(start)
		logger.Request(r.Method, r.URL.Path, wrapper.statusCode, duration)
	})
}

// RequireAPIKey API Key 验证中间件
func RequireAPIKey(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := config.Get()
		apiKey := cfg.APIKey

		// 如果没有配置 API Key，跳过验证
		if apiKey == "" {
			next(w, r)
			return
		}

		var providedKey string

		// 1. Authorization header: Bearer sk-xxx 或直接 sk-xxx
		if authHeader := r.Header.Get("Authorization"); authHeader != "" {
			providedKey = strings.TrimPrefix(authHeader, "Bearer ")
		}
		// 2. x-api-key header (Claude 标准)
		if providedKey == "" {
			providedKey = r.Header.Get("x-api-key")
		}
		// 3. x-goog-api-key header (Gemini 标准)
		if providedKey == "" {
			providedKey = r.Header.Get("x-goog-api-key")
		}
		// 4. Query 参数 ?key=
		if providedKey == "" {
			providedKey = r.URL.Query().Get("key")
		}

		if providedKey != apiKey {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"message": "Invalid API Key",
					"type":    "invalid_request_error",
				},
			})
			return
		}

		next(w, r)
	}
}

// RequirePanelAuth 管理面板认证中间件
func RequirePanelAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := auth.GetSessionToken(r)
		if token == "" {
			handleUnauthorized(w, r)
			return
		}

		if !auth.ValidateSession(token) {
			handleUnauthorized(w, r)
			return
		}

		next(w, r)
	}
}

func handleUnauthorized(w http.ResponseWriter, r *http.Request) {
	// API 请求返回 JSON
	if strings.HasPrefix(r.URL.Path, "/auth/") ||
		strings.HasPrefix(r.URL.Path, "/admin/api/") ||
		r.Header.Get("Accept") == "application/json" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Unauthorized",
		})
		return
	}

	// 页面请求重定向到登录页
	http.Redirect(w, r, "/admin/login", http.StatusFound)
}

// CORS 中间件
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Session-Token, x-api-key, x-goog-api-key, anthropic-version")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
