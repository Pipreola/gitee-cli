package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"gitee-cli/pkg/api"
	"gitee-cli/pkg/config"
)

// fakeGitRunner 是测试用的 git 桩实现。
type fakeGitRunner struct {
	remoteURL      string
	currentBranch  string
	verifyBranches map[string]bool // branch -> exists
	lsRemoteOutput map[string]string
	pushErr        error

	// runCalls / interactiveCalls 记录历史调用，供 repo clone 等用例断言。
	runCalls         [][]string
	interactiveCalls [][]string
	// cloneErr 注入 git clone（runInteractive）的错误。
	cloneErr error
	// setURLErr 注入 remote set-url（run）的错误。
	setURLErr error
}

func (f *fakeGitRunner) run(args ...string) (string, error) {
	f.runCalls = append(f.runCalls, args)
	switch {
	case len(args) >= 3 && args[0] == "remote" && args[1] == "get-url" && args[2] == "origin":
		if f.remoteURL == "" {
			return "", errors.New("not a git repository")
		}
		return f.remoteURL, nil
	case len(args) >= 2 && args[0] == "branch" && args[1] == "--show-current":
		if f.currentBranch == "" {
			return "", errors.New("not in a branch")
		}
		return f.currentBranch, nil
	case len(args) >= 3 && args[0] == "rev-parse" && args[1] == "--verify":
		branch := args[2]
		if f.verifyBranches[branch] {
			return "abc123", nil
		}
		return "", errors.New("not found")
	case len(args) >= 3 && args[0] == "ls-remote" && args[1] == "--heads":
		key := strings.Join(args[2:], " ")
		if out, ok := f.lsRemoteOutput[key]; ok {
			return out, nil
		}
		return "", nil
	// repo clone 重置 origin：git -C <dir> remote set-url origin <url>
	case len(args) >= 5 && args[0] == "-C" && args[2] == "remote" && args[3] == "set-url":
		return "", f.setURLErr
	}
	return "", fmt.Errorf("未桩化的 git 调用: %v", args)
}

func (f *fakeGitRunner) runInteractive(args ...string) error {
	f.interactiveCalls = append(f.interactiveCalls, args)
	if len(args) >= 4 && args[0] == "push" && args[1] == "-u" {
		return f.pushErr
	}
	if len(args) >= 1 && args[0] == "clone" {
		return f.cloneErr
	}
	return fmt.Errorf("未桩化的交互 git 调用: %v", args)
}

// TestRunPRCreateNoAuth 验证未登录时报错。
func TestRunPRCreateNoAuth(t *testing.T) {
	opts := prCreateOptions{title: "test", head: "feat", base: "main"}
	env := prCreateEnv{
		git: &fakeGitRunner{},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: ""}, nil
		},
		in:  strings.NewReader(""),
		out: &bytes.Buffer{},
	}

	err := runPRCreate(context.Background(), opts, env)
	if err == nil || !strings.Contains(err.Error(), "未登录") {
		t.Errorf("期望未登录错误，实际: %v", err)
	}
}

// TestRunPRCreateGetRepoError 验证获取仓库信息失败时的错误处理。
func TestRunPRCreateGetRepoError(t *testing.T) {
	opts := prCreateOptions{title: "test", head: "feat", base: "main"}
	env := prCreateEnv{
		git: &fakeGitRunner{remoteURL: ""}, // 返回错误
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		in:  strings.NewReader(""),
		out: &bytes.Buffer{},
	}

	err := runPRCreate(context.Background(), opts, env)
	if err == nil || !strings.Contains(err.Error(), "获取仓库信息失败") {
		t.Errorf("期望仓库信息错误，实际: %v", err)
	}
}

// TestRunPRCreateCurrentBranchDetection 验证自动检测当前分支。
func TestRunPRCreateCurrentBranchDetection(t *testing.T) {
	var capturedHead string
	opts := prCreateOptions{title: "test", head: "", base: "main"}
	env := prCreateEnv{
		git: &fakeGitRunner{
			remoteURL:      "https://gitee.com/owner/repo.git",
			currentBranch:  "feature-x",
			lsRemoteOutput: map[string]string{"origin feature-x": "abc123"},
		},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		createPR: func(ctx context.Context, host, token, owner, repo string, input *api.CreatePullRequestInput) (*api.PullRequest, error) {
			capturedHead = input.Head
			return &api.PullRequest{Number: 1, Title: input.Title, HTMLURL: "https://gitee.com/owner/repo/pulls/1"}, nil
		},
		in:  strings.NewReader(""),
		out: &bytes.Buffer{},
	}

	err := runPRCreate(context.Background(), opts, env)
	if err != nil {
		t.Fatalf("runPRCreate 返回错误: %v", err)
	}
	if capturedHead != "feature-x" {
		t.Errorf("head = %q, 期望检测到 feature-x", capturedHead)
	}
}

// TestRunPRCreateDefaultBaseDetection 验证自动检测默认基础分支。
func TestRunPRCreateDefaultBaseDetection(t *testing.T) {
	var capturedBase string
	opts := prCreateOptions{title: "test", head: "feat", base: ""}
	env := prCreateEnv{
		git: &fakeGitRunner{
			remoteURL:      "https://gitee.com/owner/repo.git",
			currentBranch:  "feat",
			verifyBranches: map[string]bool{"origin/main": true},
			lsRemoteOutput: map[string]string{"origin feat": "abc123"},
		},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		createPR: func(ctx context.Context, host, token, owner, repo string, input *api.CreatePullRequestInput) (*api.PullRequest, error) {
			capturedBase = input.Base
			return &api.PullRequest{Number: 1, Title: input.Title, HTMLURL: "https://gitee.com/owner/repo/pulls/1"}, nil
		},
		in:  strings.NewReader(""),
		out: &bytes.Buffer{},
	}

	err := runPRCreate(context.Background(), opts, env)
	if err != nil {
		t.Fatalf("runPRCreate 返回错误: %v", err)
	}
	if capturedBase != "main" {
		t.Errorf("base = %q, 期望检测到 main", capturedBase)
	}
}

// TestRunPRCreateSameHeadBase 验证源分支和目标分支相同时报错。
func TestRunPRCreateSameHeadBase(t *testing.T) {
	opts := prCreateOptions{title: "test", head: "main", base: "main"}
	env := prCreateEnv{
		git: &fakeGitRunner{
			remoteURL: "https://gitee.com/owner/repo.git",
		},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		in:  strings.NewReader(""),
		out: &bytes.Buffer{},
	}

	err := runPRCreate(context.Background(), opts, env)
	if err == nil || !strings.Contains(err.Error(), "源分支和目标分支不能相同") {
		t.Errorf("期望分支相同错误，实际: %v", err)
	}
}

// TestRunPRCreateBranchNotPushed 验证分支未推送时自动推送。
func TestRunPRCreateBranchNotPushed(t *testing.T) {
	out := &bytes.Buffer{}
	opts := prCreateOptions{title: "test", head: "feat", base: "main"}
	env := prCreateEnv{
		git: &fakeGitRunner{
			remoteURL:      "https://gitee.com/owner/repo.git",
			currentBranch:  "feat",
			lsRemoteOutput: map[string]string{"origin feat": ""}, // 未推送
			pushErr:        nil,
		},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		createPR: func(ctx context.Context, host, token, owner, repo string, input *api.CreatePullRequestInput) (*api.PullRequest, error) {
			return &api.PullRequest{Number: 1, Title: input.Title, HTMLURL: "https://gitee.com/owner/repo/pulls/1"}, nil
		},
		in:  strings.NewReader(""),
		out: out,
	}

	err := runPRCreate(context.Background(), opts, env)
	if err != nil {
		t.Fatalf("runPRCreate 返回错误: %v", err)
	}
	if !strings.Contains(out.String(), "正在推送") {
		t.Errorf("期望输出推送提示，实际输出: %s", out.String())
	}
}

// TestRunPRCreateInteractiveTitle 验证交互式输入标题。
func TestRunPRCreateInteractiveTitle(t *testing.T) {
	var capturedTitle string
	opts := prCreateOptions{title: "", head: "feat", base: "main"}
	env := prCreateEnv{
		git: &fakeGitRunner{
			remoteURL:      "https://gitee.com/owner/repo.git",
			lsRemoteOutput: map[string]string{"origin feat": "abc123"},
		},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		createPR: func(ctx context.Context, host, token, owner, repo string, input *api.CreatePullRequestInput) (*api.PullRequest, error) {
			capturedTitle = input.Title
			return &api.PullRequest{Number: 1, Title: input.Title, HTMLURL: "https://gitee.com/owner/repo/pulls/1"}, nil
		},
		in:  strings.NewReader("添加新功能\n\n"), // 标题 + 描述（空）
		out: &bytes.Buffer{},
	}

	err := runPRCreate(context.Background(), opts, env)
	if err != nil {
		t.Fatalf("runPRCreate 返回错误: %v", err)
	}
	if capturedTitle != "添加新功能" {
		t.Errorf("title = %q, 期望交互输入的标题", capturedTitle)
	}
}

// TestRunPRCreateInteractiveTitleEmpty 验证交互式输入标题为空时报错。
func TestRunPRCreateInteractiveTitleEmpty(t *testing.T) {
	opts := prCreateOptions{title: "", head: "feat", base: "main"}
	env := prCreateEnv{
		git: &fakeGitRunner{
			remoteURL:      "https://gitee.com/owner/repo.git",
			lsRemoteOutput: map[string]string{"origin feat": "abc123"},
		},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		in:  strings.NewReader("\n"), // 空标题
		out: &bytes.Buffer{},
	}

	err := runPRCreate(context.Background(), opts, env)
	if err == nil || !strings.Contains(err.Error(), "PR 标题不能为空") {
		t.Errorf("期望标题为空错误，实际: %v", err)
	}
}

// TestRunPRCreateSuccess 验证成功创建 PR 的完整流程。
func TestRunPRCreateSuccess(t *testing.T) {
	out := &bytes.Buffer{}
	opts := prCreateOptions{
		title:       "测试 PR",
		body:        "这是描述",
		bodyChanged: true,
		head:        "feat",
		base:        "main",
		draft:       true,
	}
	env := prCreateEnv{
		git: &fakeGitRunner{
			remoteURL:      "https://gitee.com/owner/repo.git",
			lsRemoteOutput: map[string]string{"origin feat": "abc123"},
		},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		createPR: func(ctx context.Context, host, token, owner, repo string, input *api.CreatePullRequestInput) (*api.PullRequest, error) {
			if input.Title != "测试 PR" || input.Body != "这是描述" || !input.Draft {
				t.Errorf("input = %+v, 参数不符合预期", input)
			}
			return &api.PullRequest{
				Number:  123,
				Title:   input.Title,
				Body:    input.Body,
				HTMLURL: "https://gitee.com/owner/repo/pulls/123",
			}, nil
		},
		in:  strings.NewReader(""),
		out: out,
	}

	err := runPRCreate(context.Background(), opts, env)
	if err != nil {
		t.Fatalf("runPRCreate 返回错误: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "PR 创建成功") {
		t.Errorf("期望成功提示，实际输出: %s", output)
	}
	if !strings.Contains(output, "#123") {
		t.Errorf("期望输出 PR 编号，实际输出: %s", output)
	}
	if !strings.Contains(output, "草稿") {
		t.Errorf("期望输出草稿状态，实际输出: %s", output)
	}
}

// TestGetCurrentRepoHTTPS 验证 HTTPS 格式的仓库 URL 解析。
func TestGetCurrentRepoHTTPS(t *testing.T) {
	git := &fakeGitRunner{remoteURL: "https://gitee.com/alice/my-repo.git"}
	owner, repo, err := getCurrentRepo(git)
	if err != nil {
		t.Fatalf("getCurrentRepo 返回错误: %v", err)
	}
	if owner != "alice" || repo != "my-repo" {
		t.Errorf("owner = %q, repo = %q, 期望 alice, my-repo", owner, repo)
	}
}

// TestGetCurrentRepoSSH 验证 SSH 格式的仓库 URL 解析。
func TestGetCurrentRepoSSH(t *testing.T) {
	git := &fakeGitRunner{remoteURL: "git@gitee.com:bob/his-repo.git"}
	owner, repo, err := getCurrentRepo(git)
	if err != nil {
		t.Fatalf("getCurrentRepo 返回错误: %v", err)
	}
	if owner != "bob" || repo != "his-repo" {
		t.Errorf("owner = %q, repo = %q, 期望 bob, his-repo", owner, repo)
	}
}

// TestGetCurrentRepoInvalidURL 验证无效 URL 格式的错误处理。
func TestGetCurrentRepoInvalidURL(t *testing.T) {
	git := &fakeGitRunner{remoteURL: "ftp://example.com/repo"}
	_, _, err := getCurrentRepo(git)
	if err == nil || !strings.Contains(err.Error(), "不支持的仓库 URL 格式") {
		t.Errorf("期望 URL 格式错误，实际: %v", err)
	}
}

// TestGetDefaultBaseBranchMain 验证优先选择 main 分支。
func TestGetDefaultBaseBranchMain(t *testing.T) {
	git := &fakeGitRunner{
		verifyBranches: map[string]bool{"origin/main": true, "origin/master": true},
	}
	branch, err := getDefaultBaseBranch(git)
	if err != nil {
		t.Fatalf("getDefaultBaseBranch 返回错误: %v", err)
	}
	if branch != "main" {
		t.Errorf("branch = %q, 期望 main", branch)
	}
}

// TestGetDefaultBaseBranchMaster 验证 main 不存在时回退到 master。
func TestGetDefaultBaseBranchMaster(t *testing.T) {
	git := &fakeGitRunner{
		verifyBranches: map[string]bool{"origin/master": true},
	}
	branch, err := getDefaultBaseBranch(git)
	if err != nil {
		t.Fatalf("getDefaultBaseBranch 返回错误: %v", err)
	}
	if branch != "master" {
		t.Errorf("branch = %q, 期望 master", branch)
	}
}

// TestGetDefaultBaseBranchNotFound 验证找不到默认分支时的错误。
func TestGetDefaultBaseBranchNotFound(t *testing.T) {
	git := &fakeGitRunner{verifyBranches: map[string]bool{}}
	_, err := getDefaultBaseBranch(git)
	if err == nil || !strings.Contains(err.Error(), "找不到默认分支") {
		t.Errorf("期望找不到默认分支错误，实际: %v", err)
	}
}
