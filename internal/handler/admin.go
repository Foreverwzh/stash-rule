package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"text/template"

	"my-stash-rule/internal/store"
)

type adminPageData struct {
	ActivePage string
	PageTitle  string
}

func renderAdminPage(w http.ResponseWriter, page, title string) {
	tmpl, err := template.ParseFS(TemplatesFS, "templates/admin.html")
	if err != nil {
		http.Error(w, "Failed to load template", http.StatusInternalServerError)
		log.Printf("Template error: %v", err)
		return
	}
	_ = tmpl.Execute(w, adminPageData{
		ActivePage: page,
		PageTitle:  title,
	})
}

// HandleAdminPage 入口，跳转到默认模块页面
func HandleAdminPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	http.Redirect(w, r, "/admin/config", http.StatusFound)
}

// HandleAdminConfigPage 订阅链接配置页
func HandleAdminConfigPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	renderAdminPage(w, "config", "订阅链接配置")
}

// HandleAdminProfilesPage 模板管理页
func HandleAdminProfilesPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	renderAdminPage(w, "profiles", "Stash 模板管理")
}

// HandleAdminSubscribersPage 订阅用户管理页
func HandleAdminSubscribersPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	renderAdminPage(w, "subscribers", "订阅用户管理")
}

// HandleAdminAccountPage 管理员账号设置页
func HandleAdminAccountPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	renderAdminPage(w, "account", "管理员账号设置")
}

// HandleAdminProfileAPI 管理员资料接口
// GET: 获取当前管理员用户名
// POST: 更新管理员用户名/密码
func HandleAdminProfileAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodGet {
		username, err := store.GetAdminUsername()
		if err != nil {
			http.Error(w, `{"error":"failed to load admin profile"}`, http.StatusInternalServerError)
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]string{
			"username": username,
		})
		return
	}

	if r.Method == http.MethodPost {
		var req struct {
			CurrentPassword string `json:"current_password"`
			NewUsername     string `json:"new_username"`
			NewPassword     string `json:"new_password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
			return
		}

		oldUsername, err := store.GetAdminUsername()
		if err != nil {
			http.Error(w, `{"error":"failed to load admin profile"}`, http.StatusInternalServerError)
			return
		}

		newUsername := strings.TrimSpace(req.NewUsername)
		if newUsername == "" {
			http.Error(w, `{"error":"新用户名不能为空"}`, http.StatusBadRequest)
			return
		}

		if err := store.UpdateAdminCredentials(req.CurrentPassword, newUsername, req.NewPassword); err != nil {
			statusCode := http.StatusBadRequest
			if strings.Contains(err.Error(), "incorrect") {
				statusCode = http.StatusUnauthorized
			}
			http.Error(w, `{"error":"`+err.Error()+`"}`, statusCode)
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":           "ok",
			"username":         newUsername,
			"relogin_required": oldUsername != "" && oldUsername != newUsername,
		})
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}
