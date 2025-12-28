package claude

import (
	"testing"

	"anti2api-golang/internal/store"
)

func TestConvertClaudeContentToParts(t *testing.T) {
	tests := []struct {
		name     string
		content  interface{}
		expected int
		verify   func(t *testing.T, parts []Part)
	}{
		{
			name:     "Simple Text",
			content:  "Hello world",
			expected: 1,
			verify: func(t *testing.T, parts []Part) {
				if parts[0].Text != "Hello world" {
					t.Errorf("Expected 'Hello world', got '%s'", parts[0].Text)
				}
			},
		},
		{
			name: "Thinking + Tool Use",
			content: []interface{}{
				map[string]interface{}{
					"type":      "thinking",
					"thinking":  "I should call a tool",
					"signature": "sig123",
				},
				map[string]interface{}{
					"type": "tool_use",
					"id":   "tool_1",
					"name": "get_weather",
					"input": map[string]interface{}{
						"city": "London",
					},
				},
			},
			expected: 2,
			verify: func(t *testing.T, parts []Part) {
				// Part 0: Thinking (无签名，签名已移至 functionCall)
				if parts[0].Text != "I should call a tool" || !parts[0].Thought {
					t.Errorf("Thinking part mismatch: %+v", parts[0])
				}
				if parts[0].ThoughtSignature != "" {
					t.Errorf("Expected no signature on thinking part, got %s", parts[0].ThoughtSignature)
				}
				// Part 1: Tool Use (签名在这里)
				if parts[1].FunctionCall == nil || parts[1].FunctionCall.Name != "get_weather" || parts[1].FunctionCall.ID != "tool_1" {
					t.Errorf("Tool use part mismatch: %+v", parts[1])
				}
				// 签名应在 functionCall 上
				if parts[1].ThoughtSignature != "sig123" {
					t.Errorf("Expected signature sig123 on tool call, got %s", parts[1].ThoughtSignature)
				}
			},
		},
		{
			name: "Tool Result (JSON) with Name Mapping",
			content: []interface{}{
				map[string]interface{}{
					"type":        "tool_result",
					"tool_use_id": "tool_1",
					"content":     `{"temp": 20}`,
				},
			},
			expected: 1,
			verify: func(t *testing.T, parts []Part) {
				if parts[0].FunctionResponse == nil || parts[0].FunctionResponse.ID != "tool_1" {
					t.Errorf("Tool result part mismatch: %+v", parts[0])
				}
				if parts[0].FunctionResponse.Name != "get_weather" {
					t.Errorf("Expected name 'get_weather', got '%s'", parts[0].FunctionResponse.Name)
				}
				if parts[0].FunctionResponse.Response["temp"] != float64(20) {
					t.Errorf("Expected temp 20, got %v", parts[0].FunctionResponse.Response["temp"])
				}
			},
		},
		{
			name: "Tool Result (Plain Text)",
			content: []interface{}{
				map[string]interface{}{
					"type":        "tool_result",
					"tool_use_id": "tool_1",
					"content":     "Success",
				},
			},
			expected: 1,
			verify: func(t *testing.T, parts []Part) {
				if parts[0].FunctionResponse == nil || parts[0].FunctionResponse.Response["result"] != "Success" {
					t.Errorf("Expected result 'Success', got %+v", parts[0].FunctionResponse.Response)
				}
			},
		},
		{
			name: "Thinking + Text (signature on text)",
			content: []interface{}{
				map[string]interface{}{
					"type":      "thinking",
					"thinking":  "Let me think...",
					"signature": "sig_text_123",
				},
				map[string]interface{}{
					"type": "text",
					"text": "Here is my answer",
				},
			},
			expected: 2,
			verify: func(t *testing.T, parts []Part) {
				// Part 0: Thinking (无签名)
				if parts[0].ThoughtSignature != "" {
					t.Errorf("Expected no signature on thinking, got %s", parts[0].ThoughtSignature)
				}
				// Part 1: Text (签名在这里)
				if parts[1].ThoughtSignature != "sig_text_123" {
					t.Errorf("Expected signature sig_text_123 on text, got %s", parts[1].ThoughtSignature)
				}
			},
		},
		{
			name: "Parallel Tool Calls (signature on first only)",
			content: []interface{}{
				map[string]interface{}{
					"type":      "thinking",
					"thinking":  "I need to call two tools",
					"signature": "sig_parallel",
				},
				map[string]interface{}{
					"type":  "tool_use",
					"id":    "tool_a",
					"name":  "get_weather",
					"input": map[string]interface{}{"city": "Paris"},
				},
				map[string]interface{}{
					"type":  "tool_use",
					"id":    "tool_b",
					"name":  "get_weather",
					"input": map[string]interface{}{"city": "London"},
				},
			},
			expected: 3,
			verify: func(t *testing.T, parts []Part) {
				// 只有第一个 functionCall 有签名
				if parts[1].ThoughtSignature != "sig_parallel" {
					t.Errorf("Expected signature on first tool call, got %s", parts[1].ThoughtSignature)
				}
				if parts[2].ThoughtSignature != "" {
					t.Errorf("Expected no signature on second tool call, got %s", parts[2].ThoughtSignature)
				}
			},
		},
	}

	toolIDToName := map[string]string{"tool_1": "get_weather"}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts := convertClaudeContentToParts(tt.content, toolIDToName)
			if len(parts) != tt.expected {
				t.Errorf("Expected %d parts, got %d", tt.expected, len(parts))
				return
			}
			tt.verify(t, parts)
		})
	}
}

func TestConvertClaudeToAntigravity(t *testing.T) {
	req := &ClaudeMessagesRequest{
		Model:     "claude-3-5-sonnet",
		MaxTokens: 1024,
		Messages: []ClaudeMessage{
			{
				Role: "assistant",
				Content: []interface{}{
					map[string]interface{}{
						"type": "tool_use",
						"id":   "tool_1",
						"name": "get_weather",
						"input": map[string]interface{}{
							"city": "London",
						},
					},
				},
			},
			{
				Role: "user",
				Content: []interface{}{
					map[string]interface{}{
						"type":        "tool_result",
						"tool_use_id": "tool_1",
						"content":     `{"temp": 20}`,
					},
				},
			},
		},
	}
	account := &store.Account{
		ProjectID: "test-project",
		SessionID: "test-session",
	}

	antireq, err := ConvertClaudeToAntigravity(req, account)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if antireq.Project != "test-project" {
		t.Errorf("Expected project test-project, got %s", antireq.Project)
	}
	// Verify that the second message (index 1) has a functionResponse with the correct name
	if len(antireq.Request.Contents) != 2 {
		t.Fatalf("Expected 2 messages, got %d", len(antireq.Request.Contents))
	}
	respPart := antireq.Request.Contents[1].Parts[0]
	if respPart.FunctionResponse == nil {
		t.Fatalf("Expected functionResponse, got %+v", respPart)
	}
	if respPart.FunctionResponse.Name != "get_weather" {
		t.Errorf("Expected functionResponse name 'get_weather', got '%s'", respPart.FunctionResponse.Name)
	}
}
