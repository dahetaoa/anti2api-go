package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// WriteJSON 写入 JSON 响应
func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// WriteError 写入错误响应
func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, map[string]interface{}{
		"error": map[string]interface{}{
			"message": message,
			"type":    getErrorType(status),
		},
	})
}

func getErrorType(status int) string {
	switch {
	case status == 400:
		return "invalid_request_error"
	case status == 401:
		return "authentication_error"
	case status == 403:
		return "permission_error"
	case status == 404:
		return "not_found_error"
	case status == 429:
		return "rate_limit_error"
	default:
		return "server_error"
	}
}

// HandleHealthz 健康检查
func HandleHealthz(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

// HandleRoot 根路径处理
func HandleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/admin/", http.StatusFound)
}

// HandleAdminRedirect 管理面板重定向
func HandleAdminRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin/", http.StatusFound)
}

// HandleAdminPage 管理面板页面
func HandleAdminPage(w http.ResponseWriter, r *http.Request) {
	// 返回简单的管理面板 HTML
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, adminPageHTML)
}

const adminPageHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Antigravity2API - 管理面板</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #0f172a; color: #e2e8f0; min-height: 100vh; }
        .container { max-width: 1200px; margin: 0 auto; padding: 20px; }
        .header { display: flex; justify-content: space-between; align-items: center; padding: 20px 0; border-bottom: 1px solid #334155; margin-bottom: 30px; }
        .header h1 { font-size: 24px; color: #38bdf8; }
        .header .actions { display: flex; gap: 10px; }
        .btn { padding: 8px 16px; border-radius: 6px; border: none; cursor: pointer; font-size: 14px; transition: all 0.2s; }
        .btn-primary { background: #3b82f6; color: white; }
        .btn-primary:hover { background: #2563eb; }
        .btn-danger { background: #ef4444; color: white; }
        .btn-danger:hover { background: #dc2626; }
        .btn-secondary { background: #475569; color: white; }
        .btn-secondary:hover { background: #64748b; }
        .card { background: #1e293b; border-radius: 12px; padding: 20px; margin-bottom: 20px; }
        .card-title { font-size: 18px; margin-bottom: 15px; color: #94a3b8; }
        .stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 20px; margin-bottom: 30px; }
        .stat-card { background: #1e293b; border-radius: 12px; padding: 20px; text-align: center; }
        .stat-value { font-size: 36px; font-weight: bold; color: #38bdf8; }
        .stat-label { color: #94a3b8; margin-top: 5px; }
        .accounts-list { display: flex; flex-direction: column; gap: 10px; }
        .account-item { display: flex; justify-content: space-between; align-items: center; padding: 15px; background: #0f172a; border-radius: 8px; }
        .account-info { flex: 1; }
        .account-email { font-weight: 500; }
        .account-project { color: #64748b; font-size: 12px; margin-top: 4px; }
        .account-status { display: flex; align-items: center; gap: 8px; }
        .status-badge { padding: 4px 8px; border-radius: 4px; font-size: 12px; }
        .status-enabled { background: #065f46; color: #34d399; }
        .status-disabled { background: #7f1d1d; color: #fca5a5; }
        .endpoint-select { display: flex; gap: 10px; flex-wrap: wrap; }
        .endpoint-option { padding: 10px 20px; border: 2px solid #334155; border-radius: 8px; cursor: pointer; transition: all 0.2s; }
        .endpoint-option:hover { border-color: #3b82f6; }
        .endpoint-option.active { border-color: #3b82f6; background: rgba(59, 130, 246, 0.1); }
        .input-group { margin-bottom: 15px; }
        .input-group label { display: block; margin-bottom: 5px; color: #94a3b8; }
        .input-group input, .input-group textarea { width: 100%; padding: 10px; border: 1px solid #334155; border-radius: 6px; background: #0f172a; color: #e2e8f0; font-family: inherit; }
        .input-group textarea { min-height: 150px; resize: vertical; }
        .modal { display: none; position: fixed; top: 0; left: 0; width: 100%; height: 100%; background: rgba(0,0,0,0.5); justify-content: center; align-items: center; z-index: 1000; }
        .modal.show { display: flex; }
        .modal-content { background: #1e293b; border-radius: 12px; padding: 30px; max-width: 500px; width: 90%; }
        .modal-title { font-size: 20px; margin-bottom: 20px; }
        .modal-actions { display: flex; gap: 10px; justify-content: flex-end; margin-top: 20px; }
        .loading { display: inline-block; width: 20px; height: 20px; border: 2px solid #334155; border-radius: 50%; border-top-color: #3b82f6; animation: spin 1s linear infinite; }
        @keyframes spin { to { transform: rotate(360deg); } }
        .empty { text-align: center; padding: 40px; color: #64748b; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🚀 Antigravity2API</h1>
            <div class="actions">
                <button class="btn btn-primary" onclick="showAddAccount()">添加账号</button>
                <button class="btn btn-secondary" onclick="refreshAll()">刷新全部</button>
                <button class="btn btn-danger" onclick="logout()">退出登录</button>
            </div>
        </div>

        <div class="stats">
            <div class="stat-card">
                <div class="stat-value" id="totalAccounts">0</div>
                <div class="stat-label">总账号数</div>
            </div>
            <div class="stat-card">
                <div class="stat-value" id="enabledAccounts">0</div>
                <div class="stat-label">已启用</div>
            </div>
            <div class="stat-card">
                <div class="stat-value" id="currentEndpoint">-</div>
                <div class="stat-label">当前端点</div>
            </div>
        </div>

        <div class="card">
            <div class="card-title">端点模式</div>
            <div class="endpoint-select" id="endpointSelect"></div>
        </div>

        <div class="card">
            <div class="card-title">账号列表</div>
            <div class="accounts-list" id="accountsList">
                <div class="empty">暂无账号，请添加</div>
            </div>
        </div>
    </div>

    <div class="modal" id="addAccountModal">
        <div class="modal-content">
            <div class="modal-title">添加账号</div>
            <div class="input-group">
                <label>TOML 格式导入</label>
                <textarea id="tomlInput" placeholder="粘贴 TOML 格式的账号数据..."></textarea>
            </div>
            <div class="modal-actions">
                <button class="btn btn-secondary" onclick="closeModal()">取消</button>
                <button class="btn btn-primary" onclick="importTOML()">导入</button>
            </div>
        </div>
    </div>

    <script>
        const API_BASE = '';

        async function fetchAPI(path, options = {}) {
            const resp = await fetch(API_BASE + path, {
                ...options,
                headers: { 'Content-Type': 'application/json', ...options.headers }
            });
            if (resp.status === 401) {
                window.location.href = '/admin/login';
                return null;
            }
            return resp.json();
        }

        async function loadAccounts() {
            const data = await fetchAPI('/auth/accounts');
            if (!data) return;

            document.getElementById('totalAccounts').textContent = data.length;
            document.getElementById('enabledAccounts').textContent = data.filter(a => a.enable).length;

            const list = document.getElementById('accountsList');
            if (data.length === 0) {
                list.innerHTML = '<div class="empty">暂无账号，请添加</div>';
                return;
            }

            list.innerHTML = data.map((acc, i) => ` + "`" + `
                <div class="account-item">
                    <div class="account-info">
                        <div class="account-email">${acc.email || '未知邮箱'}</div>
                        <div class="account-project">${acc.projectId || '无项目ID'}</div>
                    </div>
                    <div class="account-status">
                        <span class="status-badge ${acc.enable ? 'status-enabled' : 'status-disabled'}">
                            ${acc.enable ? '已启用' : '已禁用'}
                        </span>
                        <button class="btn btn-secondary" onclick="toggleAccount(${i}, ${!acc.enable})">
                            ${acc.enable ? '禁用' : '启用'}
                        </button>
                        <button class="btn btn-secondary" onclick="refreshAccount(${i})">刷新</button>
                        <button class="btn btn-danger" onclick="deleteAccount(${i})">删除</button>
                    </div>
                </div>
            ` + "`" + `).join('');
        }

        async function loadEndpoints() {
            const data = await fetchAPI('/admin/api/endpoints');
            if (!data) return;

            const modes = ['daily', 'autopush', 'production', 'round-robin', 'round-robin-dp'];
            const labels = { 'daily': 'Daily', 'autopush': 'Autopush', 'production': 'Production', 'round-robin': '轮询(全部)', 'round-robin-dp': '轮询(D+P)' };

            document.getElementById('currentEndpoint').textContent = labels[data.mode] || data.mode;

            const select = document.getElementById('endpointSelect');
            select.innerHTML = modes.map(mode => ` + "`" + `
                <div class="endpoint-option ${data.mode === mode ? 'active' : ''}" onclick="setEndpointMode('${mode}')">
                    ${labels[mode]}
                </div>
            ` + "`" + `).join('');
        }

        async function setEndpointMode(mode) {
            await fetchAPI('/admin/api/endpoints/mode', {
                method: 'POST',
                body: JSON.stringify({ mode })
            });
            loadEndpoints();
        }

        async function toggleAccount(index, enable) {
            await fetchAPI(` + "`" + `/auth/accounts/${index}/enable` + "`" + `, {
                method: 'POST',
                body: JSON.stringify({ enable })
            });
            loadAccounts();
        }

        async function refreshAccount(index) {
            await fetchAPI(` + "`" + `/auth/accounts/${index}/refresh` + "`" + `, { method: 'POST' });
            loadAccounts();
        }

        async function deleteAccount(index) {
            if (!confirm('确定删除此账号？')) return;
            await fetchAPI(` + "`" + `/auth/accounts/${index}` + "`" + `, { method: 'DELETE' });
            loadAccounts();
        }

        async function refreshAll() {
            await fetchAPI('/auth/accounts/refresh-all', { method: 'POST' });
            loadAccounts();
        }

        async function importTOML() {
            const toml = document.getElementById('tomlInput').value;
            if (!toml.trim()) return;

            await fetchAPI('/auth/accounts/import-toml', {
                method: 'POST',
                body: JSON.stringify({ toml })
            });
            closeModal();
            loadAccounts();
        }

        function showAddAccount() {
            document.getElementById('addAccountModal').classList.add('show');
        }

        function closeModal() {
            document.getElementById('addAccountModal').classList.remove('show');
            document.getElementById('tomlInput').value = '';
        }

        async function logout() {
            await fetch('/admin/logout', { method: 'POST' });
            window.location.href = '/admin/login';
        }

        // 初始化
        loadAccounts();
        loadEndpoints();
    </script>
</body>
</html>`
