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
	Username    string `json:"username"`
	Token       string `json:"token"`
	ProfileName string `json:"profile_name"`
}

func generateRandomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// AddSubscriber 创建订阅用户并返回随机 token
func AddSubscriber(username, profileName string) (string, error) {
	if rdb == nil {
		return "", fmt.Errorf("redis not initialized")
	}

	username = strings.TrimSpace(username)
	profileName = normalizeProfileName(profileName)
	if username == "" {
		return "", fmt.Errorf("username is required")
	}

	profileExists, err := ValidateStashProfileExists(profileName)
	if err != nil {
		return "", err
	}
	if !profileExists {
		return "", fmt.Errorf("stash profile not found")
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
		pipe.HSet(ctx, redisUserProfileKey, username, profileName)
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

	tokenMap, err := rdb.HGetAll(ctx, redisUserTokenKey).Result()
	if err != nil {
		return nil, err
	}
	profileMap, err := rdb.HGetAll(ctx, redisUserProfileKey).Result()
	if err != nil {
		return nil, err
	}

	subscribers := make([]Subscriber, 0, len(tokenMap))
	for username, token := range tokenMap {
		profileName := normalizeProfileName(profileMap[username])
		subscribers = append(subscribers, Subscriber{
			Username:    username,
			Token:       token,
			ProfileName: profileName,
		})
	}

	sort.Slice(subscribers, func(i, j int) bool {
		return subscribers[i].Username < subscribers[j].Username
	})

	return subscribers, nil
}

// GetSubscriberProfile 获取订阅用户绑定的模板名。
func GetSubscriberProfile(username string) (string, error) {
	if rdb == nil {
		return "", fmt.Errorf("redis not initialized")
	}

	profileName, err := rdb.HGet(ctx, redisUserProfileKey, username).Result()
	if err == redis.Nil {
		return DefaultStashProfileName, nil
	}
	if err != nil {
		return "", err
	}

	if strings.TrimSpace(profileName) == "" {
		return DefaultStashProfileName, nil
	}
	return profileName, nil
}

// UpdateSubscriberProfile 更新订阅用户绑定的模板名。
func UpdateSubscriberProfile(username, profileName string) error {
	if rdb == nil {
		return fmt.Errorf("redis not initialized")
	}

	username = strings.TrimSpace(username)
	profileName = normalizeProfileName(profileName)
	if username == "" {
		return fmt.Errorf("username is required")
	}

	userExists, err := rdb.HExists(ctx, redisUserTokenKey, username).Result()
	if err != nil {
		return err
	}
	if !userExists {
		return fmt.Errorf("subscriber not found")
	}

	profileExists, err := ValidateStashProfileExists(profileName)
	if err != nil {
		return err
	}
	if !profileExists {
		return fmt.Errorf("stash profile not found")
	}

	return rdb.HSet(ctx, redisUserProfileKey, username, profileName).Err()
}

// DeleteSubscriber 删除订阅用户及其 token/profile 绑定。
func DeleteSubscriber(username string) error {
	if rdb == nil {
		return fmt.Errorf("redis not initialized")
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return fmt.Errorf("username is required")
	}

	token, err := rdb.HGet(ctx, redisUserTokenKey, username).Result()
	if err == redis.Nil {
		return fmt.Errorf("subscriber not found")
	}
	if err != nil {
		return err
	}

	pipe := rdb.Pipeline()
	pipe.HDel(ctx, redisUserTokenKey, username)
	pipe.HDel(ctx, redisUserProfileKey, username)
	if token != "" {
		pipe.HDel(ctx, redisTokenKey, token)
	}

	_, err = pipe.Exec(ctx)
	return err
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
