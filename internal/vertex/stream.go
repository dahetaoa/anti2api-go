package vertex

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"anti2api-golang/internal/core"
)

// StreamData 原始流式数据
type StreamData struct {
	Response struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text             string             `json:"text,omitempty"`
					FunctionCall     *core.FunctionCall `json:"functionCall,omitempty"`
					Thought          bool               `json:"thought,omitempty"`
					ThoughtSignature string             `json:"thoughtSignature,omitempty"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason,omitempty"`
		} `json:"candidates"`
		UsageMetadata *core.UsageMetadata `json:"usageMetadata,omitempty"`
	} `json:"response"`
}

// StreamReceiver 接收流式数据的接口
type StreamReceiver interface {
	ProcessData(data *StreamData) error
}

// StreamResult 流式解析结果（用于日志记录）
// 保留原始 JSON 结构，仅合并 text 内容
type StreamResult struct {
	// RawChunks 原始流式数据块（用于透传日志）
	RawChunks []map[string]interface{} `json:"-"`
	// MergedResponse 合并后的响应（text 合并，其他字段透传）
	MergedResponse map[string]interface{} `json:"-"`
	// 简化字段用于快速访问
	Text              string              `json:"-"`
	Thinking          string              `json:"-"`
	ThinkingSignature string              `json:"-"`
	ToolCalls         []core.ToolCallInfo `json:"-"`
	FinishReason      string              `json:"-"`
	Usage             *core.UsageMetadata `json:"-"`
}

// ParseStream 解析流式响应
func ParseStream(resp *http.Response, receiver func(data *StreamData) error) (*core.UsageMetadata, error) {
	result, err := ParseStreamWithResult(resp, receiver)
	if err != nil {
		return result.Usage, err
	}
	return result.Usage, nil
}

// ParseStreamWithResult 解析流式响应并返回合并结果（用于日志记录）
// 保留原始 JSON 结构，仅合并 text 内容
func ParseStreamWithResult(resp *http.Response, receiver func(data *StreamData) error) (*StreamResult, error) {
	defer resp.Body.Close()

	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return &StreamResult{}, err
		}
		defer gzReader.Close()
		reader = gzReader
	}

	// 4KB 缓冲区
	bufReader := bufio.NewReaderSize(reader, 4*1024)

	result := &StreamResult{}
	var textBuilder strings.Builder
	var thinkingBuilder strings.Builder

	// 收集所有原始 JSON 块
	var rawChunks []map[string]interface{}
	// 收集所有 parts（用于合并）
	var mergedParts []interface{}
	var lastFinishReason string
	var lastUsage interface{}

	for {
		line, err := bufReader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			result.Text = textBuilder.String()
			result.Thinking = thinkingBuilder.String()
			return result, err
		}

		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		jsonData := line[6:]
		if jsonData == "[DONE]" {
			break
		}

		// 解析为原始 map 保留所有字段
		var rawChunk map[string]interface{}
		if err := json.Unmarshal([]byte(jsonData), &rawChunk); err != nil {
			continue
		}
		rawChunks = append(rawChunks, rawChunk)

		// 同时解析为结构化数据用于处理
		var data StreamData
		if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
			continue
		}

		// 提取 usage
		if data.Response.UsageMetadata != nil {
			result.Usage = data.Response.UsageMetadata
			// 保留原始 usage
			if resp, ok := rawChunk["response"].(map[string]interface{}); ok {
				if usage, ok := resp["usageMetadata"]; ok {
					lastUsage = usage
				}
			}
		}

		// 收集原始 parts 和合并 text
		if len(data.Response.Candidates) > 0 {
			candidate := data.Response.Candidates[0]
			if candidate.FinishReason != "" {
				result.FinishReason = candidate.FinishReason
				lastFinishReason = candidate.FinishReason
			}

			// 从原始 JSON 中提取 parts
			if resp, ok := rawChunk["response"].(map[string]interface{}); ok {
				if candidates, ok := resp["candidates"].([]interface{}); ok && len(candidates) > 0 {
					if cand, ok := candidates[0].(map[string]interface{}); ok {
						if content, ok := cand["content"].(map[string]interface{}); ok {
							if parts, ok := content["parts"].([]interface{}); ok {
								mergedParts = append(mergedParts, parts...)
							}
						}
					}
				}
			}

			for _, part := range candidate.Content.Parts {
				if part.ThoughtSignature != "" {
					result.ThinkingSignature = part.ThoughtSignature
				}
				if part.Thought {
					thinkingBuilder.WriteString(part.Text)
				} else if part.Text != "" {
					textBuilder.WriteString(part.Text)
				} else if part.FunctionCall != nil {
					tc := core.ToolCallInfo{
						ID:               part.FunctionCall.ID,
						Name:             part.FunctionCall.Name,
						Args:             part.FunctionCall.Args,
						ThoughtSignature: part.ThoughtSignature,
					}
					result.ToolCalls = append(result.ToolCalls, tc)
				}
			}
		}

		if err := receiver(&data); err != nil {
			result.Text = textBuilder.String()
			result.Thinking = thinkingBuilder.String()
			return result, err
		}
	}

	result.Text = textBuilder.String()
	result.Thinking = thinkingBuilder.String()
	result.RawChunks = rawChunks

	// 构建合并后的响应（保留原始结构，合并 parts 中的 text）
	result.MergedResponse = map[string]interface{}{
		"response": map[string]interface{}{
			"candidates": []interface{}{
				map[string]interface{}{
					"content": map[string]interface{}{
						"role":  "model",
						"parts": mergeParts(mergedParts),
					},
					"finishReason": lastFinishReason,
				},
			},
			"usageMetadata": lastUsage,
		},
	}

	return result, nil
}

// mergeParts 合并 parts，将连续的 text 合并，保留其他字段原样
// 如果一个 text part 包含其他字段（如 thoughtSignature），也会保留这些字段
func mergeParts(parts []interface{}) []interface{} {
	if len(parts) == 0 {
		return parts
	}

	var merged []interface{}
	var textBuilder strings.Builder
	var thinkingBuilder strings.Builder
	// 收集 text part 中的非 text 字段（用于合并到最终的 text part）
	var textExtraFields map[string]interface{}
	var thinkingExtraFields map[string]interface{}

	// 辅助函数：从 part 中提取非 text/thought 字段
	extractExtraFields := func(part map[string]interface{}) map[string]interface{} {
		extra := make(map[string]interface{})
		for k, v := range part {
			if k != "text" && k != "thought" {
				extra[k] = v
			}
		}
		if len(extra) == 0 {
			return nil
		}
		return extra
	}

	// 辅助函数：合并额外字段
	mergeExtraFields := func(existing, newFields map[string]interface{}) map[string]interface{} {
		if newFields == nil {
			return existing
		}
		if existing == nil {
			existing = make(map[string]interface{})
		}
		for k, v := range newFields {
			existing[k] = v
		}
		return existing
	}

	// 辅助函数：构建带有额外字段的 part
	buildPart := func(text string, thought bool, extra map[string]interface{}) map[string]interface{} {
		result := map[string]interface{}{"text": text}
		if thought {
			result["thought"] = true
		}
		for k, v := range extra {
			result[k] = v
		}
		return result
	}

	for _, p := range parts {
		part, ok := p.(map[string]interface{})
		if !ok {
			merged = append(merged, p)
			continue
		}

		// 检查是否为思考内容
		thought, isThought := part["thought"].(bool)

		if text, hasText := part["text"].(string); hasText && text != "" {
			extra := extractExtraFields(part)
			if isThought && thought {
				// 如果之前有普通文本，先输出
				if textBuilder.Len() > 0 {
					merged = append(merged, buildPart(textBuilder.String(), false, textExtraFields))
					textBuilder.Reset()
					textExtraFields = nil
				}
				thinkingBuilder.WriteString(text)
				thinkingExtraFields = mergeExtraFields(thinkingExtraFields, extra)
			} else {
				// 如果之前有思考内容，先输出
				if thinkingBuilder.Len() > 0 {
					merged = append(merged, buildPart(thinkingBuilder.String(), true, thinkingExtraFields))
					thinkingBuilder.Reset()
					thinkingExtraFields = nil
				}
				textBuilder.WriteString(text)
				textExtraFields = mergeExtraFields(textExtraFields, extra)
			}
		} else {
			// 非文本 part（如 functionCall），先输出累积的文本
			if textBuilder.Len() > 0 {
				merged = append(merged, buildPart(textBuilder.String(), false, textExtraFields))
				textBuilder.Reset()
				textExtraFields = nil
			}
			if thinkingBuilder.Len() > 0 {
				merged = append(merged, buildPart(thinkingBuilder.String(), true, thinkingExtraFields))
				thinkingBuilder.Reset()
				thinkingExtraFields = nil
			}
			// 保留原始 part（包含所有字段如 functionCall, thoughtSignature 等）
			merged = append(merged, part)
		}
	}

	// 输出剩余的文本
	if thinkingBuilder.Len() > 0 {
		merged = append(merged, buildPart(thinkingBuilder.String(), true, thinkingExtraFields))
	}
	if textBuilder.Len() > 0 {
		merged = append(merged, buildPart(textBuilder.String(), false, textExtraFields))
	}

	return merged
}

// SetStreamHeaders 设置流式响应头
func SetStreamHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
}

// WriteStreamData 写入流式数据
func WriteStreamData(w http.ResponseWriter, data interface{}) error {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", jsonBytes)
	if err != nil {
		return err
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}

// WriteStreamDone 写入流结束标记
func WriteStreamDone(w http.ResponseWriter) {
	w.Write([]byte("data: [DONE]\n\n"))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// WriteStreamError 写入流错误
func WriteStreamError(w http.ResponseWriter, errMsg string) {
	errResp := map[string]interface{}{
		"error": map[string]interface{}{
			"message": errMsg,
			"type":    "server_error",
		},
	}
	WriteStreamData(w, errResp)
	WriteStreamDone(w)
}
