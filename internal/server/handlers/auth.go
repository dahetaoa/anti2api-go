package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"anti2api-golang/internal/auth"
	"anti2api-golang/internal/config"
	"anti2api-golang/internal/store"
)

// HandleLoginPage login page
func HandleLoginPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, loginPageHTML)
}

// HandleLogin login handler
func HandleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	cfg := config.Get()
	if req.Username != cfg.PanelUser || req.Password != cfg.PanelPassword {
		WriteError(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	token := auth.CreateSession()
	auth.SetSessionCookie(w, token)

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"token":   token,
	})
}

// HandleLogout logout handler
func HandleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("panel_session")
	if err == nil {
		auth.DeleteSession(cookie.Value)
	}
	auth.ClearSessionCookie(w)

	WriteJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// HandleGetOAuthURL get oauth url
func HandleGetOAuthURL(w http.ResponseWriter, r *http.Request) {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	redirectURI := fmt.Sprintf("%s://%s/oauth-callback", scheme, r.Host)

	authURL := auth.BuildAuthURL(redirectURI, "state")

	WriteJSON(w, http.StatusOK, map[string]string{
		"url": authURL,
	})
}

// HandleOAuthCallback oauth callback handler
// 不自动交换token，而是显示页面让用户复制URL
func HandleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	fullURL := r.URL.String()
	if r.URL.Host == "" {
		scheme := "http"
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		fullURL = fmt.Sprintf("%s://%s%s", scheme, r.Host, r.URL.RequestURI())
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if code == "" {
		fmt.Fprint(w, `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>授权失败</title>
<style>body{font-family:sans-serif;max-width:600px;margin:50px auto;padding:20px;background:#1e293b;color:#e2e8f0;}
h1{color:#ef4444;}.url-box{background:#0f172a;padding:15px;border-radius:8px;word-break:break-all;margin:20px 0;}
a{color:#3b82f6;}</style></head>
<body><h1>授权失败</h1><p>未收到授权码，请重新授权。</p>
<a href="/admin/">返回管理面板</a></body></html>`)
		return
	}

	// 显示复制URL页面
	fmt.Fprintf(w, `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>授权成功 - 请复制URL</title>
<style>
body{font-family:sans-serif;max-width:700px;margin:50px auto;padding:20px;background:#1e293b;color:#e2e8f0;}
h1{color:#22c55e;}
.url-box{background:#0f172a;padding:15px;border-radius:8px;word-break:break-all;margin:20px 0;border:1px solid #334155;}
.copy-btn{background:#3b82f6;color:white;border:none;padding:10px 20px;border-radius:6px;cursor:pointer;font-size:16px;margin-right:10px;}
.copy-btn:hover{background:#2563eb;}
.back-btn{background:#475569;color:white;border:none;padding:10px 20px;border-radius:6px;cursor:pointer;font-size:16px;text-decoration:none;}
.back-btn:hover{background:#64748b;}
.success{color:#22c55e;display:none;margin-left:10px;}
.instructions{background:#0f172a;padding:15px;border-radius:8px;margin:20px 0;border-left:4px solid #3b82f6;}
</style></head>
<body>
<h1>Google 授权成功</h1>
<div class="instructions">
<p><strong>请按以下步骤完成：</strong></p>
<ol>
<li>点击下方"复制 URL"按钮</li>
<li>返回管理面板的"授权"标签页</li>
<li>将复制的 URL 粘贴到输入框中</li>
<li>点击"提交回调 URL 并交换凭证"</li>
</ol>
</div>
<p><strong>回调 URL：</strong></p>
<div class="url-box" id="urlBox">%s</div>
<button class="copy-btn" onclick="copyUrl()">复制 URL</button>
<a class="back-btn" href="/admin/">返回管理面板</a>
<span class="success" id="successMsg">已复制!</span>
<script>
function copyUrl() {
  const url = document.getElementById('urlBox').textContent;
  navigator.clipboard.writeText(url).then(function() {
    document.getElementById('successMsg').style.display = 'inline';
    setTimeout(function() {
      document.getElementById('successMsg').style.display = 'none';
    }, 2000);
  });
}
</script>
</body></html>`, fullURL)
}

// HandleParseOAuthURL parse oauth url
func HandleParseOAuthURL(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	code, _, err := auth.ParseOAuthURL(req.URL)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	redirectURI := fmt.Sprintf("%s://%s/oauth-callback", scheme, r.Host)

	tokenResp, err := auth.ExchangeCodeForToken(code, redirectURI)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	userInfo, _ := auth.GetUserInfo(tokenResp.AccessToken)
	email := ""
	if userInfo != nil {
		email = userInfo.Email
	}

	account := store.Account{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresIn:    tokenResp.ExpiresIn,
		Email:        email,
		Enable:       true,
	}

	if err := store.GetAccountStore().Add(account); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]bool{"success": true})
}

const loginPageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Login - Antigravity2API</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #0f172a; color: #e2e8f0; min-height: 100vh; display: flex; justify-content: center; align-items: center; }
        .login-card { background: #1e293b; border-radius: 12px; padding: 40px; width: 100%; max-width: 400px; }
        .login-title { font-size: 24px; text-align: center; margin-bottom: 30px; color: #38bdf8; }
        .input-group { margin-bottom: 20px; }
        .input-group label { display: block; margin-bottom: 8px; color: #94a3b8; }
        .input-group input { width: 100%; padding: 12px; border: 1px solid #334155; border-radius: 8px; background: #0f172a; color: #e2e8f0; font-size: 16px; }
        .input-group input:focus { outline: none; border-color: #3b82f6; }
        .btn { width: 100%; padding: 12px; border: none; border-radius: 8px; background: #3b82f6; color: white; font-size: 16px; cursor: pointer; transition: background 0.2s; }
        .btn:hover { background: #2563eb; }
        .btn:disabled { background: #475569; cursor: not-allowed; }
        .error { color: #ef4444; text-align: center; margin-top: 15px; display: none; }
    </style>
</head>
<body>
    <div class="login-card">
        <h1 class="login-title">Antigravity2API</h1>
        <form id="loginForm">
            <div class="input-group">
                <label for="username">Username</label>
                <input type="text" id="username" name="username" required autocomplete="username">
            </div>
            <div class="input-group">
                <label for="password">Password</label>
                <input type="password" id="password" name="password" required autocomplete="current-password">
            </div>
            <button type="submit" class="btn" id="submitBtn">Login</button>
            <p class="error" id="errorMsg"></p>
        </form>
    </div>
    <script>
        document.getElementById('loginForm').addEventListener('submit', async (e) => {
            e.preventDefault();
            const btn = document.getElementById('submitBtn');
            const errorMsg = document.getElementById('errorMsg');
            btn.disabled = true;
            btn.textContent = 'Loading...';
            errorMsg.style.display = 'none';
            try {
                const resp = await fetch('/admin/login', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        username: document.getElementById('username').value,
                        password: document.getElementById('password').value
                    })
                });
                const data = await resp.json();
                if (data.success) {
                    window.location.href = '/admin/';
                } else {
                    errorMsg.textContent = data.error?.message || 'Login failed';
                    errorMsg.style.display = 'block';
                }
            } catch (err) {
                errorMsg.textContent = 'Network error';
                errorMsg.style.display = 'block';
            } finally {
                btn.disabled = false;
                btn.textContent = 'Login';
            }
        });
    </script>
</body>
</html>`
