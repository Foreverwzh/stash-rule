package store

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/redis/go-redis/v9"
	"gopkg.in/yaml.v3"
)

const (
	// DefaultStashProfileName 默认配置模板名。
	DefaultStashProfileName = "default"
	defaultStashProfileYAML = "{}\n"
)

var ErrStashProfileNotFound = errors.New("stash profile not found")

// StashProfile 表示一个可编辑的 Stash 配置模板（YAML 内容）。
type StashProfile struct {
	Name      string `json:"name"`
	Content   string `json:"content"`
	IsDefault bool   `json:"is_default"`
}

func normalizeProfileName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return DefaultStashProfileName
	}
	return name
}

// DefaultStashProfileNameIfEmpty 返回非空模板名，空则返回默认模板名。
func DefaultStashProfileNameIfEmpty(name string) string {
	return normalizeProfileName(name)
}

func normalizeProfileContent(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return defaultStashProfileYAML
	}
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return content
}

// ParseStashProfileContent 解析配置模板 YAML，并确保根节点为 map。
func ParseStashProfileContent(content string) (map[string]interface{}, error) {
	content = normalizeProfileContent(content)

	var parsed map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &parsed); err != nil {
		return nil, fmt.Errorf("invalid yaml: %w", err)
	}
	if parsed == nil {
		parsed = map[string]interface{}{}
	}
	return parsed, nil
}

// InitDefaultStashProfile 初始化默认配置模板。
func InitDefaultStashProfile() error {
	if rdb == nil {
		return fmt.Errorf("redis not initialized")
	}

	exists, err := rdb.HExists(ctx, redisProfileKey, DefaultStashProfileName).Result()
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	if err := rdb.HSet(ctx, redisProfileKey, DefaultStashProfileName, defaultStashProfileYAML).Err(); err != nil {
		return err
	}

	return nil
}

// ListStashProfiles 获取全部模板列表。
func ListStashProfiles() ([]StashProfile, error) {
	if rdb == nil {
		return nil, fmt.Errorf("redis not initialized")
	}

	if err := InitDefaultStashProfile(); err != nil {
		return nil, err
	}

	m, err := rdb.HGetAll(ctx, redisProfileKey).Result()
	if err != nil {
		return nil, err
	}

	profiles := make([]StashProfile, 0, len(m))
	for name, content := range m {
		profiles = append(profiles, StashProfile{
			Name:      name,
			Content:   content,
			IsDefault: name == DefaultStashProfileName,
		})
	}

	sort.Slice(profiles, func(i, j int) bool {
		if profiles[i].IsDefault != profiles[j].IsDefault {
			return profiles[i].IsDefault
		}
		return profiles[i].Name < profiles[j].Name
	})

	return profiles, nil
}

// ValidateStashProfileExists 检查模板是否存在。
func ValidateStashProfileExists(name string) (bool, error) {
	if rdb == nil {
		return false, fmt.Errorf("redis not initialized")
	}

	name = normalizeProfileName(name)
	return rdb.HExists(ctx, redisProfileKey, name).Result()
}

// GetStashProfileYAML 获取模板原始 YAML。
func GetStashProfileYAML(name string) (string, error) {
	if rdb == nil {
		return "", fmt.Errorf("redis not initialized")
	}

	name = normalizeProfileName(name)
	content, err := rdb.HGet(ctx, redisProfileKey, name).Result()
	if err == redis.Nil {
		return "", ErrStashProfileNotFound
	}
	if err != nil {
		return "", err
	}
	return content, nil
}

// GetStashProfileMap 获取模板并解析为 map。
func GetStashProfileMap(name string) (map[string]interface{}, error) {
	content, err := GetStashProfileYAML(name)
	if err != nil {
		return nil, err
	}
	return ParseStashProfileContent(content)
}

// CreateStashProfile 创建模板（不存在时）。
func CreateStashProfile(name, content string) error {
	if rdb == nil {
		return fmt.Errorf("redis not initialized")
	}

	name = normalizeProfileName(name)
	if name == "" {
		return fmt.Errorf("profile name is required")
	}
	if _, err := ParseStashProfileContent(content); err != nil {
		return err
	}

	exists, err := rdb.HExists(ctx, redisProfileKey, name).Result()
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("stash profile already exists")
	}

	return rdb.HSet(ctx, redisProfileKey, name, normalizeProfileContent(content)).Err()
}

// UpdateStashProfile 更新模板（存在时）。
func UpdateStashProfile(name, content string) error {
	if rdb == nil {
		return fmt.Errorf("redis not initialized")
	}

	name = normalizeProfileName(name)
	if name == "" {
		return fmt.Errorf("profile name is required")
	}
	if _, err := ParseStashProfileContent(content); err != nil {
		return err
	}

	exists, err := rdb.HExists(ctx, redisProfileKey, name).Result()
	if err != nil {
		return err
	}
	if !exists {
		return ErrStashProfileNotFound
	}

	return rdb.HSet(ctx, redisProfileKey, name, normalizeProfileContent(content)).Err()
}
