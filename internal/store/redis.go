package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"my-stash-rule/internal/config"
)

var (
	ctx                = context.Background()
	rdb                *redis.Client
	redisKey           = "stash-rule:subscribe-urls"
	redisAdminKey      = "stash-rule:admin"
	redisTokenKey      = "stash-rule:subscriber_tokens"      // token -> username
	redisUserTokenKey  = "stash-rule:subscriber_user_tokens" // username -> token
	redisSessionPrefix = "stash-rule:session:"
)

// InitRedis 初始化 Redis 连接
func InitRedis() error {
	addr := config.GetRedisAddr()
	password := config.GetRedisPassword()
	db := config.GetRedisDB()

	rdb = redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		return fmt.Errorf("redis connection failed: %v", err)
	}

	log.Printf("Connected to Redis at %s (DB %d)", addr, db)

	// Initialize default admin
	if err := InitDefaultAdmin(); err != nil {
		log.Printf("Warning: failed to init default admin: %v", err)
	}

	return nil
}

// InitDefaultAdmin 初始化默认管理员 admin/admin
func InitDefaultAdmin() error {
	exists, err := rdb.HExists(ctx, redisAdminKey, "username").Result()
	if err != nil {
		return err
	}

	passwordExists, err := rdb.HExists(ctx, redisAdminKey, "password_hash").Result()
	if err != nil {
		return err
	}

	// Only initialize defaults when no admin config exists.
	if exists && passwordExists {
		return nil
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	if err := rdb.HSet(ctx, redisAdminKey,
		"username", "admin",
		"password_hash", string(hashedPassword),
	).Err(); err != nil {
		return err
	}

	log.Println("Created default admin account: admin")
	return nil
}

// AuthenticateAdmin 验证管理员账号
func AuthenticateAdmin(username, password string) (bool, error) {
	if rdb == nil {
		return false, fmt.Errorf("redis not initialized")
	}

	storedUsername, err := GetAdminUsername()
	if err != nil {
		return false, err
	}
	if storedUsername == "" || username != storedUsername {
		return false, nil
	}

	hashedPassword, err := rdb.HGet(ctx, redisAdminKey, "password_hash").Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	err = bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	return err == nil, nil
}

// GetAdminUsername 获取当前管理员用户名
func GetAdminUsername() (string, error) {
	if rdb == nil {
		return "", fmt.Errorf("redis not initialized")
	}

	username, err := rdb.HGet(ctx, redisAdminKey, "username").Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return username, nil
}

// UpdateAdminCredentials 更新管理员用户名和密码。
// currentPassword 必填用于校验；newPassword 为空时保持原密码不变。
func UpdateAdminCredentials(currentPassword, newUsername, newPassword string) error {
	if rdb == nil {
		return fmt.Errorf("redis not initialized")
	}

	currentPassword = strings.TrimSpace(currentPassword)
	newUsername = strings.TrimSpace(newUsername)
	if currentPassword == "" {
		return fmt.Errorf("current password is required")
	}
	if newUsername == "" {
		return fmt.Errorf("new username is required")
	}

	storedUsername, err := GetAdminUsername()
	if err != nil {
		return err
	}
	hashedPassword, err := rdb.HGet(ctx, redisAdminKey, "password_hash").Result()
	if err == redis.Nil {
		return fmt.Errorf("admin not initialized")
	}
	if err != nil {
		return err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(currentPassword)); err != nil {
		return fmt.Errorf("current password is incorrect")
	}

	newHashedPassword := hashedPassword
	if strings.TrimSpace(newPassword) != "" {
		encoded, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		newHashedPassword = string(encoded)
	}

	// Update admin profile.
	pipe := rdb.Pipeline()
	pipe.HSet(ctx, redisAdminKey,
		"username", newUsername,
		"password_hash", newHashedPassword,
	)

	// If username changed, clear all sessions of old admin in a best-effort way by not
	// reusing old username in middleware checks. No scan/delete needed here.
	if storedUsername != "" && storedUsername != newUsername {
		log.Printf("Admin username changed: %s -> %s", storedUsername, newUsername)
	}

	_, err = pipe.Exec(ctx)
	return err
}

// CreateSession 创建 Session 并返回 Token
func CreateSession(username string) (string, error) {
	if rdb == nil {
		return "", fmt.Errorf("redis not initialized")
	}

	// Generate random token
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)

	key := redisSessionPrefix + token
	err := rdb.Set(ctx, key, username, 24*time.Hour).Err()
	if err != nil {
		return "", err
	}

	return token, nil
}

// ValidateSession 验证 Session，返回用户名
func ValidateSession(token string) (string, error) {
	if rdb == nil {
		return "", fmt.Errorf("redis not initialized")
	}

	key := redisSessionPrefix + token
	username, err := rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil // Invalid session
	}
	if err != nil {
		return "", err
	}
	return username, nil
}

// GetStoredSubscribeUrls 从 Redis 获取订阅链接
func GetStoredSubscribeUrls() ([]string, error) {
	if rdb == nil {
		return nil, fmt.Errorf("redis not initialized")
	}

	val, err := rdb.Get(ctx, redisKey).Result()
	if err == redis.Nil {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}

	var urls []string
	if err := json.Unmarshal([]byte(val), &urls); err != nil {
		return nil, err
	}
	return urls, nil
}

// SaveSubscribeUrls 保存订阅链接到 Redis
func SaveSubscribeUrls(urls []string) error {
	if rdb == nil {
		return fmt.Errorf("redis not initialized")
	}

	data, err := json.Marshal(urls)
	if err != nil {
		return err
	}

	return rdb.Set(ctx, redisKey, data, 0).Err()
}
