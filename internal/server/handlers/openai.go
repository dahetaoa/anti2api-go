package handlers

import (
	"context"
	"encoding/json"
	"io"
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
	// 读取原始请求体
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}

	// 记录原始客户端请求
	logger.ClientRequest(r.Method, r.URL.Path, rawBody)

	// 反序列化用于业务逻辑
	var req openai.OpenAIChatRequest
	if err := json.Unmarshal(rawBody, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

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

	// 读取原始请求体
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}

	// 记录原始客户端请求
	logger.ClientRequest(r.Method, r.URL.Path, rawBody)

	// 反序列化用于业务逻辑
	var req openai.OpenAIChatRequest
	if err := json.Unmarshal(rawBody, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	// 按凭证获取 token
	var token *store.Account

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

	// 处理流式响应
	// 绑定 StreamWriter.ProcessData 作为回调
	streamResult, err := vertex.ParseStreamWithResult(resp, func(data *vertex.StreamData) error {
		// 处理每个 part
		if len(data.Response.Candidates) > 0 {
			for _, part := range data.Response.Candidates[0].Content.Parts {
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

	// 记录后端流式响应日志（原始 Vertex 格式，仅合并 text）
	logger.BackendStreamResponse(http.StatusOK, duration, streamResult.MergedResponse)

	if err != nil {
		logger.Error("Stream processing error: %v", err)
		// 记录失败日志
		recordLog(r.Method, r.URL.Path, req, token, http.StatusInternalServerError, false, duration, err.Error(), streamResult.Text)
	} else {
		// 记录成功日志
		recordLog(r.Method, r.URL.Path, req, token, http.StatusOK, true, duration, "", streamResult.Text)
	}

	// 发送结束
	finishReason := "stop"
	if streamResult.FinishReason != "" {
		finishReason = streamResult.FinishReason
	}

	var usageData *openai.Usage
	if streamResult.Usage != nil {
		usageData = openai.ConvertUsage(streamResult.Usage)
	}

	streamWriter.WriteFinish(finishReason, usageData)

	// 记录客户端流式响应日志（透传原始 SSE 事件）
	logger.ClientStreamResponse(http.StatusOK, duration, streamWriter.GetMergedResponse())
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
				var signature string
				if tc.ExtraContent != nil && tc.ExtraContent.Google != nil {
					signature = tc.ExtraContent.Google.ThoughtSignature
				}
				coreToolCalls[i] = core.ToolCallInfo{
					ID:               tc.ID,
					Name:             tc.Function.Name,
					Args:             openai.ParseArgs(tc.Function.Arguments),
					ThoughtSignature: signature,
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
