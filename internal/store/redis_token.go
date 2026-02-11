package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/redis/go-redis/v9"
)

// Subscriber 表示一个订阅用户
type Subscriber struct {
	Username string `json:"username"`
	Token    string `json:"token"`
}

func generateRandomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// AddSubscriber 创建订阅用户并返回随机 token
func AddSubscriber(username string) (string, error) {
	if rdb == nil {
		return "", fmt.Errorf("redis not initialized")
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return "", fmt.Errorf("username is required")
	}

	exists, err := rdb.HExists(ctx, redisUserTokenKey, username).Result()
	if err != nil {
		return "", err
	}
	if exists {
		return "", fmt.Errorf("subscriber already exists")
	}

	// Retry until token is unique.
	for i := 0; i < 5; i++ {
		token, err := generateRandomToken()
		if err != nil {
			return "", err
		}

		tokenExists, err := rdb.HExists(ctx, redisTokenKey, token).Result()
		if err != nil {
			return "", err
		}
		if tokenExists {
			continue
		}

		pipe := rdb.Pipeline()
		pipe.HSet(ctx, redisTokenKey, token, username)
		pipe.HSet(ctx, redisUserTokenKey, username, token)
		if _, err := pipe.Exec(ctx); err != nil {
			return "", err
		}
		return token, nil
	}

	return "", fmt.Errorf("failed to generate unique token")
}

// ListSubscribers 获取所有订阅用户
func ListSubscribers() ([]Subscriber, error) {
	if rdb == nil {
		return nil, fmt.Errorf("redis not initialized")
	}

	m, err := rdb.HGetAll(ctx, redisUserTokenKey).Result()
	if err != nil {
		return nil, err
	}

	subscribers := make([]Subscriber, 0, len(m))
	for username, token := range m {
		subscribers = append(subscribers, Subscriber{
			Username: username,
			Token:    token,
		})
	}

	sort.Slice(subscribers, func(i, j int) bool {
		return subscribers[i].Username < subscribers[j].Username
	})

	return subscribers, nil
}

// ValidateAPIToken 验证订阅 token，返回用户名
func ValidateAPIToken(token string) (string, error) {
	if rdb == nil {
		return "", fmt.Errorf("redis not initialized")
	}
	username, err := rdb.HGet(ctx, redisTokenKey, token).Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return username, nil
}

// GetAPIToken 获取订阅用户 token
func GetAPIToken(username string) (string, error) {
	if rdb == nil {
		return "", fmt.Errorf("redis not initialized")
	}
	token, err := rdb.HGet(ctx, redisUserTokenKey, username).Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return token, nil
}
