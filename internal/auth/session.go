package auth

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

// 会话管理
var (
	panelSessions = sync.Map{} // token -> expiresAt
	sessionTTL    = 2 * time.Hour
)

// CreateSession 创建会话
func CreateSession() string {
	token := generateSecureToken(24)
	expiresAt := time.Now().Add(sessionTTL)
	panelSessions.Store(token, expiresAt)
	return token
}

// ValidateSession 验证会话
func ValidateSession(token string) bool {
	value, ok := panelSessions.Load(token)
	if !ok {
		return false
	}

	expiresAt := value.(time.Time)
	if time.Now().After(expiresAt) {
		panelSessions.Delete(token)
		return false
	}

	return true
}

// DeleteSession 删除会话
func DeleteSession(token string) {
	panelSessions.Delete(token)
}

// SetSessionCookie 设置会话 Cookie
func SetSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "panel_session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
}

// ClearSessionCookie 清除会话 Cookie
func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "panel_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// GetSessionToken 从请求中获取会话 Token
func GetSessionToken(r *http.Request) string {
	// 从 cookie 获取
	cookie, err := r.Cookie("panel_session")
	if err == nil && cookie.Value != "" {
		return cookie.Value
	}

	// 从 header 获取
	return r.Header.Get("X-Session-Token")
}

func generateSecureToken(length int) string {
	bytes := make([]byte, length)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
