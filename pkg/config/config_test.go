package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadMissingReturnsDefault 验证配置不存在时返回带默认 Host 的空配置。
func TestLoadMissingReturnsDefault(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envConfigDir, dir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load 返回错误: %v", err)
	}
	if cfg.Host != DefaultHost {
		t.Errorf("Host = %q, 期望默认值 %q", cfg.Host, DefaultHost)
	}
	if cfg.Token != "" {
		t.Errorf("Token = %q, 期望为空", cfg.Token)
	}
}

// TestSaveAndLoadRoundTrip 验证保存后再读取能得到一致的配置。
func TestSaveAndLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envConfigDir, dir)

	want := &Config{Host: "https://example.com/api/v5", Token: "secret-token", User: "alice"}
	if err := want.Save(); err != nil {
		t.Fatalf("Save 返回错误: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load 返回错误: %v", err)
	}
	if got.Host != want.Host || got.Token != want.Token || got.User != want.User {
		t.Errorf("读回配置 = %+v, 期望 %+v", got, want)
	}
}

// TestSaveFilePermission 验证配置文件以 0600 权限写入，保护敏感令牌。
func TestSaveFilePermission(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envConfigDir, dir)

	cfg := &Config{Host: DefaultHost, Token: "t"}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save 返回错误: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, configFileName))
	if err != nil {
		t.Fatalf("Stat 返回错误: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("文件权限 = %o, 期望 0600", perm)
	}
}

// TestSaveOverExistingWidePermsConvergesTo0600 验证当目标文件已存在且权限较宽（0644）时，
// 保存后权限会被收敛到 0600，防止已有的宽松权限保留导致令牌泄露。
func TestSaveOverExistingWidePermsConvergesTo0600(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envConfigDir, dir)

	path := filepath.Join(dir, configFileName)
	// 预置一个 0644 的旧配置文件，模拟历史遗留的宽松权限。
	if err := os.WriteFile(path, []byte("host: https://old.example.com\n"), 0o644); err != nil {
		t.Fatalf("预置旧配置文件失败: %v", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("设置旧配置文件权限失败: %v", err)
	}

	cfg := &Config{Host: DefaultHost, Token: "secret"}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save 返回错误: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat 返回错误: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("覆盖已有宽权限文件后权限 = %o, 期望收敛到 0600", perm)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load 返回错误: %v", err)
	}
	if got.Token != "secret" {
		t.Errorf("Token = %q, 期望 %q", got.Token, "secret")
	}
}

// TestDirRespectsEnvOverride 验证环境变量可覆盖配置目录。
func TestDirRespectsEnvOverride(t *testing.T) {
	custom := "/tmp/my-gitee-cli-config"
	t.Setenv(envConfigDir, custom)

	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir 返回错误: %v", err)
	}
	if dir != custom {
		t.Errorf("Dir = %q, 期望 %q", dir, custom)
	}
}
