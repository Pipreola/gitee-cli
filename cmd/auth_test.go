package cmd

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"gitee-cli/pkg/config"
)

// TestAuthLoginWithToken 测试使用 --token 参数的登录路径（绕过交互式输入）
func TestAuthLoginWithToken(t *testing.T) {
	// 设置临时配置目录
	tmpDir := t.TempDir()
	oldConfigDir := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer func() {
		if oldConfigDir != "" {
			os.Setenv("HOME", oldConfigDir)
		}
	}()

	// 创建 mock API server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Gitee API 使用查询参数 access_token 进行认证
		if r.URL.Path == "/user" && r.URL.Query().Get("access_token") == "test-valid-token" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id": 123, "login": "testuser", "name": "Test User", "html_url": "https://gitee.com/testuser"}`))
		} else {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message": "401 Unauthorized"}`))
		}
	}))
	defer server.Close()

	// 测试用例：使用 --token 参数成功登录
	t.Run("login with valid token flag", func(t *testing.T) {
		cmd := newAuthLoginCmd()
		out := &bytes.Buffer{}
		cmd.SetOut(out)
		cmd.SetErr(io.Discard)
		cmd.SetArgs([]string{"--token", "test-valid-token", "--host", server.URL})

		err := cmd.ExecuteContext(context.Background())
		if err != nil {
			t.Fatalf("auth login 应该成功，但返回错误: %v", err)
		}

		output := out.String()
		if !strings.Contains(output, "登录成功") {
			t.Errorf("输出应包含 '登录成功'，实际输出: %s", output)
		}
		if !strings.Contains(output, "testuser") {
			t.Errorf("输出应包含用户名 'testuser'，实际输出: %s", output)
		}

		// 验证配置文件已保存
		cfgPath, _ := config.Path()
		if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
			t.Errorf("配置文件应该存在: %s", cfgPath)
		}
	})

	// 测试用例：使用无效 token
	t.Run("login with invalid token", func(t *testing.T) {
		cmd := newAuthLoginCmd()
		out := &bytes.Buffer{}
		cmd.SetOut(out)
		cmd.SetErr(io.Discard)
		cmd.SetArgs([]string{"--token", "invalid-token", "--host", server.URL})

		err := cmd.ExecuteContext(context.Background())
		if err == nil {
			t.Fatal("无效 token 应该返回错误")
		}
	})

	// 测试用例：空 token
	t.Run("login with empty token flag", func(t *testing.T) {
		cmd := newAuthLoginCmd()
		out := &bytes.Buffer{}
		cmd.SetOut(out)
		cmd.SetErr(io.Discard)
		cmd.SetArgs([]string{"--token", "", "--host", server.URL})

		err := cmd.ExecuteContext(context.Background())
		if err == nil {
			t.Fatal("空 token 应该返回错误")
		}
	})
}

// TestAuthLoginInteractiveUsesSecureInput 验证交互式路径不使用普通 stdin 读取
// 注意：此测试无法在非 TTY 环境真实调用 term.ReadPassword，
// 但可以验证当未提供 --token 且在非 TTY 环境时会返回明确的错误提示
func TestAuthLoginInteractiveRequiresTTY(t *testing.T) {
	// 设置临时配置目录
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// 创建 mock API server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id": 123, "login": "testuser", "name": "Test User", "html_url": "https://gitee.com/testuser"}`))
	}))
	defer server.Close()

	cmd := newAuthLoginCmd()
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(io.Discard)
	// 不设置 --token 参数，让它尝试交互式读取
	cmd.SetArgs([]string{"--host", server.URL})

	// 在单元测试环境（非 TTY）中，term.ReadPassword 会失败
	err := cmd.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("在非 TTY 环境下交互式登录应该失败，提示用户使用 --token 参数")
	}

	// 验证错误消息包含明确的提示
	errMsg := err.Error()
	if !strings.Contains(errMsg, "读取密码失败") && !strings.Contains(errMsg, "--token") {
		t.Errorf("错误消息应该提示使用 --token 参数，实际: %s", errMsg)
	}
}

// TestAuthStatus 测试认证状态查询
func TestAuthStatus(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// 创建 mock API server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id": 123, "login": "testuser", "name": "Test User", "html_url": "https://gitee.com/testuser"}`))
		}
	}))
	defer server.Close()

	// 先登录
	loginCmd := newAuthLoginCmd()
	loginCmd.SetOut(io.Discard)
	loginCmd.SetErr(io.Discard)
	loginCmd.SetArgs([]string{"--token", "test-token", "--host", server.URL})
	if err := loginCmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("登录失败: %v", err)
	}

	// 测试 status 命令
	t.Run("status when logged in", func(t *testing.T) {
		// 需要将 host 配置更新为 mock server
		cfg, _ := config.Load()
		cfg.Host = server.URL
		_ = cfg.Save()

		cmd := newAuthStatusCmd()
		out := &bytes.Buffer{}
		cmd.SetOut(out)
		cmd.SetErr(io.Discard)

		err := cmd.ExecuteContext(context.Background())
		if err != nil {
			t.Fatalf("status 命令失败: %v", err)
		}

		output := out.String()
		if !strings.Contains(output, "已登录") {
			t.Errorf("输出应包含 '已登录'，实际: %s", output)
		}
	})

	t.Run("status when not logged in", func(t *testing.T) {
		// 清空配置
		cfgPath, _ := config.Path()
		os.Remove(cfgPath)

		cmd := newAuthStatusCmd()
		out := &bytes.Buffer{}
		cmd.SetOut(out)
		cmd.SetErr(io.Discard)

		_ = cmd.ExecuteContext(context.Background())
		output := out.String()
		if !strings.Contains(output, "未登录") {
			t.Errorf("输出应包含 '未登录'，实际: %s", output)
		}
	})
}

// TestAuthLogout 测试登出功能
func TestAuthLogout(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// 创建一个假的配置文件
	cfg := &config.Config{
		Host:  config.DefaultHost,
		Token: "fake-token",
		User:  "testuser",
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("保存配置失败: %v", err)
	}

	cmd := newAuthLogoutCmd()
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(io.Discard)

	err := cmd.ExecuteContext(context.Background())
	if err != nil {
		t.Fatalf("logout 失败: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "已登出") {
		t.Errorf("输出应包含 '已登出'，实际: %s", output)
	}

	// 验证配置文件已删除或清空
	cfg, _ = config.Load()
	if cfg.Token != "" {
		t.Error("logout 后 token 应该被清空")
	}
}
