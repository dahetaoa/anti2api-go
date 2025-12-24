package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"anti2api-golang/internal/config"
	"anti2api-golang/internal/store"
	"anti2api-golang/internal/utils"
)

// HandleGetSettings 获取设置
func HandleGetSettings(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()
	epMgr := config.GetEndpointManager()

	// 构建分组配置显示
	groups := []map[string]interface{}{
		{
			"name": "面板配置",
			"items": []map[string]interface{}{
				{"key": "PANEL_USER", "label": "面板用户名", "value": cfg.PanelUser, "isDefault": cfg.PanelUser == "admin", "defaultValue": "admin"},
				{"key": "PANEL_PASSWORD", "label": "面板密码", "value": "******", "sensitive": true, "isDefault": false},
			},
		},
		{
			"name": "网络配置",
			"items": []map[string]interface{}{
				{"key": "PORT", "label": "服务端口", "value": cfg.Port, "isDefault": cfg.Port == 8045, "defaultValue": 8045},
				{"key": "HOST", "label": "监听地址", "value": cfg.Host, "isDefault": cfg.Host == "0.0.0.0", "defaultValue": "0.0.0.0"},
				{"key": "PROXY", "label": "代理地址", "value": valueOrDefault(cfg.Proxy, "未设置"), "isDefault": cfg.Proxy == ""},
				{"key": "TIMEOUT", "label": "请求超时(ms)", "value": cfg.Timeout, "isDefault": cfg.Timeout == 180000, "defaultValue": 180000},
			},
		},
		{
			"name": "API 配置",
			"items": []map[string]interface{}{
				{"key": "API_KEY", "label": "API密钥", "value": maskString(cfg.APIKey), "sensitive": true, "isDefault": cfg.APIKey == ""},
				{"key": "ENDPOINT_MODE", "label": "端点模式", "value": epMgr.GetMode(), "isDefault": os.Getenv("ENDPOINT_MODE") == "", "defaultValue": "daily"},
				{"key": "DEBUG", "label": "调试级别", "value": cfg.Debug, "isDefault": cfg.Debug == "off", "defaultValue": "off"},
			},
		},
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"groups":    groups,
		"updatedAt": time.Now().Format(time.RFC3339),
	})
}

func valueOrDefault(val, def string) string {
	if val == "" {
		return def
	}
	return val
}

func maskString(s string) string {
	if s == "" {
		return "未设置"
	}
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}

// maskEmail 对邮箱地址进行脱敏，只显示第一个字符
func maskEmail(email string) string {
	if email == "" {
		return ""
	}
	// 按 @ 分割
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		// 不是有效邮箱格式，只显示第一个字符
		if len(email) <= 1 {
			return email
		}
		return string([]rune(email)[0]) + "****"
	}

	// 用户名部分：只显示第一个字符
	username := parts[0]
	maskedUsername := string([]rune(username)[0]) + "****"

	// 域名部分：只显示第一个字符
	domain := parts[1]
	maskedDomain := string([]rune(domain)[0]) + "****"

	return maskedUsername + "@" + maskedDomain
}

// HandleGetEndpoints 获取端点信息
func HandleGetEndpoints(w http.ResponseWriter, r *http.Request) {
	epMgr := config.GetEndpointManager()
	allEndpoints := epMgr.GetAllEndpoints()
	mode := epMgr.GetMode()

	// 转换为前端期望的格式
	endpoints := make([]map[string]interface{}, 0)
	var current map[string]interface{}

	for key, ep := range allEndpoints {
		item := map[string]interface{}{
			"key":   key,
			"label": ep.Label,
			"host":  ep.Host,
		}
		endpoints = append(endpoints, item)

		// 设置当前端点
		if key == mode || (mode == "round-robin" && key == "daily") || (mode == "round-robin-dp" && key == "daily") {
			current = item
		}
	}

	// 如果是轮询模式，显示轮询信息
	if mode == "round-robin" || mode == "round-robin-dp" {
		current = map[string]interface{}{
			"key":   mode,
			"label": getModeLabel(mode),
			"host":  "多端点轮询",
		}
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"endpoints": endpoints,
		"current":   current,
		"mode":      mode,
	})
}

func getModeLabel(mode string) string {
	switch mode {
	case "round-robin":
		return "轮询(全部)"
	case "round-robin-dp":
		return "轮询(D+P)"
	case "daily":
		return "Daily"
	case "autopush":
		return "Autopush"
	case "production":
		return "Production"
	default:
		return mode
	}
}

// HandleSetEndpoint 设置当前端点
func HandleSetEndpoint(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Endpoint string `json:"endpoint"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	epMgr := config.GetEndpointManager()
	if err := epMgr.SetMode(req.Endpoint); err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "端点已切换至 " + getModeLabel(req.Endpoint),
		"current": map[string]string{
			"key":   req.Endpoint,
			"label": getModeLabel(req.Endpoint),
		},
	})
}

// HandleSetEndpointMode 设置端点模式
func HandleSetEndpointMode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Mode string `json:"mode"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	epMgr := config.GetEndpointManager()
	if err := epMgr.SetMode(req.Mode); err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "模式已切换至 " + getModeLabel(req.Mode),
		"mode":    req.Mode,
	})
}

// HandleGetLogs 获取请求日志
func HandleGetLogs(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 200
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	logs := store.GetLogStore().GetAll(limit)

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"logs": logs,
	})
}

// HandleGetLogDetail 获取日志详情
func HandleGetLogDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "Missing log ID")
		return
	}

	log := store.GetLogStore().GetByID(id)
	if log == nil {
		WriteError(w, http.StatusNotFound, "Log not found")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"log": log,
	})
}

// HandleGetLogsUsage 获取用量统计
func HandleGetLogsUsage(w http.ResponseWriter, r *http.Request) {
	windowMinutes := 60
	usage := store.GetLogStore().GetUsageStats(windowMinutes)

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"usage":         usage,
		"windowMinutes": windowMinutes,
	})
}

// HandleGetUsage 获取使用统计
func HandleGetUsage(w http.ResponseWriter, r *http.Request) {
	// 获取全部时间的统计
	allUsage := store.GetLogStore().GetAllAccountsUsage()

	totalRequests := 0
	for _, usage := range allUsage {
		totalRequests += usage.Count
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"requests": totalRequests,
		"tokens":   0, // Token 统计暂不支持
	})
}

// HandleGetAccounts 获取账号列表
func HandleGetAccounts(w http.ResponseWriter, r *http.Request) {
	accounts := store.GetAccountStore().GetAll()
	allUsage := store.GetLogStore().GetAllAccountsUsage()

	// 构建前端期望的格式
	result := make([]map[string]interface{}, len(accounts))
	for i, acc := range accounts {
		// 获取该账号的用量统计（优先用 email 匹配，其次用 projectId）
		usageData := map[string]interface{}{
			"total":      0,
			"success":    0,
			"failed":     0,
			"lastUsedAt": nil,
			"models":     []string{},
		}

		// 优先按 email 查找，其次按 projectId
		var usage *store.UsageStats
		if acc.Email != "" {
			usage = allUsage[acc.Email]
		}
		if usage == nil && acc.ProjectID != "" {
			usage = allUsage[acc.ProjectID]
		}

		if usage != nil {
			usageData["total"] = usage.Count
			usageData["success"] = usage.Success
			usageData["failed"] = usage.Failed
			usageData["models"] = usage.Models
			if usage.LastUsedAt != nil {
				usageData["lastUsedAt"] = usage.LastUsedAt.Format(time.RFC3339)
			}
		}

		result[i] = map[string]interface{}{
			"index":     i,
			"email":     maskEmail(acc.Email),
			"projectId": acc.ProjectID,
			"enable":    acc.Enable,
			"expired":   acc.IsExpired(),
			"createdAt": acc.CreatedAt.Format(time.RFC3339),
			"usage":     usageData,
		}
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"accounts": result,
	})
}

// HandleImportTOML 导入 TOML 格式账号
func HandleImportTOML(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TOML           string `json:"toml"`
		ReplaceExist   bool   `json:"replaceExisting"`
		FilterDisabled bool   `json:"filterDisabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	tomlData, err := utils.ParseTOML(req.TOML)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid TOML: "+err.Error())
		return
	}

	// 如果需要覆盖现有账号，先清空
	if req.ReplaceExist {
		store.GetAccountStore().Clear()
	}

	imported, err := store.GetAccountStore().ImportFromTOML(tomlData)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	total := store.GetAccountStore().Count()
	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"imported": imported,
		"skipped":  0,
		"total":    total,
	})
}

// HandleRefreshAllAccounts 刷新所有账号
func HandleRefreshAllAccounts(w http.ResponseWriter, r *http.Request) {
	refreshed, failed := store.GetAccountStore().RefreshAll()

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"refreshed": refreshed,
		"failed":    failed,
	})
}

// HandleRefreshAccount 刷新单个账号
func HandleRefreshAccount(w http.ResponseWriter, r *http.Request) {
	indexStr := r.PathValue("index")
	index, err := strconv.Atoi(indexStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid index")
		return
	}

	if err := store.GetAccountStore().RefreshAccount(index); err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// HandleToggleAccount 切换账号启用状态
func HandleToggleAccount(w http.ResponseWriter, r *http.Request) {
	indexStr := r.PathValue("index")
	index, err := strconv.Atoi(indexStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid index")
		return
	}

	var req struct {
		Enable bool `json:"enable"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	if err := store.GetAccountStore().SetEnable(index, req.Enable); err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// HandleDeleteAccount 删除账号
func HandleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	indexStr := r.PathValue("index")
	index, err := strconv.Atoi(indexStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid index")
		return
	}

	if err := store.GetAccountStore().Delete(index); err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]bool{"success": true})
}
