package auth

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"anti2api-golang/internal/config"
	"anti2api-golang/internal/logger"
	"anti2api-golang/internal/store"
)

// OAuthScopes OAuth 授权范围
var OAuthScopes = []string{
	"https://www.googleapis.com/auth/cloud-platform",
	"https://www.googleapis.com/auth/userinfo.email",
	"https://www.googleapis.com/auth/userinfo.profile",
	"https://www.googleapis.com/auth/cclog",
	"https://www.googleapis.com/auth/experimentsandconfigs",
}

// TokenResponse OAuth Token 响应
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
}

// UserInfo 用户信息
type UserInfo struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

func init() {
	// 注册刷新函数到 store
	store.SetRefreshFunc(RefreshToken)
}

// BuildAuthURL 构建授权 URL
func BuildAuthURL(redirectURI, state string) string {
	params := url.Values{
		"access_type":   {"offline"},
		"client_id":     {config.GetClientID()},
		"prompt":        {"consent"},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"scope":         {strings.Join(OAuthScopes, " ")},
		"state":         {state},
	}
	return "https://accounts.google.com/o/oauth2/v2/auth?" + params.Encode()
}

// ExchangeCodeForToken 用授权码交换 Token
func ExchangeCodeForToken(code, redirectURI string) (*TokenResponse, error) {
	data := url.Values{
		"code":          {code},
		"client_id":     {config.GetClientID()},
		"client_secret": {config.GetClientSecret()},
		"redirect_uri":  {redirectURI},
		"grant_type":    {"authorization_code"},
	}

	resp, err := http.PostForm("https://oauth2.googleapis.com/token", data)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, errors.New("token exchange failed: " + string(body))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, err
	}

	return &tokenResp, nil
}

// RefreshToken 刷新 Token
func RefreshToken(account *store.Account) error {
	if account.RefreshToken == "" {
		return errors.New("no refresh token")
	}

	data := url.Values{
		"client_id":     {config.GetClientID()},
		"client_secret": {config.GetClientSecret()},
		"grant_type":    {"refresh_token"},
		"refresh_token": {account.RefreshToken},
	}

	resp, err := http.PostForm("https://oauth2.googleapis.com/token", data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		logger.Warn("Token refresh failed: %s", string(body))
		return errors.New("token refresh failed")
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return err
	}

	// 更新账号信息
	account.AccessToken = tokenResp.AccessToken
	account.ExpiresIn = tokenResp.ExpiresIn
	account.Timestamp = time.Now().UnixMilli()

	// 如果返回了新的 refresh_token，也更新
	if tokenResp.RefreshToken != "" {
		account.RefreshToken = tokenResp.RefreshToken
	}

	logger.Info("Token refreshed for %s", account.Email)
	return nil
}

// GetUserInfo 获取用户信息
func GetUserInfo(accessToken string) (*UserInfo, error) {
	req, err := http.NewRequest("GET", "https://www.googleapis.com/oauth2/v2/userinfo", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, errors.New("failed to get user info")
	}

	var userInfo UserInfo
	if err := json.Unmarshal(body, &userInfo); err != nil {
		return nil, err
	}

	return &userInfo, nil
}

// ParseOAuthURL 解析 OAuth URL 中的参数（用于手动导入）
func ParseOAuthURL(oauthURL string) (code, state string, err error) {
	u, err := url.Parse(oauthURL)
	if err != nil {
		return "", "", err
	}

	query := u.Query()
	code = query.Get("code")
	state = query.Get("state")

	if code == "" {
		return "", "", errors.New("no code in URL")
	}

	return code, state, nil
}
