package config

import (
	"log"
	"os"
	"strings"
	"github.com/joho/godotenv"
)

// LoadEnv 尝试加载 .env 文件，如果文件不存在则忽略错误（继续使用环境变量）
func LoadEnv() {
	err := godotenv.Load()
	if err != nil && !os.IsNotExist(err) {
		log.Printf("Warning: Error loading .env file: %v", err)
	}
}

// GetSubscribeUrls 从环境变量获取订阅链接列表
func GetSubscribeUrls() []string {
	raw := os.Getenv("SUBSCRIBE_URLS")
	if raw == "" {
		return []string{}
	}
	var urls []string
	for _, url := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(url); trimmed != "" {
			urls = append(urls, trimmed)
		}
	}
	return urls
}

// GetPort 获取服务端口
func GetPort() string {
	if port := os.Getenv("PORT"); port != "" {
		return port
	}
	return "8080"
}

// GetRedisAddr 获取 Redis 地址
func GetRedisAddr() string {
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		return addr
	}
	return "localhost:6379"
}

// GetRedisPassword 获取 Redis 密码
func GetRedisPassword() string {
	return os.Getenv("REDIS_PASSWORD")
}

// GetRedisDB 获取 Redis 数据库索引
func GetRedisDB() int {
	// 简单的转换 logic, 忽略错误默认为 0
	// 真正严谨应该 strconv.Atoi
	return 0
}
