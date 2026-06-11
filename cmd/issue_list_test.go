package cmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gitee-cli/pkg/api"
	"gitee-cli/pkg/config"
)

// newIssueListGitFake 返回一个仓库 URL 已配置的 fakeGitRunner。
func newIssueListGitFake() *fakeGitRunner {
	return &fakeGitRunner{
		remoteURL: "https://gitee.com/testowner/testrepo.git",
	}
}

func TestIssueListCmd_Basic(t *testing.T) {
	tests := []struct {
		name        string
		opts        issueListOptions
		issues      []api.Issue
		wantErr     bool
		errContains string
		wantOutput  []string
	}{
		{
			name: "空列表",
			opts: issueListOptions{state: "open", limit: 30, direction: "desc", sort: "created"},
			issues: []api.Issue{},
			wantOutput: []string{"没有匹配的 Issue"},
		},
		{
			name: "单条Issue",
			opts: issueListOptions{state: "open", limit: 30, direction: "desc", sort: "created"},
			issues: []api.Issue{
				{
					Number:    "I1234",
					Title:     "测试Issue",
					State:     "open",
					User:      api.User{Login: "alice"},
					CreatedAt: "2024-01-01T10:00:00+08:00",
					UpdatedAt: "2024-01-02T10:00:00+08:00",
				},
			},
			wantOutput: []string{"Showing 1 issue", "I1234", "测试Issue", "OPEN", "alice"},
		},
		{
			name: "多条Issue",
			opts: issueListOptions{state: "all", limit: 30, direction: "desc", sort: "created"},
			issues: []api.Issue{
				{
					Number:    "I1",
					Title:     "开放Issue",
					State:     "open",
					User:      api.User{Login: "bob"},
					CreatedAt: "2024-01-01T10:00:00+08:00",
					UpdatedAt: "2024-01-01T10:00:00+08:00",
				},
				{
					Number:    "I2",
					Title:     "已关闭Issue",
					State:     "closed",
					User:      api.User{Login: "charlie"},
					CreatedAt: "2024-01-02T10:00:00+08:00",
					UpdatedAt: "2024-01-02T10:00:00+08:00",
				},
			},
			wantOutput: []string{"Showing 2 issues", "I1", "开放Issue", "I2", "已关闭Issue"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			env := issueListEnv{
				git: newIssueListGitFake(),
				loadConfig: func() (*config.Config, error) {
					return &config.Config{Token: "fake-token", User: "testuser"}, nil
				},
				listIssues: func(ctx context.Context, host, token, owner, repo string, input *api.ListIssuesInput) ([]api.Issue, error) {
					return tt.issues, nil
				},
				out:   &buf,
				isTTY: func() bool { return false },
				now:   func() time.Time { return time.Date(2024, 1, 10, 12, 0, 0, 0, time.UTC) },
			}

			err := runIssueList(context.Background(), tt.opts, env)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("期望错误但没有返回")
				}
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("期望错误包含 %q，实际: %v", tt.errContains, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("意外错误: %v", err)
			}

			output := buf.String()
			for _, want := range tt.wantOutput {
				if !strings.Contains(output, want) {
					t.Errorf("输出缺少 %q\n完整输出:\n%s", want, output)
				}
			}
		})
	}
}

func TestIssueListCmd_AuthorFilter(t *testing.T) {
	issues := []api.Issue{
		{Number: "I1", Title: "Alice的Issue", State: "open", User: api.User{Login: "alice"}, CreatedAt: "2024-01-01T10:00:00+08:00"},
		{Number: "I2", Title: "Bob的Issue", State: "open", User: api.User{Login: "bob"}, CreatedAt: "2024-01-02T10:00:00+08:00"},
	}

	var buf bytes.Buffer
	env := issueListEnv{
		git: newIssueListGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Token: "fake-token", User: "testuser"}, nil
		},
		listIssues: func(ctx context.Context, host, token, owner, repo string, input *api.ListIssuesInput) ([]api.Issue, error) {
			return issues, nil
		},
		out:   &buf,
		isTTY: func() bool { return false },
		now:   func() time.Time { return time.Date(2024, 1, 10, 12, 0, 0, 0, time.UTC) },
	}

	opts := issueListOptions{state: "open", limit: 30, direction: "desc", sort: "created", author: "alice"}
	err := runIssueList(context.Background(), opts, env)
	if err != nil {
		t.Fatalf("意外错误: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "I1") || !strings.Contains(output, "Alice的Issue") {
		t.Errorf("输出应包含 Alice 的 Issue")
	}
	if strings.Contains(output, "I2") || strings.Contains(output, "Bob的Issue") {
		t.Errorf("输出不应包含 Bob 的 Issue")
	}
}

func TestIssueListCmd_AssigneeFilter(t *testing.T) {
	issues := []api.Issue{
		{Number: "I1", Title: "指派给Alice", State: "open", User: api.User{Login: "bob"}, Assignee: &api.User{Login: "alice"}, CreatedAt: "2024-01-01T10:00:00+08:00"},
		{Number: "I2", Title: "未指派", State: "open", User: api.User{Login: "charlie"}, CreatedAt: "2024-01-02T10:00:00+08:00"},
	}

	var buf bytes.Buffer
	env := issueListEnv{
		git: newIssueListGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Token: "fake-token", User: "testuser"}, nil
		},
		listIssues: func(ctx context.Context, host, token, owner, repo string, input *api.ListIssuesInput) ([]api.Issue, error) {
			return issues, nil
		},
		out:   &buf,
		isTTY: func() bool { return false },
		now:   func() time.Time { return time.Date(2024, 1, 10, 12, 0, 0, 0, time.UTC) },
	}

	opts := issueListOptions{state: "open", limit: 30, direction: "desc", sort: "created", assignee: "alice"}
	err := runIssueList(context.Background(), opts, env)
	if err != nil {
		t.Fatalf("意外错误: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "I1") {
		t.Errorf("输出应包含指派给 alice 的 Issue")
	}
	if strings.Contains(output, "I2") {
		t.Errorf("输出不应包含未指派的 Issue")
	}
}

func TestIssueListCmd_ValidationErrors(t *testing.T) {
	tests := []struct {
		name        string
		opts        issueListOptions
		errContains string
	}{
		{
			name:        "无效state",
			opts:        issueListOptions{state: "invalid", limit: 30, direction: "desc", sort: "created"},
			errContains: "无效的 --state 值",
		},
		{
			name:        "无效direction",
			opts:        issueListOptions{state: "open", limit: 30, direction: "invalid", sort: "created"},
			errContains: "无效的 --direction 值",
		},
		{
			name:        "无效sort",
			opts:        issueListOptions{state: "open", limit: 30, direction: "desc", sort: "invalid"},
			errContains: "无效的 --sort 值",
		},
		{
			name:        "limit太小",
			opts:        issueListOptions{state: "open", limit: 0, direction: "desc", sort: "created"},
			errContains: "无效的 --limit 值",
		},
		{
			name:        "limit太大",
			opts:        issueListOptions{state: "open", limit: 101, direction: "desc", sort: "created"},
			errContains: "无效的 --limit 值",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := issueListEnv{
				git: newIssueListGitFake(),
				loadConfig: func() (*config.Config, error) {
					return &config.Config{Token: "fake-token", User: "testuser"}, nil
				},
				listIssues: func(ctx context.Context, host, token, owner, repo string, input *api.ListIssuesInput) ([]api.Issue, error) {
					return nil, nil
				},
				out:   &bytes.Buffer{},
				isTTY: func() bool { return false },
				now:   func() time.Time { return time.Now() },
			}

			err := runIssueList(context.Background(), tt.opts, env)
			if err == nil {
				t.Fatalf("期望错误但没有返回")
			}
			if !strings.Contains(err.Error(), tt.errContains) {
				t.Fatalf("期望错误包含 %q，实际: %v", tt.errContains, err)
			}
		})
	}
}

func TestIssueListCmd_APIError(t *testing.T) {
	env := issueListEnv{
		git: newIssueListGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Token: "fake-token", User: "testuser"}, nil
		},
		listIssues: func(ctx context.Context, host, token, owner, repo string, input *api.ListIssuesInput) ([]api.Issue, error) {
			return nil, errors.New("网络错误")
		},
		out:   &bytes.Buffer{},
		isTTY: func() bool { return false },
		now:   func() time.Time { return time.Now() },
	}

	opts := issueListOptions{state: "open", limit: 30, direction: "desc", sort: "created"}
	err := runIssueList(context.Background(), opts, env)
	if err == nil {
		t.Fatalf("期望错误但没有返回")
	}
	if !strings.Contains(err.Error(), "查询 Issue 列表失败") {
		t.Fatalf("期望错误消息包含 '查询 Issue 列表失败'，实际: %v", err)
	}
}

func TestIssueListCmd_JSONOutput(t *testing.T) {
	issues := []api.Issue{
		{
			Number:    "I123",
			Title:     "测试Issue",
			State:     "open",
			User:      api.User{Login: "alice"},
			CreatedAt: "2024-01-01T10:00:00+08:00",
			UpdatedAt: "2024-01-01T10:00:00+08:00",
		},
	}

	var buf bytes.Buffer
	env := issueListEnv{
		git: newIssueListGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Token: "fake-token", User: "testuser"}, nil
		},
		listIssues: func(ctx context.Context, host, token, owner, repo string, input *api.ListIssuesInput) ([]api.Issue, error) {
			return issues, nil
		},
		out:   &buf,
		isTTY: func() bool { return false },
		now:   func() time.Time { return time.Date(2024, 1, 10, 12, 0, 0, 0, time.UTC) },
	}

	opts := issueListOptions{state: "open", limit: 30, direction: "desc", sort: "created", jsonOut: true}
	err := runIssueList(context.Background(), opts, env)
	if err != nil {
		t.Fatalf("意外错误: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, `"number": "I123"`) {
		t.Errorf("JSON 输出应包含 Issue 编号")
	}
	if !strings.Contains(output, `"title": "测试Issue"`) {
		t.Errorf("JSON 输出应包含 Issue 标题")
	}
}
