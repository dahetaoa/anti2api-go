package claude

import (
	"anti2api-golang/internal/core"
)

// ==================== Core 类型别名 ====================

// AntigravityRequest 内部请求格式
type AntigravityRequest = core.AntigravityRequest

// AntigravityInnerReq 内部请求体
type AntigravityInnerReq = core.AntigravityInnerReq

// AntigravityResponse 内部响应格式
type AntigravityResponse = core.AntigravityResponse

// Content 消息内容
type Content = core.Content

// Part 消息部分
type Part = core.Part

// FunctionCall 函数调用
type FunctionCall = core.FunctionCall

// FunctionResponse 函数响应
type FunctionResponse = core.FunctionResponse

// InlineData 内联数据
type InlineData = core.InlineData

// SystemInstruction 系统指令
type SystemInstruction = core.SystemInstruction

// Tool 工具定义
type Tool = core.Tool

// FunctionDeclaration 函数声明
type FunctionDeclaration = core.FunctionDeclaration

// ToolConfig 工具配置
type ToolConfig = core.ToolConfig

// FunctionCallingConfig 函数调用配置
type FunctionCallingConfig = core.FunctionCallingConfig

// GenerationConfig 生成配置
type GenerationConfig = core.GenerationConfig

// ThinkingConfig 思考配置
type ThinkingConfig = core.ThinkingConfig

// Candidate 候选响应
type Candidate = core.Candidate

// UsageMetadata 使用统计
type UsageMetadata = core.UsageMetadata

// ==================== Core Models 函数/变量别名 ====================

// Model 模型定义
type Model = core.Model

// SupportedModels 支持的模型列表
var SupportedModels = core.SupportedModels

// DefaultStopSequences 默认停止序列
var DefaultStopSequences = core.DefaultStopSequences

// ResolveModelName 解析真实模型名
var ResolveModelName = core.ResolveModelName

// IsBypassModel 检测是否为 bypass 模型
var IsBypassModel = core.IsBypassModel

// IsClaudeModel 检测是否为 Claude 模型
var IsClaudeModel = core.IsClaudeModel

// IsThinkingModel 检测是否为思考模型
var IsThinkingModel = core.IsThinkingModel

// ShouldEnableThinking 判断是否应该启用思考模式
var ShouldEnableThinking = core.ShouldEnableThinking

// BuildThinkingConfig 构建思考配置
var BuildThinkingConfig = core.BuildThinkingConfig

// GetClaudeMaxOutputTokens 获取 Claude 模型最大输出 Token
var GetClaudeMaxOutputTokens = core.GetClaudeMaxOutputTokens

// ==================== Core 工具调用类型 ====================

// ToolCallInfo 工具调用信息（通用中间格式）
type ToolCallInfo = core.ToolCallInfo

// Usage 通用 token 使用统计
type Usage = core.Usage

// ==================== Claude 请求格式 ====================

// ClaudeMessagesRequest Claude /v1/messages 请求
type ClaudeMessagesRequest struct {
	Model         string          `json:"model"`
	MaxTokens     int             `json:"max_tokens"`
	Messages      []ClaudeMessage `json:"messages"`
	System        interface{}     `json:"system,omitempty"` // string 或 []ClaudeSystemBlock
	Stream        bool            `json:"stream"`
	Temperature   *float64        `json:"temperature,omitempty"`
	TopP          *float64        `json:"top_p,omitempty"`
	StopSequences []string        `json:"stop_sequences,omitempty"`
	Tools         []ClaudeTool    `json:"tools,omitempty"`
	ToolChoice    interface{}     `json:"tool_choice,omitempty"`
	Thinking      *ClaudeThinking `json:"thinking,omitempty"`
	Metadata      *ClaudeMetadata `json:"metadata,omitempty"`
}

// ClaudeMessage Claude 消息
type ClaudeMessage struct {
	Role    string      `json:"role"`    // user, assistant
	Content interface{} `json:"content"` // string 或 []ClaudeContentBlock
}

// ClaudeSystemBlock Claude 系统消息块
type ClaudeSystemBlock struct {
	Type string `json:"type"` // text
	Text string `json:"text"`
}

// ClaudeContentBlock Claude 内容块
type ClaudeContentBlock struct {
	Type      string             `json:"type"`                  // text, thinking, tool_use, tool_result, image
	Text      string             `json:"text,omitempty"`        // type=text
	Thinking  string             `json:"thinking,omitempty"`    // type=thinking
	Signature string             `json:"signature,omitempty"`   // type=thinking 的签名验证字段
	ID        string             `json:"id,omitempty"`          // type=tool_use
	Name      string             `json:"name,omitempty"`        // type=tool_use
	Input     interface{}        `json:"input,omitempty"`       // type=tool_use
	ToolUseID string             `json:"tool_use_id,omitempty"` // type=tool_result
	Content   interface{}        `json:"content,omitempty"`     // type=tool_result (string 或 []ClaudeContentBlock)
	IsError   bool               `json:"is_error,omitempty"`    // type=tool_result
	Source    *ClaudeImageSource `json:"source,omitempty"`      // type=image
}

// ClaudeImageSource Claude 图片源
type ClaudeImageSource struct {
	Type      string `json:"type"`       // base64
	MediaType string `json:"media_type"` // image/jpeg, image/png, etc.
	Data      string `json:"data"`       // base64 encoded data
}

// ClaudeTool Claude 工具定义
type ClaudeTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

// ClaudeThinking Claude 思考配置
type ClaudeThinking struct {
	Type   string `json:"type"`                   // enabled, disabled
	Budget int    `json:"budget,omitempty"`         // thinking token budget
	Level  string `json:"thinking_level,omitempty"` // thinking level
}

// ClaudeMetadata Claude 元数据
type ClaudeMetadata struct {
	UserID string `json:"user_id,omitempty"`
}

// ==================== Claude 响应格式 ====================

// ClaudeMessagesResponse Claude /v1/messages 响应
type ClaudeMessagesResponse struct {
	ID           string               `json:"id"`
	Type         string               `json:"type"` // message
	Role         string               `json:"role"` // assistant
	Model        string               `json:"model"`
	Content      []ClaudeContentBlock `json:"content"`
	StopReason   string               `json:"stop_reason"` // end_turn, tool_use, max_tokens, stop_sequence
	StopSequence *string              `json:"stop_sequence"`
	Usage        ClaudeUsage          `json:"usage"`
}

// ClaudeUsage Claude 使用统计
type ClaudeUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// ClaudeTokenCountResponse Claude token 计数响应
type ClaudeTokenCountResponse struct {
	InputTokens int `json:"input_tokens"`
	TokenCount  int `json:"token_count"`
	Tokens      int `json:"tokens"`
}

// ==================== Claude SSE 事件格式 ====================

// ClaudeSSEMessageStart message_start 事件
type ClaudeSSEMessageStart struct {
	Type    string                  `json:"type"` // message_start
	Message ClaudeSSEMessagePayload `json:"message"`
}

// ClaudeSSEMessagePayload message 负载
type ClaudeSSEMessagePayload struct {
	ID           string        `json:"id"`
	Type         string        `json:"type"` // message
	Role         string        `json:"role"` // assistant
	Model        string        `json:"model"`
	StopSequence *string       `json:"stop_sequence"`
	Usage        ClaudeUsage   `json:"usage"`
	Content      []interface{} `json:"content"`
	StopReason   *string       `json:"stop_reason"`
}

// ClaudeSSEContentBlockStart content_block_start 事件
// ContentBlock 使用 map[string]interface{} 以确保可以精确控制序列化，
// 包括空字符串的显式包含（Claude API 规范要求）
type ClaudeSSEContentBlockStart struct {
	Type         string                 `json:"type"` // content_block_start
	Index        int                    `json:"index"`
	ContentBlock map[string]interface{} `json:"content_block"`
}

// NewTextContentBlock 创建文本内容块（包含 text: ""）
func NewTextContentBlock() map[string]interface{} {
	return map[string]interface{}{
		"type": "text",
		"text": "",
	}
}

// NewThinkingContentBlock 创建思考内容块（包含 thinking: ""）
func NewThinkingContentBlock() map[string]interface{} {
	return map[string]interface{}{
		"type":     "thinking",
		"thinking": "",
	}
}

// NewToolUseContentBlock 创建工具调用内容块
func NewToolUseContentBlock(id, name string) map[string]interface{} {
	return map[string]interface{}{
		"type":  "tool_use",
		"id":    id,
		"name":  name,
		"input": map[string]interface{}{},
	}
}

// ClaudeSSEContentBlockDelta content_block_delta 事件
type ClaudeSSEContentBlockDelta struct {
	Type  string         `json:"type"` // content_block_delta
	Index int            `json:"index"`
	Delta ClaudeSSEDelta `json:"delta"`
}

// ClaudeSSEDelta 增量负载
type ClaudeSSEDelta struct {
	Type        string `json:"type"`                   // text_delta, thinking_delta, input_json_delta, signature_delta
	Text        string `json:"text,omitempty"`         // type=text_delta
	Thinking    string `json:"thinking,omitempty"`     // type=thinking_delta
	PartialJSON string `json:"partial_json,omitempty"` // type=input_json_delta
	Signature   string `json:"signature,omitempty"`    // type=signature_delta
}

// ClaudeSSEContentBlockStop content_block_stop 事件
type ClaudeSSEContentBlockStop struct {
	Type  string `json:"type"` // content_block_stop
	Index int    `json:"index"`
}

// ClaudeSSEMessageDelta message_delta 事件
type ClaudeSSEMessageDelta struct {
	Type  string                       `json:"type"` // message_delta
	Delta ClaudeSSEMessageDeltaPayload `json:"delta"`
	Usage ClaudeUsage                  `json:"usage"`
}

// ClaudeSSEMessageDeltaPayload message_delta 负载
type ClaudeSSEMessageDeltaPayload struct {
	StopReason   string  `json:"stop_reason"`
	StopSequence *string `json:"stop_sequence"`
}

// ClaudeSSEMessageStop message_stop 事件
type ClaudeSSEMessageStop struct {
	Type string `json:"type"` // message_stop
}

// ClaudeErrorResponse Claude 错误响应
type ClaudeErrorResponse struct {
	Type  string `json:"type"` // error
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}
