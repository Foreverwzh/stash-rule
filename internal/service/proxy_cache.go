package service

import (
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"my-stash-rule/internal/model"
	"my-stash-rule/internal/store"
)

const (
	proxyCacheRefreshInterval = 24 * time.Hour
	proxyCacheSchedulerTick   = time.Hour
)

var proxyCacheRefreshMu sync.Mutex

// ProxyCacheRefreshItem 表示单个订阅链接刷新结果。
type ProxyCacheRefreshItem struct {
	URL       string `json:"url"`
	Count     int    `json:"count"`
	UpdatedAt int64  `json:"updated_at"`
	Error     string `json:"error,omitempty"`
}

// ProxyCacheRefreshResult 表示一次刷新任务的整体结果。
type ProxyCacheRefreshResult struct {
	Total       int                     `json:"total"`
	Success     int                     `json:"success"`
	Failed      int                     `json:"failed"`
	RefreshedAt int64                   `json:"refreshed_at"`
	Items       []ProxyCacheRefreshItem `json:"items"`
}

// ProxyCacheStatusResult 表示当前缓存状态信息。
type ProxyCacheStatusResult struct {
	LastRunAt int64                    `json:"last_run_at"`
	Statuses  []store.ProxyCacheStatus `json:"statuses"`
}

func normalizeSubscribeURLs(urls []string) []string {
	seen := make(map[string]struct{}, len(urls))
	out := make([]string, 0, len(urls))
	for _, raw := range urls {
		url := strings.TrimSpace(raw)
		if url == "" {
			continue
		}
		if _, exists := seen[url]; exists {
			continue
		}
		seen[url] = struct{}{}
		out = append(out, url)
	}
	return out
}

// BuildProxiesFromCache 返回按订阅链接合并后的节点列表。
// 仅在某个链接没有缓存时才触发远程拉取并回写缓存。
func BuildProxiesFromCache(urls []string) ([]model.ProxyNode, error) {
	normalizedURLs := normalizeSubscribeURLs(urls)
	if len(normalizedURLs) == 0 {
		return []model.ProxyNode{}, nil
	}

	_, missing, err := store.GetProxyCachesByURLs(normalizedURLs)
	if err != nil {
		return nil, err
	}

	if len(missing) > 0 {
		if _, refreshErr := RefreshProxyCache(missing); refreshErr != nil {
			log.Printf("Failed to refresh missing proxy cache: %v", refreshErr)
		}
	}

	all := make([]model.ProxyNode, 0)
	for _, url := range normalizedURLs {
		nodes, _, found, err := store.GetProxyCache(url)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		all = append(all, nodes...)
	}

	return all, nil
}

// RefreshProxyCache 按订阅链接刷新缓存（每个链接独立存储）。
func RefreshProxyCache(urls []string) (ProxyCacheRefreshResult, error) {
	proxyCacheRefreshMu.Lock()
	defer proxyCacheRefreshMu.Unlock()

	normalizedURLs := normalizeSubscribeURLs(urls)
	refreshedAt := time.Now()
	result := ProxyCacheRefreshResult{
		Total:       len(normalizedURLs),
		RefreshedAt: refreshedAt.Unix(),
		Items:       make([]ProxyCacheRefreshItem, 0, len(normalizedURLs)),
	}
	if len(normalizedURLs) == 0 {
		return result, nil
	}

	client := &http.Client{Timeout: 30 * time.Second}
	type workerResult struct {
		item ProxyCacheRefreshItem
	}

	ch := make(chan workerResult, len(normalizedURLs))
	var wg sync.WaitGroup

	for _, targetURL := range normalizedURLs {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()

			item := ProxyCacheRefreshItem{
				URL:       url,
				Count:     0,
				UpdatedAt: refreshedAt.Unix(),
			}

			proxies, err := fetchSingleURL(client, url)
			if err != nil {
				item.Error = err.Error()
				ch <- workerResult{item: item}
				return
			}

			if err := store.SaveProxyCache(url, proxies, refreshedAt); err != nil {
				item.Error = err.Error()
				ch <- workerResult{item: item}
				return
			}

			item.Count = len(proxies)
			ch <- workerResult{item: item}
		}(targetURL)
	}

	wg.Wait()
	close(ch)

	itemsByURL := make(map[string]ProxyCacheRefreshItem, len(normalizedURLs))
	for r := range ch {
		itemsByURL[r.item.URL] = r.item
	}

	for _, url := range normalizedURLs {
		item := itemsByURL[url]
		result.Items = append(result.Items, item)
		if item.Error != "" {
			result.Failed++
		} else {
			result.Success++
		}
	}

	if err := store.SetProxyCacheLastRunAt(refreshedAt); err != nil {
		return result, err
	}

	return result, nil
}

// RefreshProxyCacheFromStore 刷新当前配置中的全部订阅链接缓存。
func RefreshProxyCacheFromStore() (ProxyCacheRefreshResult, error) {
	urls, err := store.GetStoredSubscribeUrls()
	if err != nil {
		return ProxyCacheRefreshResult{}, err
	}
	return RefreshProxyCache(urls)
}

// GetProxyCacheStatus 获取缓存状态（含最近一次刷新时间）。
func GetProxyCacheStatus() (ProxyCacheStatusResult, error) {
	urls, err := store.GetStoredSubscribeUrls()
	if err != nil {
		return ProxyCacheStatusResult{}, err
	}

	statuses, err := store.ListProxyCacheStatus(normalizeSubscribeURLs(urls))
	if err != nil {
		return ProxyCacheStatusResult{}, err
	}

	lastRunAt, err := store.GetProxyCacheLastRunAt()
	if err != nil {
		return ProxyCacheStatusResult{}, err
	}

	return ProxyCacheStatusResult{
		LastRunAt: lastRunAt,
		Statuses:  statuses,
	}, nil
}

func shouldRunDailyRefresh(now time.Time) (bool, error) {
	urls, err := store.GetStoredSubscribeUrls()
	if err != nil {
		return false, err
	}
	if len(normalizeSubscribeURLs(urls)) == 0 {
		return false, nil
	}

	lastRunAt, err := store.GetProxyCacheLastRunAt()
	if err != nil {
		return false, err
	}
	if lastRunAt <= 0 {
		return true, nil
	}
	lastRunTime := time.Unix(lastRunAt, 0)
	return now.Sub(lastRunTime) >= proxyCacheRefreshInterval, nil
}

// StartDailyProxyCacheScheduler 后台启动每日缓存刷新。
func StartDailyProxyCacheScheduler() {
	runRefresh := func(source string) {
		result, err := RefreshProxyCacheFromStore()
		if err != nil {
			log.Printf("Proxy cache refresh failed (%s): %v", source, err)
			return
		}
		if result.Total == 0 {
			log.Printf("Proxy cache refresh skipped (%s): no subscribe urls", source)
			return
		}
		log.Printf("Proxy cache refreshed (%s): success=%d failed=%d total=%d", source, result.Success, result.Failed, result.Total)
	}

	go func() {
		ok, err := shouldRunDailyRefresh(time.Now())
		if err != nil {
			log.Printf("Failed to check proxy cache refresh time: %v", err)
		} else if ok {
			runRefresh("startup")
		}

		ticker := time.NewTicker(proxyCacheSchedulerTick)
		defer ticker.Stop()

		for now := range ticker.C {
			ok, err := shouldRunDailyRefresh(now)
			if err != nil {
				log.Printf("Failed to check proxy cache refresh time: %v", err)
				continue
			}
			if ok {
				runRefresh("daily")
			}
		}
	}()
}
