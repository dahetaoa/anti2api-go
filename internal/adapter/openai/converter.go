package openai

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"anti2api-golang/internal/config"
	"anti2api-golang/internal/store"
	"anti2api-golang/internal/utils"
)

// ConvertOpenAIToAntigravity 将 OpenAI 请求转换为 Antigravity 格式
func ConvertOpenAIToAntigravity(req *OpenAIChatRequest, account *store.Account) *AntigravityRequest {
	modelName := ResolveModelName(req.Model)

	antigravityReq := &AntigravityRequest{
		Project:   getProjectID(account),
		RequestID: utils.GenerateRequestID(),
		Model:     modelName,
		UserAgent: config.Get().UserAgent,
	}

	// 转换消息
	contents := convertMessages(req.Messages)

	// 构建内部请求
	innerReq := AntigravityInnerReq{
		Contents:  contents,
		SessionID: account.SessionID,
	}

	// 提取系统消息
	systemText := extractSystemInstruction(req.Messages)
	if systemText != "" {
		innerReq.SystemInstruction = &SystemInstruction{
			Parts: []Part{{Text: systemText}},
		}
	}

	// 转换工具
	if len(req.Tools) > 0 {
		innerReq.Tools = ConvertOpenAIToolsToAntigravity(req.Tools)
		innerReq.ToolConfig = &ToolConfig{
			FunctionCallingConfig: &FunctionCallingConfig{
				Mode: "AUTO",
			},
		}
	}

	// 构建生成配置
	innerReq.GenerationConfig = buildGenerationConfig(req, modelName)

	antigravityReq.Request = innerReq
	return antigravityReq
}

func getProjectID(account *store.Account) string {
	if account.ProjectID != "" {
		return account.ProjectID
	}
	return utils.GenerateProjectID()
}

func convertMessages(messages []OpenAIMessage) []Content {
	var result []Content

	for _, msg := range messages {
		switch msg.Role {
		case "system":
			// 跳过，单独处理到 systemInstruction
			continue

		case "user":
			parts := extractParts(msg.Content)
			result = append(result, Content{Role: "user", Parts: parts})

		case "assistant":
			parts := []Part{}

			// 首先尝试添加 thinking 内容（必须在最前面，Claude API 要求）
			if msg.Reasoning != "" {
				parts = append(parts, Part{
					Text:    msg.Reasoning,
					Thought: true,
				})
			}

			// 然后添加正文内容
			if text := getTextContent(msg.Content); text != "" {
				parts = append(parts, Part{Text: text})
			}
			// 转换工具调用
			for _, tc := range msg.ToolCalls {
				args := ParseArgs(tc.Function.Arguments)
				var signature string
				if tc.ExtraContent != nil && tc.ExtraContent.Google != nil {
					signature = tc.ExtraContent.Google.ThoughtSignature
				}

				parts = append(parts, Part{
					FunctionCall: &FunctionCall{
						ID:   tc.ID,
						Name: tc.Function.Name,
						Args: args,
					},
					ThoughtSignature: signature,
				})
			}
			if len(parts) > 0 {
				result = append(result, Content{Role: "model", Parts: parts})
			}

		case "tool":
			// 查找对应的 function name
			funcName := findFunctionName(result, msg.ToolCallID)
			part := Part{
				FunctionResponse: &FunctionResponse{
					ID:   msg.ToolCallID,
					Name: funcName,
					Response: map[string]interface{}{
						"output": getTextContent(msg.Content),
					},
				},
			}
			// 合并到上一个 user 消息或新建
			appendFunctionResponse(&result, part)
		}
	}

	return result
}

func extractSystemInstruction(messages []OpenAIMessage) string {
	var texts []string
	for _, msg := range messages {
		if msg.Role == "system" {
			texts = append(texts, getTextContent(msg.Content))
		}
	}
	return strings.Join(texts, "\n\n")
}

func extractParts(content interface{}) []Part {
	var parts []Part

	switch v := content.(type) {
	case string:
		parts = append(parts, Part{Text: v})
	case []interface{}:
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				switch m["type"] {
				case "text":
					if text, ok := m["text"].(string); ok {
						parts = append(parts, Part{Text: text})
					}
				case "image_url":
					if imgURL, ok := m["image_url"].(map[string]interface{}); ok {
						if url, ok := imgURL["url"].(string); ok {
							if inlineData := parseImageURL(url); inlineData != nil {
								parts = append(parts, Part{InlineData: inlineData})
							}
						}
					}
				}
			}
		}
	}

	return parts
}

func parseImageURL(url string) *InlineData {
	// 解析 data:image/{format};base64,{data}
	re := regexp.MustCompile(`^data:image/(\w+);base64,(.+)$`)
	if matches := re.FindStringSubmatch(url); len(matches) == 3 {
		return &InlineData{
			MimeType: "image/" + matches[1],
			Data:     matches[2],
		}
	}
	return nil
}

func getTextContent(content interface{}) string {
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

// ParseArgs 解析 JSON 字符串参数为 map
func ParseArgs(argsStr string) map[string]interface{} {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
		return map[string]interface{}{}
	}
	return args
}

func findFunctionName(contents []Content, toolCallID string) string {
	for i := len(contents) - 1; i >= 0; i-- {
		for _, part := range contents[i].Parts {
			if part.FunctionCall != nil && part.FunctionCall.ID == toolCallID {
				return part.FunctionCall.Name
			}
		}
	}
	return ""
}

func appendFunctionResponse(contents *[]Content, part Part) {
	// functionResponse 应该在 functionCall 之后的新 user turn 中
	// 检查最后一个消息是否是 model 消息（包含 functionCall）
	if len(*contents) > 0 && (*contents)[len(*contents)-1].Role == "model" {
		// 在 model 消息后添加新的 user 消息来包含 functionResponse
		*contents = append(*contents, Content{
			Role:  "user",
			Parts: []Part{part},
		})
		return
	}
	// 如果最后已经是 user 消息，合并 functionResponse（多个 tool 响应的情况）
	if len(*contents) > 0 && (*contents)[len(*contents)-1].Role == "user" {
		(*contents)[len(*contents)-1].Parts = append((*contents)[len(*contents)-1].Parts, part)
		return
	}
	// 新建 user 消息
	*contents = append(*contents, Content{
		Role:  "user",
		Parts: []Part{part},
	})
}

// ConvertOpenAIToolsToAntigravity 将 OpenAI 工具转换为 Antigravity 格式
func ConvertOpenAIToolsToAntigravity(tools []OpenAITool) []Tool {
	var result []Tool

	for _, tool := range tools {
		params := tool.Function.Parameters
		// 移除 $schema 字段
		delete(params, "$schema")

		result = append(result, Tool{
			FunctionDeclarations: []FunctionDeclaration{{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  params,
			}},
		})
	}

	return result
}

func buildGenerationConfig(req *OpenAIChatRequest, modelName string) *GenerationConfig {
	config := &GenerationConfig{
		CandidateCount: 1,
		StopSequences:  DefaultStopSequences,
	}

	// 添加自定义停止序列
	if len(req.Stop) > 0 {
		config.StopSequences = append(config.StopSequences, req.Stop...)
	}

	// Claude 模型特殊处理
	if IsClaudeModel(modelName) {
		config.MaxOutputTokens = GetClaudeMaxOutputTokens(modelName)
		// Claude thinking 模式不支持工具调用，当有工具时禁用 thinking
		if len(req.Tools) == 0 && ShouldEnableThinking(modelName, nil) {
			config.ThinkingConfig = BuildThinkingConfig(modelName)
		}
		return config
	}

	// 其他模型
	if req.Temperature != nil {
		config.Temperature = req.Temperature
	}
	if req.TopP != nil {
		config.TopP = req.TopP
	}
	if req.MaxTokens > 0 {
		config.MaxOutputTokens = req.MaxTokens
	}

	// 思考模式
	if ShouldEnableThinking(modelName, nil) {
		config.ThinkingConfig = BuildThinkingConfig(modelName)
	}

	return config
}

// ConvertToOpenAIResponse 将 Antigravity 响应转换为 OpenAI 格式
func ConvertToOpenAIResponse(antigravityResp *AntigravityResponse, model string) *OpenAIChatCompletion {
	parts := antigravityResp.Response.Candidates[0].Content.Parts

	var content, thinkingContent string
	var toolCalls []OpenAIToolCall
	var imageURLs []string

	for _, part := range parts {
		if part.Thought {
			thinkingContent += part.Text
		} else if part.Text != "" {
			content += part.Text
		} else if part.FunctionCall != nil {
			argsJSON, _ := json.Marshal(part.FunctionCall.Args)
			id := part.FunctionCall.ID
			if id == "" {
				id = utils.GenerateToolCallID()
			}

			var extraContent *ExtraContent
			if part.ThoughtSignature != "" {
				extraContent = &ExtraContent{
					Google: &GoogleExtra{
						ThoughtSignature: part.ThoughtSignature,
					},
				}
			}

			toolCalls = append(toolCalls, OpenAIToolCall{
				ID:   id,
				Type: "function",
				Function: OpenAIFunctionCall{
					Name:      part.FunctionCall.Name,
					Arguments: string(argsJSON),
				},
				ExtraContent: extraContent,
			})
		} else if part.InlineData != nil {
			dataURL := fmt.Sprintf("data:%s;base64,%s", part.InlineData.MimeType, part.InlineData.Data)
			imageURLs = append(imageURLs, dataURL)
		}
	}

	// 处理图片输出
	if len(imageURLs) > 0 {
		var md strings.Builder
		if content != "" {
			md.WriteString(content + "\n\n")
		}
		for _, url := range imageURLs {
			md.WriteString(fmt.Sprintf("![image](%s)\n\n", url))
		}
		content = md.String()
	}

	finishReason := "stop"
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
	}

	return &OpenAIChatCompletion{
		ID:      utils.GenerateChatCompletionID(),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []Choice{{
			Index: 0,
			Message: Message{
				Role:      "assistant",
				Content:   content,
				ToolCalls: toolCalls,
				Reasoning: thinkingContent,
			},
			FinishReason: &finishReason,
		}},
		Usage: ConvertUsage(antigravityResp.Response.UsageMetadata),
	}
}

// ConvertUsage 转换使用统计
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

// CreateStreamChunk 创建流式 Chunk
func CreateStreamChunk(id string, created int64, model string, delta *Delta, finishReason *string, usage *Usage) *OpenAIStreamChunk {
	return &OpenAIStreamChunk{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []Choice{{
			Index:        0,
			Delta:        delta,
			FinishReason: finishReason,
		}},
		Usage: usage,
	}
}
