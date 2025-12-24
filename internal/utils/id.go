package utils

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/google/uuid"
)

// GenerateRequestID 生成请求 ID (agent-{uuid})
func GenerateRequestID() string {
	return "agent-" + uuid.New().String()
}

// GenerateSessionID 生成会话 ID (随机负数)
func GenerateSessionID() string {
	max := new(big.Int).SetUint64(9e18)
	n, _ := rand.Int(rand.Reader, max)
	return "-" + n.String()
}

// GenerateProjectID 生成项目 ID ({adjective}-{noun}-{random})
func GenerateProjectID() string {
	adjectives := []string{"useful", "bright", "swift", "calm", "bold", "happy", "clever", "gentle", "quick", "brave"}
	nouns := []string{"fuze", "wave", "spark", "flow", "core", "beam", "star", "wind", "leaf", "cloud"}

	adj := adjectives[randInt(len(adjectives))]
	noun := nouns[randInt(len(nouns))]
	suffix := randomAlphanumeric(5)

	return fmt.Sprintf("%s-%s-%s", adj, noun, suffix)
}

// GenerateToolCallID 生成工具调用 ID (call_{uuid without dashes})
func GenerateToolCallID() string {
	id := uuid.New().String()
	return "call_" + strings.ReplaceAll(id, "-", "")
}

// GenerateSecureToken 生成安全令牌
func GenerateSecureToken(length int) string {
	bytes := make([]byte, length)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// GenerateChatCompletionID 生成聊天完成 ID
func GenerateChatCompletionID() string {
	return fmt.Sprintf("chatcmpl-%s", uuid.New().String()[:8])
}

// 辅助函数

func randInt(max int) int {
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(max)))
	return int(n.Int64())
}

func randomAlphanumeric(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[randInt(len(charset))]
	}
	return string(result)
}
