package core

import "strings"

// Model 模型定义
type Model struct {
	ID      string `json:"id"`
	OwnedBy string `json:"owned_by"`
	Object  string `json:"object"`
}

// SupportedModels 支持的模型列表
var SupportedModels = []Model{
	// Gemini 系列
	{ID: "gemini-3-pro-high", OwnedBy: "google", Object: "model"},
	{ID: "gemini-3-pro-low", OwnedBy: "google", Object: "model"},
	// Gemini Bypass 模式（非流式规避截断）
	{ID: "gemini-3-pro-high-bypass", OwnedBy: "google", Object: "model"},
	{ID: "gemini-3-pro-low-bypass", OwnedBy: "google", Object: "model"},
	// Claude 系列
	{ID: "claude-opus-4-5-thinking", OwnedBy: "anthropic", Object: "model"},
	{ID: "claude-sonnet-4-5", OwnedBy: "anthropic", Object: "model"},
	{ID: "claude-sonnet-4-5-thinking", OwnedBy: "anthropic", Object: "model"},
}

// ModelAliasMap 模型别名映射（bypass 模式）
var ModelAliasMap = map[string]string{
	"gemini-3-pro-high-bypass": "gemini-3-pro-high",
	"gemini-3-pro-low-bypass":  "gemini-3-pro-low",
}

// DefaultStopSequences 默认停止序列
var DefaultStopSequences = []string{
	"<|user|>",
	"<|bot|>",
	"<|context_request|>",
	"<|endoftext|>",
	"<|end_of_turn|>",
}

// ResolveModelName 解析真实模型名
func ResolveModelName(modelName string) string {
	if alias, ok := ModelAliasMap[modelName]; ok {
		return alias
	}
	return modelName
}

// IsBypassModel 检测是否为 bypass 模型
func IsBypassModel(modelName string) bool {
	return strings.HasSuffix(modelName, "-bypass")
}

// IsClaudeModel 检测是否为 Claude 模型
func IsClaudeModel(modelName string) bool {
	return strings.Contains(strings.ToLower(modelName), "claude")
}

// IsThinkingModel 检测是否为思考模型
func IsThinkingModel(modelName string) bool {
	return strings.HasSuffix(modelName, "-thinking")
}

// ShouldEnableThinking 判断是否应该启用思考模式
func ShouldEnableThinking(modelName string, thinkingConfig *ThinkingConfig) bool {
	// 强制禁用检查（bypass 模式映射）
	if _, isBypass := ModelAliasMap[modelName]; isBypass {
		return false
	}

	// 检查 -thinking 后缀
	if strings.HasSuffix(modelName, "-thinking") {
		return true
	}

	// Gemini 3 Pro 系列默认启用
	if strings.HasPrefix(modelName, "gemini-3-pro-") {
		return true
	}

	// 检查请求中的显式配置
	if thinkingConfig != nil {
		return thinkingConfig.IncludeThoughts
	}

	return false
}

// BuildThinkingConfig 构建思考配置
func BuildThinkingConfig(modelName string) *ThinkingConfig {
	actualModel := ResolveModelName(modelName)

	if strings.HasPrefix(actualModel, "gemini-3-pro-") {
		// Gemini 3 Pro：不传 thinkingBudget，让后端决定
		return &ThinkingConfig{IncludeThoughts: true}
	}

	if IsClaudeModel(actualModel) {
		// Claude：thinkingBudget 默认 32000
		return &ThinkingConfig{
			IncludeThoughts: true,
			ThinkingBudget:  32000,
		}
	}

	// 其他模型默认
	return &ThinkingConfig{
		IncludeThoughts: true,
		ThinkingBudget:  1024,
	}
}

// GetClaudeMaxOutputTokens 获取 Claude 模型最大输出 Token
func GetClaudeMaxOutputTokens(modelName string) int {
	// 统一返回 64000
	return 64000
}
