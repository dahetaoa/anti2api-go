package server

import (
	"net/http"
	"strings"

	"anti2api-golang/internal/server/handlers"
)

// SetupRoutes 注册路由
func SetupRoutes(mux *http.ServeMux) {
	// ===== 静态文件 =====
	fileServer := http.FileServer(http.Dir("public/admin"))
	mux.Handle("GET /admin/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 检查是否需要认证
		path := r.URL.Path
		if path == "/admin/" || path == "/admin/index.html" || path == "/admin/api.html" {
			// 页面需要认证（除了静态资源）
			RequirePanelAuth(func(w http.ResponseWriter, r *http.Request) {
				http.StripPrefix("/admin/", fileServer).ServeHTTP(w, r)
			})(w, r)
			return
		}
		// 静态资源直接提供
		http.StripPrefix("/admin/", fileServer).ServeHTTP(w, r)
	}))

	// ===== 健康检查 =====
	mux.HandleFunc("GET /healthz", handlers.HandleHealthz)
	mux.HandleFunc("GET /health", handlers.HandleHealthz)

	// ===== 根路径 =====
	mux.HandleFunc("GET /{$}", handlers.HandleRoot)
	mux.HandleFunc("GET /admin", handlers.HandleAdminRedirect)

	// ===== 管理面板登录 =====
	mux.HandleFunc("GET /admin/login", handlers.HandleLoginPage)
	mux.HandleFunc("POST /admin/login", handlers.HandleLogin)
	mux.HandleFunc("POST /admin/logout", handlers.HandleLogout)

	// ===== 管理面板 API（需要认证）=====
	mux.HandleFunc("GET /admin/settings", RequirePanelAuth(handlers.HandleGetSettings))
	mux.HandleFunc("GET /admin/endpoints", RequirePanelAuth(handlers.HandleGetEndpoints))
	mux.HandleFunc("POST /admin/endpoints", RequirePanelAuth(handlers.HandleSetEndpoint))
	mux.HandleFunc("POST /admin/endpoints/mode", RequirePanelAuth(handlers.HandleSetEndpointMode))
	mux.HandleFunc("GET /admin/logs", RequirePanelAuth(handlers.HandleGetLogs))
	mux.HandleFunc("GET /admin/logs/usage", RequirePanelAuth(handlers.HandleGetLogsUsage))
	mux.HandleFunc("GET /admin/logs/{id}", RequirePanelAuth(handlers.HandleGetLogDetail))

	// ===== OAuth =====
	mux.HandleFunc("GET /auth/oauth/url", RequirePanelAuth(handlers.HandleGetOAuthURL))
	mux.HandleFunc("GET /oauth-callback", handlers.HandleOAuthCallback)
	mux.HandleFunc("POST /auth/oauth/parse-url", RequirePanelAuth(handlers.HandleParseOAuthURL))

	// ===== 账号管理（需要认证）=====
	mux.HandleFunc("GET /auth/accounts", RequirePanelAuth(handlers.HandleGetAccounts))
	mux.HandleFunc("POST /auth/accounts/import-toml", RequirePanelAuth(handlers.HandleImportTOML))
	mux.HandleFunc("POST /auth/accounts/refresh-all", RequirePanelAuth(handlers.HandleRefreshAllAccounts))
	mux.HandleFunc("POST /auth/accounts/{index}/refresh", RequirePanelAuth(handlers.HandleRefreshAccount))
	mux.HandleFunc("POST /auth/accounts/{index}/enable", RequirePanelAuth(handlers.HandleToggleAccount))
	mux.HandleFunc("DELETE /auth/accounts/{index}", RequirePanelAuth(handlers.HandleDeleteAccount))

	// ===== OpenAI 兼容 API =====
	mux.HandleFunc("GET /v1/models", RequireAPIKey(handlers.HandleGetModels))
	mux.HandleFunc("POST /v1/chat/completions", RequireAPIKey(handlers.HandleChatCompletions))
	mux.HandleFunc("POST /v1/chat/completions/", RequireAPIKey(handlers.HandleChatCompletions))
	mux.HandleFunc("POST /{credential}/v1/chat/completions", RequireAPIKey(handlers.HandleChatCompletionsWithCredential))

	// ===== Claude 兼容 API =====
	mux.HandleFunc("POST /v1/messages", RequireAPIKey(handlers.HandleClaudeMessages))
	mux.HandleFunc("POST /v1/messages/count_tokens", RequireAPIKey(handlers.HandleClaudeCountTokens))

	// ===== Gemini 兼容 API =====
	mux.HandleFunc("GET /v1beta/models", RequireAPIKey(handlers.HandleGeminiModels))
	mux.HandleFunc("POST /v1beta/models/", RequireAPIKey(handlers.HandleGeminiAPI))

	// ===== 原始 Gemini 透传 =====
	mux.HandleFunc("POST /gemini/v1beta/models/", RequireAPIKey(handlers.HandleRawGeminiAPI))
}

// isStaticAsset 检查是否是静态资源
func isStaticAsset(path string) bool {
	return strings.HasSuffix(path, ".css") ||
		strings.HasSuffix(path, ".js") ||
		strings.HasSuffix(path, ".png") ||
		strings.HasSuffix(path, ".jpg") ||
		strings.HasSuffix(path, ".ico") ||
		strings.HasSuffix(path, ".svg") ||
		strings.HasSuffix(path, ".woff") ||
		strings.HasSuffix(path, ".woff2")
}
