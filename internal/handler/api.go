package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

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

// HandleGetConfig 生成 Stash 配置
func HandleGetConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	allowed, err := HasConfigAccess(r)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if !allowed {
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

	log.Printf("开始获取 %d 个订阅链接...", len(urls))
	proxies, err := service.FetchProxies(urls)
	if err != nil {
		log.Printf("Failed to fetch proxies: %v", err)
		http.Error(w, "Failed to fetch proxies", http.StatusBadGateway)
		return
	}

	log.Printf("共获取 %d 个代理节点，开始生成配置...", len(proxies))
	configBytes, err := service.GenerateConfig(proxies)
	if err != nil {
		log.Printf("Failed to generate config: %v", err)
		http.Error(w, "Failed to generate config", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	w.Write(configBytes)
}
