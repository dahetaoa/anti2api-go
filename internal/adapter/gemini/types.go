package gemini

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

// ==================== Gemini 格式 ====================

// GeminiRequest 标准 Gemini 请求
type GeminiRequest struct {
	Contents          []Content          `json:"contents"`
	SystemInstruction *SystemInstruction `json:"systemInstruction,omitempty"`
	GenerationConfig  *GenerationConfig  `json:"generationConfig,omitempty"`
	Tools             []Tool             `json:"tools,omitempty"`
	ToolConfig        *ToolConfig        `json:"toolConfig,omitempty"`
}

// GeminiResponse 标准 Gemini 响应
type GeminiResponse struct {
	Candidates    []Candidate    `json:"candidates"`
	UsageMetadata *UsageMetadata `json:"usageMetadata,omitempty"`
}
