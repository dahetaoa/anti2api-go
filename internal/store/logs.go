package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"anti2api-golang/internal/config"
)

// LogEntry 日志条目
type LogEntry struct {
	ID         string      `json:"id"`
	Timestamp  time.Time   `json:"timestamp"`
	Status     int         `json:"status"`
	Success    bool        `json:"success"`
	ProjectID  string      `json:"projectId"`
	Email      string      `json:"email,omitempty"`
	Model      string      `json:"model"`
	Method     string      `json:"method"`
	Path       string      `json:"path"`
	DurationMs int64       `json:"durationMs"`
	Message    string      `json:"message,omitempty"`
	HasDetail  bool        `json:"hasDetail"`
	Detail     *LogDetail  `json:"detail,omitempty"`
}

// LogDetail 日志详情
type LogDetail struct {
	Request  *RequestSnapshot  `json:"request,omitempty"`
	Response *ResponseSnapshot `json:"response,omitempty"`
}

// RequestSnapshot 请求快照
type RequestSnapshot struct {
	Headers map[string]string `json:"headers,omitempty"`
	Body    interface{}       `json:"body,omitempty"`
}

// ResponseSnapshot 响应快照
type ResponseSnapshot struct {
	StatusCode  int         `json:"statusCode,omitempty"`
	Body        interface{} `json:"body,omitempty"`
	ModelOutput string      `json:"modelOutput,omitempty"`
}

// UsageStats 用量统计
type UsageStats struct {
	ProjectID   string     `json:"projectId"`
	Email       string     `json:"email,omitempty"`
	Count       int        `json:"count"`
	Success     int        `json:"success"`
	Failed      int        `json:"failed"`
	LastUsedAt  *time.Time `json:"lastUsedAt,omitempty"`
	Models      []string   `json:"models,omitempty"`
}

// LogStore 日志存储
type LogStore struct {
	mu         sync.RWMutex
	logs       []LogEntry
	filePath   string
	maxLogs    int
	usageCache map[string]*UsageStats // 按 email 或 projectId 缓存用量
}

// getAccountKey 获取账号的唯一标识（优先 email，其次 projectId）
func getAccountKey(email, projectID string) string {
	if email != "" {
		return email
	}
	if projectID != "" {
		return projectID
	}
	return "unknown"
}

var (
	logStore     *LogStore
	logStoreOnce sync.Once
)

// GetLogStore 获取日志存储单例
func GetLogStore() *LogStore {
	logStoreOnce.Do(func() {
		cfg := config.Get()
		logStore = &LogStore{
			filePath:   filepath.Join(cfg.DataDir, "logs.json"),
			maxLogs:    1000, // 最多保存 1000 条日志
			usageCache: make(map[string]*UsageStats),
		}
		logStore.Load()
	})
	return logStore
}

// Load 加载日志
func (s *LogStore) Load() error {
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
			s.logs = []LogEntry{}
			return nil
		}
		return err
	}

	if err := json.Unmarshal(data, &s.logs); err != nil {
		s.logs = []LogEntry{}
		return err
	}

	// 重建用量缓存
	s.rebuildUsageCache()
	return nil
}

// Save 保存日志
func (s *LogStore) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.saveUnlocked()
}

func (s *LogStore) saveUnlocked() error {
	// 保存时不保存详情，减少文件大小
	logsWithoutDetail := make([]LogEntry, len(s.logs))
	for i, log := range s.logs {
		logsWithoutDetail[i] = log
		logsWithoutDetail[i].Detail = nil
	}

	data, err := json.MarshalIndent(logsWithoutDetail, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0644)
}

// Add 添加日志
func (s *LogStore) Add(entry LogEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 设置时间戳
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	// 设置 HasDetail
	entry.HasDetail = entry.Detail != nil

	// 添加到头部（最新的在前）
	s.logs = append([]LogEntry{entry}, s.logs...)

	// 限制数量
	if len(s.logs) > s.maxLogs {
		s.logs = s.logs[:s.maxLogs]
	}

	// 更新用量缓存
	s.updateUsageCache(&entry)

	// 异步保存
	go func() {
		s.mu.RLock()
		defer s.mu.RUnlock()
		s.saveUnlocked()
	}()
}

// GetAll 获取所有日志（不含详情）
func (s *LogStore) GetAll(limit int) []LogEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit > len(s.logs) {
		limit = len(s.logs)
	}

	result := make([]LogEntry, limit)
	for i := 0; i < limit; i++ {
		result[i] = s.logs[i]
		result[i].Detail = nil // 列表不返回详情
	}
	return result
}

// GetByID 按 ID 获取日志（含详情）
func (s *LogStore) GetByID(id string) *LogEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, log := range s.logs {
		if log.ID == id {
			return &log
		}
	}
	return nil
}

// GetUsageStats 获取用量统计
func (s *LogStore) GetUsageStats(windowMinutes int) []UsageStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cutoff := time.Now().Add(-time.Duration(windowMinutes) * time.Minute)

	// 统计窗口内的调用
	statsMap := make(map[string]*UsageStats)
	modelMap := make(map[string]map[string]bool)

	for _, log := range s.logs {
		if log.Timestamp.Before(cutoff) {
			continue
		}

		key := getAccountKey(log.Email, log.ProjectID)

		stats, ok := statsMap[key]
		if !ok {
			stats = &UsageStats{
				ProjectID: log.ProjectID,
				Email:     log.Email,
			}
			statsMap[key] = stats
			modelMap[key] = make(map[string]bool)
		}

		stats.Count++
		if log.Success {
			stats.Success++
		} else {
			stats.Failed++
		}

		if stats.LastUsedAt == nil || log.Timestamp.After(*stats.LastUsedAt) {
			t := log.Timestamp
			stats.LastUsedAt = &t
		}

		if log.Model != "" {
			modelMap[key][log.Model] = true
		}
	}

	// 转换为数组
	result := make([]UsageStats, 0, len(statsMap))
	for key, stats := range statsMap {
		// 添加模型列表
		models := make([]string, 0)
		for model := range modelMap[key] {
			models = append(models, model)
		}
		stats.Models = models
		result = append(result, *stats)
	}

	return result
}

// GetAccountUsage 获取指定账号的用量（全部时间）
func (s *LogStore) GetAccountUsage(projectID string) *UsageStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if stats, ok := s.usageCache[projectID]; ok {
		return stats
	}
	return nil
}

// GetAllAccountsUsage 获取所有账号的用量
func (s *LogStore) GetAllAccountsUsage() map[string]*UsageStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*UsageStats)
	for k, v := range s.usageCache {
		copied := *v
		result[k] = &copied
	}
	return result
}

// rebuildUsageCache 重建用量缓存
func (s *LogStore) rebuildUsageCache() {
	s.usageCache = make(map[string]*UsageStats)
	modelMap := make(map[string]map[string]bool)

	for _, log := range s.logs {
		key := getAccountKey(log.Email, log.ProjectID)
		if key == "unknown" {
			continue
		}

		stats, ok := s.usageCache[key]
		if !ok {
			stats = &UsageStats{
				ProjectID: log.ProjectID,
				Email:     log.Email,
			}
			s.usageCache[key] = stats
			modelMap[key] = make(map[string]bool)
		}

		stats.Count++
		if log.Success {
			stats.Success++
		} else {
			stats.Failed++
		}

		if stats.LastUsedAt == nil || log.Timestamp.After(*stats.LastUsedAt) {
			t := log.Timestamp
			stats.LastUsedAt = &t
		}

		if log.Model != "" {
			modelMap[key][log.Model] = true
		}
	}

	// 添加模型列表
	for key, stats := range s.usageCache {
		models := make([]string, 0)
		for model := range modelMap[key] {
			models = append(models, model)
		}
		stats.Models = models
	}
}

// updateUsageCache 更新用量缓存
func (s *LogStore) updateUsageCache(entry *LogEntry) {
	key := getAccountKey(entry.Email, entry.ProjectID)
	if key == "unknown" {
		return
	}

	stats, ok := s.usageCache[key]
	if !ok {
		stats = &UsageStats{
			ProjectID: entry.ProjectID,
			Email:     entry.Email,
			Models:    []string{},
		}
		s.usageCache[key] = stats
	}

	stats.Count++
	if entry.Success {
		stats.Success++
	} else {
		stats.Failed++
	}

	t := entry.Timestamp
	stats.LastUsedAt = &t

	// 添加模型（避免重复）
	if entry.Model != "" {
		found := false
		for _, m := range stats.Models {
			if m == entry.Model {
				found = true
				break
			}
		}
		if !found {
			stats.Models = append(stats.Models, entry.Model)
		}
	}
}

// Clear 清空日志
func (s *LogStore) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.logs = []LogEntry{}
	s.usageCache = make(map[string]*UsageStats)
	return s.saveUnlocked()
}
