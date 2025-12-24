package claude

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"anti2api-golang/internal/config"
	"anti2api-golang/internal/store"
	"anti2api-golang/internal/utils"
)

// Claude thinking 相关常量
const (
	ThinkingStartTag = "<thinking>"
	ThinkingEndTag   = "</thinking>"
)

// ConvertClaudeToAntigravity 将 Claude 请求直接转换为 Antigravity 格式（跳过 OpenAI 中间层）
func ConvertClaudeToAntigravity(req *ClaudeMessagesRequest, account *store.Account) (*AntigravityRequest, error) {
	if req == nil {
		return nil, fmt.Errorf("请求体格式不合法")
	}
	if req.MaxTokens <= 0 {
		return nil, fmt.Errorf("max_tokens 是必填数字")
	}
	if len(req.Messages) == 0 {
		return nil, fmt.Errorf("messages 不能为空")
	}

	modelName := ResolveModelName(req.Model)

	antigravityReq := &AntigravityRequest{
		Project:   getClaudeProjectID(account),
		RequestID: utils.GenerateRequestID(),
		Model:     modelName,
		UserAgent: config.Get().UserAgent,
	}

	// 构建内部请求
	innerReq := AntigravityInnerReq{
		SessionID: account.SessionID,
	}

	// 处理 system 字段
	if req.System != nil {
		systemText := extractClaudeSystem(req.System)
		if systemText != "" {
			innerReq.SystemInstruction = &SystemInstruction{
				Parts: []Part{{Text: systemText}},
			}
		}
	}

	// 检查是否启用 thinking
	thinkingEnabled := req.Thinking != nil && req.Thinking.Type == "enabled"

	// 转换消息为 Antigravity contents 格式
	contents := convertClaudeMessagesToContents(req.Messages, thinkingEnabled)
	innerReq.Contents = contents

	// 转换工具
	if len(req.Tools) > 0 {
		innerReq.Tools = ConvertClaudeToolsToAntigravity(req.Tools)
		innerReq.ToolConfig = &ToolConfig{
			FunctionCallingConfig: &FunctionCallingConfig{
				Mode: "AUTO",
			},
		}
	}

	// 构建生成配置
	innerReq.GenerationConfig = buildClaudeGenerationConfig(req, modelName)

	antigravityReq.Request = innerReq
	return antigravityReq, nil
}

// getClaudeProjectID 获取项目ID
func getClaudeProjectID(account *store.Account) string {
	if account.ProjectID != "" {
		return account.ProjectID
	}
	return utils.GenerateProjectID()
}

// convertClaudeMessagesToContents 将 Claude 消息转换为 Antigravity contents
func convertClaudeMessagesToContents(messages []ClaudeMessage, thinkingEnabled bool) []Content {
	var contents []Content

	for _, msg := range messages {
		role := mapClaudeRoleToAntigravity(msg.Role)

		// 将消息内容转换为 parts
		parts := convertClaudeContentToParts(msg.Content, thinkingEnabled && msg.Role == "user")

		if len(parts) > 0 {
			contents = append(contents, Content{
				Role:  role,
				Parts: parts,
			})
		}
	}

	return contents
}

// mapClaudeRoleToAntigravity 将 Claude 角色映射为 Antigravity 角色
func mapClaudeRoleToAntigravity(role string) string {
	if role == "assistant" {
		return "model"
	}
	return "user"
}

// convertClaudeContentToParts 将 Claude 内容转换为 Antigravity parts
func convertClaudeContentToParts(content interface{}, appendThinkingHint bool) []Part {
	var textContent string

	switch v := content.(type) {
	case string:
		// 移除 invoke 和 tool_result XML 标签（防止重复）
		textContent = invokeRegex.ReplaceAllString(v, "")
		textContent = toolResultRegex.ReplaceAllString(textContent, "")
	case []interface{}:
		// 复杂内容块数组
		var textParts []string
		for _, item := range v {
			if block, ok := item.(map[string]interface{}); ok {
				part := normalizeClaudeBlock(block)
				if part != "" {
					textParts = append(textParts, part)
				}
			}
		}
		textContent = strings.Join(textParts, "\n")
	}

	if textContent == "" {
		return nil
	}

	return []Part{{Text: textContent}}
}

// ConvertClaudeToolsToAntigravity 将 Claude 工具定义转换为 Antigravity 格式
func ConvertClaudeToolsToAntigravity(tools []ClaudeTool) []Tool {
	if len(tools) == 0 {
		return nil
	}

	var result []Tool
	for _, tool := range tools {
		params := tool.InputSchema
		// 移除 $schema 字段
		delete(params, "$schema")

		result = append(result, Tool{
			FunctionDeclarations: []FunctionDeclaration{{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  params,
			}},
		})
	}
	return result
}

// buildClaudeGenerationConfig 构建 Claude 请求的生成配置
func buildClaudeGenerationConfig(req *ClaudeMessagesRequest, modelName string) *GenerationConfig {
	cfg := &GenerationConfig{
		CandidateCount:  1,
		MaxOutputTokens: req.MaxTokens,
		StopSequences:   DefaultStopSequences,
	}

	// 添加自定义停止序列
	if len(req.StopSequences) > 0 {
		cfg.StopSequences = append(cfg.StopSequences, req.StopSequences...)
	}

	// 设置温度和 top_p
	if req.Temperature != nil {
		cfg.Temperature = req.Temperature
	}
	if req.TopP != nil {
		cfg.TopP = req.TopP
	}

	// thinking 配置
	if ShouldEnableThinking(modelName, nil) {
		cfg.ThinkingConfig = BuildThinkingConfig(modelName)
	}

	return cfg
}

// ConvertAntigravityToClaudeResponse 将 Antigravity 响应转换为 Claude 响应
func ConvertAntigravityToClaudeResponse(resp *AntigravityResponse, requestID, model string, inputTokens int) *ClaudeMessagesResponse {
	if len(resp.Response.Candidates) == 0 {
		return &ClaudeMessagesResponse{
			ID:         "msg_" + requestID,
			Type:       "message",
			Role:       "assistant",
			Model:      model,
			Content:    []ClaudeContentBlock{},
			StopReason: "end_turn",
			Usage: ClaudeUsage{
				InputTokens:  inputTokens,
				OutputTokens: 0,
			},
		}
	}

	parts := resp.Response.Candidates[0].Content.Parts

	var thinking, content string
	var toolCalls []ToolCallInfo

	for _, part := range parts {
		if part.Thought {
			thinking += part.Text
		} else if part.Text != "" {
			content += part.Text
		} else if part.FunctionCall != nil {
			id := part.FunctionCall.ID
			if id == "" {
				id = utils.GenerateToolCallID()
			}

			toolCalls = append(toolCalls, ToolCallInfo{
				ID:               id,
				Name:             part.FunctionCall.Name,
				Args:             part.FunctionCall.Args,
				ThoughtSignature: part.ThoughtSignature,
			})
		}
	}

	// 构建内容块
	contentBlocks := BuildClaudeContentBlocksWithThinking(thinking, content, toolCalls)

	// 计算 output tokens
	outputTokens := 0
	if resp.Response.UsageMetadata != nil {
		outputTokens = resp.Response.UsageMetadata.CandidatesTokenCount
	}
	if outputTokens == 0 {
		outputTokens = EstimateClaudeTokens(thinking + content)
	}

	return &ClaudeMessagesResponse{
		ID:           "msg_" + requestID,
		Type:         "message",
		Role:         "assistant",
		Model:        model,
		Content:      contentBlocks,
		StopReason:   GetClaudeStopReason(len(toolCalls) > 0),
		StopSequence: nil,
		Usage: ClaudeUsage{
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
		},
	}
}

// 正则表达式
var (
	invokeRegex     = regexp.MustCompile(`(?i)<invoke\b[^>]*>[\s\S]*?</invoke>`)
	toolResultRegex = regexp.MustCompile(`(?i)<tool_result\b[^>]*>[\s\S]*?</tool_result>`)
)

// extractClaudeSystem 提取 Claude system 内容
func extractClaudeSystem(system interface{}) string {
	switch v := system.(type) {
	case string:
		return v
	case []interface{}:
		var texts []string
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				if text, ok := m["text"].(string); ok {
					texts = append(texts, text)
				}
			}
		}
		return strings.Join(texts, "\n")
	}
	return ""
}

// normalizeClaudeBlock 规范化单个 Claude 内容块
func normalizeClaudeBlock(block map[string]interface{}) string {
	blockType, _ := block["type"].(string)

	switch blockType {
	case "text":
		text, _ := block["text"].(string)
		// 移除 invoke 和 tool_result XML 标签
		result := invokeRegex.ReplaceAllString(text, "")
		result = toolResultRegex.ReplaceAllString(result, "")
		return result

	case "thinking":
		thinking, _ := block["thinking"].(string)
		return ThinkingStartTag + thinking + ThinkingEndTag

	case "tool_result":
		toolUseID, _ := block["tool_use_id"].(string)
		content := extractToolResultContent(block["content"])
		return fmt.Sprintf(`<tool_result id="%s">%s</tool_result>`, toolUseID, content)

	case "tool_use":
		name, _ := block["name"].(string)
		input := block["input"]
		params := formatToolUseParams(input)
		return fmt.Sprintf("<invoke name=\"%s\">\n%s\n</invoke>", name, params)

	case "image":
		// 图片内容 - 暂时忽略，因为需要特殊处理
		return ""
	}

	return ""
}

// extractToolResultContent 提取工具结果内容
func extractToolResultContent(content interface{}) string {
	switch v := content.(type) {
	case string:
		return v
	case []interface{}:
		var texts []string
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				if m["type"] == "text" {
					if text, ok := m["text"].(string); ok {
						texts = append(texts, text)
					}
				}
			}
		}
		return strings.Join(texts, "\n")
	}
	return ""
}

// formatToolUseParams 格式化工具调用参数为 XML
func formatToolUseParams(input interface{}) string {
	if input == nil {
		return ""
	}

	inputMap, ok := input.(map[string]interface{})
	if !ok {
		return ""
	}

	var params []string
	for key, value := range inputMap {
		var stringValue string
		switch v := value.(type) {
		case string:
			stringValue = v
		default:
			jsonBytes, _ := json.Marshal(v)
			stringValue = string(jsonBytes)
		}
		params = append(params, fmt.Sprintf(`<parameter name="%s">%s</parameter>`, key, stringValue))
	}
	return strings.Join(params, "\n")
}

// ConvertToolCallsToClaudeBlocks 将工具调用转换为 Claude 内容块
func ConvertToolCallsToClaudeBlocks(toolCalls []ToolCallInfo) []ClaudeContentBlock {
	if len(toolCalls) == 0 {
		return nil
	}

	blocks := make([]ClaudeContentBlock, len(toolCalls))
	for i, tc := range toolCalls {
		input := tc.Args
		if input == nil {
			input = map[string]interface{}{}
		}

		id := tc.ID
		if id == "" {
			id = "toolu_" + utils.GenerateRequestID()
		}

		blocks[i] = ClaudeContentBlock{
			Type:  "tool_use",
			ID:    id,
			Name:  tc.Name,
			Input: input,
		}
	}
	return blocks
}

// BuildClaudeContentBlocks 构建 Claude 响应内容块
func BuildClaudeContentBlocks(content string, toolCalls []ToolCallInfo) []ClaudeContentBlock {
	return BuildClaudeContentBlocksWithThinking("", content, toolCalls)
}

// BuildClaudeContentBlocksWithThinking 构建 Claude 响应内容块（包含 thinking）
func BuildClaudeContentBlocksWithThinking(thinking, content string, toolCalls []ToolCallInfo) []ClaudeContentBlock {
	var blocks []ClaudeContentBlock

	// thinking 块必须在 text 块之前
	if thinking != "" {
		blocks = append(blocks, ClaudeContentBlock{
			Type:     "thinking",
			Thinking: thinking,
		})
	}

	if content != "" {
		blocks = append(blocks, ClaudeContentBlock{
			Type: "text",
			Text: content,
		})
	}

	if len(toolCalls) > 0 {
		blocks = append(blocks, ConvertToolCallsToClaudeBlocks(toolCalls)...)
	}

	return blocks
}

// EstimateClaudeTokens 估算 token 数量
func EstimateClaudeTokens(text string) int {
	if text == "" {
		return 0
	}
	// 简单估算：每 4 个字符约 1 个 token
	count := len(text) / 4
	if count < 1 {
		count = 1
	}
	return count
}

// CountClaudeTokens 计算 Claude 请求的 token 数量
func CountClaudeTokens(req *ClaudeMessagesRequest) (*ClaudeTokenCountResponse, error) {
	if req == nil || len(req.Messages) == 0 {
		return nil, fmt.Errorf("messages 不能为空")
	}

	var totalText string

	// 提取消息文本
	for _, msg := range req.Messages {
		totalText += extractClaudeMessageText(msg.Content) + "\n"
	}

	// 提取系统文本
	if req.System != nil {
		totalText += extractClaudeSystem(req.System) + "\n"
	}

	// 提取工具定义
	if len(req.Tools) > 0 {
		toolsJSON, _ := json.Marshal(req.Tools)
		totalText += string(toolsJSON)
	}

	inputTokens := EstimateClaudeTokens(totalText)

	return &ClaudeTokenCountResponse{
		InputTokens: inputTokens,
		TokenCount:  inputTokens,
		Tokens:      inputTokens,
	}, nil
}

// extractClaudeMessageText 从 Claude 消息中提取文本
func extractClaudeMessageText(content interface{}) string {
	switch v := content.(type) {
	case string:
		return v
	case []interface{}:
		var texts []string
		for _, item := range v {
			if block, ok := item.(map[string]interface{}); ok {
				blockType, _ := block["type"].(string)
				switch blockType {
				case "text":
					if text, ok := block["text"].(string); ok {
						texts = append(texts, text)
					}
				case "thinking":
					if thinking, ok := block["thinking"].(string); ok {
						texts = append(texts, thinking)
					}
				case "tool_use":
					name, _ := block["name"].(string)
					inputJSON, _ := json.Marshal(block["input"])
					texts = append(texts, fmt.Sprintf(`<invoke name="%s">%s</invoke>`, name, string(inputJSON)))
				case "tool_result":
					toolUseID, _ := block["tool_use_id"].(string)
					content := extractToolResultContent(block["content"])
					texts = append(texts, fmt.Sprintf(`<tool_result id="%s">%s</tool_result>`, toolUseID, content))
				}
			}
		}
		return strings.Join(texts, "")
	}
	return ""
}

// GetClaudeStopReason 根据工具调用情况返回 stop_reason
func GetClaudeStopReason(hasToolCalls bool) string {
	if hasToolCalls {
		return "tool_use"
	}
	return "end_turn"
}

// ConvertUsage 将 UsageMetadata 转换为 Claude 格式的 Usage
func ConvertUsage(metadata *UsageMetadata) *Usage {
	if metadata == nil {
		return nil
	}
	return &Usage{
		PromptTokens:     metadata.PromptTokenCount,
		CompletionTokens: metadata.CandidatesTokenCount,
		TotalTokens:      metadata.TotalTokenCount,
	}
}
