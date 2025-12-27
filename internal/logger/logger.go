package logger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"anti2api-golang/internal/config"
)

// LogLevel 日志级别
type LogLevel int

const (
	LogOff  LogLevel = 0 // 仅基本日志
	LogLow  LogLevel = 1 // + 客户端请求/响应
	LogHigh LogLevel = 2 // + 后端 API 请求/响应
)

// 颜色常量
const (
	ColorReset  = "\x1b[0m"
	ColorGreen  = "\x1b[32m"
	ColorYellow = "\x1b[33m"
	ColorRed    = "\x1b[31m"
	ColorCyan   = "\x1b[36m"
	ColorGray   = "\x1b[90m"
	ColorBlue   = "\x1b[34m"
	ColorPurple = "\x1b[35m"
)

var currentLogLevel LogLevel

// Init 初始化日志系统
func Init() {
	cfg := config.Get()
	currentLogLevel = parseLogLevel(cfg.Debug)
}

func parseLogLevel(debug string) LogLevel {
	switch strings.ToLower(debug) {
	case "low":
		return LogLow
	case "high":
		return LogHigh
	default:
		return LogOff
	}
}

// GetLevel 获取当前日志级别
func GetLevel() LogLevel {
	return currentLogLevel
}

// Info 信息日志
func Info(format string, args ...interface{}) {
	timestamp := time.Now().Format("15:04:05")
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("%s%s%s %s[info]%s %s\n", ColorGray, timestamp, ColorReset, ColorGreen, ColorReset, msg)
}

// Warn 警告日志
func Warn(format string, args ...interface{}) {
	timestamp := time.Now().Format("15:04:05")
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("%s%s%s %s[warn]%s %s\n", ColorGray, timestamp, ColorReset, ColorYellow, ColorReset, msg)
}

// Error 错误日志
func Error(format string, args ...interface{}) {
	timestamp := time.Now().Format("15:04:05")
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("%s%s%s %s[error]%s %s\n", ColorGray, timestamp, ColorReset, ColorRed, ColorReset, msg)
}

// Debug 调试日志
func Debug(format string, args ...interface{}) {
	if currentLogLevel < LogLow {
		return
	}
	timestamp := time.Now().Format("15:04:05")
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("%s%s%s %s[debug]%s %s\n", ColorGray, timestamp, ColorReset, ColorBlue, ColorReset, msg)
}

// Request 请求日志
func Request(method, path string, status int, duration time.Duration) {
	statusColor := ColorGreen
	if status >= 500 {
		statusColor = ColorRed
	} else if status >= 400 {
		statusColor = ColorYellow
	}

	fmt.Printf("%s[%s]%s %s %s%d%s %s%dms%s\n",
		ColorCyan, method, ColorReset,
		path,
		statusColor, status, ColorReset,
		ColorGray, duration.Milliseconds(), ColorReset)
}

// ClientRequest 客户端请求日志（原始 JSON 透传）
func ClientRequest(method, path string, rawJSON []byte) {
	if currentLogLevel < LogLow {
		return
	}

	fmt.Printf("%s===================== 客户端请求 ======================%s\n", ColorPurple, ColorReset)
	fmt.Printf("%s[客户端请求]%s %s%s%s %s\n", ColorPurple, ColorReset, ColorCyan, method, ColorReset, path)
	if len(rawJSON) > 0 {
		fmt.Println(formatRawJSON(rawJSON))
	}
	fmt.Printf("%s=========================================================%s\n", ColorPurple, ColorReset)
}

// ClientResponse 客户端响应日志
func ClientResponse(status int, duration time.Duration, body interface{}) {
	if currentLogLevel < LogLow {
		return
	}

	statusColor := ColorGreen
	if status >= 400 {
		statusColor = ColorRed
	}

	fmt.Printf("%s===================== 客户端响应 ======================%s\n", ColorPurple, ColorReset)
	fmt.Printf("%s[客户端响应]%s %s%d%s %s%dms%s\n", ColorPurple, ColorReset, statusColor, status, ColorReset, ColorGray, duration.Milliseconds(), ColorReset)
	if body != nil {
		printJSON(body)
	}
	fmt.Printf("%s==========================================================%s\n", ColorPurple, ColorReset)
}

// BackendRequest 后端请求日志（原始 JSON 透传）
func BackendRequest(method, url string, rawJSON []byte) {
	if currentLogLevel < LogHigh {
		return
	}

	fmt.Printf("%s====================== 后端请求 ========================%s\n", ColorYellow, ColorReset)
	fmt.Printf("%s[后端请求]%s %s%s%s %s\n", ColorYellow, ColorReset, ColorCyan, method, ColorReset, url)
	if len(rawJSON) > 0 {
		fmt.Println(formatRawJSON(rawJSON))
	}
	fmt.Printf("%s==========================================================%s\n", ColorYellow, ColorReset)
}

// BackendResponse 后端响应日志
func BackendResponse(status int, duration time.Duration, body interface{}) {
	if currentLogLevel < LogHigh {
		return
	}

	statusColor := ColorGreen
	if status >= 400 {
		statusColor = ColorRed
	}

	fmt.Printf("%s====================== 后端响应 ========================%s\n", ColorGreen, ColorReset)
	fmt.Printf("%s[后端响应]%s %s%d%s %s%dms%s\n", ColorGreen, ColorReset, statusColor, status, ColorReset, ColorGray, duration.Milliseconds(), ColorReset)
	if body != nil {
		printJSON(body)
	}
	fmt.Printf("%s==========================================================%s\n", ColorGreen, ColorReset)
}

// BackendStreamResponse 后端流式响应日志（合并后的）
func BackendStreamResponse(status int, duration time.Duration, body interface{}) {
	if currentLogLevel < LogHigh {
		return
	}

	statusColor := ColorGreen
	if status >= 400 {
		statusColor = ColorRed
	}

	fmt.Printf("%s==================== 后端流式响应 =======================%s\n", ColorGreen, ColorReset)
	fmt.Printf("%s[后端流式]%s %s%d%s %s%dms%s\n", ColorGreen, ColorReset, statusColor, status, ColorReset, ColorGray, duration.Milliseconds(), ColorReset)
	if body != nil {
		printJSON(body)
	}
	fmt.Printf("%s==========================================================%s\n", ColorGreen, ColorReset)
}

// ClientStreamResponse 客户端流式响应日志（合并后的）
func ClientStreamResponse(status int, duration time.Duration, body interface{}) {
	if currentLogLevel < LogLow {
		return
	}

	statusColor := ColorGreen
	if status >= 400 {
		statusColor = ColorRed
	}

	fmt.Printf("%s=================== 客户端流式响应 =======================%s\n", ColorPurple, ColorReset)
	fmt.Printf("%s[客户端流式]%s %s%d%s %s%dms%s\n", ColorPurple, ColorReset, statusColor, status, ColorReset, ColorGray, duration.Milliseconds(), ColorReset)
	if body != nil {
		printJSON(body)
	}
	fmt.Printf("%s==========================================================%s\n", ColorPurple, ColorReset)
}

func printJSON(v interface{}) {
	jsonBytes, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Printf("%v\n", v)
		return
	}

	// 限制输出长度
	output := string(jsonBytes)
	if len(output) > 5000 {
		output = output[:5000] + "\n... (truncated)"
	}
	fmt.Println(output)
}

// formatRawJSON 格式化原始 JSON 字节（直接透传，仅美化格式）
func formatRawJSON(rawJSON []byte) string {
	var indented bytes.Buffer
	if err := json.Indent(&indented, rawJSON, "", "  "); err != nil {
		// 无法格式化时直接返回原始字符串
		return string(rawJSON)
	}
	output := indented.String()
	if len(output) > 5000 {
		output = output[:5000] + "\n... (truncated)"
	}
	return output
}

// Banner 打印启动横幅
func Banner(port int, endpointMode string) {
	fmt.Printf(`
%s╔════════════════════════════════════════════════════════════╗
║           %sAntigravity2API%s - Go Version                      ║
╚════════════════════════════════════════════════════════════╝%s
`, ColorCyan, ColorGreen, ColorCyan, ColorReset)

	Info("Server starting on port %d", port)
	Info("Endpoint mode: %s", endpointMode)
	Info("Debug level: %s", config.Get().Debug)

	if os.Getenv("API_KEY") == "" {
		Warn("API_KEY not set - API authentication disabled")
	}

	fmt.Println()
}
