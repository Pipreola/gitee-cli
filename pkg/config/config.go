// Package config 负责 gitee-cli 的配置文件读写与凭证管理。
// 配置文件默认存放于 ~/.config/gitee-cli/config.yaml。
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	// configDirName 是配置目录相对于用户配置根目录的名称。
	configDirName = "gitee-cli"
	// configFileName 是配置文件名。
	configFileName = "config.yaml"
	// envConfigDir 允许通过环境变量覆盖配置目录，便于测试与自定义部署。
	envConfigDir = "GITEE_CLI_CONFIG_DIR"
)

// Config 表示 gitee-cli 的持久化配置。
type Config struct {
	// Host 是 Gitee API 的基础地址，默认 https://gitee.com/api/v5。
	Host string `yaml:"host"`
	// Token 是访问 Gitee OpenAPI 的私人令牌（personal access token）。
	Token string `yaml:"token"`
	// User 是当前登录用户的登录名，用于展示与校验。
	User string `yaml:"user,omitempty"`
}

// DefaultHost 是 Gitee OpenAPI v5 的默认基础地址。
const DefaultHost = "https://gitee.com/api/v5"

// Dir 返回配置目录的绝对路径。
// 优先读取环境变量 GITEE_CLI_CONFIG_DIR；否则使用 os.UserConfigDir() 下的 gitee-cli 目录。
func Dir() (string, error) {
	if custom := os.Getenv(envConfigDir); custom != "" {
		return custom, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("无法定位用户配置目录: %w", err)
	}
	return filepath.Join(base, configDirName), nil
}

// Path 返回配置文件的绝对路径。
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configFileName), nil
}

// Load 从磁盘读取配置。
// 当配置文件不存在时，返回一个带默认值的空配置，而非错误，方便首次运行。
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{Host: DefaultHost}, nil
		}
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}
	if cfg.Host == "" {
		cfg.Host = DefaultHost
	}
	return &cfg, nil
}

// Save 将配置写入磁盘。
// 自动创建配置目录，并以 0600 权限写入文件以保护令牌等敏感信息。
func (c *Config) Save() error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	path := filepath.Join(dir, configFileName)
	// 通过“临时文件 + 原子 rename”写入，避免覆盖已存在文件时保留其原有的宽松权限。
	// os.WriteFile 仅在创建新文件时应用传入的权限位，若 config.yaml 已是 0644/0666，
	// 直接覆盖写入会沿用旧权限，导致私人令牌可能被同机其他用户读取。
	tmp, err := os.CreateTemp(dir, configFileName+".tmp-*")
	if err != nil {
		return fmt.Errorf("创建临时配置文件失败: %w", err)
	}
	tmpPath := tmp.Name()
	// 任意失败路径都清理临时文件，避免残留。
	defer func() { _ = os.Remove(tmpPath) }()

	// 显式将临时文件权限收敛到 0600，CreateTemp 默认即为 0600，这里再确保一次。
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("设置配置文件权限失败: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("写入配置文件失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭临时配置文件失败: %w", err)
	}

	// 原子替换目标文件；rename 后新文件继承临时文件的 0600 权限，覆盖旧文件的宽松权限。
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}
	// 再次显式收敛权限，防御目标路径已存在且 rename 在个别平台保留旧权限的边界情况。
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("设置配置文件权限失败: %w", err)
	}
	return nil
}
