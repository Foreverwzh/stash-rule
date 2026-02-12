package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"my-stash-rule/internal/service"
	"my-stash-rule/internal/store"
)

// HandleHealthCheck 健康检查
func HandleHealthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// HandleConfigAPI 处理配置 API (GET/POST)
func HandleConfigAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodGet {
		urls, err := store.GetStoredSubscribeUrls()
		if err != nil {
			http.Error(w, `{"error": "failed to get config"}`, http.StatusInternalServerError)
			log.Printf("Redis get error: %v", err)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"urls": urls})
		return
	}

	if r.Method == http.MethodPost {
		var req struct {
			Urls []string `json:"urls"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "invalid request"}`, http.StatusBadRequest)
			return
		}

		if err := store.SaveSubscribeUrls(req.Urls); err != nil {
			http.Error(w, `{"error": "failed to save config"}`, http.StatusInternalServerError)
			log.Printf("Redis save error: %v", err)
			return
		}

		w.Write([]byte(`{"status": "ok"}`))
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// HandleStashProfilesAPI 管理 Stash 模板配置
// GET: 列出模板
// POST: 创建模板
// PUT: 更新模板
func HandleStashProfilesAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		profiles, err := store.ListStashProfiles()
		if err != nil {
			http.Error(w, `{"error":"failed to load stash profiles"}`, http.StatusInternalServerError)
			return
		}

		dynamicPreviewBytes, err := service.GenerateConfig([]service.ProxyNode{})
		if err != nil {
			http.Error(w, `{"error":"failed to build dynamic preview"}`, http.StatusInternalServerError)
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"profiles":     profiles,
			"default_name": store.DefaultStashProfileName,
			"readonly_presets": []map[string]interface{}{
				{
					"name":         "__dynamic_default_preview__",
					"display_name": "默认动态配置（只读预览）",
					"content":      string(dynamicPreviewBytes),
					"is_readonly":  true,
				},
			},
		})
		return
	case http.MethodPost, http.MethodPut:
		var req struct {
			Name    string `json:"name"`
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
			return
		}

		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			http.Error(w, `{"error":"模板名称不能为空"}`, http.StatusBadRequest)
			return
		}

		var err error
		if r.Method == http.MethodPost {
			err = store.CreateStashProfile(req.Name, req.Content)
		} else {
			err = store.UpdateStashProfile(req.Name, req.Content)
		}
		if err != nil {
			statusCode := http.StatusBadRequest
			if strings.Contains(err.Error(), "already exists") {
				statusCode = http.StatusConflict
			}
			if strings.Contains(err.Error(), "not found") {
				statusCode = http.StatusNotFound
			}
			http.Error(w, `{"error":"`+err.Error()+`"}`, statusCode)
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "ok",
			"name":   req.Name,
		})
		return
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
}

// HandleProxyCacheAPI 管理订阅链接缓存。
// GET: 获取缓存状态
// POST: 手动刷新缓存
func HandleProxyCacheAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		status, err := service.GetProxyCacheStatus()
		if err != nil {
			http.Error(w, `{"error":"failed to load proxy cache status"}`, http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(status)
		return
	case http.MethodPost:
		result, err := service.RefreshProxyCacheFromStore()
		if err != nil {
			http.Error(w, `{"error":"failed to refresh proxy cache"}`, http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(result)
		return
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
}

// HandleGetConfig 生成 Stash 配置
func HandleGetConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username, isAdmin, err := ResolveConfigRequester(r)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if username == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get URLs from Redis
	urls, err := store.GetStoredSubscribeUrls()
	if err != nil {
		log.Printf("Failed to get URLs from Redis: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if len(urls) == 0 {
		w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "# Warning: 未配置订阅链接，请访问 /admin 进行配置\n")
		return
	}

	log.Printf("开始读取 %d 个订阅链接缓存...", len(urls))
	proxies, err := service.BuildProxiesFromCache(urls)
	if err != nil {
		log.Printf("Failed to load proxies from cache: %v", err)
		http.Error(w, "Failed to load proxies", http.StatusInternalServerError)
		return
	}

	defaultProfileMap, err := store.GetStashProfileMap(store.DefaultStashProfileName)
	if err != nil {
		log.Printf("Failed to load default stash profile: %v", err)
		http.Error(w, "Failed to load stash profile", http.StatusInternalServerError)
		return
	}

	selectedProfileName := store.DefaultStashProfileName
	if !isAdmin {
		selectedProfileName, err = store.GetSubscriberProfile(username)
		if err != nil {
			log.Printf("Failed to load subscriber profile for %s: %v", username, err)
			http.Error(w, "Failed to load subscriber profile", http.StatusInternalServerError)
			return
		}
	}

	overlays := []map[string]interface{}{defaultProfileMap}
	if selectedProfileName != store.DefaultStashProfileName {
		selectedProfileMap, err := store.GetStashProfileMap(selectedProfileName)
		if err != nil {
			log.Printf("Failed to load stash profile %s: %v", selectedProfileName, err)
			http.Error(w, "Failed to load stash profile", http.StatusInternalServerError)
			return
		}
		overlays = append(overlays, selectedProfileMap)
	}

	log.Printf("共获取 %d 个代理节点，开始生成配置（用户: %s, 模板: %s）...", len(proxies), username, selectedProfileName)
	configBytes, err := service.GenerateConfig(proxies, overlays...)
	if err != nil {
		log.Printf("Failed to generate config: %v", err)
		http.Error(w, "Failed to generate config", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	w.Write(configBytes)
}
