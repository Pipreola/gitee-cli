package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"gitee-cli/pkg/config"
)

// errFakeClone 是测试用的注入错误。
var errFakeClone = errors.New("fake git failure")

// cloneTestEnv 构造带桩 git 与配置的 repo clone 测试环境。
func cloneTestEnv(git *fakeGitRunner, cfg *config.Config) (repoCloneEnv, *bytes.Buffer) {
	out := &bytes.Buffer{}
	env := repoCloneEnv{
		git:        git,
		loadConfig: func() (*config.Config, error) { return cfg, nil },
		out:        out,
	}
	return env, out
}

// TestRunRepoCloneNoAuth 未登录时报错。
func TestRunRepoCloneNoAuth(t *testing.T) {
	git := &fakeGitRunner{}
	env, _ := cloneTestEnv(git, &config.Config{Host: config.DefaultHost, Token: ""})

	err := runRepoClone(repoCloneOptions{repo: "owner/repo"}, env)
	if err == nil || !strings.Contains(err.Error(), "未登录") {
		t.Fatalf("期望未登录错误，实际: %v", err)
	}
	if len(git.interactiveCalls) != 0 {
		t.Errorf("未登录时不应调用 git clone，实际调用: %v", git.interactiveCalls)
	}
}

// TestRunRepoCloneBasic 验证基本克隆流程：构造鉴权 URL、调用 git clone、重置 origin。
func TestRunRepoCloneBasic(t *testing.T) {
	git := &fakeGitRunner{}
	cfg := &config.Config{Host: "https://gitee.com/api/v5", Token: "secrettoken", User: "me"}
	env, out := cloneTestEnv(git, cfg)

	if err := runRepoClone(repoCloneOptions{repo: "owner/repo"}, env); err != nil {
		t.Fatalf("克隆不应失败: %v", err)
	}

	// 断言 git clone 调用
	if len(git.interactiveCalls) != 1 {
		t.Fatalf("期望 1 次 clone 调用，实际: %v", git.interactiveCalls)
	}
	clone := git.interactiveCalls[0]
	if clone[0] != "clone" {
		t.Errorf("期望 clone 子命令，实际: %v", clone)
	}
	authURL := clone[len(clone)-1]
	if !strings.Contains(authURL, "oauth2:secrettoken@gitee.com") {
		t.Errorf("clone URL 应包含鉴权信息，实际: %s", authURL)
	}
	if !strings.HasSuffix(authURL, "/owner/repo.git") {
		t.Errorf("clone URL 路径应为 /owner/repo.git，实际: %s", authURL)
	}

	// 断言 origin 被重置为干净 URL（不含令牌）
	var foundReset bool
	for _, call := range git.runCalls {
		if len(call) >= 5 && call[0] == "-C" && call[2] == "remote" && call[3] == "set-url" {
			foundReset = true
			cleanURL := call[len(call)-1]
			if strings.Contains(cleanURL, "secrettoken") {
				t.Errorf("重置后的 origin URL 不应含令牌，实际: %s", cleanURL)
			}
			if cleanURL != "https://gitee.com/owner/repo.git" {
				t.Errorf("干净 URL 不符预期，实际: %s", cleanURL)
			}
			if call[1] != "repo" {
				t.Errorf("set-url 应在仓库目录 repo 下执行，实际目录: %s", call[1])
			}
		}
	}
	if !foundReset {
		t.Error("未发现 origin 重置调用")
	}

	if !strings.Contains(out.String(), "已克隆") {
		t.Errorf("输出应包含成功提示，实际: %s", out.String())
	}
}

// TestRunRepoClonePassThroughArgs 验证 -- 之后的参数透传给 git clone。
func TestRunRepoClonePassThroughArgs(t *testing.T) {
	git := &fakeGitRunner{}
	cfg := &config.Config{Host: "https://gitee.com/api/v5", Token: "tok", User: "me"}
	env, _ := cloneTestEnv(git, cfg)

	opts := repoCloneOptions{repo: "owner/repo", gitArgs: []string{"--depth", "1"}}
	if err := runRepoClone(opts, env); err != nil {
		t.Fatalf("克隆不应失败: %v", err)
	}

	clone := git.interactiveCalls[0]
	// 期望: clone --depth 1 <authURL>
	if clone[1] != "--depth" || clone[2] != "1" {
		t.Errorf("透传参数应紧跟 clone 之后，实际: %v", clone)
	}
	if !strings.HasSuffix(clone[len(clone)-1], "/owner/repo.git") {
		t.Errorf("末尾应为 clone URL，实际: %v", clone)
	}
}

// TestRunRepoCloneWithDir 验证显式目标目录会被传给 git clone 并用于 origin 重置。
func TestRunRepoCloneWithDir(t *testing.T) {
	git := &fakeGitRunner{}
	cfg := &config.Config{Host: "https://gitee.com/api/v5", Token: "tok", User: "me"}
	env, _ := cloneTestEnv(git, cfg)

	opts := repoCloneOptions{repo: "owner/repo", dir: "my-dir"}
	if err := runRepoClone(opts, env); err != nil {
		t.Fatalf("克隆不应失败: %v", err)
	}

	clone := git.interactiveCalls[0]
	if clone[len(clone)-1] != "my-dir" {
		t.Errorf("末尾位置参数应为目标目录 my-dir，实际: %v", clone)
	}

	// origin 重置应在 my-dir 下执行
	var dir string
	for _, call := range git.runCalls {
		if len(call) >= 5 && call[0] == "-C" && call[2] == "remote" {
			dir = call[1]
		}
	}
	if dir != "my-dir" {
		t.Errorf("origin 重置目录应为 my-dir，实际: %s", dir)
	}
}

// TestRunRepoCloneOwnerFromUser 验证仅给出 repo 时 owner 取当前登录用户。
func TestRunRepoCloneOwnerFromUser(t *testing.T) {
	git := &fakeGitRunner{}
	cfg := &config.Config{Host: "https://gitee.com/api/v5", Token: "tok", User: "alice"}
	env, _ := cloneTestEnv(git, cfg)

	if err := runRepoClone(repoCloneOptions{repo: "myrepo"}, env); err != nil {
		t.Fatalf("克隆不应失败: %v", err)
	}
	clone := git.interactiveCalls[0]
	if !strings.HasSuffix(clone[len(clone)-1], "/alice/myrepo.git") {
		t.Errorf("owner 应取登录用户 alice，实际: %v", clone[len(clone)-1])
	}
}

// TestRunRepoCloneOwnerMissing 验证仅给出 repo 且无登录用户时报错。
func TestRunRepoCloneOwnerMissing(t *testing.T) {
	git := &fakeGitRunner{}
	cfg := &config.Config{Host: "https://gitee.com/api/v5", Token: "tok", User: ""}
	env, _ := cloneTestEnv(git, cfg)

	err := runRepoClone(repoCloneOptions{repo: "myrepo"}, env)
	if err == nil || !strings.Contains(err.Error(), "owner") {
		t.Fatalf("期望 owner 缺失错误，实际: %v", err)
	}
}

// TestRunRepoCloneCloneError 验证 git clone 失败时返回错误且不重置 origin。
func TestRunRepoCloneCloneError(t *testing.T) {
	git := &fakeGitRunner{cloneErr: errFakeClone}
	cfg := &config.Config{Host: "https://gitee.com/api/v5", Token: "tok", User: "me"}
	env, _ := cloneTestEnv(git, cfg)

	err := runRepoClone(repoCloneOptions{repo: "owner/repo"}, env)
	if err == nil || !strings.Contains(err.Error(), "克隆仓库失败") {
		t.Fatalf("期望克隆失败错误，实际: %v", err)
	}
	for _, call := range git.runCalls {
		if len(call) >= 3 && call[2] == "remote" {
			t.Error("克隆失败后不应重置 origin")
		}
	}
}

// TestRunRepoCloneSetURLErrorNonFatal 验证重置 origin 失败仅告警、不影响整体成功。
func TestRunRepoCloneSetURLErrorNonFatal(t *testing.T) {
	git := &fakeGitRunner{setURLErr: errFakeClone}
	cfg := &config.Config{Host: "https://gitee.com/api/v5", Token: "tok", User: "me"}
	env, out := cloneTestEnv(git, cfg)

	if err := runRepoClone(repoCloneOptions{repo: "owner/repo"}, env); err != nil {
		t.Fatalf("重置 origin 失败不应导致命令失败: %v", err)
	}
	if !strings.Contains(out.String(), "无法重置 origin") {
		t.Errorf("应输出重置失败告警，实际: %s", out.String())
	}
}

// TestParseRepoCloneArgs 验证位置参数与 -- 透传参数的拆分逻辑。
func TestParseRepoCloneArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		dashIdx  int
		wantRepo string
		wantDir  string
		wantGit  []string
		wantErr  bool
	}{
		{name: "仅 repo", args: []string{"owner/repo"}, dashIdx: -1, wantRepo: "owner/repo"},
		{name: "repo+dir", args: []string{"owner/repo", "dir"}, dashIdx: -1, wantRepo: "owner/repo", wantDir: "dir"},
		{name: "repo+透传", args: []string{"owner/repo", "--depth", "1"}, dashIdx: 1, wantRepo: "owner/repo", wantGit: []string{"--depth", "1"}},
		{name: "无参数", args: []string{}, dashIdx: -1, wantErr: true},
		{name: "位置参数过多", args: []string{"a", "b", "c"}, dashIdx: -1, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newRepoCloneCmd()
			// 通过 fakeDashCmd 模拟 ArgsLenAtDash 行为。
			opts, err := parseRepoCloneArgsWithDash(cmd, tt.args, tt.dashIdx)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("期望错误，实际成功: %+v", opts)
				}
				return
			}
			if err != nil {
				t.Fatalf("不应出错: %v", err)
			}
			if opts.repo != tt.wantRepo {
				t.Errorf("repo: 期望 %q 实际 %q", tt.wantRepo, opts.repo)
			}
			if opts.dir != tt.wantDir {
				t.Errorf("dir: 期望 %q 实际 %q", tt.wantDir, opts.dir)
			}
			if strings.Join(opts.gitArgs, " ") != strings.Join(tt.wantGit, " ") {
				t.Errorf("gitArgs: 期望 %v 实际 %v", tt.wantGit, opts.gitArgs)
			}
		})
	}
}

// TestDeriveGitHost 验证从 API Host 派生 git 主机地址。
func TestDeriveGitHost(t *testing.T) {
	tests := []struct {
		in   string
		want string
		err  bool
	}{
		{in: "https://gitee.com/api/v5", want: "https://gitee.com"},
		{in: "https://gitee.com", want: "https://gitee.com"},
		{in: "http://git.internal:8080/api/v5", want: "http://git.internal:8080"},
		{in: "", want: "https://gitee.com"},
		{in: "://bad", err: true},
	}
	for _, tt := range tests {
		got, err := deriveGitHost(tt.in)
		if tt.err {
			if err == nil {
				t.Errorf("deriveGitHost(%q) 期望错误", tt.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("deriveGitHost(%q) 意外错误: %v", tt.in, err)
		}
		if got != tt.want {
			t.Errorf("deriveGitHost(%q) = %q, 期望 %q", tt.in, got, tt.want)
		}
	}
}

// TestBuildAuthCloneURL 验证鉴权 clone URL 的构造与令牌转义。
func TestBuildAuthCloneURL(t *testing.T) {
	got := buildAuthCloneURL("https://gitee.com", "owner", "repo", "tok123")
	want := "https://oauth2:tok123@gitee.com/owner/repo.git"
	if got != want {
		t.Errorf("buildAuthCloneURL = %q, 期望 %q", got, want)
	}

	// 含特殊字符的令牌应被正确转义
	got = buildAuthCloneURL("https://gitee.com", "o", "r", "a/b@c")
	if strings.Contains(got, "a/b@c") {
		t.Errorf("令牌中的特殊字符应被转义，实际: %q", got)
	}
}
