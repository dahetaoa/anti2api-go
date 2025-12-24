package openai

import "anti2api-golang/internal/core"

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

// ==================== OpenAI 格式 ====================

// OpenAIChatRequest OpenAI 聊天请求
type OpenAIChatRequest struct {
	Model       string          `json:"model"`
	Messages    []OpenAIMessage `json:"messages"`
	Stream      bool            `json:"stream"`
	Temperature *float64        `json:"temperature,omitempty"`
	TopP        *float64        `json:"top_p,omitempty"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Stop        []string        `json:"stop,omitempty"`
	Tools       []OpenAITool    `json:"tools,omitempty"`
	ToolChoice  interface{}     `json:"tool_choice,omitempty"`
}

// OpenAIMessage OpenAI 消息格式
type OpenAIMessage struct {
	Role       string           `json:"role"`
	Content    interface{}      `json:"content"`
	ToolCalls  []OpenAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	Name       string           `json:"name,omitempty"`
	Reasoning  string           `json:"reasoning,omitempty"`
}

// OpenAIContentPart OpenAI 内容部分
type OpenAIContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL 图片 URL
type ImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

// OpenAITool OpenAI 工具定义
type OpenAITool struct {
	Type     string         `json:"type"`
	Function OpenAIFunction `json:"function"`
}

// OpenAIFunction OpenAI 函数定义
type OpenAIFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// OpenAIToolCall OpenAI 工具调用
type OpenAIToolCall struct {
	ID               string             `json:"id"`
	Type             string             `json:"type"`
	Function         OpenAIFunctionCall `json:"function"`
	ThoughtSignature string             `json:"thought_signature,omitempty"`
}

// OpenAIFunctionCall OpenAI 函数调用
type OpenAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// OpenAIChatCompletion OpenAI 聊天完成响应
type OpenAIChatCompletion struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   *Usage   `json:"usage,omitempty"`
}

// Choice 选择
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message,omitempty"`
	Delta        *Delta  `json:"delta,omitempty"`
	FinishReason *string `json:"finish_reason"`
}

// Message 消息
type Message struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []OpenAIToolCall `json:"tool_calls,omitempty"`
	Reasoning string           `json:"reasoning,omitempty"`
}

// Delta 流式增量
type Delta struct {
	Role      string           `json:"role,omitempty"`
	Content   string           `json:"content,omitempty"`
	ToolCalls []OpenAIToolCall `json:"tool_calls,omitempty"`
	Reasoning string           `json:"reasoning,omitempty"`
}

// Usage 使用统计
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// OpenAIStreamChunk 流式 Chunk
type OpenAIStreamChunk struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   *Usage   `json:"usage,omitempty"`
}

// ModelsResponse 模型列表响应
type ModelsResponse struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}
