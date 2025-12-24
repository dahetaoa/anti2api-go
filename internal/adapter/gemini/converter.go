package gemini

import (
	"encoding/json"
	"strings"

	"anti2api-golang/internal/config"
	"anti2api-golang/internal/store"
	"anti2api-golang/internal/utils"
)

// ConvertGeminiToAntigravity 标准 Gemini → Antigravity 内部格式
func ConvertGeminiToAntigravity(model string, geminiReq *GeminiRequest, account *store.Account) *AntigravityRequest {
	modelName := ResolveModelName(model)

	return &AntigravityRequest{
		Project:   getProjectID(account),
		RequestID: utils.GenerateRequestID(),
		Request: AntigravityInnerReq{
			Contents:          geminiReq.Contents,
			SystemInstruction: geminiReq.SystemInstruction,
			GenerationConfig:  buildGeminiGenerationConfig(geminiReq.GenerationConfig, modelName),
			Tools:             geminiReq.Tools,
			ToolConfig:        geminiReq.ToolConfig,
			SessionID:         account.SessionID,
		},
		Model:     modelName,
		UserAgent: config.Get().UserAgent,
	}
}

func buildGeminiGenerationConfig(reqConfig *GenerationConfig, modelName string) *GenerationConfig {
	config := &GenerationConfig{
		CandidateCount: 1,
		StopSequences:  DefaultStopSequences,
	}

	if reqConfig != nil {
		if reqConfig.MaxOutputTokens > 0 {
			config.MaxOutputTokens = reqConfig.MaxOutputTokens
		}
		if reqConfig.Temperature != nil {
			config.Temperature = reqConfig.Temperature
		}
		if reqConfig.TopP != nil {
			config.TopP = reqConfig.TopP
		}
		if reqConfig.TopK > 0 {
			config.TopK = reqConfig.TopK
		}
		if len(reqConfig.StopSequences) > 0 {
			config.StopSequences = append(config.StopSequences, reqConfig.StopSequences...)
		}
		if reqConfig.ThinkingConfig != nil {
			config.ThinkingConfig = reqConfig.ThinkingConfig
		}
	}

	// 如果没有显式配置 ThinkingConfig，根据模型名判断
	if config.ThinkingConfig == nil && ShouldEnableThinking(modelName, nil) {
		config.ThinkingConfig = BuildThinkingConfig(modelName)
	}

	return config
}

// ExtractGeminiResponse Antigravity 响应 → 标准 Gemini 响应
func ExtractGeminiResponse(antigravityResp *AntigravityResponse) *GeminiResponse {
	resp := &GeminiResponse{
		Candidates:    antigravityResp.Response.Candidates,
		UsageMetadata: antigravityResp.Response.UsageMetadata,
	}

	// 清理非标准字段
	for i := range resp.Candidates {
		for _ = range resp.Candidates[i].Content.Parts {
			// 如果有 thoughtSignature 等非标准字段，这里清理
			// 在 Go 中由于使用 struct，非定义字段自动忽略
		}
		// 确保有 index 字段
		if resp.Candidates[i].Index == 0 && i > 0 {
			resp.Candidates[i].Index = i
		}
	}

	return resp
}

// TransformGeminiStreamLine 流式行转换
func TransformGeminiStreamLine(line string) string {
	if !strings.HasPrefix(line, "data: ") {
		return line
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(line[6:]), &data); err != nil {
		return line
	}

	// 提取 response 字段
	if resp, ok := data["response"].(map[string]interface{}); ok {
		// 清理 candidates
		sanitizeCandidates(resp)
		transformed, err := json.Marshal(resp)
		if err != nil {
			return line
		}
		return "data: " + string(transformed)
	}

	return line
}

func sanitizeCandidates(resp map[string]interface{}) {
	candidates, ok := resp["candidates"].([]interface{})
	if !ok {
		return
	}

	for i, c := range candidates {
		candidate, ok := c.(map[string]interface{})
		if !ok {
			continue
		}

		// 清理 parts 中的非标准字段
		if content, ok := candidate["content"].(map[string]interface{}); ok {
			if parts, ok := content["parts"].([]interface{}); ok {
				for _, p := range parts {
					if part, ok := p.(map[string]interface{}); ok {
						// 删除 thoughtSignature 等非标准字段
						delete(part, "thoughtSignature")
					}
				}
			}
		}

		// 确保有 index
		if _, ok := candidate["index"]; !ok {
			candidate["index"] = i
		}
	}
}

// GeminiModelsResponse Gemini 模型列表响应
type GeminiModelsResponse struct {
	Models []GeminiModel `json:"models"`
}

// GeminiModel Gemini 模型
type GeminiModel struct {
	Name                       string   `json:"name"`
	Version                    string   `json:"version,omitempty"`
	DisplayName                string   `json:"displayName"`
	Description                string   `json:"description,omitempty"`
	InputTokenLimit            int      `json:"inputTokenLimit,omitempty"`
	OutputTokenLimit           int      `json:"outputTokenLimit,omitempty"`
	SupportedGenerationMethods []string `json:"supportedGenerationMethods,omitempty"`
}

// GetGeminiModels 获取 Gemini 格式的模型列表
func GetGeminiModels() *GeminiModelsResponse {
	models := []GeminiModel{}

	for _, m := range SupportedModels {
		models = append(models, GeminiModel{
			Name:        "models/" + m.ID,
			DisplayName: m.ID,
			Description: "Model provided by " + m.OwnedBy,
			SupportedGenerationMethods: []string{
				"generateContent",
				"streamGenerateContent",
			},
		})
	}

	return &GeminiModelsResponse{Models: models}
}
