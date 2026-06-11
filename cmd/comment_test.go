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

// TestRunPRCommentNoAuth 验证未登录时报错。
func TestRunPRCommentNoAuth(t *testing.T) {
	env := commentEnv{
		git: &fakeGitRunner{remoteURL: "https://gitee.com/owner/repo.git"},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: ""}, nil
		},
		in:  strings.NewReader(""),
		out: &bytes.Buffer{},
	}

	opts := commentOptions{body: "test comment"}
	err := runPRComment(context.Background(), 123, opts, env)
	if err == nil || !strings.Contains(err.Error(), "未登录") {
		t.Errorf("期望未登录错误，实际: %v", err)
	}
}

// TestRunPRCommentGetRepoError 验证获取仓库信息失败时的错误处理。
func TestRunPRCommentGetRepoError(t *testing.T) {
	env := commentEnv{
		git: &fakeGitRunner{remoteURL: ""}, // 返回错误
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		in:  strings.NewReader(""),
		out: &bytes.Buffer{},
	}

	opts := commentOptions{body: "test comment"}
	err := runPRComment(context.Background(), 123, opts, env)
	if err == nil || !strings.Contains(err.Error(), "获取仓库信息失败") {
		t.Errorf("期望仓库信息错误，实际: %v", err)
	}
}

// TestRunPRCommentWithBody 验证使用 --body 指定评论内容。
func TestRunPRCommentWithBody(t *testing.T) {
	var capturedNumber int64
	var capturedBody string
	out := &bytes.Buffer{}

	env := commentEnv{
		git:        &fakeGitRunner{remoteURL: "https://gitee.com/owner/repo.git"},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		createPRComment: func(ctx context.Context, host, token, owner, repo string, number int64, input *api.CreatePullRequestCommentInput) (*api.Comment, error) {
			capturedNumber = number
			capturedBody = input.Body
			return &api.Comment{
				ID:        1,
				Body:      input.Body,
				User:      api.User{Login: "testuser"},
				CreatedAt: "2024-01-01T00:00:00+08:00",
			}, nil
		},
		in:  strings.NewReader(""),
		out: out,
	}

	opts := commentOptions{body: "LGTM"}
	err := runPRComment(context.Background(), 123, opts, env)
	if err != nil {
		t.Fatalf("期望成功，实际错误: %v", err)
	}

	if capturedNumber != 123 {
		t.Errorf("期望 PR 编号 123，实际: %d", capturedNumber)
	}
	if capturedBody != "LGTM" {
		t.Errorf("期望评论内容 'LGTM'，实际: %s", capturedBody)
	}
	if !strings.Contains(out.String(), "评论添加成功") {
		t.Errorf("期望输出包含成功信息，实际: %s", out.String())
	}
}

// TestRunPRCommentWithBodyFile 验证使用 --body-file 从文件读取评论内容。
func TestRunPRCommentWithBodyFile(t *testing.T) {
	var capturedBody string
	out := &bytes.Buffer{}

	env := commentEnv{
		git:        &fakeGitRunner{remoteURL: "https://gitee.com/owner/repo.git"},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		createPRComment: func(ctx context.Context, host, token, owner, repo string, number int64, input *api.CreatePullRequestCommentInput) (*api.Comment, error) {
			capturedBody = input.Body
			return &api.Comment{
				ID:        1,
				Body:      input.Body,
				User:      api.User{Login: "testuser"},
				CreatedAt: "2024-01-01T00:00:00+08:00",
			}, nil
		},
		readFile: func(filename string) ([]byte, error) {
			if filename == "comment.txt" {
				return []byte("Comment from file\nLine 2"), nil
			}
			return nil, errors.New("file not found")
		},
		in:  strings.NewReader(""),
		out: out,
	}

	opts := commentOptions{bodyFile: "comment.txt"}
	err := runPRComment(context.Background(), 123, opts, env)
	if err != nil {
		t.Fatalf("期望成功，实际错误: %v", err)
	}

	expectedBody := "Comment from file\nLine 2"
	if capturedBody != expectedBody {
		t.Errorf("期望评论内容 '%s'，实际: %s", expectedBody, capturedBody)
	}
}

// TestRunPRCommentInteractive 验证交互式输入评论内容。
func TestRunPRCommentInteractive(t *testing.T) {
	var capturedBody string
	out := &bytes.Buffer{}
	in := strings.NewReader("This is a comment\nLine 2\n\n")

	env := commentEnv{
		git:        &fakeGitRunner{remoteURL: "https://gitee.com/owner/repo.git"},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		createPRComment: func(ctx context.Context, host, token, owner, repo string, number int64, input *api.CreatePullRequestCommentInput) (*api.Comment, error) {
			capturedBody = input.Body
			return &api.Comment{
				ID:        1,
				Body:      input.Body,
				User:      api.User{Login: "testuser"},
				CreatedAt: "2024-01-01T00:00:00+08:00",
			}, nil
		},
		in:  in,
		out: out,
	}

	opts := commentOptions{} // 无 body 和 bodyFile，触发交互式输入
	err := runPRComment(context.Background(), 123, opts, env)
	if err != nil {
		t.Fatalf("期望成功，实际错误: %v", err)
	}

	expectedBody := "This is a comment\nLine 2"
	if capturedBody != expectedBody {
		t.Errorf("期望评论内容 '%s'，实际: %s", expectedBody, capturedBody)
	}
}

// TestRunPRCommentAPIError 验证 API 错误处理。
func TestRunPRCommentAPIError(t *testing.T) {
	env := commentEnv{
		git:        &fakeGitRunner{remoteURL: "https://gitee.com/owner/repo.git"},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		createPRComment: func(ctx context.Context, host, token, owner, repo string, number int64, input *api.CreatePullRequestCommentInput) (*api.Comment, error) {
			return nil, &api.APIError{StatusCode: 404, Message: "PR not found"}
		},
		in:  strings.NewReader(""),
		out: &bytes.Buffer{},
	}

	opts := commentOptions{body: "test"}
	err := runPRComment(context.Background(), 123, opts, env)
	if err == nil || !strings.Contains(err.Error(), "创建评论失败") {
		t.Errorf("期望 API 错误，实际: %v", err)
	}
}

// TestRunIssueCommentWithBody 验证 Issue 评论功能。
func TestRunIssueCommentWithBody(t *testing.T) {
	var capturedNumber string
	var capturedBody string
	out := &bytes.Buffer{}

	env := commentEnv{
		git:        &fakeGitRunner{remoteURL: "https://gitee.com/owner/repo.git"},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		createIssueComment: func(ctx context.Context, host, token, owner, repo, number string, input *api.CreateIssueCommentInput) (*api.Comment, error) {
			capturedNumber = number
			capturedBody = input.Body
			return &api.Comment{
				ID:        1,
				Body:      input.Body,
				User:      api.User{Login: "testuser"},
				CreatedAt: "2024-01-01T00:00:00+08:00",
			}, nil
		},
		in:  strings.NewReader(""),
		out: out,
	}

	opts := commentOptions{body: "Fixed"}
	err := runIssueComment(context.Background(), "I123", opts, env)
	if err != nil {
		t.Fatalf("期望成功，实际错误: %v", err)
	}

	if capturedNumber != "I123" {
		t.Errorf("期望 Issue 编号 'I123'，实际: %s", capturedNumber)
	}
	if capturedBody != "Fixed" {
		t.Errorf("期望评论内容 'Fixed'，实际: %s", capturedBody)
	}
	if !strings.Contains(out.String(), "评论添加成功") {
		t.Errorf("期望输出包含成功信息，实际: %s", out.String())
	}
}

// TestGetCommentBodyBothFlagsError 验证同时指定 --body 和 --body-file 时报错。
func TestGetCommentBodyBothFlagsError(t *testing.T) {
	env := commentEnv{}
	opts := commentOptions{
		body:     "test",
		bodyFile: "file.txt",
	}

	_, err := getCommentBody(opts, env)
	if err == nil || !strings.Contains(err.Error(), "不能同时指定") {
		t.Errorf("期望同时指定错误，实际: %v", err)
	}
}

// TestGetCommentBodyFileReadError 验证文件读取失败时的错误处理。
func TestGetCommentBodyFileReadError(t *testing.T) {
	env := commentEnv{
		readFile: func(filename string) ([]byte, error) {
			return nil, errors.New("file not found")
		},
	}
	opts := commentOptions{bodyFile: "nonexistent.txt"}

	_, err := getCommentBody(opts, env)
	if err == nil || !strings.Contains(err.Error(), "读取文件失败") {
		t.Errorf("期望文件读取错误，实际: %v", err)
	}
}

// TestGetCommentBodyEmptyContent 验证空内容时报错。
func TestGetCommentBodyEmptyContent(t *testing.T) {
	env := commentEnv{
		in:  strings.NewReader("\n"),
		out: &bytes.Buffer{},
	}
	opts := commentOptions{} // 交互式输入空行

	_, err := getCommentBody(opts, env)
	if err == nil || !strings.Contains(err.Error(), "不能为空") {
		t.Errorf("期望空内容错误，实际: %v", err)
	}
}

// TestTruncateString 验证字符串截断功能。
func TestTruncateString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "短字符串不截断",
			input:    "Short",
			maxLen:   10,
			expected: "Short",
		},
		{
			name:     "长字符串截断",
			input:    "This is a very long string that needs to be truncated",
			maxLen:   20,
			expected: "This is a very long ...",
		},
		{
			name:     "多行转单行",
			input:    "Line 1\nLine 2\nLine 3",
			maxLen:   15,
			expected: "Line 1 Line 2 L...",
		},
		{
			name:     "包含多个空格",
			input:    "Multiple   spaces    here",
			maxLen:   50,
			expected: "Multiple spaces here",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateString(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("期望 '%s'，实际 '%s'", tt.expected, result)
			}
		})
	}
}
