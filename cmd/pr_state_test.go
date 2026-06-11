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

// TestRunPRUpdateStateNoAuth 验证未登录时报错。
func TestRunPRUpdateStateNoAuth(t *testing.T) {
	env := prStateEnv{
		git: &fakeGitRunner{remoteURL: "https://gitee.com/owner/repo.git"},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Token: ""}, nil
		},
		out: &bytes.Buffer{},
	}
	err := runPRUpdateState(context.Background(), 123, "closed", env)
	if err == nil || !strings.Contains(err.Error(), "未登录") {
		t.Errorf("期望未登录错误，实际: %v", err)
	}
}

// TestRunPRUpdateStateSuccess 验证成功关闭 PR。
func TestRunPRUpdateStateSuccess(t *testing.T) {
	out := &bytes.Buffer{}
	env := prStateEnv{
		git: &fakeGitRunner{remoteURL: "https://gitee.com/owner/repo.git"},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		updatePRState: func(ctx context.Context, host, token, owner, repo string, number int64, state string) (*api.PullRequest, error) {
			if number != 123 {
				t.Errorf("number = %d, 期望 123", number)
			}
			if state != "closed" {
				t.Errorf("state = %q, 期望 closed", state)
			}
			return &api.PullRequest{
				Number:  123,
				Title:   "Test PR",
				State:   "closed",
				HTMLURL: "https://gitee.com/owner/repo/pulls/123",
			}, nil
		},
		out: out,
	}

	err := runPRUpdateState(context.Background(), 123, "closed", env)
	if err != nil {
		t.Fatalf("runPRUpdateState 返回错误: %v", err)
	}

	output := out.String()
	wantStrings := []string{"PR #123", "已关闭", "Test PR", "CLOSED"}
	for _, want := range wantStrings {
		if !strings.Contains(output, want) {
			t.Errorf("输出缺少 %q\n完整输出:\n%s", want, output)
		}
	}
}

// TestRunPRUpdateStateReopen 验证成功重新打开 PR。
func TestRunPRUpdateStateReopen(t *testing.T) {
	out := &bytes.Buffer{}
	env := prStateEnv{
		git: &fakeGitRunner{remoteURL: "https://gitee.com/owner/repo.git"},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		updatePRState: func(ctx context.Context, host, token, owner, repo string, number int64, state string) (*api.PullRequest, error) {
			if state != "open" {
				t.Errorf("state = %q, 期望 open", state)
			}
			return &api.PullRequest{
				Number:  456,
				Title:   "Reopened PR",
				State:   "open",
				HTMLURL: "https://gitee.com/owner/repo/pulls/456",
			}, nil
		},
		out: out,
	}

	err := runPRUpdateState(context.Background(), 456, "open", env)
	if err != nil {
		t.Fatalf("runPRUpdateState 返回错误: %v", err)
	}

	output := out.String()
	wantStrings := []string{"PR #456", "重新打开", "Reopened PR", "OPEN"}
	for _, want := range wantStrings {
		if !strings.Contains(output, want) {
			t.Errorf("输出缺少 %q\n完整输出:\n%s", want, output)
		}
	}
}

// TestRunPRUpdateStateAPIError 验证 API 错误向上传播。
func TestRunPRUpdateStateAPIError(t *testing.T) {
	env := prStateEnv{
		git: &fakeGitRunner{remoteURL: "https://gitee.com/owner/repo.git"},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		updatePRState: func(ctx context.Context, host, token, owner, repo string, number int64, state string) (*api.PullRequest, error) {
			return nil, errors.New("API 错误")
		},
		out: &bytes.Buffer{},
	}

	err := runPRUpdateState(context.Background(), 123, "closed", env)
	if err == nil {
		t.Fatal("期望错误但没有返回")
	}
	if !strings.Contains(err.Error(), "更新 PR 状态失败") {
		t.Errorf("期望错误消息包含 '更新 PR 状态失败'，实际: %v", err)
	}
}

// TestRunPRUpdateStateGetRepoError 验证获取仓库信息失败时的错误处理。
func TestRunPRUpdateStateGetRepoError(t *testing.T) {
	env := prStateEnv{
		git: &fakeGitRunner{remoteURL: ""}, // 返回错误
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		out: &bytes.Buffer{},
	}

	err := runPRUpdateState(context.Background(), 123, "closed", env)
	if err == nil || !strings.Contains(err.Error(), "获取仓库信息失败") {
		t.Errorf("期望仓库信息错误，实际: %v", err)
	}
}
