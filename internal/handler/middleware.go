package handler

import (
	"net/http"

	"my-stash-rule/internal/store"
)

func getSessionUsername(r *http.Request) (string, error) {
	cookie, err := r.Cookie("session_token")
	if err != nil {
		return "", nil
	}
	return store.ValidateSession(cookie.Value)
}

func isAdminUsername(username string) (bool, error) {
	adminUsername, err := store.GetAdminUsername()
	if err != nil {
		return false, err
	}
	return username != "" && adminUsername != "" && username == adminUsername, nil
}

// AdminAuthMiddleware 仅允许管理员 session 访问
func AdminAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username, err := getSessionUsername(r)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if username == "" {
			if r.Method == http.MethodGet {
				http.Redirect(w, r, "/login", http.StatusFound)
				return
			}
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		ok, err := isAdminUsername(username)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		if !ok {
			if r.Method == http.MethodGet {
				http.Redirect(w, r, "/login", http.StatusFound)
				return
			}
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

// HasConfigAccess 判断请求是否可以访问订阅配置：
// 1) query token 命中有效订阅用户；或
// 2) 已登录管理员 session。
func HasConfigAccess(r *http.Request) (bool, error) {
	username, _, err := ResolveConfigRequester(r)
	if err != nil {
		return false, err
	}
	return username != "", nil
}

// ResolveConfigRequester 解析配置请求方身份。
// 返回值: username, isAdmin, err
func ResolveConfigRequester(r *http.Request) (string, bool, error) {
	token := r.URL.Query().Get("token")
	if token != "" {
		username, err := store.ValidateAPIToken(token)
		if err != nil {
			return "", false, err
		}
		if username != "" {
			return username, false, nil
		}
	}

	username, err := getSessionUsername(r)
	if err != nil {
		return "", false, err
	}
	if username == "" {
		return "", false, nil
	}

	isAdmin, err := isAdminUsername(username)
	if err != nil {
		return "", false, err
	}
	if !isAdmin {
		return "", false, nil
	}
	return username, true, nil
}
