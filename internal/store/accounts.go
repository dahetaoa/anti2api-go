package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"anti2api-golang/internal/config"
	"anti2api-golang/internal/logger"
	"anti2api-golang/internal/utils"
)

// Account 账号信息
type Account struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresIn    int       `json:"expires_in"`
	Timestamp    int64     `json:"timestamp"`
	ProjectID    string    `json:"projectId,omitempty"`
	Email        string    `json:"email,omitempty"`
	Enable       bool      `json:"enable"`
	CreatedAt    time.Time `json:"created_at"`
	SessionID    string    `json:"-"` // 运行时生成，不持久化
}

// AccountStore 账号存储
type AccountStore struct {
	mu           sync.RWMutex
	accounts     []Account
	currentIndex int
	filePath     string
}

var (
	accountStore     *AccountStore
	accountStoreOnce sync.Once
)

// GetAccountStore 获取账号存储单例
func GetAccountStore() *AccountStore {
	accountStoreOnce.Do(func() {
		cfg := config.Get()
		accountStore = &AccountStore{
			filePath: filepath.Join(cfg.DataDir, "accounts.json"),
		}
		accountStore.Load()
	})
	return accountStore
}

// Load 加载账号
func (s *AccountStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 确保目录存在
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			s.accounts = []Account{}
			return nil
		}
		return err
	}

	if err := json.Unmarshal(data, &s.accounts); err != nil {
		s.accounts = []Account{}
		return err
	}

	// 为每个账号生成 SessionID
	for i := range s.accounts {
		s.accounts[i].SessionID = utils.GenerateSessionID()
	}

	logger.Info("Loaded %d accounts", len(s.accounts))
	return nil
}

// Save 保存账号
func (s *AccountStore) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := json.MarshalIndent(s.accounts, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.filePath, data, 0644)
}

// IsExpired 检查 Token 是否过期（提前 5 分钟刷新）
func (a *Account) IsExpired() bool {
	if a.Timestamp == 0 || a.ExpiresIn == 0 {
		return true
	}
	expiresAt := a.Timestamp + int64(a.ExpiresIn*1000)
	return time.Now().UnixMilli() >= expiresAt-300000
}

// GetToken 获取可用 Token（轮询 + 自动刷新）
func (s *AccountStore) GetToken() (*Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.accounts) == 0 {
		return nil, errors.New("没有可用的账号")
	}

	for attempts := 0; attempts < len(s.accounts); attempts++ {
		account := &s.accounts[s.currentIndex]
		s.currentIndex = (s.currentIndex + 1) % len(s.accounts)

		if !account.Enable {
			continue
		}

		if account.IsExpired() {
			if err := s.refreshToken(account); err != nil {
				logger.Warn("Token refresh failed for %s: %v", account.Email, err)
				continue
			}
			s.saveUnlocked()
		}

		return account, nil
	}

	return nil, errors.New("没有可用的 token")
}

// GetTokenByProjectID 按 ProjectID 获取指定 Token
func (s *AccountStore) GetTokenByProjectID(projectID string) (*Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.accounts {
		account := &s.accounts[i]
		if account.ProjectID == projectID && account.Enable {
			if account.IsExpired() {
				if err := s.refreshToken(account); err != nil {
					return nil, err
				}
				s.saveUnlocked()
			}
			return account, nil
		}
	}

	return nil, errors.New("未找到指定的账号")
}

// GetTokenByEmail 按 Email 获取指定 Token
func (s *AccountStore) GetTokenByEmail(email string) (*Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.accounts {
		account := &s.accounts[i]
		if account.Email == email && account.Enable {
			if account.IsExpired() {
				if err := s.refreshToken(account); err != nil {
					return nil, err
				}
				s.saveUnlocked()
			}
			return account, nil
		}
	}

	return nil, errors.New("未找到指定的账号")
}

// refreshToken 刷新 Token（内部方法，需要已持有锁）
func (s *AccountStore) refreshToken(account *Account) error {
	// 这里调用 OAuth 刷新逻辑
	// 实际实现在 auth/oauth.go 中
	return refreshAccountToken(account)
}

// saveUnlocked 保存（内部方法，不加锁）
func (s *AccountStore) saveUnlocked() error {
	data, err := json.MarshalIndent(s.accounts, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0644)
}

// GetAll 获取所有账号
func (s *AccountStore) GetAll() []Account {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]Account, len(s.accounts))
	copy(result, s.accounts)
	return result
}

// Count 获取账号数量
func (s *AccountStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.accounts)
}

// EnabledCount 获取启用的账号数量
func (s *AccountStore) EnabledCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for _, a := range s.accounts {
		if a.Enable {
			count++
		}
	}
	return count
}

// Clear 清空所有账号
func (s *AccountStore) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.accounts = []Account{}
	s.currentIndex = 0
	return s.saveUnlocked()
}

// Add 添加账号
func (s *AccountStore) Add(account Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 生成 SessionID
	account.SessionID = utils.GenerateSessionID()

	// 设置创建时间
	if account.CreatedAt.IsZero() {
		account.CreatedAt = time.Now()
	}

	// 检查是否已存在（按 email 或 refresh_token）
	for i, a := range s.accounts {
		if (account.Email != "" && a.Email == account.Email) ||
			(account.RefreshToken != "" && a.RefreshToken == account.RefreshToken) {
			// 更新现有账号，保留创建时间
			account.CreatedAt = a.CreatedAt
			s.accounts[i] = account
			return s.saveUnlocked()
		}
	}

	s.accounts = append(s.accounts, account)
	return s.saveUnlocked()
}

// Delete 删除账号
func (s *AccountStore) Delete(index int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if index < 0 || index >= len(s.accounts) {
		return errors.New("索引超出范围")
	}

	s.accounts = append(s.accounts[:index], s.accounts[index+1:]...)

	// 调整当前索引
	if s.currentIndex >= len(s.accounts) {
		s.currentIndex = 0
	}

	return s.saveUnlocked()
}

// SetEnable 设置账号启用状态
func (s *AccountStore) SetEnable(index int, enable bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if index < 0 || index >= len(s.accounts) {
		return errors.New("索引超出范围")
	}

	s.accounts[index].Enable = enable
	return s.saveUnlocked()
}

// RefreshAccount 刷新指定账号的 Token
func (s *AccountStore) RefreshAccount(index int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if index < 0 || index >= len(s.accounts) {
		return errors.New("索引超出范围")
	}

	if err := s.refreshToken(&s.accounts[index]); err != nil {
		return err
	}

	return s.saveUnlocked()
}

// RefreshAll 刷新所有账号的 Token
func (s *AccountStore) RefreshAll() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	success := 0
	failed := 0

	for i := range s.accounts {
		if err := s.refreshToken(&s.accounts[i]); err != nil {
			failed++
			logger.Warn("Refresh failed for account %d: %v", i, err)
		} else {
			success++
		}
	}

	s.saveUnlocked()
	return success, failed
}

// ImportFromTOML 从 TOML 导入账号
func (s *AccountStore) ImportFromTOML(tomlData map[string]interface{}) (int, error) {
	accounts, ok := tomlData["accounts"].([]map[string]interface{})
	if !ok {
		return 0, errors.New("无效的 TOML 格式")
	}

	imported := 0
	for _, acc := range accounts {
		account := Account{
			Enable: true,
		}

		if v, ok := acc["access_token"].(string); ok {
			account.AccessToken = v
		}
		if v, ok := acc["refresh_token"].(string); ok {
			account.RefreshToken = v
		}
		if v, ok := acc["expires_in"].(int64); ok {
			account.ExpiresIn = int(v)
		} else if v, ok := acc["expires_in"].(float64); ok {
			account.ExpiresIn = int(v)
		}
		if v, ok := acc["timestamp"].(int64); ok {
			account.Timestamp = v
		} else if v, ok := acc["timestamp"].(float64); ok {
			account.Timestamp = int64(v)
		}
		if v, ok := acc["projectId"].(string); ok {
			account.ProjectID = v
		}
		if v, ok := acc["email"].(string); ok {
			account.Email = v
		}
		if v, ok := acc["enable"].(bool); ok {
			account.Enable = v
		}

		if account.RefreshToken != "" {
			if err := s.Add(account); err == nil {
				imported++
			}
		}
	}

	return imported, nil
}

// 占位函数，实际实现在 auth 包中
var refreshAccountToken = func(account *Account) error {
	return errors.New("token refresh not implemented")
}

// SetRefreshFunc 设置刷新函数
func SetRefreshFunc(fn func(*Account) error) {
	refreshAccountToken = fn
}
