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

// TestRunIssueUpdateStateNoAuth 验证未登录时报错。
func TestRunIssueUpdateStateNoAuth(t *testing.T) {
	env := issueStateEnv{
		git: &fakeGitRunner{remoteURL: "https://gitee.com/owner/repo.git"},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Token: ""}, nil
		},
		out: &bytes.Buffer{},
	}
	err := runIssueUpdateState(context.Background(), "I123", "closed", env)
	if err == nil || !strings.Contains(err.Error(), "未登录") {
		t.Errorf("期望未登录错误，实际: %v", err)
	}
}

// TestRunIssueUpdateStateSuccess 验证成功关闭 Issue。
func TestRunIssueUpdateStateSuccess(t *testing.T) {
	out := &bytes.Buffer{}
	env := issueStateEnv{
		git: &fakeGitRunner{remoteURL: "https://gitee.com/owner/repo.git"},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		updateIssueState: func(ctx context.Context, host, token, owner, repo, number, state string) (*api.Issue, error) {
			if number != "I123" {
				t.Errorf("number = %q, 期望 I123", number)
			}
			if state != "closed" {
				t.Errorf("state = %q, 期望 closed", state)
			}
			return &api.Issue{
				Number:    "I123",
				Title:     "Test Issue",
				State:     "closed",
				HTMLURL:   "https://gitee.com/owner/repo/issues/I123",
				User:      api.User{Login: "testuser"},
				CreatedAt: "2024-01-01T10:00:00+08:00",
				UpdatedAt: "2024-01-02T10:00:00+08:00",
			}, nil
		},
		out: out,
	}

	err := runIssueUpdateState(context.Background(), "I123", "closed", env)
	if err != nil {
		t.Fatalf("runIssueUpdateState 返回错误: %v", err)
	}

	output := out.String()
	wantStrings := []string{"Issue I123", "已关闭", "Test Issue", "CLOSED"}
	for _, want := range wantStrings {
		if !strings.Contains(output, want) {
			t.Errorf("输出缺少 %q\n完整输出:\n%s", want, output)
		}
	}
}

// TestRunIssueUpdateStateReopen 验证成功重新打开 Issue。
func TestRunIssueUpdateStateReopen(t *testing.T) {
	out := &bytes.Buffer{}
	env := issueStateEnv{
		git: &fakeGitRunner{remoteURL: "https://gitee.com/owner/repo.git"},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		updateIssueState: func(ctx context.Context, host, token, owner, repo, number, state string) (*api.Issue, error) {
			if state != "open" {
				t.Errorf("state = %q, 期望 open", state)
			}
			return &api.Issue{
				Number:    "I456",
				Title:     "Reopened Issue",
				State:     "open",
				HTMLURL:   "https://gitee.com/owner/repo/issues/I456",
				User:      api.User{Login: "testuser"},
				CreatedAt: "2024-01-01T10:00:00+08:00",
				UpdatedAt: "2024-01-02T10:00:00+08:00",
			}, nil
		},
		out: out,
	}

	err := runIssueUpdateState(context.Background(), "I456", "open", env)
	if err != nil {
		t.Fatalf("runIssueUpdateState 返回错误: %v", err)
	}

	output := out.String()
	wantStrings := []string{"Issue I456", "重新打开", "Reopened Issue", "OPEN"}
	for _, want := range wantStrings {
		if !strings.Contains(output, want) {
			t.Errorf("输出缺少 %q\n完整输出:\n%s", want, output)
		}
	}
}

// TestRunIssueUpdateStateAPIError 验证 API 错误向上传播。
func TestRunIssueUpdateStateAPIError(t *testing.T) {
	env := issueStateEnv{
		git: &fakeGitRunner{remoteURL: "https://gitee.com/owner/repo.git"},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		updateIssueState: func(ctx context.Context, host, token, owner, repo, number, state string) (*api.Issue, error) {
			return nil, errors.New("API 错误")
		},
		out: &bytes.Buffer{},
	}

	err := runIssueUpdateState(context.Background(), "I123", "closed", env)
	if err == nil {
		t.Fatal("期望错误但没有返回")
	}
	if !strings.Contains(err.Error(), "更新 Issue 状态失败") {
		t.Errorf("期望错误消息包含 '更新 Issue 状态失败'，实际: %v", err)
	}
}

// TestRunIssueUpdateStateGetRepoError 验证获取仓库信息失败时的错误处理。
func TestRunIssueUpdateStateGetRepoError(t *testing.T) {
	env := issueStateEnv{
		git: &fakeGitRunner{remoteURL: ""}, // 返回错误
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		out: &bytes.Buffer{},
	}

	err := runIssueUpdateState(context.Background(), "I123", "closed", env)
	if err == nil || !strings.Contains(err.Error(), "获取仓库信息失败") {
		t.Errorf("期望仓库信息错误，实际: %v", err)
	}
}

// TestRunIssueUpdateStateEmptyNumber 验证空编号时由 API 层校验并报错。
func TestRunIssueUpdateStateEmptyNumber(t *testing.T) {
	env := issueStateEnv{
		git: &fakeGitRunner{remoteURL: "https://gitee.com/owner/repo.git"},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		updateIssueState: func(ctx context.Context, host, token, owner, repo, number, state string) (*api.Issue, error) {
			// API 层会校验空编号
			if number == "" {
				return nil, errors.New("Issue 编号不能为空")
			}
			return nil, nil
		},
		out: &bytes.Buffer{},
	}

	err := runIssueUpdateState(context.Background(), "", "closed", env)
	if err == nil {
		t.Fatal("期望错误但没有返回")
	}
	if !strings.Contains(err.Error(), "Issue 编号不能为空") {
		t.Errorf("期望错误消息包含 'Issue 编号不能为空'，实际: %v", err)
	}
}
