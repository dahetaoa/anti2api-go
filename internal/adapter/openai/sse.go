package openai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"unicode/utf8"

	"anti2api-golang/internal/core"
	"anti2api-golang/internal/utils"
)

// StreamData 原始流式数据（从 vertex 包复制，用于解耦）
type StreamData struct {
	Response struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text             string             `json:"text,omitempty"`
					FunctionCall     *core.FunctionCall `json:"functionCall,omitempty"`
					Thought          bool               `json:"thought,omitempty"`
					ThoughtSignature string             `json:"thoughtSignature,omitempty"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason,omitempty"`
		} `json:"candidates"`
		UsageMetadata *core.UsageMetadata `json:"usageMetadata,omitempty"`
	} `json:"response"`
}

// StreamDataPart 单个 Part 数据（用于从外部逐个处理）
type StreamDataPart struct {
	Text             string
	FunctionCall     *core.FunctionCall
	Thought          bool
	ThoughtSignature string
}

// SSEWriter 流式写入器（带 UTF-8 缓冲，线程安全）
type SSEWriter struct {
	w               http.ResponseWriter
	id              string
	created         int64
	model           string
	sentRole        bool
	contentBuffer   []byte              // 缓冲不完整的 UTF-8 内容字节
	reasoningBuffer []byte              // 缓冲不完整的 UTF-8 思考字节
	toolCalls       []core.ToolCallInfo // 累积工具调用
	mu              sync.Mutex          // 保护并发写入
}

// NewSSEWriter 创建流式写入器
func NewSSEWriter(w http.ResponseWriter, id string, created int64, model string) *SSEWriter {
	SetSSEHeaders(w)
	return &SSEWriter{
		w:       w,
		id:      id,
		created: created,
		model:   model,
	}
}

// ProcessData 处理 Vertex 流式数据并转换为 OpenAI 格式
func (sw *SSEWriter) ProcessData(data *StreamData) error {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if len(data.Response.Candidates) == 0 {
		return nil
	}

	candidate := data.Response.Candidates[0]

	for _, part := range candidate.Content.Parts {
		if part.Thought {
			// 1. 处理 Thinking
			if err := sw.writeReasoningLocked(part.Text); err != nil {
				return err
			}
		} else if part.Text != "" {
			// 2. 处理普通文本
			if err := sw.writeContentLocked(part.Text); err != nil {
				return err
			}

		} else if part.FunctionCall != nil {
			// 3. 处理工具调用
			id := part.FunctionCall.ID
			if id == "" {
				id = utils.GenerateToolCallID()
			}

			sw.toolCalls = append(sw.toolCalls, core.ToolCallInfo{
				ID:               id,
				Name:             part.FunctionCall.Name,
				Args:             part.FunctionCall.Args,
				ThoughtSignature: part.ThoughtSignature,
			})
		}
	}

	// 响应结束或遇到停止原因时发送工具调用
	if candidate.FinishReason != "" && len(sw.toolCalls) > 0 {
		if err := sw.writeToolCallsLocked(sw.toolCalls); err != nil {
			return err
		}
		sw.toolCalls = nil
	}

	return nil
}

// ProcessPart 处理单个 Part 数据（外部调用）
func (sw *SSEWriter) ProcessPart(part StreamDataPart) error {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if part.Thought {
		return sw.writeReasoningLocked(part.Text)
	} else if part.Text != "" {
		return sw.writeContentLocked(part.Text)
	} else if part.FunctionCall != nil {
		id := part.FunctionCall.ID
		if id == "" {
			id = utils.GenerateToolCallID()
		}
		sw.toolCalls = append(sw.toolCalls, core.ToolCallInfo{
			ID:               id,
			Name:             part.FunctionCall.Name,
			Args:             part.FunctionCall.Args,
			ThoughtSignature: part.ThoughtSignature,
		})
	}
	return nil
}

// FlushToolCalls 刷新累积的工具调用（当收到 FinishReason 时调用）
func (sw *SSEWriter) FlushToolCalls() error {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if len(sw.toolCalls) > 0 {
		if err := sw.writeToolCallsLocked(sw.toolCalls); err != nil {
			return err
		}
		sw.toolCalls = nil
	}
	return nil
}

// HasToolCalls 检查是否有处理过的工具调用
func (sw *SSEWriter) HasToolCalls() bool {
	return false
}

// writeRoleLocked 写入角色（内部使用，调用者必须持有锁）
func (sw *SSEWriter) writeRoleLocked() error {
	if sw.sentRole {
		return nil
	}
	sw.sentRole = true

	chunk := CreateStreamChunk(
		sw.id, sw.created, sw.model,
		&Delta{Role: "assistant"},
		nil, nil,
	)
	return WriteSSEData(sw.w, chunk)
}

// WriteRole 写入角色（首次，线程安全）
func (sw *SSEWriter) WriteRole() error {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.writeRoleLocked()
}

// extractValidUTF8 从字节切片中提取有效的 UTF-8 字符串，返回有效部分和剩余的不完整字节
func extractValidUTF8(data []byte) (valid string, remaining []byte) {
	if len(data) == 0 {
		return "", nil
	}

	// 检查整个字符串是否是有效的 UTF-8
	if utf8.Valid(data) {
		return string(data), nil
	}

	// 从末尾向前查找不完整的 UTF-8 字符
	checkLen := 4
	if len(data) < checkLen {
		checkLen = len(data)
	}

	for i := 1; i <= checkLen; i++ {
		idx := len(data) - i
		b := data[idx]

		// 如果是多字节起始字节
		if b >= 0xC0 {
			var expectedLen int
			if b >= 0xF0 {
				expectedLen = 4
			} else if b >= 0xE0 {
				expectedLen = 3
			} else {
				expectedLen = 2
			}

			actualLen := len(data) - idx
			if actualLen < expectedLen {
				return string(data[:idx]), data[idx:]
			}
			break
		}
		if b >= 0x80 && b < 0xC0 {
			continue
		}
		break
	}

	// 再次验证，移除末尾无效字节
	for len(data) > 0 {
		if utf8.Valid(data) {
			return string(data), nil
		}
		remaining = append([]byte{data[len(data)-1]}, remaining...)
		data = data[:len(data)-1]
	}

	return "", remaining
}

// writeContentLocked 写入内容（内部使用，带 UTF-8 缓冲）
func (sw *SSEWriter) writeContentLocked(content string) error {
	sw.writeRoleLocked()

	data := append(sw.contentBuffer, []byte(content)...)
	sw.contentBuffer = nil

	validContent, remaining := extractValidUTF8(data)
	sw.contentBuffer = remaining

	if validContent == "" {
		return nil
	}

	chunk := CreateStreamChunk(
		sw.id, sw.created, sw.model,
		&Delta{Content: validContent},
		nil, nil,
	)
	return WriteSSEData(sw.w, chunk)
}

// WriteContent 写入内容（带 UTF-8 缓冲，线程安全）
func (sw *SSEWriter) WriteContent(content string) error {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.writeContentLocked(content)
}

// writeReasoningLocked 写入思考内容（内部使用，带 UTF-8 缓冲）
func (sw *SSEWriter) writeReasoningLocked(reasoning string) error {
	sw.writeRoleLocked()

	data := append(sw.reasoningBuffer, []byte(reasoning)...)
	sw.reasoningBuffer = nil

	validReasoning, remaining := extractValidUTF8(data)
	sw.reasoningBuffer = remaining

	if validReasoning == "" {
		return nil
	}

	chunk := CreateStreamChunk(
		sw.id, sw.created, sw.model,
		&Delta{Reasoning: validReasoning},
		nil, nil,
	)
	return WriteSSEData(sw.w, chunk)
}

// WriteReasoning 写入思考内容（带 UTF-8 缓冲，线程安全）
func (sw *SSEWriter) WriteReasoning(reasoning string) error {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.writeReasoningLocked(reasoning)
}

// writeToolCallsLocked 写入工具调用（内部使用）
func (sw *SSEWriter) writeToolCallsLocked(toolCalls []core.ToolCallInfo) error {
	sw.writeRoleLocked()

	openaiCalls := make([]OpenAIToolCall, len(toolCalls))
	for i, tc := range toolCalls {
		argsJSON, _ := json.Marshal(tc.Args)
		openaiCalls[i] = OpenAIToolCall{
			ID:   tc.ID,
			Type: "function",
			Function: OpenAIFunctionCall{
				Name:      tc.Name,
				Arguments: string(argsJSON),
			},
			ThoughtSignature: tc.ThoughtSignature,
		}
	}

	chunk := CreateStreamChunk(
		sw.id, sw.created, sw.model,
		&Delta{ToolCalls: openaiCalls},
		nil, nil,
	)
	return WriteSSEData(sw.w, chunk)
}

// WriteToolCalls 写入工具调用（线程安全）
func (sw *SSEWriter) WriteToolCalls(toolCalls []core.ToolCallInfo) error {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.writeToolCallsLocked(toolCalls)
}

// flushLocked 刷新缓冲区中剩余的内容
func (sw *SSEWriter) flushLocked() error {
	if len(sw.contentBuffer) > 0 {
		content := string(sw.contentBuffer)
		sw.contentBuffer = nil
		if content != "" {
			chunk := CreateStreamChunk(
				sw.id, sw.created, sw.model,
				&Delta{Content: content},
				nil, nil,
			)
			if err := WriteSSEData(sw.w, chunk); err != nil {
				return err
			}
		}
	}

	if len(sw.reasoningBuffer) > 0 {
		reasoning := string(sw.reasoningBuffer)
		sw.reasoningBuffer = nil
		if reasoning != "" {
			chunk := CreateStreamChunk(
				sw.id, sw.created, sw.model,
				&Delta{Reasoning: reasoning},
				nil, nil,
			)
			if err := WriteSSEData(sw.w, chunk); err != nil {
				return err
			}
		}
	}

	return nil
}

// Flush 刷新缓冲区中剩余的内容（线程安全）
func (sw *SSEWriter) Flush() error {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.flushLocked()
}

// WriteFinish 写入结束（线程安全）
func (sw *SSEWriter) WriteFinish(reason string, usage *Usage) error {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	sw.flushLocked()

	chunk := CreateStreamChunk(
		sw.id, sw.created, sw.model,
		&Delta{},
		&reason, usage,
	)
	if err := WriteSSEData(sw.w, chunk); err != nil {
		return err
	}
	WriteSSEDone(sw.w)
	return nil
}

// WriteHeartbeat 写入心跳（发送空 delta 的有效数据包，线程安全）
func (sw *SSEWriter) WriteHeartbeat() error {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	sw.writeRoleLocked()

	chunk := CreateStreamChunk(
		sw.id, sw.created, sw.model,
		&Delta{},
		nil, nil,
	)
	return WriteSSEData(sw.w, chunk)
}

// SetSSEHeaders 设置流式响应头
func SetSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
}

// WriteSSEData 写入流式数据
func WriteSSEData(w http.ResponseWriter, data interface{}) error {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", jsonBytes)
	if err != nil {
		return err
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}

// WriteSSEDone 写入流结束标记
func WriteSSEDone(w http.ResponseWriter) {
	w.Write([]byte("data: [DONE]\n\n"))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// WriteSSEError 写入流错误
func WriteSSEError(w http.ResponseWriter, errMsg string) {
	errResp := map[string]interface{}{
		"error": map[string]interface{}{
			"message": errMsg,
			"type":    "server_error",
		},
	}
	WriteSSEData(w, errResp)
	WriteSSEDone(w)
}
