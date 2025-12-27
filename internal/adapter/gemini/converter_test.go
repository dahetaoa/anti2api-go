package gemini

import (
	"testing"
)

func TestSanitizeRequestContents(t *testing.T) {
	tests := []struct {
		name     string
		contents []Content
		verify   func(t *testing.T, result []Content)
	}{
		{
			name: "Remove Empty Parts",
			contents: []Content{
				{
					Role: "user",
					Parts: []Part{
						{Text: "hello"},
						{Text: ""}, // Empty Text
						{},         // Completely empty
					},
				},
				{
					Role: "model",
					Parts: []Part{
						{FunctionCall: &FunctionCall{Name: "test"}},
						{}, // Empty
					},
				},
			},
			verify: func(t *testing.T, result []Content) {
				if len(result) != 2 {
					t.Fatalf("Expected 2 contents, got %d", len(result))
				}
				if len(result[0].Parts) != 1 {
					t.Errorf("Expected 1 part in content 0, got %d", len(result[0].Parts))
				}
				if len(result[1].Parts) != 1 {
					t.Errorf("Expected 1 part in content 1, got %d", len(result[1].Parts))
				}
			},
		},
		{
			name: "Tool Name Lookback and Signature Forwarding",
			contents: []Content{
				{
					Role: "model",
					Parts: []Part{
						{
							FunctionCall: &FunctionCall{ID: "call_1", Name: "get_weather"},
						},
						{ThoughtSignature: "sig_123"},
					},
				},
				{
					Role: "user",
					Parts: []Part{
						{
							FunctionResponse: &FunctionResponse{ID: "call_1"}, // Missing Name
						},
					},
				},
				{
					Role: "user",
					Parts: []Part{
						{
							FunctionCall: &FunctionCall{ID: "call_2", Name: "next_tool"}, // Missing Signature
						},
					},
				},
			},
			verify: func(t *testing.T, result []Content) {
				// Verify name lookback
				resp := result[1].Parts[0].FunctionResponse
				if resp.Name != "get_weather" {
					t.Errorf("Expected name 'get_weather', got '%s'", resp.Name)
				}
				// Verify signature forwarding
				if result[2].Parts[0].ThoughtSignature != "sig_123" {
					t.Errorf("Expected signature 'sig_123', got '%s'", result[2].Parts[0].ThoughtSignature)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeRequestContents(tt.contents)
			tt.verify(t, result)
		})
	}
}

func TestSanitizeCandidates(t *testing.T) {
	resp := map[string]interface{}{
		"candidates": []interface{}{
			map[string]interface{}{
				"content": map[string]interface{}{
					"parts": []interface{}{
						map[string]interface{}{
							"text":             "hello",
							"thoughtSignature": "sig_abc",
						},
					},
				},
			},
		},
	}

	sanitizeCandidates(resp)

	// Verify thoughtSignature is NOT deleted
	candidates := resp["candidates"].([]interface{})
	candidate := candidates[0].(map[string]interface{})
	content := candidate["content"].(map[string]interface{})
	parts := content["parts"].([]interface{})
	part := parts[0].(map[string]interface{})

	if _, ok := part["thoughtSignature"]; !ok {
		t.Error("Expected thoughtSignature to be preserved")
	}
}
