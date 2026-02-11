package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"my-stash-rule/internal/store"
)

// HandleGetUserInfo 获取用户信息和 Token
func HandleGetUserInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username := ""
	token := r.URL.Query().Get("token")

	// 订阅 token 直连：返回订阅用户信息
	if token != "" {
		var err error
		username, err = store.ValidateAPIToken(token)
		if err != nil || username == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// 管理员 session：仅返回管理员用户名
	if username == "" {
		cookie, err := r.Cookie("session_token")
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		sessionUsername, err := store.ValidateSession(cookie.Value)
		if err != nil || sessionUsername == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		adminUsername, err := store.GetAdminUsername()
		if err != nil || adminUsername == "" || adminUsername != sessionUsername {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		username = adminUsername
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"username": username,
		"token":    token,
	})
}

// HandleSubscribersAPI 订阅用户管理接口
// GET: 获取订阅用户列表
// POST: 新增订阅用户（自动生成随机 token）
func HandleSubscribersAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodGet {
		subscribers, err := store.ListSubscribers()
		if err != nil {
			http.Error(w, `{"error":"failed to load subscribers"}`, http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"subscribers": subscribers,
		})
		return
	}

	if r.Method == http.MethodPost {
		var req struct {
			Username string `json:"username"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
			return
		}

		username := strings.TrimSpace(req.Username)
		if username == "" {
			http.Error(w, `{"error":"订阅用户名不能为空"}`, http.StatusBadRequest)
			return
		}

		token, err := store.AddSubscriber(username)
		if err != nil {
			status := http.StatusBadRequest
			if strings.Contains(err.Error(), "already exists") {
				status = http.StatusConflict
			}
			http.Error(w, `{"error":"`+err.Error()+`"}`, status)
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":   "ok",
			"username": username,
			"token":    token,
		})
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}
