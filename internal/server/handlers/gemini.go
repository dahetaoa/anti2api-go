package handlers

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"anti2api-golang/internal/adapter/gemini"
	"anti2api-golang/internal/core"
	"anti2api-golang/internal/logger"
	"anti2api-golang/internal/store"
	"anti2api-golang/internal/vertex"
)

// HandleGeminiModels 获取 Gemini 格式模型列表
func HandleGeminiModels(w http.ResponseWriter, r *http.Request) {
	models := gemini.GetGeminiModels()
	WriteJSON(w, http.StatusOK, models)
}

// parseGeminiPath 解析 Gemini API 路径，提取 model 和 action
// 路径格式: /v1beta/models/{model}:{action} 或 /gemini/v1beta/models/{model}:{action}
func parseGeminiPath(path string) (model, action string, ok bool) {
	// 移除前缀
	path = strings.TrimPrefix(path, "/gemini")
	path = strings.TrimPrefix(path, "/v1beta/models/")

	// 查找冒号分隔符
	idx := strings.LastIndex(path, ":")
	if idx == -1 {
		return "", "", false
	}

	model = path[:idx]
	action = path[idx+1:]
	return model, action, true
}

// HandleGeminiAPI 统一处理 Gemini API 请求
func HandleGeminiAPI(w http.ResponseWriter, r *http.Request) {
	model, action, ok := parseGeminiPath(r.URL.Path)
	if !ok || model == "" {
		WriteError(w, http.StatusBadRequest, "Invalid path format")
		return
	}

	switch action {
	case "generateContent":
		handleGeminiGenerateContent(w, r, model)
	case "streamGenerateContent":
		handleGeminiStreamGenerateContent(w, r, model)
	default:
		WriteError(w, http.StatusBadRequest, "Unknown action: "+action)
	}
}

// HandleRawGeminiAPI 统一处理原始 Gemini API 透传请求
func HandleRawGeminiAPI(w http.ResponseWriter, r *http.Request) {
	model, action, ok := parseGeminiPath(r.URL.Path)
	if !ok || model == "" {
		WriteError(w, http.StatusBadRequest, "Invalid path format")
		return
	}

	switch action {
	case "generateContent":
		handleRawGeminiGenerateContent(w, r, model)
	case "streamGenerateContent":
		handleRawGeminiStreamGenerateContent(w, r, model)
	default:
		WriteError(w, http.StatusBadRequest, "Unknown action: "+action)
	}
}

// handleGeminiGenerateContent 处理 Gemini 非流式请求
func handleGeminiGenerateContent(w http.ResponseWriter, r *http.Request, model string) {
	// 读取原始请求体
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}

	// 记录原始客户端请求
	logger.ClientRequest(r.Method, r.URL.Path, rawBody)

	// 反序列化用于业务逻辑
	var req gemini.GeminiRequest
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

	startTime := time.Now()

	// 转换请求
	antigravityReq := gemini.ConvertGeminiToAntigravity(model, &req, token)

	// 发送请求
	ctx := r.Context()
	resp, err := vertex.GenerateContent(ctx, antigravityReq, token)
	if err != nil {
		duration := time.Since(startTime)
		logger.ClientResponse(getErrorStatus(err), duration, err.Error())
		WriteError(w, getErrorStatus(err), err.Error())
		return
	}

	// 提取 Gemini 响应
	geminiResp := gemini.ExtractGeminiResponse(resp)

	duration := time.Since(startTime)
	logger.ClientResponse(http.StatusOK, duration, geminiResp)
	WriteJSON(w, http.StatusOK, geminiResp)
}

// handleGeminiStreamGenerateContent 处理 Gemini 流式请求
func handleGeminiStreamGenerateContent(w http.ResponseWriter, r *http.Request, model string) {
	startTime := time.Now()

	// 读取原始请求体
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}

	// 记录原始客户端请求
	logger.ClientRequest(r.Method, r.URL.Path, rawBody)

	// 反序列化用于业务逻辑
	var req gemini.GeminiRequest
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

	// 转换请求
	antigravityReq := gemini.ConvertGeminiToAntigravity(model, &req, token)

	// 发送流式请求
	ctx := r.Context()
	resp, err := vertex.GenerateContentStream(ctx, antigravityReq, token)
	if err != nil {
		WriteError(w, getErrorStatus(err), err.Error())
		return
	}
	defer resp.Body.Close()

	// 设置流式响应头
	vertex.SetStreamHeaders(w)

	// 处理 gzip
	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			vertex.WriteStreamError(w, err.Error())
			return
		}
		defer gzReader.Close()
		reader = gzReader
	}

	// 转发流式数据（16MB缓冲区）并收集日志
	scanner := bufio.NewScanner(reader)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 16*1024*1024)

	// 收集所有 parts 用于构建原始响应
	var allParts []core.Part
	var finishReason string
	var usage *core.UsageMetadata

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			// 收集数据用于日志
			jsonData := line[6:]
			if jsonData != "[DONE]" {
				var data vertex.StreamData
				if json.Unmarshal([]byte(jsonData), &data) == nil {
					if len(data.Response.Candidates) > 0 {
						candidate := data.Response.Candidates[0]
						if candidate.FinishReason != "" {
							finishReason = candidate.FinishReason
						}
						for _, part := range candidate.Content.Parts {
							allParts = append(allParts, core.Part{
								Text:             part.Text,
								Thought:          part.Thought,
								ThoughtSignature: part.ThoughtSignature,
								FunctionCall:     part.FunctionCall,
							})
						}
					}
					if data.Response.UsageMetadata != nil {
						usage = data.Response.UsageMetadata
					}
				}
			}
			// 转换行格式
			transformed := gemini.TransformGeminiStreamLine(line)
			fmt.Fprintf(w, "%s\n\n", transformed)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}

	duration := time.Since(startTime)

	if err := scanner.Err(); err != nil {
		logger.Error("Stream scan error: %v", err)
	}

	// 构建原始响应
	rawResp := &core.AntigravityResponse{}
	rawResp.Response.Candidates = []core.Candidate{
		{
			Content: core.Content{
				Role:  "model",
				Parts: allParts,
			},
			FinishReason: finishReason,
		},
	}
	rawResp.Response.UsageMetadata = usage

	// 记录流式响应日志（原始 Vertex 格式）
	logger.BackendStreamResponse(http.StatusOK, duration, rawResp)
	// Gemini API 客户端响应格式与 Vertex 类似
	geminiResp := gemini.ExtractGeminiResponse(rawResp)
	logger.ClientStreamResponse(http.StatusOK, duration, geminiResp)
}

// handleRawGeminiGenerateContent 原始 Gemini 透传（非流式）
func handleRawGeminiGenerateContent(w http.ResponseWriter, r *http.Request, model string) {
	// 读取原始请求体
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}

	// 记录原始客户端请求
	logger.ClientRequest(r.Method, r.URL.Path, rawBody)

	// 反序列化用于业务逻辑
	var req gemini.GeminiRequest
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

	startTime := time.Now()

	// 转换请求
	antigravityReq := gemini.ConvertGeminiToAntigravity(model, &req, token)

	// 发送请求
	ctx := r.Context()
	resp, err := vertex.GenerateContent(ctx, antigravityReq, token)
	if err != nil {
		duration := time.Since(startTime)
		logger.ClientResponse(getErrorStatus(err), duration, err.Error())
		WriteError(w, getErrorStatus(err), err.Error())
		return
	}

	// 直接返回原始响应（包含 response 字段）
	duration := time.Since(startTime)
	logger.ClientResponse(http.StatusOK, duration, resp)
	WriteJSON(w, http.StatusOK, resp)
}

// handleRawGeminiStreamGenerateContent 原始 Gemini 透传（流式）
func handleRawGeminiStreamGenerateContent(w http.ResponseWriter, r *http.Request, model string) {
	startTime := time.Now()

	// 读取原始请求体
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}

	// 记录原始客户端请求
	logger.ClientRequest(r.Method, r.URL.Path, rawBody)

	// 反序列化用于业务逻辑
	var req gemini.GeminiRequest
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

	// 转换请求
	antigravityReq := gemini.ConvertGeminiToAntigravity(model, &req, token)

	// 发送流式请求
	ctx := r.Context()
	resp, err := vertex.GenerateContentStream(ctx, antigravityReq, token)
	if err != nil {
		WriteError(w, getErrorStatus(err), err.Error())
		return
	}
	defer resp.Body.Close()

	// 设置流式响应头
	vertex.SetStreamHeaders(w)

	// 处理 gzip
	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			vertex.WriteStreamError(w, err.Error())
			return
		}
		defer gzReader.Close()
		reader = gzReader
	}

	// 直接转发原始流式数据（不转换，16MB缓冲区）并收集日志
	scanner := bufio.NewScanner(reader)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 16*1024*1024)

	// 收集所有 parts 用于构建原始响应
	var allParts []core.Part
	var finishReason string
	var usage *core.UsageMetadata

	for scanner.Scan() {
		line := scanner.Text()
		// 收集数据用于日志
		if strings.HasPrefix(line, "data: ") {
			jsonData := line[6:]
			if jsonData != "[DONE]" {
				var data vertex.StreamData
				if json.Unmarshal([]byte(jsonData), &data) == nil {
					if len(data.Response.Candidates) > 0 {
						candidate := data.Response.Candidates[0]
						if candidate.FinishReason != "" {
							finishReason = candidate.FinishReason
						}
						for _, part := range candidate.Content.Parts {
							allParts = append(allParts, core.Part{
								Text:             part.Text,
								Thought:          part.Thought,
								ThoughtSignature: part.ThoughtSignature,
								FunctionCall:     part.FunctionCall,
							})
						}
					}
					if data.Response.UsageMetadata != nil {
						usage = data.Response.UsageMetadata
					}
				}
			}
		}
		fmt.Fprintf(w, "%s\n", line)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}

	duration := time.Since(startTime)

	if err := scanner.Err(); err != nil {
		logger.Error("Stream scan error: %v", err)
	}

	// 构建原始响应
	rawResp := &core.AntigravityResponse{}
	rawResp.Response.Candidates = []core.Candidate{
		{
			Content: core.Content{
				Role:  "model",
				Parts: allParts,
			},
			FinishReason: finishReason,
		},
	}
	rawResp.Response.UsageMetadata = usage

	// 记录流式响应日志（原始 Vertex 格式）
	logger.BackendStreamResponse(http.StatusOK, duration, rawResp)
	// 原始 Gemini 透传，客户端响应与后端相同
	logger.ClientStreamResponse(http.StatusOK, duration, rawResp)
}
