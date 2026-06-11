package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"gitee-cli/pkg/config"
)

// setupTestConfigDir 创建临时配置目录用于测试。
func setupTestConfigDir(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	t.Setenv("GITEE_CLI_CONFIG_DIR", tmpDir)
	return tmpDir
}

func TestLogin_Success(t *testing.T) {
	setupTestConfigDir(t)

	// 模拟 Gitee API 服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"id": 123,
				"login": "testuser",
				"name": "Test User",
				"avatar_url": "https://example.com/avatar.png",
				"html_url": "https://gitee.com/testuser"
			}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	ctx := context.Background()
	user, err := Login(ctx, server.URL, "test-token")
	if err != nil {
		t.Fatalf("Login() 失败: %v", err)
	}

	if user.Login != "testuser" {
		t.Errorf("期望用户名 'testuser'，得到 '%s'", user.Login)
	}

	// 验证配置已保存
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if cfg.Token != "test-token" {
		t.Errorf("期望令牌 'test-token'，得到 '%s'", cfg.Token)
	}
	if cfg.User != "testuser" {
		t.Errorf("期望用户 'testuser'，得到 '%s'", cfg.User)
	}
}

func TestLogin_InvalidToken(t *testing.T) {
	setupTestConfigDir(t)

	// 模拟返回 401 的 API 服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message": "401 Unauthorized"}`))
	}))
	defer server.Close()

	ctx := context.Background()
	_, err := Login(ctx, server.URL, "invalid-token")
	if err == nil {
		t.Fatal("期望登录失败，但成功了")
	}
}

func TestLogin_EmptyToken(t *testing.T) {
	setupTestConfigDir(t)
	ctx := context.Background()
	_, err := Login(ctx, "http://example.com", "")
	if err == nil {
		t.Fatal("期望空令牌登录失败，但成功了")
	}
}

func TestStatus_NotLoggedIn(t *testing.T) {
	setupTestConfigDir(t)
	ctx := context.Background()
	_, loggedIn, err := Status(ctx)
	if err != nil {
		t.Fatalf("Status() 失败: %v", err)
	}
	if loggedIn {
		t.Error("期望未登录状态，但返回已登录")
	}
}

func TestStatus_LoggedIn(t *testing.T) {
	setupTestConfigDir(t)

	// 先登录
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": 123,
			"login": "testuser",
			"name": "Test User",
			"avatar_url": "https://example.com/avatar.png",
			"html_url": "https://gitee.com/testuser"
		}`))
	}))
	defer server.Close()

	ctx := context.Background()
	_, err := Login(ctx, server.URL, "test-token")
	if err != nil {
		t.Fatalf("Login() 失败: %v", err)
	}

	// 检查状态
	user, loggedIn, err := Status(ctx)
	if err != nil {
		t.Fatalf("Status() 失败: %v", err)
	}
	if !loggedIn {
		t.Error("期望已登录状态，但返回未登录")
	}
	if user.Login != "testuser" {
		t.Errorf("期望用户名 'testuser'，得到 '%s'", user.Login)
	}
}

func TestStatus_ExpiredToken(t *testing.T) {
	setupTestConfigDir(t)

	// 先保存一个配置（模拟之前登录过）
	cfg := &config.Config{
		Host:  "http://example.com",
		Token: "expired-token",
		User:  "olduser",
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("保存配置失败: %v", err)
	}

	// 模拟返回 401 的 API 服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message": "401 Unauthorized"}`))
	}))
	defer server.Close()

	// 更新配置的 host 为测试服务器
	cfg.Host = server.URL
	if err := cfg.Save(); err != nil {
		t.Fatalf("更新配置失败: %v", err)
	}

	ctx := context.Background()
	_, _, err := Status(ctx)
	if err == nil {
		t.Fatal("期望令牌过期错误，但成功了")
	}
}

func TestLogout(t *testing.T) {
	tmpDir := setupTestConfigDir(t)

	// 先保存一个配置（模拟登录状态）
	cfg := &config.Config{
		Host:  "http://example.com",
		Token: "test-token",
		User:  "testuser",
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("保存配置失败: %v", err)
	}

	// 登出
	if err := Logout(); err != nil {
		t.Fatalf("Logout() 失败: %v", err)
	}

	// 验证令牌已清除
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if cfg.Token != "" {
		t.Errorf("期望令牌为空，得到 '%s'", cfg.Token)
	}
	if cfg.User != "" {
		t.Errorf("期望用户为空，得到 '%s'", cfg.User)
	}

	// 验证配置文件仍然存在（只是内容被清空）
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		t.Error("期望配置文件存在，但被删除了")
	}
}
