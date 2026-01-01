package claude

import (
	"fmt"
	"strings"

	"github.com/bytedance/sonic"

	"anti2api-golang/internal/config"
	"anti2api-golang/internal/store"
	"anti2api-golang/internal/utils"
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

	// 检查是否为 Prefill 请求（最后一条消息是 assistant）
	isPrefill := false
	if len(req.Messages) > 0 {
		lastMsg := req.Messages[len(req.Messages)-1]
		if lastMsg.Role == "assistant" {
			isPrefill = true
		}
	}

	// 检测是否启用 thinking 模式
	// 注意：如果是 Prefill 请求，强制禁用 thinking，因为 prefill 的文本（如 "{"）会导致
	// "Expected thinking but found text" 错误，且无法在 prefill 文本前插入有效的 thinking 块
	thinkingEnabled := !isPrefill && (ShouldEnableThinking(modelName, nil) ||
		(req.Thinking != nil && req.Thinking.Type == "enabled"))

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
	// 如果强制禁用了 thinking（由于 prefill），需要同步更新 generationConfig
	if isPrefill && innerReq.GenerationConfig.ThinkingConfig != nil {
		innerReq.GenerationConfig.ThinkingConfig.IncludeThoughts = false
		innerReq.GenerationConfig.ThinkingConfig.ThinkingBudget = 0
		// 恢复 ThinkingLevel 默认值或清空，避免 Vertex AI 报错
		innerReq.GenerationConfig.ThinkingConfig.ThinkingLevel = ""
	}

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
// thinkingEnabled 参数指示是否启用了 thinking 模式
func convertClaudeMessagesToContents(messages []ClaudeMessage, thinkingEnabled bool) []Content {
	var contents []Content
	toolIDToName := make(map[string]string)

	// 首先扫描所有消息，建立 tool_use_id 到 tool_name 的映射
	// 因为 Claude 的 tool_result 块只有 tool_use_id，而 Vertex API 要求 functionResponse 必须有 name
	for _, msg := range messages {
		if msg.Role == "assistant" {
			switch v := msg.Content.(type) {
			case []interface{}:
				for _, item := range v {
					if block, ok := item.(map[string]interface{}); ok {
						if block["type"] == "tool_use" {
							id, _ := block["id"].(string)
							name, _ := block["name"].(string)
							if id != "" && name != "" {
								toolIDToName[id] = name
							}
						}
					}
				}
			}
		}
	}

	for _, msg := range messages {
		role := mapClaudeRoleToAntigravity(msg.Role)

		// 将消息内容转换为 parts
		parts := convertClaudeContentToParts(msg.Content, toolIDToName)

		// 如果启用了 thinking 模式，确保 assistant 消息以 thinking 块开头
		if thinkingEnabled && msg.Role == "assistant" && len(parts) > 0 {
			parts = ensureAssistantHasThinking(parts)
		}

		if len(parts) > 0 {
			contents = append(contents, Content{
				Role:  role,
				Parts: parts,
			})
		}
	}

	return contents
}

// ensureAssistantHasThinking 确保 assistant 消息以 thinking 块开头
// 如果第一个 part 不是 thinking 类型，则添加一个占位 thinking 块
func ensureAssistantHasThinking(parts []Part) []Part {
	if len(parts) == 0 {
		return parts
	}

	// 检查第一个 part 是否是 thinking
	if parts[0].Thought {
		return parts // 已经有 thinking 块，无需修改
	}

	// 在开头插入一个占位的 thinking 块
	// 使用空字符串内容，表示 thinking 内容已被处理/编辑
	thinkingPart := Part{
		Text:    "[Thinking content from previous turn]",
		Thought: true,
	}

	return append([]Part{thinkingPart}, parts...)
}

// mapClaudeRoleToAntigravity 将 Claude 角色映射为 Antigravity 角色
func mapClaudeRoleToAntigravity(role string) string {
	if role == "assistant" {
		return "model"
	}
	return "user"
}

// convertClaudeContentToParts 将 Claude 内容转换为 Antigravity parts
// 签名处理：从 thinking 块提取签名，根据内容类型决定放置位置（functionCall > text > thinking）
func convertClaudeContentToParts(content interface{}, toolIDToName map[string]string) []Part {
	var parts []Part
	var thinkingSignature string // 从 thinking 块提取的签名

	switch v := content.(type) {
	case string:
		if v != "" {
			parts = append(parts, Part{Text: v})
		}
	case []interface{}:
		// 第一阶段：解析所有内容块，提取签名
		for _, item := range v {
			if block, ok := item.(map[string]interface{}); ok {
				blockType, _ := block["type"].(string)

				switch blockType {
				case "text":
					text, _ := block["text"].(string)
					if text != "" {
						parts = append(parts, Part{Text: text})
					}

				case "thinking":
					thinking, _ := block["thinking"].(string)
					signature, _ := block["signature"].(string)
					// 只取第一个非空签名
					if signature != "" && thinkingSignature == "" {
						thinkingSignature = signature
					}
					if thinking != "" {
						parts = append(parts, Part{
							Text:    thinking,
							Thought: true,
							// 不在这里放签名，稍后统一决定位置
						})
					}

				case "tool_use":
					name, _ := block["name"].(string)
					id, _ := block["id"].(string)
					input := block["input"]

					var args map[string]interface{}
					if m, ok := input.(map[string]interface{}); ok {
						args = m
					}

					parts = append(parts, Part{
						FunctionCall: &FunctionCall{
							ID:   id,
							Name: name,
							Args: args,
						},
					})

				case "tool_result":
					toolUseID, _ := block["tool_use_id"].(string)
					isError, _ := block["is_error"].(bool)
					rawContent := block["content"]

					// 提取工具结果内容并尝试解析为 JSON
					contentStr := extractToolResultContent(rawContent)
					var response map[string]interface{}

					// 使用 Sonic 解析 JSON
					if err := sonic.UnmarshalString(contentStr, &response); err != nil {
						// 如果不是完整的 JSON，则包装在 "result" 或 "error" 字段中
						response = make(map[string]interface{})
						if isError {
							response["error"] = contentStr
						} else {
							response["result"] = contentStr
						}
					}

					// 从映射中寻找对应的工具名称
					toolName := toolIDToName[toolUseID]

					parts = append(parts, Part{
						FunctionResponse: &FunctionResponse{
							ID:       toolUseID,
							Name:     toolName,
							Response: response,
						},
					})
				}
			}
		}

		// 第二阶段：根据内容类型决定签名放置位置（只放一处）
		if thinkingSignature != "" {
			applySignatureToParts(parts, thinkingSignature)
		}
	}

	return parts
}

// applySignatureToParts 根据内容类型决定签名放置位置
// 优先级：functionCall > text > thinking
// 确保单轮对话中只有一个 Part 携带 thoughtSignature
func applySignatureToParts(parts []Part, signature string) {
	// 优先级 1: 有 functionCall → 放在第一个 functionCall
	for i := range parts {
		if parts[i].FunctionCall != nil {
			parts[i].ThoughtSignature = signature
			return
		}
	}
	// 优先级 2: 纯文本 → 放在最后一个非 thinking 的 text
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i].Text != "" && !parts[i].Thought {
			parts[i].ThoughtSignature = signature
			return
		}
	}
	// 优先级 3: 只有 thinking → 放在最后一个 thinking
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i].Thought {
			parts[i].ThoughtSignature = signature
			return
		}
	}
}

// ConvertClaudeToolsToAntigravity 将 Claude 工具定义转换为 Antigravity 格式
func ConvertClaudeToolsToAntigravity(tools []ClaudeTool) []Tool {
	if len(tools) == 0 {
		return nil
	}

	var result []Tool
	for _, tool := range tools {
		// 深拷贝 schema 以避免修改原始数据
		params := deepCopyMap(tool.InputSchema)
		// 递归清理 Vertex AI 不支持的 JSON Schema 字段
		cleanSchemaForVertexAI(params)

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

// cleanSchemaForVertexAI 递归清理 Vertex AI 不支持的 JSON Schema 字段
// 同时将 exclusiveMinimum/exclusiveMaximum 转换为 minimum/maximum
func cleanSchemaForVertexAI(schema map[string]interface{}) {
	if schema == nil {
		return
	}

	// 将 exclusiveMinimum 转换为 minimum（+1）
	if exMin, ok := schema["exclusiveMinimum"].(float64); ok {
		if _, hasMin := schema["minimum"]; !hasMin {
			schema["minimum"] = exMin + 1
		}
		delete(schema, "exclusiveMinimum")
	}

	// 将 exclusiveMaximum 转换为 maximum（-1）
	if exMax, ok := schema["exclusiveMaximum"].(float64); ok {
		if _, hasMax := schema["maximum"]; !hasMax {
			schema["maximum"] = exMax - 1
		}
		delete(schema, "exclusiveMaximum")
	}

	// 移除 Vertex AI 不支持的字段
	unsupportedFields := []string{
		"$schema",
		"$ref",
		"$id",
		"$defs",
		"definitions",
		"minItems",
		"maxItems",
		"uniqueItems",
		"pattern",
		"additionalProperties",
		"patternProperties",
		"dependencies",
		"if",
		"then",
		"else",
		"allOf",
		"anyOf",
		"oneOf",
		"not",
		"contentMediaType",
		"contentEncoding",
		"examples",
		"default",
		"const",
		"minLength",
		"maxLength",
		"format",
	}
	for _, field := range unsupportedFields {
		delete(schema, field)
	}

	// 递归处理 properties
	if props, ok := schema["properties"].(map[string]interface{}); ok {
		for _, propValue := range props {
			if propSchema, ok := propValue.(map[string]interface{}); ok {
				cleanSchemaForVertexAI(propSchema)
			}
		}
	}

	// 递归处理 items（数组类型）
	if items, ok := schema["items"].(map[string]interface{}); ok {
		cleanSchemaForVertexAI(items)
	}

	// 递归处理 items 数组形式
	if itemsArr, ok := schema["items"].([]interface{}); ok {
		for _, item := range itemsArr {
			if itemSchema, ok := item.(map[string]interface{}); ok {
				cleanSchemaForVertexAI(itemSchema)
			}
		}
	}
}

// deepCopyMap 深拷贝 map 以避免修改原始数据
func deepCopyMap(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		switch val := v.(type) {
		case map[string]interface{}:
			result[k] = deepCopyMap(val)
		case []interface{}:
			result[k] = deepCopySlice(val)
		default:
			result[k] = v
		}
	}
	return result
}

// deepCopySlice 深拷贝 slice
func deepCopySlice(s []interface{}) []interface{} {
	if s == nil {
		return nil
	}
	result := make([]interface{}, len(s))
	for i, v := range s {
		switch val := v.(type) {
		case map[string]interface{}:
			result[i] = deepCopyMap(val)
		case []interface{}:
			result[i] = deepCopySlice(val)
		default:
			result[i] = v
		}
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

		// 如果请求中显式提供了 thinking 配置，尝试合并
		if req.Thinking != nil && req.Thinking.Type == "enabled" {
			if req.Thinking.Budget > 0 {
				cfg.ThinkingConfig.ThinkingBudget = req.Thinking.Budget
			}
			if req.Thinking.Level != "" {
				cfg.ThinkingConfig.ThinkingLevel = req.Thinking.Level
			}
		}

		// 针对 Gemini 3 模型的特殊处理：强制使用 thinking_level = high
		if strings.HasPrefix(modelName, "gemini-3-pro-") {
			cfg.ThinkingConfig.ThinkingLevel = "high"
			cfg.ThinkingConfig.ThinkingBudget = 0 // 使用 level 时清空 budget
		}

		// 确保 MaxOutputTokens > ThinkingBudget（Vertex AI 要求）
		if cfg.ThinkingConfig.ThinkingBudget > 0 {
			minDiff := 1024 // 最小差值，确保有足够的输出空间
			if cfg.MaxOutputTokens <= cfg.ThinkingConfig.ThinkingBudget+minDiff {
				// 减少 ThinkingBudget 以满足约束
				cfg.ThinkingConfig.ThinkingBudget = cfg.MaxOutputTokens - minDiff
				if cfg.ThinkingConfig.ThinkingBudget < 1024 {
					cfg.ThinkingConfig.ThinkingBudget = 1024
					cfg.MaxOutputTokens = 2048 // 确保最小可用配置
				}
			}
		}
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
	var thinkingSignature string
	var toolCalls []ToolCallInfo

	for _, part := range parts {
		// 捕获任意 part 的 thought signature
		if part.ThoughtSignature != "" {
			thinkingSignature = part.ThoughtSignature
		}

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

	// 构建内容块（包含 signature）
	contentBlocks := BuildClaudeContentBlocksWithThinking(thinking, content, toolCalls, thinkingSignature)

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

// BuildClaudeContentBlocksWithThinking 构建 Claude 响应内容块（包含 thinking）
func BuildClaudeContentBlocksWithThinking(thinking, content string, toolCalls []ToolCallInfo, thinkingSignature string) []ClaudeContentBlock {
	var blocks []ClaudeContentBlock

	// thinking 块必须在 text 块之前
	if thinking != "" {
		blocks = append(blocks, ClaudeContentBlock{
			Type:      "thinking",
			Thinking:  thinking,
			Signature: thinkingSignature,
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
	return BuildClaudeContentBlocksWithThinking("", content, toolCalls, "")
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
		toolsJSON, _ := sonic.Marshal(req.Tools)
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
					texts = append(texts, name)
				case "tool_result":
					content := extractToolResultContent(block["content"])
					texts = append(texts, content)
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
