package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"anti2api-golang/internal/adapter/claude"
	"anti2api-golang/internal/logger"
	"anti2api-golang/internal/store"
	"anti2api-golang/internal/utils"
	"anti2api-golang/internal/vertex"
)

// HandleClaudeMessages 处理 Claude /v1/messages 端点
func HandleClaudeMessages(w http.ResponseWriter, r *http.Request) {
	var req claude.ClaudeMessagesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteClaudeError(w, http.StatusBadRequest, "invalid_request_error", "Invalid request: "+err.Error())
		return
	}

	// 记录客户端请求
	logger.ClientRequest(r.Method, r.URL.Path, req)

	// 获取 token
	token, err := store.GetAccountStore().GetToken()
	if err != nil {
		WriteClaudeError(w, http.StatusServiceUnavailable, "api_error", err.Error())
		return
	}

	// 处理请求
	if req.Stream {
		handleClaudeStreamRequest(w, r, &req, token)
	} else {
		handleClaudeNonStreamRequest(w, r, &req, token)
	}
}

// HandleClaudeCountTokens 处理 Claude /v1/messages/count_tokens 端点
func HandleClaudeCountTokens(w http.ResponseWriter, r *http.Request) {
	var req claude.ClaudeMessagesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteClaudeError(w, http.StatusBadRequest, "invalid_request_error", "Invalid request: "+err.Error())
		return
	}

	logger.ClientRequest(r.Method, r.URL.Path, req)

	result, err := claude.CountClaudeTokens(&req)
	if err != nil {
		WriteClaudeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, result)
}

// handleClaudeNonStreamRequest 处理 Claude 非流式请求
func handleClaudeNonStreamRequest(w http.ResponseWriter, r *http.Request, req *claude.ClaudeMessagesRequest, token *store.Account) {
	startTime := time.Now()

	// 计算输入 token
	tokenStats, _ := claude.CountClaudeTokens(req)
	inputTokens := 0
	if tokenStats != nil {
		inputTokens = tokenStats.InputTokens
	}

	// 直接转换为 Antigravity 格式（跳过 OpenAI 中间层）
	antigravityReq, err := claude.ConvertClaudeToAntigravity(req, token)
	if err != nil {
		WriteClaudeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	requestID := antigravityReq.RequestID

	// 发送请求
	ctx := r.Context()
	resp, err := vertex.GenerateContent(ctx, antigravityReq, token)
	if err != nil {
		duration := time.Since(startTime)
		logger.ClientResponse(getErrorStatus(err), duration, err.Error())
		WriteClaudeError(w, getErrorStatus(err), "api_error", err.Error())
		return
	}

	// 直接转换为 Claude 响应格式
	claudeResp := claude.ConvertAntigravityToClaudeResponse(resp, requestID, req.Model, inputTokens)

	duration := time.Since(startTime)
	logger.ClientResponse(http.StatusOK, duration, claudeResp)

	WriteJSON(w, http.StatusOK, claudeResp)
}

// handleClaudeStreamRequest 处理 Claude 流式请求
func handleClaudeStreamRequest(w http.ResponseWriter, r *http.Request, req *claude.ClaudeMessagesRequest, token *store.Account) {
	startTime := time.Now()

	// 计算输入 token
	tokenStats, _ := claude.CountClaudeTokens(req)
	inputTokens := 0
	if tokenStats != nil {
		inputTokens = tokenStats.InputTokens
	}

	// 直接转换为 Antigravity 格式（跳过 OpenAI 中间层）
	antigravityReq, err := claude.ConvertClaudeToAntigravity(req, token)
	if err != nil {
		WriteClaudeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	requestID := antigravityReq.RequestID

	// 发送流式请求
	ctx := r.Context()
	resp, err := vertex.GenerateContentStream(ctx, antigravityReq, token)
	if err != nil {
		duration := time.Since(startTime)
		logger.Error("Claude stream request failed: %v", err)
		claude.SetSSEHeaders(w)
		WriteClaudeStreamError(w, err.Error())
		recordClaudeLog(r, req, token, getErrorStatus(err), false, duration, err.Error(), "")
		return
	}

	// 设置 SSE 响应头
	claude.SetSSEHeaders(w)

	// 创建 Claude SSE 发射器
	emitter := claude.NewSSEEmitter(w, requestID, req.Model, inputTokens)
	emitter.Start()

	var contentBuilder strings.Builder

	// 处理流式响应
	// 绑定 ClaudeSSEEmitter.ProcessData
	usage, err := vertex.ParseStream(resp, func(data *vertex.StreamData) error {
		// 记录内容用于日志
		if len(data.Response.Candidates) > 0 {
			for _, part := range data.Response.Candidates[0].Content.Parts {
				if part.Text != "" {
					contentBuilder.WriteString(part.Text)
				}
				// 处理每个 part
				if err := emitter.ProcessPart(claude.StreamDataPart{
					Text:             part.Text,
					FunctionCall:     part.FunctionCall,
					Thought:          part.Thought,
					ThoughtSignature: part.ThoughtSignature,
				}); err != nil {
					return err
				}
			}
		}
		return nil
	})

	duration := time.Since(startTime)

	if err != nil {
		logger.Error("Claude stream processing error: %v", err)
		recordClaudeLog(r, req, token, http.StatusInternalServerError, false, duration, err.Error(), contentBuilder.String())
	} else {
		recordClaudeLog(r, req, token, http.StatusOK, true, duration, "", contentBuilder.String())
	}

	// 发送结束事件
	var usageData *claude.Usage
	if usage != nil {
		usageData = claude.ConvertUsage(usage)
	}
	// Finish 会自动从 Emitter 内部状态判断 stopReason
	emitter.Finish(usageData)
}

// recordClaudeLog 记录 Claude API 日志
func recordClaudeLog(r *http.Request, req *claude.ClaudeMessagesRequest, token *store.Account, status int, success bool, duration time.Duration, errMsg string, responseContent string) {
	entry := store.LogEntry{
		ID:         utils.GenerateRequestID(),
		Timestamp:  time.Now(),
		Status:     status,
		Success:    success,
		Model:      req.Model,
		Method:     r.Method,
		Path:       r.URL.Path,
		DurationMs: duration.Milliseconds(),
		Message:    errMsg,
		HasDetail:  true,
		Detail: &store.LogDetail{
			Request: &store.RequestSnapshot{
				Body: req,
			},
			Response: &store.ResponseSnapshot{
				StatusCode:  status,
				ModelOutput: responseContent,
			},
		},
	}

	if token != nil {
		entry.ProjectID = token.ProjectID
		entry.Email = token.Email
	}

	store.GetLogStore().Add(entry)
}

// WriteClaudeError 写入 Claude 格式错误响应
func WriteClaudeError(w http.ResponseWriter, status int, errorType string, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(claude.ClaudeErrorResponse{
		Type: "error",
		Error: struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		}{
			Type:    errorType,
			Message: message,
		},
	})
}

// WriteClaudeStreamError 写入 Claude 流式错误
func WriteClaudeStreamError(w http.ResponseWriter, message string) {
	errData := map[string]interface{}{
		"type": "error",
		"error": map[string]string{
			"type":    "api_error",
			"message": message,
		},
	}
	jsonData, _ := json.Marshal(errData)
	w.Write([]byte("event: error\ndata: "))
	w.Write(jsonData)
	w.Write([]byte("\n\n"))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}
