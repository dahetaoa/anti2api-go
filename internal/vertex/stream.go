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

// ParseStream 解析流式响应
func ParseStream(resp *http.Response, receiver func(data *StreamData) error) (*core.UsageMetadata, error) {
	defer resp.Body.Close()

	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		defer gzReader.Close()
		reader = gzReader
	}

	// 4KB 缓冲区
	bufReader := bufio.NewReaderSize(reader, 4*1024)

	var usage *core.UsageMetadata

	for {
		line, err := bufReader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return usage, err
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

		var data StreamData
		if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
			continue
		}

		// 提取 usage
		if data.Response.UsageMetadata != nil {
			usage = data.Response.UsageMetadata
		}

		if err := receiver(&data); err != nil {
			return usage, err
		}
	}

	return usage, nil
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
