package store

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"my-stash-rule/internal/model"
)

const (
	redisProxyCacheDataKey    = "stash-rule:proxy_cache:data"       // url -> []ProxyNode(json)
	redisProxyCacheUpdatedKey = "stash-rule:proxy_cache:updated_at" // url -> unix timestamp
	redisProxyCacheLastRunKey = "stash-rule:proxy_cache:last_run"   // unix timestamp
)

// ProxyCacheStatus 表示单个订阅链接的缓存状态。
type ProxyCacheStatus struct {
	URL       string `json:"url"`
	Count     int    `json:"count"`
	UpdatedAt int64  `json:"updated_at"`
}

func normalizeCacheURL(u string) string {
	return strings.TrimSpace(u)
}

// SaveProxyCache 保存单个订阅链接的节点缓存。
func SaveProxyCache(url string, proxies []model.ProxyNode, updatedAt time.Time) error {
	if rdb == nil {
		return fmt.Errorf("redis not initialized")
	}

	url = normalizeCacheURL(url)
	if url == "" {
		return fmt.Errorf("url is required")
	}

	encoded, err := json.Marshal(proxies)
	if err != nil {
		return err
	}

	pipe := rdb.Pipeline()
	pipe.HSet(ctx, redisProxyCacheDataKey, url, string(encoded))
	pipe.HSet(ctx, redisProxyCacheUpdatedKey, url, updatedAt.Unix())
	_, err = pipe.Exec(ctx)
	return err
}

// GetProxyCache 读取单个订阅链接缓存。
// found=false 表示无缓存。
func GetProxyCache(url string) (proxies []model.ProxyNode, updatedAt int64, found bool, err error) {
	if rdb == nil {
		return nil, 0, false, fmt.Errorf("redis not initialized")
	}

	url = normalizeCacheURL(url)
	if url == "" {
		return nil, 0, false, fmt.Errorf("url is required")
	}

	raw, err := rdb.HGet(ctx, redisProxyCacheDataKey, url).Result()
	if err == redis.Nil {
		return nil, 0, false, nil
	}
	if err != nil {
		return nil, 0, false, err
	}

	var parsed []model.ProxyNode
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, 0, false, err
	}

	updatedRaw, err := rdb.HGet(ctx, redisProxyCacheUpdatedKey, url).Result()
	if err != nil && err != redis.Nil {
		return nil, 0, false, err
	}
	if err == redis.Nil {
		updatedRaw = "0"
	}
	updatedAt, _ = strconv.ParseInt(updatedRaw, 10, 64)

	return parsed, updatedAt, true, nil
}

// GetProxyCachesByURLs 批量读取缓存，并返回缺失链接列表。
func GetProxyCachesByURLs(urls []string) (map[string][]model.ProxyNode, []string, error) {
	cache := make(map[string][]model.ProxyNode, len(urls))
	missing := make([]string, 0)

	for _, u := range urls {
		url := normalizeCacheURL(u)
		if url == "" {
			continue
		}

		proxies, _, found, err := GetProxyCache(url)
		if err != nil {
			return nil, nil, err
		}
		if !found {
			missing = append(missing, url)
			continue
		}
		cache[url] = proxies
	}

	return cache, missing, nil
}

// ListProxyCacheStatus 返回给定链接的缓存状态。
func ListProxyCacheStatus(urls []string) ([]ProxyCacheStatus, error) {
	statuses := make([]ProxyCacheStatus, 0, len(urls))
	for _, u := range urls {
		url := normalizeCacheURL(u)
		if url == "" {
			continue
		}
		proxies, updatedAt, found, err := GetProxyCache(url)
		if err != nil {
			return nil, err
		}
		if !found {
			statuses = append(statuses, ProxyCacheStatus{
				URL:       url,
				Count:     0,
				UpdatedAt: 0,
			})
			continue
		}
		statuses = append(statuses, ProxyCacheStatus{
			URL:       url,
			Count:     len(proxies),
			UpdatedAt: updatedAt,
		})
	}
	return statuses, nil
}

// SetProxyCacheLastRunAt 设置最近一次全量刷新时间。
func SetProxyCacheLastRunAt(t time.Time) error {
	if rdb == nil {
		return fmt.Errorf("redis not initialized")
	}
	return rdb.Set(ctx, redisProxyCacheLastRunKey, strconv.FormatInt(t.Unix(), 10), 0).Err()
}

// GetProxyCacheLastRunAt 获取最近一次全量刷新时间（unix）。
func GetProxyCacheLastRunAt() (int64, error) {
	if rdb == nil {
		return 0, fmt.Errorf("redis not initialized")
	}
	raw, err := rdb.Get(ctx, redisProxyCacheLastRunKey).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	ts, _ := strconv.ParseInt(raw, 10, 64)
	return ts, nil
}

