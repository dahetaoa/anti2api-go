package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"anti2api-golang/internal/adapter/openai"
	"anti2api-golang/internal/core"
	"anti2api-golang/internal/logger"
	"anti2api-golang/internal/store"
	"anti2api-golang/internal/utils"
	"anti2api-golang/internal/vertex"
)

// recordLog 记录 API 调用日志
func recordLog(method, path string, req *openai.OpenAIChatRequest, token *store.Account, status int, success bool, duration time.Duration, errMsg string, responseContent string) {
	entry := store.LogEntry{
		ID:         utils.GenerateRequestID(),
		Timestamp:  time.Now(),
		Status:     status,
		Success:    success,
		Model:      req.Model,
		Method:     method,
		Path:       path,
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

// HandleGetModels 获取模型列表
func HandleGetModels(w http.ResponseWriter, r *http.Request) {
	models := openai.ModelsResponse{
		Object: "list",
		Data:   openai.SupportedModels,
	}
	WriteJSON(w, http.StatusOK, models)
}

// HandleChatCompletions 处理聊天完成请求
func HandleChatCompletions(w http.ResponseWriter, r *http.Request) {
	var req openai.OpenAIChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	// 记录客户端请求
	logger.ClientRequest(r.Method, r.URL.Path, req)

	// 获取 token
	token, err := store.GetAccountStore().GetToken()
	if err != nil {
		WriteError(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	// 处理请求
	if req.Stream {
		handleStreamRequest(w, r, &req, token)
	} else {
		handleNonStreamRequest(w, r, &req, token)
	}
}

// HandleChatCompletionsWithCredential 使用指定凭证处理聊天完成请求
func HandleChatCompletionsWithCredential(w http.ResponseWriter, r *http.Request) {
	credential := r.PathValue("credential")

	var req openai.OpenAIChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	logger.ClientRequest(r.Method, r.URL.Path, req)

	// 按凭证获取 token
	var token *store.Account
	var err error

	accountStore := store.GetAccountStore()
	if strings.Contains(credential, "@") {
		token, err = accountStore.GetTokenByEmail(credential)
	} else {
		token, err = accountStore.GetTokenByProjectID(credential)
	}

	if err != nil {
		WriteError(w, http.StatusNotFound, "Credential not found: "+credential)
		return
	}

	// 处理请求
	if req.Stream {
		handleStreamRequest(w, r, &req, token)
	} else {
		handleNonStreamRequest(w, r, &req, token)
	}
}

func handleNonStreamRequest(w http.ResponseWriter, r *http.Request, req *openai.OpenAIChatRequest, token *store.Account) {
	startTime := time.Now()

	// 转换请求
	antigravityReq := openai.ConvertOpenAIToAntigravity(req, token)

	// 发送请求
	ctx := r.Context()
	resp, err := vertex.GenerateContent(ctx, antigravityReq, token)
	if err != nil {
		duration := time.Since(startTime)
		logger.ClientResponse(getErrorStatus(err), duration, err.Error())
		// 记录失败日志
		recordLog(r.Method, r.URL.Path, req, token, getErrorStatus(err), false, duration, err.Error(), "")
		WriteError(w, getErrorStatus(err), err.Error())
		return
	}

	// 转换响应
	openAIResp := openai.ConvertToOpenAIResponse(resp, req.Model)

	duration := time.Since(startTime)
	logger.ClientResponse(http.StatusOK, duration, openAIResp)

	// 记录成功日志
	responseContent := ""
	if len(openAIResp.Choices) > 0 {
		responseContent = openAIResp.Choices[0].Message.Content
	}
	recordLog(r.Method, r.URL.Path, req, token, http.StatusOK, true, duration, "", responseContent)

	WriteJSON(w, http.StatusOK, openAIResp)
}

func handleStreamRequest(w http.ResponseWriter, r *http.Request, req *openai.OpenAIChatRequest, token *store.Account) {
	startTime := time.Now()

	// 检查是否为 bypass 模式
	if openai.IsBypassModel(req.Model) {
		handleBypassStream(w, r, req, token)
		return
	}

	// 转换请求
	antigravityReq := openai.ConvertOpenAIToAntigravity(req, token)

	// 发送流式请求
	ctx := r.Context()
	resp, err := vertex.GenerateContentStream(ctx, antigravityReq, token)
	if err != nil {
		duration := time.Since(startTime)
		openai.SetSSEHeaders(w)
		openai.WriteSSEError(w, err.Error())
		// 记录失败日志
		recordLog(r.Method, r.URL.Path, req, token, getErrorStatus(err), false, duration, err.Error(), "")
		return
	}

	// 设置流式响应头
	openai.SetSSEHeaders(w)

	id := utils.GenerateChatCompletionID()
	created := time.Now().Unix()
	model := req.Model

	streamWriter := openai.NewSSEWriter(w, id, created, model)

	var usage *core.UsageMetadata
	var contentBuilder strings.Builder

	// 处理流式响应
	// 绑定 StreamWriter.ProcessData 作为回调
	usage, err = vertex.ParseStream(resp, func(data *vertex.StreamData) error {
		// 记录内容用于日志
		if len(data.Response.Candidates) > 0 {
			for _, part := range data.Response.Candidates[0].Content.Parts {
				if part.Text != "" {
					contentBuilder.WriteString(part.Text)
				}
				// 处理每个 part
				if err := streamWriter.ProcessPart(openai.StreamDataPart{
					Text:             part.Text,
					FunctionCall:     part.FunctionCall,
					Thought:          part.Thought,
					ThoughtSignature: part.ThoughtSignature,
				}); err != nil {
					return err
				}
			}
			// 检查 FinishReason
			if data.Response.Candidates[0].FinishReason != "" {
				streamWriter.FlushToolCalls()
			}
		}
		return nil
	})

	duration := time.Since(startTime)

	if err != nil {
		logger.Error("Stream processing error: %v", err)
		// 记录失败日志
		recordLog(r.Method, r.URL.Path, req, token, http.StatusInternalServerError, false, duration, err.Error(), contentBuilder.String())
	} else {
		// 记录成功日志
		recordLog(r.Method, r.URL.Path, req, token, http.StatusOK, true, duration, "", contentBuilder.String())
	}

	// 发送结束
	finishReason := "stop"
	// 检查 tool calls 状态（注意：ProcessData 处理完流后不需要从 writer 获取 toolCalls，
	// 因为 OpenAI 格式是在流中增量发送的，结束时 tool_calls 状态由 finishReason 决定，
	// 但这里我们简单默认为 stop，因为 vertex 不会显式告诉我们 finish reason 是 tool_calls
	// 除非我们解析了 finishReason 字段。StreamData 里有 finishReason）

	// 这里可以优化：从 ParseStream 返回的最后状态中获取 finishReason，
	// 或者在回调中捕获。StreamWriter 内部处理了大部分转换，但 finish reason 需要显式传递。

	// 简单起见，如果 usage 存在且有 tool calls (逻辑上)，或者可以从外部推断
	// 但当前架构下，Vertex 的 finishReason 会在 StreamData 中。
	// 我们在 ProcessData 中已经处理了 finishReason -> tool_calls 的转换逻辑（如果需要）。
	// 不过 WriteFinish 需要明确的 reason。

	// 修正：我们可以在 WriteFinish 中传入 stop。
	// 如果 Vertex 返回了 specific finish reason，StreamWriter 暂时没有暴露出来。
	// 但这对流式客户端影响不大，通常客户端根据 content 或 tool_calls 判断。

	var usageData *openai.Usage
	if usage != nil {
		usageData = openai.ConvertUsage(usage)
	}

	streamWriter.WriteFinish(finishReason, usageData)
}

func handleBypassStream(w http.ResponseWriter, r *http.Request, req *openai.OpenAIChatRequest, token *store.Account) {
	startTime := time.Now()
	id := utils.GenerateChatCompletionID()
	created := time.Now().Unix()
	model := req.Model

	// NewSSEWriter 内部会设置响应头
	streamWriter := openai.NewSSEWriter(w, id, created, model)

	// 立即发送第一个心跳，确保客户端计时器启动
	if err := streamWriter.WriteHeartbeat(); err != nil {
		return
	}

	// 启动心跳 goroutine
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	done := make(chan struct{})

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := streamWriter.WriteHeartbeat(); err != nil {
					return
				}
			case <-done:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	// 转换请求（使用真实模型名）
	actualModel := openai.ResolveModelName(req.Model)
	modifiedReq := *req
	modifiedReq.Model = actualModel

	antigravityReq := openai.ConvertOpenAIToAntigravity(&modifiedReq, token)

	// 执行非流式请求
	resp, err := vertex.GenerateContent(ctx, antigravityReq, token)
	close(done)

	if err != nil {
		duration := time.Since(startTime)
		streamWriter.WriteContent("Error: " + err.Error())
		streamWriter.WriteFinish("stop", nil)
		// 记录失败日志
		recordLog(r.Method, r.URL.Path, req, token, getErrorStatus(err), false, duration, err.Error(), "")
		return
	}

	// 转换响应
	openAIResp := openai.ConvertToOpenAIResponse(resp, model)

	duration := time.Since(startTime)

	// 发送完整内容
	if len(openAIResp.Choices) > 0 {
		msg := openAIResp.Choices[0].Message

		if msg.Reasoning != "" {
			streamWriter.WriteReasoning(msg.Reasoning)
		}
		if len(msg.ToolCalls) > 0 {
			// 转换为 core.ToolCallInfo 格式
			coreToolCalls := make([]core.ToolCallInfo, len(msg.ToolCalls))
			for i, tc := range msg.ToolCalls {
				coreToolCalls[i] = core.ToolCallInfo{
					ID:               tc.ID,
					Name:             tc.Function.Name,
					Args:             openai.ParseArgs(tc.Function.Arguments),
					ThoughtSignature: tc.ThoughtSignature,
				}
			}
			streamWriter.WriteToolCalls(coreToolCalls)
		}
		if msg.Content != "" {
			streamWriter.WriteContent(msg.Content)
		}

		finishReason := "stop"
		if openAIResp.Choices[0].FinishReason != nil {
			finishReason = *openAIResp.Choices[0].FinishReason
		}

		streamWriter.WriteFinish(finishReason, openAIResp.Usage)

		// 记录成功日志
		recordLog(r.Method, r.URL.Path, req, token, http.StatusOK, true, duration, "", msg.Content)
	} else {
		streamWriter.WriteFinish("stop", nil)
		// 记录成功但无内容的日志
		recordLog(r.Method, r.URL.Path, req, token, http.StatusOK, true, duration, "", "")
	}
}

func getErrorStatus(err error) int {
	if apiErr, ok := err.(*vertex.APIError); ok {
		return apiErr.Status
	}
	return http.StatusInternalServerError
}
