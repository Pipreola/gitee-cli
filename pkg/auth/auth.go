// Package auth 提供认证相关的辅助逻辑，桥接配置存储与 API 客户端。
package auth

import (
	"context"
	"fmt"

	"gitee-cli/pkg/api"
	"gitee-cli/pkg/config"
)

// Login 使用给定令牌验证身份，验证成功后将凭证持久化到配置文件。
// 返回已认证的用户信息。
func Login(ctx context.Context, host, token string) (*api.User, error) {
	if token == "" {
		return nil, fmt.Errorf("令牌不能为空")
	}
	if host == "" {
		host = config.DefaultHost
	}

	client := api.NewClient(host, token)
	user, err := client.GetCurrentUser(ctx)
	if err != nil {
		return nil, fmt.Errorf("令牌校验失败: %w", err)
	}

	cfg := &config.Config{
		Host:  host,
		Token: token,
		User:  user.Login,
	}
	if err := cfg.Save(); err != nil {
		return nil, fmt.Errorf("保存凭证失败: %w", err)
	}
	return user, nil
}

// Status 读取本地配置并验证令牌，返回当前登录用户的详细信息。
// 未登录时返回 nil 与 false。
func Status(ctx context.Context) (*api.User, bool, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, false, err
	}
	if cfg.Token == "" {
		return nil, false, nil
	}

	// 验证令牌是否仍然有效
	client := api.NewClient(cfg.Host, cfg.Token)
	user, err := client.GetCurrentUser(ctx)
	if err != nil {
		// 令牌可能已失效
		return nil, false, fmt.Errorf("令牌校验失败（可能已过期）: %w", err)
	}

	return user, true, nil
}

// Logout 清除本地配置文件中的凭证信息。
func Logout() error {
	cfg := &config.Config{
		Host:  config.DefaultHost,
		Token: "",
		User:  "",
	}
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("清除凭证失败: %w", err)
	}
	return nil
}
