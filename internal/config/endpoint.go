package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// EndpointManager 端点管理器
type EndpointManager struct {
	mu                sync.Mutex
	mode              string
	roundRobinIndex   int
	roundRobinDpIndex int
	settingsPath      string
}

// Settings 持久化设置
type Settings struct {
	EndpointMode    string    `json:"endpointMode"`
	CurrentEndpoint string    `json:"currentEndpoint"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

var (
	endpointMgr     *EndpointManager
	endpointMgrOnce sync.Once
)

// GetEndpointManager 获取端点管理器单例
func GetEndpointManager() *EndpointManager {
	endpointMgrOnce.Do(func() {
		cfg := Get()
		endpointMgr = &EndpointManager{
			mode:         cfg.EndpointMode,
			settingsPath: filepath.Join(cfg.DataDir, "settings.json"),
		}
		endpointMgr.loadSettings()
	})
	return endpointMgr
}

// loadSettings 加载持久化设置
func (m *EndpointManager) loadSettings() {
	data, err := os.ReadFile(m.settingsPath)
	if err != nil {
		return
	}

	var settings Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		return
	}

	// 环境变量优先级高于持久化设置
	if os.Getenv("ENDPOINT_MODE") == "" && settings.EndpointMode != "" {
		m.mode = settings.EndpointMode
	}
}

// saveSettings 保存设置
func (m *EndpointManager) saveSettings() error {
	settings := Settings{
		EndpointMode:    m.mode,
		CurrentEndpoint: m.getCurrentEndpointKey(),
		UpdatedAt:       time.Now(),
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	// 确保目录存在
	dir := filepath.Dir(m.settingsPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(m.settingsPath, data, 0644)
}

func (m *EndpointManager) getCurrentEndpointKey() string {
	switch m.mode {
	case "round-robin":
		idx := m.roundRobinIndex
		if idx < 0 {
			idx = 0
		}
		return RoundRobinEndpoints[idx%len(RoundRobinEndpoints)]
	case "round-robin-dp":
		idx := m.roundRobinDpIndex
		if idx < 0 {
			idx = 0
		}
		return RoundRobinDpEndpoints[idx%len(RoundRobinDpEndpoints)]
	default:
		return m.mode
	}
}

// GetActiveEndpoint 获取当前活动端点
func (m *EndpointManager) GetActiveEndpoint() Endpoint {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch m.mode {
	case "round-robin":
		key := RoundRobinEndpoints[m.roundRobinIndex]
		m.roundRobinIndex = (m.roundRobinIndex + 1) % len(RoundRobinEndpoints)
		return APIEndpoints[key]
	case "round-robin-dp":
		key := RoundRobinDpEndpoints[m.roundRobinDpIndex]
		m.roundRobinDpIndex = (m.roundRobinDpIndex + 1) % len(RoundRobinDpEndpoints)
		return APIEndpoints[key]
	default:
		if ep, ok := APIEndpoints[m.mode]; ok {
			return ep
		}
		return APIEndpoints["daily"]
	}
}

// GetMode 获取当前模式
func (m *EndpointManager) GetMode() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.mode
}

// SetMode 设置端点模式
func (m *EndpointManager) SetMode(mode string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 验证模式
	validModes := map[string]bool{
		"daily": true, "autopush": true, "production": true,
		"round-robin": true, "round-robin-dp": true,
	}
	if !validModes[mode] {
		return nil // 忽略无效模式
	}

	m.mode = mode
	return m.saveSettings()
}

// GetAllEndpoints 获取所有端点信息
func (m *EndpointManager) GetAllEndpoints() map[string]Endpoint {
	return APIEndpoints
}
