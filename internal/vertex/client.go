package vertex

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"anti2api-golang/internal/config"
	"anti2api-golang/internal/core"
	"anti2api-golang/internal/logger"
	"anti2api-golang/internal/store"
)

// Client API 客户端
type Client struct {
	httpClient *http.Client
	config     *config.Config
}

// APIError API 错误
type APIError struct {
	Status       int
	Message      string
	RetryDelay   time.Duration
	DisableToken bool
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API error %d: %s", e.Status, e.Message)
}

// NewClient 创建新的 API 客户端
func NewClient() *Client {
	cfg := config.Get()

	transport := &http.Transport{
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second, // 等待响应头的超时
		// 禁用 HTTP/2 以避免其多路复用带来的流式延迟
		ForceAttemptHTTP2: false,
	}

	// 设置代理
	if cfg.Proxy != "" {
		proxyURL, err := url.Parse(cfg.Proxy)
		if err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}

	return &Client{
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   time.Duration(cfg.Timeout) * time.Millisecond,
		},
		config: cfg,
	}
}

// BuildHeaders 构建请求头（非流式请求）
func (c *Client) BuildHeaders(token *store.Account, endpoint config.Endpoint) http.Header {
	return http.Header{
		"Host":            {endpoint.Host},
		"User-Agent":      {c.config.UserAgent},
		"Authorization":   {"Bearer " + token.AccessToken},
		"Content-Type":    {"application/json"},
		"Accept-Encoding": {"gzip"},
	}
}

// BuildStreamHeaders 构建流式请求头（禁用 gzip 以保证流式输出平滑）
func (c *Client) BuildStreamHeaders(token *store.Account, endpoint config.Endpoint) http.Header {
	return http.Header{
		"Host":          {endpoint.Host},
		"User-Agent":    {c.config.UserAgent},
		"Authorization": {"Bearer " + token.AccessToken},
		"Content-Type":  {"application/json"},
		// 不设置 Accept-Encoding: gzip，避免上游服务器缓冲压缩数据导致流式输出不平滑
	}
}

// SendRequest 发送非流式请求
func (c *Client) SendRequest(ctx context.Context, req *core.AntigravityRequest, token *store.Account) (*core.AntigravityResponse, error) {
	endpoint := config.GetEndpointManager().GetActiveEndpoint()
	reqURL := endpoint.NoStreamURL()

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	logger.BackendRequest("POST", reqURL, req)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	for key, values := range c.BuildHeaders(token, endpoint) {
		for _, value := range values {
			httpReq.Header.Add(key, value)
		}
	}

	startTime := time.Now()
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 处理 gzip
	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		defer gzReader.Close()
		reader = gzReader
	}

	respBody, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	duration := time.Since(startTime)

	if resp.StatusCode != 200 {
		apiErr := ExtractErrorDetails(resp, respBody)
		logger.BackendResponse(resp.StatusCode, duration, string(respBody))
		return nil, apiErr
	}

	var antigravityResp core.AntigravityResponse
	if err := json.Unmarshal(respBody, &antigravityResp); err != nil {
		logger.BackendResponse(resp.StatusCode, duration, string(respBody))
		return nil, err
	}

	logger.BackendResponse(resp.StatusCode, duration, antigravityResp)
	return &antigravityResp, nil
}

// SendStreamRequest 发送流式请求
func (c *Client) SendStreamRequest(ctx context.Context, req *core.AntigravityRequest, token *store.Account) (*http.Response, error) {
	endpoint := config.GetEndpointManager().GetActiveEndpoint()
	reqURL := endpoint.StreamURL()

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	logger.BackendRequest("POST", reqURL, req)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	// 流式请求使用专用请求头（禁用 gzip）
	for key, values := range c.BuildStreamHeaders(token, endpoint) {
		for _, value := range values {
			httpReq.Header.Add(key, value)
		}
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		defer resp.Body.Close()

		// 处理 gzip
		var reader io.Reader = resp.Body
		if resp.Header.Get("Content-Encoding") == "gzip" {
			gzReader, err := gzip.NewReader(resp.Body)
			if err != nil {
				return nil, &APIError{Status: resp.StatusCode, Message: "failed to decompress response"}
			}
			defer gzReader.Close()
			reader = gzReader
		}

		respBody, _ := io.ReadAll(reader)
		apiErr := ExtractErrorDetails(resp, respBody)
		logger.BackendResponse(resp.StatusCode, 0, string(respBody))
		return nil, apiErr
	}

	return resp, nil
}

// ExtractErrorDetails 提取错误详情
func ExtractErrorDetails(resp *http.Response, body []byte) *APIError {
	apiErr := &APIError{
		Status:  resp.StatusCode,
		Message: "Unknown error",
	}

	var errorResp struct {
		Error struct {
			Code    interface{} `json:"code"`
			Status  string      `json:"status"`
			Message string      `json:"message"`
			Details []struct {
				Type       string `json:"@type"`
				RetryDelay string `json:"retryDelay"`
			} `json:"details"`
		} `json:"error"`
	}

	if json.Unmarshal(body, &errorResp) == nil {
		apiErr.Message = errorResp.Error.Message

		// 解析状态码
		switch v := errorResp.Error.Code.(type) {
		case string:
			switch strings.ToUpper(v) {
			case "RESOURCE_EXHAUSTED":
				apiErr.Status = 429
			case "INTERNAL":
				apiErr.Status = 500
			case "UNAUTHENTICATED":
				apiErr.Status = 401
				apiErr.DisableToken = true
			}
		case float64:
			apiErr.Status = int(v)
		}

		// 解析重试延迟
		for _, detail := range errorResp.Error.Details {
			if strings.Contains(detail.Type, "RetryInfo") {
				re := regexp.MustCompile(`(\d+(?:\.\d+)?)s`)
				if matches := re.FindStringSubmatch(detail.RetryDelay); len(matches) > 1 {
					if seconds, err := strconv.ParseFloat(matches[1], 64); err == nil {
						apiErr.RetryDelay = time.Duration(seconds * float64(time.Second))
					}
				}
			}
		}
	}

	return apiErr
}

// WithRetry 带重试的请求
func (c *Client) WithRetry(ctx context.Context, operation func() error) error {
	var lastErr error

	for attempt := 0; attempt < c.config.RetryMaxAttempts; attempt++ {
		err := operation()
		if err == nil {
			return nil
		}

		lastErr = err

		apiErr, ok := err.(*APIError)
		if !ok {
			return err
		}

		// 401 错误不重试
		if apiErr.Status == 401 {
			return err
		}

		// 检查是否应该重试
		shouldRetry := false
		for _, code := range c.config.RetryStatusCodes {
			if apiErr.Status == code {
				shouldRetry = true
				break
			}
		}

		if !shouldRetry || attempt == c.config.RetryMaxAttempts-1 {
			return err
		}

		// 计算延迟
		delay := apiErr.RetryDelay
		if delay == 0 {
			delay = time.Duration(min(1000*(attempt+1), 5000)) * time.Millisecond
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}

		logger.Warn("Retrying request (attempt %d/%d)", attempt+2, c.config.RetryMaxAttempts)
	}

	return lastErr
}

// GetClient 获取全局客户端单例
var apiClient *Client

func GetClient() *Client {
	if apiClient == nil {
		apiClient = NewClient()
	}
	return apiClient
}

// GenerateContent 非流式生成内容
func GenerateContent(ctx context.Context, req *core.AntigravityRequest, token *store.Account) (*core.AntigravityResponse, error) {
	client := GetClient()
	var result *core.AntigravityResponse
	var err error

	retryErr := client.WithRetry(ctx, func() error {
		result, err = client.SendRequest(ctx, req, token)
		return err
	})

	if retryErr != nil {
		return nil, retryErr
	}

	return result, nil
}

// GenerateContentStream 流式生成内容
func GenerateContentStream(ctx context.Context, req *core.AntigravityRequest, token *store.Account) (*http.Response, error) {
	client := GetClient()
	var result *http.Response
	var err error

	retryErr := client.WithRetry(ctx, func() error {
		result, err = client.SendStreamRequest(ctx, req, token)
		return err
	})

	if retryErr != nil {
		return nil, retryErr
	}

	return result, nil
}

// IsRetryableError 检查是否为可重试错误
func IsRetryableError(err error) bool {
	apiErr, ok := err.(*APIError)
	if !ok {
		return false
	}

	cfg := config.Get()
	for _, code := range cfg.RetryStatusCodes {
		if apiErr.Status == code {
			return true
		}
	}
	return false
}

// ShouldDisableToken 检查是否应禁用 token
func ShouldDisableToken(err error) bool {
	apiErr, ok := err.(*APIError)
	if !ok {
		return false
	}
	return apiErr.DisableToken
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
