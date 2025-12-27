package claude

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

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

// SSEEmitter Claude SSE 发射器
type SSEEmitter struct {
	w                  http.ResponseWriter
	requestID          string
	model              string
	inputTokens        int
	nextIndex          int
	textBlockIndex     *int
	thinkingBlockIndex *int
	finished           bool
	totalOutputTokens  int
	hasToolCalls       bool   // 记录是否遇到过工具调用
	pendingSignature   string // 待发送的 thinking block signature
	mu                 sync.Mutex
	// 用于收集原始 JSON 以便日志记录（透传）
	collectedEvents []map[string]interface{}
}

// NewSSEEmitter 创建 Claude SSE 发射器
func NewSSEEmitter(w http.ResponseWriter, requestID string, model string, inputTokens int) *SSEEmitter {
	if requestID == "" {
		requestID = utils.GenerateRequestID()
	}
	if model == "" {
		model = "claude-proxy"
	}

	return &SSEEmitter{
		w:                  w,
		requestID:          requestID,
		model:              model,
		inputTokens:        inputTokens,
		nextIndex:          0,
		textBlockIndex:     nil,
		thinkingBlockIndex: nil,
		finished:           false,
		totalOutputTokens:  0,
		pendingSignature:   "",
		collectedEvents:    nil,
	}
}

// ProcessData 处理 Vertex 原始流式数据并转换为 Claude 格式
func (e *SSEEmitter) ProcessData(data *StreamData) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(data.Response.Candidates) == 0 {
		return nil
	}

	candidate := data.Response.Candidates[0]

	for _, part := range candidate.Content.Parts {
		if part.Thought {
			// 1. 处理 Thinking
			if err := e.sendThinkingLocked(part.Text); err != nil {
				return err
			}
			// 捕获 thinking block 的 signature
			if part.ThoughtSignature != "" {
				e.pendingSignature = part.ThoughtSignature
			}
		} else if part.Text != "" {
			// 2. 处理普通文本
			if err := e.sendTextLocked(part.Text); err != nil {
				return err
			}
		} else if part.FunctionCall != nil {
			// 3. 处理工具调用
			id := part.FunctionCall.ID
			if id == "" {
				id = utils.GenerateToolCallID()
			}

			// 单个 tool call
			tc := core.ToolCallInfo{
				ID:               id,
				Name:             part.FunctionCall.Name,
				Args:             part.FunctionCall.Args,
				ThoughtSignature: part.ThoughtSignature,
			}
			if err := e.sendToolCallLocked(tc); err != nil {
				return err
			}
		}
	}

	return nil
}

// ProcessPart 处理单个 Part 数据（外部调用）
func (e *SSEEmitter) ProcessPart(part StreamDataPart) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if part.Thought {
		// 捕获 thinking block 的 signature
		if part.ThoughtSignature != "" {
			e.pendingSignature = part.ThoughtSignature
		}
		return e.sendThinkingLocked(part.Text)
	} else if part.Text != "" {
		return e.sendTextLocked(part.Text)
	} else if part.FunctionCall != nil {
		id := part.FunctionCall.ID
		if id == "" {
			id = utils.GenerateToolCallID()
		}
		tc := core.ToolCallInfo{
			ID:               id,
			Name:             part.FunctionCall.Name,
			Args:             part.FunctionCall.Args,
			ThoughtSignature: part.ThoughtSignature,
		}
		return e.sendToolCallLocked(tc)
	}
	return nil
}

// writeSSE 写入 SSE 事件并收集原始 JSON
func (e *SSEEmitter) writeSSE(event string, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	// 收集原始 JSON 用于日志透传
	var eventData map[string]interface{}
	if err := json.Unmarshal(jsonData, &eventData); err == nil {
		e.collectedEvents = append(e.collectedEvents, eventData)
	}

	_, err = fmt.Fprintf(e.w, "event: %s\ndata: %s\n\n", event, string(jsonData))
	if err != nil {
		return err
	}
	if f, ok := e.w.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}

// Start 发送 message_start 事件
func (e *SSEEmitter) Start() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.writeSSE("message_start", ClaudeSSEMessageStart{
		Type: "message_start",
		Message: ClaudeSSEMessagePayload{
			ID:           "msg_" + e.requestID,
			Type:         "message",
			Role:         "assistant",
			Model:        e.model,
			StopSequence: nil,
			Usage: ClaudeUsage{
				InputTokens:  e.inputTokens,
				OutputTokens: 0,
			},
			Content:    []interface{}{},
			StopReason: nil,
		},
	})
}

// ensureTextBlock 确保文本块已启动
func (e *SSEEmitter) ensureTextBlock() error {
	if e.textBlockIndex != nil {
		return nil
	}
	index := e.nextIndex
	e.nextIndex++
	e.textBlockIndex = &index

	return e.writeSSE("content_block_start", ClaudeSSEContentBlockStart{
		Type:  "content_block_start",
		Index: index,
		ContentBlock: ClaudeSSEContentBlock{
			Type: "text",
			Text: "",
		},
	})
}

// ensureThinkingBlock 确保思考块已启动
func (e *SSEEmitter) ensureThinkingBlock() error {
	if e.thinkingBlockIndex != nil {
		return nil
	}
	index := e.nextIndex
	e.nextIndex++
	e.thinkingBlockIndex = &index

	return e.writeSSE("content_block_start", ClaudeSSEContentBlockStart{
		Type:  "content_block_start",
		Index: index,
		ContentBlock: ClaudeSSEContentBlock{
			Type:     "thinking",
			Thinking: "",
		},
	})
}

// closeTextBlock 关闭文本块
func (e *SSEEmitter) closeTextBlock() error {
	if e.textBlockIndex == nil {
		return nil
	}
	index := *e.textBlockIndex
	e.textBlockIndex = nil
	return e.writeSSE("content_block_stop", ClaudeSSEContentBlockStop{
		Type:  "content_block_stop",
		Index: index,
	})
}

// closeThinkingBlock 关闭思考块
func (e *SSEEmitter) closeThinkingBlock() error {
	if e.thinkingBlockIndex == nil {
		return nil
	}
	index := *e.thinkingBlockIndex
	e.thinkingBlockIndex = nil

	// 在关闭 thinking block 之前发送 signature_delta 事件（按 Claude API 规范）
	if e.pendingSignature != "" {
		if err := e.writeSSE("content_block_delta", ClaudeSSEContentBlockDelta{
			Type:  "content_block_delta",
			Index: index,
			Delta: ClaudeSSEDelta{
				Type:      "signature_delta",
				Signature: e.pendingSignature,
			},
		}); err != nil {
			return err
		}
		e.pendingSignature = "" // 清空已发送的 signature
	}

	return e.writeSSE("content_block_stop", ClaudeSSEContentBlockStop{
		Type:  "content_block_stop",
		Index: index,
	})
}

// sendTextLocked 发送文本增量（内部）
func (e *SSEEmitter) sendTextLocked(text string) error {
	if text == "" {
		return nil
	}

	// 确保思考块先关闭，避免与正文交叉
	if err := e.closeThinkingBlock(); err != nil {
		return err
	}

	if err := e.ensureTextBlock(); err != nil {
		return err
	}

	e.totalOutputTokens += EstimateClaudeTokens(text)

	return e.writeSSE("content_block_delta", ClaudeSSEContentBlockDelta{
		Type:  "content_block_delta",
		Index: *e.textBlockIndex,
		Delta: ClaudeSSEDelta{
			Type: "text_delta",
			Text: text,
		},
	})
}

// sendThinkingLocked 发送思考内容（内部）
func (e *SSEEmitter) sendThinkingLocked(thinking string) error {
	if thinking == "" {
		return nil
	}

	// thinking 到来时关闭已有正文块，避免嵌套
	if err := e.closeTextBlock(); err != nil {
		return err
	}

	if err := e.ensureThinkingBlock(); err != nil {
		return err
	}

	e.totalOutputTokens += EstimateClaudeTokens(thinking)

	return e.writeSSE("content_block_delta", ClaudeSSEContentBlockDelta{
		Type:  "content_block_delta",
		Index: *e.thinkingBlockIndex,
		Delta: ClaudeSSEDelta{
			Type:     "thinking_delta",
			Thinking: thinking,
		},
	})
}

// sendToolCallLocked 发送单个工具调用（内部）
func (e *SSEEmitter) sendToolCallLocked(tc core.ToolCallInfo) error {
	e.hasToolCalls = true

	// 先关闭所有已有块
	if err := e.closeTextBlock(); err != nil {
		return err
	}
	if err := e.closeThinkingBlock(); err != nil {
		return err
	}

	index := e.nextIndex
	e.nextIndex++

	// 序列化 args
	argsJSON, _ := json.Marshal(tc.Args)
	args := string(argsJSON)
	if args == "" || args == "null" {
		args = "{}"
	}

	e.totalOutputTokens += EstimateClaudeTokens(args)

	// content_block_start
	if err := e.writeSSE("content_block_start", ClaudeSSEContentBlockStart{
		Type:  "content_block_start",
		Index: index,
		ContentBlock: ClaudeSSEContentBlock{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Name,
			Input: map[string]interface{}{},
		},
	}); err != nil {
		return err
	}

	// content_block_delta
	if err := e.writeSSE("content_block_delta", ClaudeSSEContentBlockDelta{
		Type:  "content_block_delta",
		Index: index,
		Delta: ClaudeSSEDelta{
			Type:        "input_json_delta",
			PartialJSON: args,
		},
	}); err != nil {
		return err
	}

	// content_block_stop
	if err := e.writeSSE("content_block_stop", ClaudeSSEContentBlockStop{
		Type:  "content_block_stop",
		Index: index,
	}); err != nil {
		return err
	}

	return nil
}

// HasToolCalls 返回是否遇到过工具调用
func (e *SSEEmitter) HasToolCalls() bool {
	return e.hasToolCalls
}

// Finish 完成并发送结束事件
func (e *SSEEmitter) Finish(usage *Usage) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.finished {
		return nil
	}
	e.finished = true

	// 关闭所有打开的块
	e.closeTextBlock()
	e.closeThinkingBlock()

	// 计算 token
	outputTokens := e.totalOutputTokens
	inputTokens := e.inputTokens
	if usage != nil {
		if usage.CompletionTokens > 0 {
			outputTokens = usage.CompletionTokens
		}
		if usage.PromptTokens > 0 {
			inputTokens = usage.PromptTokens
		}
	}

	stopReason := GetClaudeStopReason(e.hasToolCalls)

	// message_delta
	if err := e.writeSSE("message_delta", ClaudeSSEMessageDelta{
		Type: "message_delta",
		Delta: ClaudeSSEMessageDeltaPayload{
			StopReason:   stopReason,
			StopSequence: nil,
		},
		Usage: ClaudeUsage{
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
		},
	}); err != nil {
		return err
	}

	// message_stop
	return e.writeSSE("message_stop", ClaudeSSEMessageStop{
		Type: "message_stop",
	})
}

// GetMergedResponse 返回收集的原始 SSE 事件（用于透传日志记录）
// 合并连续的 thinking_delta 和 text_delta 事件以提高可读性
func (e *SSEEmitter) GetMergedResponse() []interface{} {
	e.mu.Lock()
	defer e.mu.Unlock()

	var result []interface{}
	var pendingThinking string
	var pendingText string
	var pendingIndex int

	// 辅助函数：刷新待处理的合并内容
	flushPending := func() {
		if pendingThinking != "" {
			result = append(result, map[string]interface{}{
				"type":  "content_block_delta",
				"index": pendingIndex,
				"delta": map[string]interface{}{
					"type":     "thinking_delta",
					"thinking": pendingThinking,
				},
			})
			pendingThinking = ""
		}
		if pendingText != "" {
			result = append(result, map[string]interface{}{
				"type":  "content_block_delta",
				"index": pendingIndex,
				"delta": map[string]interface{}{
					"type": "text_delta",
					"text": pendingText,
				},
			})
			pendingText = ""
		}
	}

	for _, event := range e.collectedEvents {
		eventType, _ := event["type"].(string)

		// 检查是否是 content_block_delta 事件
		if eventType == "content_block_delta" {
			delta, _ := event["delta"].(map[string]interface{})
			deltaType, _ := delta["type"].(string)
			index, _ := event["index"].(float64)

			switch deltaType {
			case "thinking_delta":
				// 合并 thinking_delta
				thinking, _ := delta["thinking"].(string)
				if pendingText != "" {
					// 先刷新待处理的 text
					flushPending()
				}
				if pendingThinking == "" {
					pendingIndex = int(index)
				}
				pendingThinking += thinking
				continue

			case "text_delta":
				// 合并 text_delta
				text, _ := delta["text"].(string)
				if pendingThinking != "" {
					// 先刷新待处理的 thinking
					flushPending()
				}
				if pendingText == "" {
					pendingIndex = int(index)
				}
				pendingText += text
				continue
			}
		}

		// 非 thinking_delta/text_delta 事件：先刷新待处理内容，再添加当前事件
		flushPending()
		result = append(result, event)
	}

	// 刷新最后的待处理内容
	flushPending()

	return result
}

// SetSSEHeaders 设置 Claude SSE 响应头
func SetSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
}
