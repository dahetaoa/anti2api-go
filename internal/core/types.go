package core

// ==================== Antigravity 内部格式 ====================

// AntigravityRequest Antigravity 内部请求格式
type AntigravityRequest struct {
	Project     string              `json:"project"`
	RequestID   string              `json:"requestId"`
	Request     AntigravityInnerReq `json:"request"`
	Model       string              `json:"model"`
	UserAgent   string              `json:"userAgent"`
	RequestType string              `json:"requestType,omitempty"`
}

// AntigravityInnerReq 内部请求体
type AntigravityInnerReq struct {
	SystemInstruction *SystemInstruction `json:"systemInstruction,omitempty"`
	Contents          []Content          `json:"contents"`
	Tools             []Tool             `json:"tools,omitempty"`
	ToolConfig        *ToolConfig        `json:"toolConfig,omitempty"`
	GenerationConfig  *GenerationConfig  `json:"generationConfig,omitempty"`
	SessionID         string             `json:"sessionId"`
}

// Content 消息内容
type Content struct {
	Role  string `json:"role"` // "user" 或 "model"
	Parts []Part `json:"parts"`
}

// Part 消息部分
type Part struct {
	Text             string            `json:"text,omitempty"`
	FunctionCall     *FunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *FunctionResponse `json:"functionResponse,omitempty"`
	InlineData       *InlineData       `json:"inlineData,omitempty"`
	Thought          bool              `json:"thought,omitempty"`          // 思维链标记
	ThoughtSignature string            `json:"thoughtSignature,omitempty"` // 函数调用签名（API必需）
}

// FunctionCall 函数调用
type FunctionCall struct {
	ID   string                 `json:"id,omitempty"`
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args"`
}

// FunctionResponse 函数响应
type FunctionResponse struct {
	ID       string                 `json:"id,omitempty"`
	Name     string                 `json:"name"`
	Response map[string]interface{} `json:"response"`
}

// InlineData 内联数据（图片等）
type InlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

// SystemInstruction 系统指令
type SystemInstruction struct {
	Parts []Part `json:"parts"`
}

// Tool 工具定义
type Tool struct {
	FunctionDeclarations []FunctionDeclaration `json:"functionDeclarations,omitempty"`
}

// FunctionDeclaration 函数声明
type FunctionDeclaration struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// ToolConfig 工具配置
type ToolConfig struct {
	FunctionCallingConfig *FunctionCallingConfig `json:"functionCallingConfig,omitempty"`
}

// FunctionCallingConfig 函数调用配置
type FunctionCallingConfig struct {
	Mode                 string   `json:"mode,omitempty"` // AUTO, ANY, NONE
	AllowedFunctionNames []string `json:"allowedFunctionNames,omitempty"`
}

// GenerationConfig 生成配置
type GenerationConfig struct {
	CandidateCount  int             `json:"candidateCount,omitempty"`
	StopSequences   []string        `json:"stopSequences,omitempty"`
	MaxOutputTokens int             `json:"maxOutputTokens,omitempty"`
	Temperature     *float64        `json:"temperature,omitempty"`
	TopP            *float64        `json:"topP,omitempty"`
	TopK            int             `json:"topK,omitempty"`
	ThinkingConfig  *ThinkingConfig `json:"thinkingConfig,omitempty"`
}

// ThinkingConfig 思考配置
type ThinkingConfig struct {
	IncludeThoughts bool   `json:"includeThoughts"`
	ThinkingBudget  int    `json:"thinkingBudget,omitempty"`
	ThinkingLevel   string `json:"thinking_level,omitempty"`
}

// ==================== Antigravity 响应格式 ====================

// AntigravityResponse Antigravity 响应
type AntigravityResponse struct {
	Response struct {
		Candidates    []Candidate    `json:"candidates"`
		UsageMetadata *UsageMetadata `json:"usageMetadata,omitempty"`
	} `json:"response"`
}

// Candidate 候选响应
type Candidate struct {
	Content      Content `json:"content"`
	FinishReason string  `json:"finishReason,omitempty"`
	Index        int     `json:"index"`
}

// UsageMetadata 使用统计
type UsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
	ThoughtsTokenCount   int `json:"thoughtsTokenCount,omitempty"`
}

// ToolCallInfo 流式处理中的工具调用信息（通用中间格式）
type ToolCallInfo struct {
	ID               string                 `json:"id"`
	Name             string                 `json:"name"`
	Args             map[string]interface{} `json:"args"`
	ThoughtSignature string                 `json:"thoughtSignature,omitempty"`
}

// Usage 通用 token 使用统计
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
