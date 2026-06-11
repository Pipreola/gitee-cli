package cmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"gitee-cli/pkg/api"
	"gitee-cli/pkg/config"
)

// newIssueCreateGitFake 返回一个仓库 URL 已配置的 fakeGitRunner。
func newIssueCreateGitFake() *fakeGitRunner {
	return &fakeGitRunner{
		remoteURL: "https://gitee.com/testowner/testrepo.git",
	}
}

// TestRunIssueCreateNoAuth 验证未登录时报错。
func TestRunIssueCreateNoAuth(t *testing.T) {
	opts := issueCreateOptions{title: "test"}
	env := issueCreateEnv{
		git: newIssueCreateGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: ""}, nil
		},
		in:  strings.NewReader(""),
		out: &bytes.Buffer{},
	}

	err := runIssueCreate(context.Background(), opts, env)
	if err == nil || !strings.Contains(err.Error(), "未登录") {
		t.Errorf("期望未登录错误，实际: %v", err)
	}
}

// TestRunIssueCreateGetRepoError 验证获取仓库信息失败时的错误处理。
func TestRunIssueCreateGetRepoError(t *testing.T) {
	opts := issueCreateOptions{title: "test"}
	env := issueCreateEnv{
		git: &fakeGitRunner{remoteURL: ""}, // 返回错误
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		in:  strings.NewReader(""),
		out: &bytes.Buffer{},
	}

	err := runIssueCreate(context.Background(), opts, env)
	if err == nil || !strings.Contains(err.Error(), "获取仓库信息失败") {
		t.Errorf("期望仓库信息错误，实际: %v", err)
	}
}

// TestRunIssueCreateInteractiveTitle 验证交互式输入标题。
func TestRunIssueCreateInteractiveTitle(t *testing.T) {
	var capturedTitle string
	opts := issueCreateOptions{title: ""}
	env := issueCreateEnv{
		git: newIssueCreateGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		createIssue: func(ctx context.Context, host, token, owner, repo string, input *api.CreateIssueInput) (*api.Issue, error) {
			capturedTitle = input.Title
			return &api.Issue{Number: "I123", Title: input.Title, HTMLURL: "https://gitee.com/owner/repo/issues/I123"}, nil
		},
		readFile: nil,
		in:       strings.NewReader("修复登录Bug\n\n"), // 标题 + 描述（空）
		out:      &bytes.Buffer{},
	}

	err := runIssueCreate(context.Background(), opts, env)
	if err != nil {
		t.Fatalf("runIssueCreate 返回错误: %v", err)
	}
	if capturedTitle != "修复登录Bug" {
		t.Errorf("title = %q, 期望交互输入的标题", capturedTitle)
	}
}

// TestRunIssueCreateInteractiveTitleEmpty 验证交互式输入标题为空时报错。
func TestRunIssueCreateInteractiveTitleEmpty(t *testing.T) {
	opts := issueCreateOptions{title: ""}
	env := issueCreateEnv{
		git: newIssueCreateGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		in:  strings.NewReader("\n"), // 空标题
		out: &bytes.Buffer{},
	}

	err := runIssueCreate(context.Background(), opts, env)
	if err == nil || !strings.Contains(err.Error(), "Issue 标题不能为空") {
		t.Errorf("期望标题为空错误，实际: %v", err)
	}
}

// TestRunIssueCreateWithBodyFile 验证从文件读取描述。
func TestRunIssueCreateWithBodyFile(t *testing.T) {
	var capturedBody string
	opts := issueCreateOptions{title: "测试Issue", bodyFile: "description.md"}
	env := issueCreateEnv{
		git: newIssueCreateGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		createIssue: func(ctx context.Context, host, token, owner, repo string, input *api.CreateIssueInput) (*api.Issue, error) {
			capturedBody = input.Body
			return &api.Issue{Number: "I456", Title: input.Title, Body: input.Body, HTMLURL: "https://gitee.com/owner/repo/issues/I456"}, nil
		},
		readFile: func(path string) ([]byte, error) {
			if path == "description.md" {
				return []byte("这是从文件读取的描述内容。"), nil
			}
			return nil, errors.New("文件不存在")
		},
		in:  strings.NewReader(""),
		out: &bytes.Buffer{},
	}

	err := runIssueCreate(context.Background(), opts, env)
	if err != nil {
		t.Fatalf("runIssueCreate 返回错误: %v", err)
	}
	if capturedBody != "这是从文件读取的描述内容。" {
		t.Errorf("body = %q, 期望从文件读取的内容", capturedBody)
	}
}

// TestRunIssueCreateBodyFileError 验证文件读取失败时的错误处理。
func TestRunIssueCreateBodyFileError(t *testing.T) {
	opts := issueCreateOptions{title: "测试Issue", bodyFile: "missing.md"}
	env := issueCreateEnv{
		git: newIssueCreateGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		readFile: func(path string) ([]byte, error) {
			return nil, errors.New("文件不存在")
		},
		in:  strings.NewReader(""),
		out: &bytes.Buffer{},
	}

	err := runIssueCreate(context.Background(), opts, env)
	if err == nil || !strings.Contains(err.Error(), "读取文件") {
		t.Errorf("期望文件读取错误，实际: %v", err)
	}
}

// TestRunIssueCreateSuccess 验证成功创建 Issue 的完整流程。
func TestRunIssueCreateSuccess(t *testing.T) {
	out := &bytes.Buffer{}
	opts := issueCreateOptions{
		title:           "修复Bug",
		body:            "这是描述",
		bodyChanged:     true,
		labels:          "bug,urgent",
		assignees:       "user1,user2",
		milestoneNumber: 1,
	}
	env := issueCreateEnv{
		git: newIssueCreateGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		createIssue: func(ctx context.Context, host, token, owner, repo string, input *api.CreateIssueInput) (*api.Issue, error) {
			if input.Title != "修复Bug" || input.Body != "这是描述" {
				t.Errorf("input = %+v, 参数不符合预期", input)
			}
			if input.Labels != "bug,urgent" || input.Assignees != "user1,user2" || input.MilestoneNumber != 1 {
				t.Errorf("input = %+v, 可选参数不符合预期", input)
			}
			return &api.Issue{
				Number:  "I789",
				Title:   input.Title,
				Body:    input.Body,
				HTMLURL: "https://gitee.com/testowner/testrepo/issues/I789",
				Labels: []struct {
					ID    int64  `json:"id"`
					Name  string `json:"name"`
					Color string `json:"color"`
				}{
					{ID: 1, Name: "bug", Color: "red"},
					{ID: 2, Name: "urgent", Color: "yellow"},
				},
				Assignee: &api.User{Login: "user1"},
			}, nil
		},
		readFile: nil,
		in:       strings.NewReader(""),
		out:      out,
	}

	err := runIssueCreate(context.Background(), opts, env)
	if err != nil {
		t.Fatalf("runIssueCreate 返回错误: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Issue 创建成功") {
		t.Errorf("期望成功提示，实际输出: %s", output)
	}
	if !strings.Contains(output, "I789") {
		t.Errorf("期望输出 Issue 编号，实际输出: %s", output)
	}
	if !strings.Contains(output, "标签: bug,urgent") {
		t.Errorf("期望输出标签，实际输出: %s", output)
	}
	if !strings.Contains(output, "指派给: user1") {
		t.Errorf("期望输出指派人，实际输出: %s", output)
	}
}

// TestRunIssueCreateAPIError 验证 API 错误的处理。
func TestRunIssueCreateAPIError(t *testing.T) {
	opts := issueCreateOptions{title: "测试Issue"}
	env := issueCreateEnv{
		git: newIssueCreateGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		createIssue: func(ctx context.Context, host, token, owner, repo string, input *api.CreateIssueInput) (*api.Issue, error) {
			return nil, errors.New("API错误")
		},
		readFile: nil,
		in:       strings.NewReader(""),
		out:      &bytes.Buffer{},
	}

	err := runIssueCreate(context.Background(), opts, env)
	if err == nil {
		t.Fatalf("期望错误但没有返回")
	}
	if !strings.Contains(err.Error(), "创建 Issue 失败") {
		t.Fatalf("期望错误消息包含 '创建 Issue 失败'，实际: %v", err)
	}
}

// TestRunIssueCreateWithWeb 验证 --web 标志打开浏览器。
func TestRunIssueCreateWithWeb(t *testing.T) {
	out := &bytes.Buffer{}
	browserOpened := false
	var browserURL string

	opts := issueCreateOptions{title: "测试Issue", web: true}
	env := issueCreateEnv{
		git: newIssueCreateGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		createIssue: func(ctx context.Context, host, token, owner, repo string, input *api.CreateIssueInput) (*api.Issue, error) {
			return &api.Issue{
				Number:  "I999",
				Title:   input.Title,
				HTMLURL: "https://gitee.com/testowner/testrepo/issues/I999",
			}, nil
		},
		openBrowser: func(url string) error {
			browserOpened = true
			browserURL = url
			return nil
		},
		readFile: nil,
		in:       strings.NewReader(""),
		out:      out,
	}

	err := runIssueCreate(context.Background(), opts, env)
	if err != nil {
		t.Fatalf("runIssueCreate 返回错误: %v", err)
	}

	if !browserOpened {
		t.Errorf("浏览器应该被打开")
	}
	if browserURL != "https://gitee.com/testowner/testrepo/issues/I999" {
		t.Errorf("浏览器打开的 URL 不正确，期望 %q，实际 %q", "https://gitee.com/testowner/testrepo/issues/I999", browserURL)
	}
}

// TestRunIssueCreateInteractiveBody 验证交互式输入标题和描述。
func TestRunIssueCreateInteractiveBody(t *testing.T) {
	var capturedTitle, capturedBody string
	opts := issueCreateOptions{title: ""}
	env := issueCreateEnv{
		git: newIssueCreateGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		createIssue: func(ctx context.Context, host, token, owner, repo string, input *api.CreateIssueInput) (*api.Issue, error) {
			capturedTitle = input.Title
			capturedBody = input.Body
			return &api.Issue{Number: "I111", Title: input.Title, Body: input.Body, HTMLURL: "https://gitee.com/owner/repo/issues/I111"}, nil
		},
		readFile: nil,
		in:       strings.NewReader("新功能\n详细描述内容\n"), // 标题 + 描述
		out:      &bytes.Buffer{},
	}

	err := runIssueCreate(context.Background(), opts, env)
	if err != nil {
		t.Fatalf("runIssueCreate 返回错误: %v", err)
	}
	if capturedTitle != "新功能" {
		t.Errorf("title = %q, 期望 '新功能'", capturedTitle)
	}
	if capturedBody != "详细描述内容" {
		t.Errorf("body = %q, 期望 '详细描述内容'", capturedBody)
	}
}
