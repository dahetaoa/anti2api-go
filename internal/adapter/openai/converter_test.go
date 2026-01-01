package openai

import (
	"anti2api-golang/internal/store"
	"testing"
)

func TestConvertOpenAIToAntigravity(t *testing.T) {
	account := &store.Account{ProjectID: "test-project"}
	req := &OpenAIChatRequest{
		Model: "gemini-3-pro",
		Messages: []OpenAIMessage{
			{
				Role: "assistant",
				ToolCalls: []OpenAIToolCall{
					{
						ID:   "call_1",
						Type: "function",
						Function: OpenAIFunctionCall{
							Name:      "get_weather",
							Arguments: `{"location": "London"}`,
						},
						ExtraContent: &ExtraContent{
							Google: &GoogleExtra{
								ThoughtSignature: "sig_abc",
							},
						},
					},
				},
			},
		},
	}

	antigravityReq := ConvertOpenAIToAntigravity(req, account)
	if len(antigravityReq.Request.Contents) != 1 {
		t.Fatalf("Expected 1 content, got %d", len(antigravityReq.Request.Contents))
	}

	parts := antigravityReq.Request.Contents[0].Parts
	if len(parts) != 1 {
		t.Fatalf("Expected 1 part, got %d", len(parts))
	}

	if parts[0].ThoughtSignature != "sig_abc" {
		t.Errorf("Expected signature 'sig_abc', got '%s'", parts[0].ThoughtSignature)
	}
}

func TestConvertToOpenAIResponse(t *testing.T) {
	resp := &AntigravityResponse{}
	resp.Response.Candidates = []Candidate{
		{
			Content: Content{
				Role: "model",
				Parts: []Part{
					{
						FunctionCall: &FunctionCall{
							ID:   "call_1",
							Name: "get_weather",
							Args: map[string]interface{}{"location": "London"},
						},
						ThoughtSignature: "sig_123",
					},
				},
			},
		},
	}

	openAIResp := ConvertToOpenAIResponse(resp, "gemini-3-pro")
	if len(openAIResp.Choices) != 1 {
		t.Fatalf("Expected 1 choice, got %d", len(openAIResp.Choices))
	}

	msg := openAIResp.Choices[0].Message
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("Expected 1 tool call, got %d", len(msg.ToolCalls))
	}

	tc := msg.ToolCalls[0]
	if tc.ExtraContent == nil || tc.ExtraContent.Google == nil || tc.ExtraContent.Google.ThoughtSignature != "sig_123" {
		t.Errorf("Expected signature 'sig_123' in extra_content, got %+v", tc.ExtraContent)
	}
}
