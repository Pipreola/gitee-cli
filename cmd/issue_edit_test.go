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

// TestRunIssueEditNoAuth 验证未登录时报错。
func TestRunIssueEditNoAuth(t *testing.T) {
	env := issueEditEnv{
		git: &fakeGitRunner{remoteURL: "https://gitee.com/owner/repo.git"},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Token: ""}, nil
		},
		out: &bytes.Buffer{},
	}
	opts := issueEditOptions{number: "I123", titleChanged: true, title: "t"}
	err := runIssueEdit(context.Background(), opts, env)
	if err == nil || !strings.Contains(err.Error(), "未登录") {
		t.Errorf("期望未登录错误，实际: %v", err)
	}
}

// TestRunIssueEditNoFieldsSpecified 验证未指定任何字段时报错。
func TestRunIssueEditNoFieldsSpecified(t *testing.T) {
	env := issueEditEnv{
		git: &fakeGitRunner{remoteURL: "https://gitee.com/owner/repo.git"},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		out: &bytes.Buffer{},
	}
	opts := issueEditOptions{number: "I123"} // 没有任何 *Changed 标志
	err := runIssueEdit(context.Background(), opts, env)
	if err == nil || !strings.Contains(err.Error(), "至少需要指定一个待修改字段") {
		t.Errorf("期望字段校验错误，实际: %v", err)
	}
}

// TestRunIssueEditTitleOnly 验证仅修改标题的成功场景。
func TestRunIssueEditTitleOnly(t *testing.T) {
	out := &bytes.Buffer{}
	env := issueEditEnv{
		git: &fakeGitRunner{remoteURL: "https://gitee.com/owner/repo.git"},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		editIssue: func(ctx context.Context, host, token, owner, repo, number string, input *api.EditIssueInput) (*api.Issue, error) {
			if number != "I123" {
				t.Errorf("number = %q, 期望 I123", number)
			}
			if input.Title == nil || *input.Title != "新标题" {
				t.Errorf("Title = %v, 期望指针指向 '新标题'", input.Title)
			}
			if input.Body != nil {
				t.Errorf("Body 应为 nil（未指定），实际: %v", input.Body)
			}
			if input.Assignee != nil {
				t.Errorf("Assignee 应为 nil（未指定），实际: %v", input.Assignee)
			}
			return &api.Issue{
				Number:  "I123",
				Title:   "新标题",
				HTMLURL: "https://gitee.com/owner/repo/issues/I123",
				State:   "open",
			}, nil
		},
		out: out,
	}
	opts := issueEditOptions{number: "I123", titleChanged: true, title: "新标题"}
	err := runIssueEdit(context.Background(), opts, env)
	if err != nil {
		t.Fatalf("runIssueEdit 返回错误: %v", err)
	}
	output := out.String()
	wantStrings := []string{"Issue I123", "已更新", "新标题"}
	for _, want := range wantStrings {
		if !strings.Contains(output, want) {
			t.Errorf("输出缺少 %q\n完整输出:\n%s", want, output)
		}
	}
}

// TestRunIssueEditMultipleFields 验证同时修改多个字段。
func TestRunIssueEditMultipleFields(t *testing.T) {
	out := &bytes.Buffer{}
	env := issueEditEnv{
		git: &fakeGitRunner{remoteURL: "https://gitee.com/owner/repo.git"},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		editIssue: func(ctx context.Context, host, token, owner, repo, number string, input *api.EditIssueInput) (*api.Issue, error) {
			if input.Title == nil || *input.Title != "t" {
				t.Errorf("Title = %v, 期望 t", input.Title)
			}
			if input.Body == nil || *input.Body != "b" {
				t.Errorf("Body = %v, 期望 b", input.Body)
			}
			if input.Assignee == nil || *input.Assignee != "user1" {
				t.Errorf("Assignee = %v, 期望 user1", input.Assignee)
			}
			if input.Labels != nil {
				t.Errorf("Labels 应为 nil（未指定），实际: %v", input.Labels)
			}
			return &api.Issue{Number: "I456", Title: "t", HTMLURL: "u", State: "open"}, nil
		},
		out: out,
	}
	opts := issueEditOptions{
		number:          "I456",
		titleChanged:    true,
		title:           "t",
		bodyChanged:     true,
		body:            "b",
		assigneeChanged: true,
		assignee:        "user1",
	}
	err := runIssueEdit(context.Background(), opts, env)
	if err != nil {
		t.Fatalf("runIssueEdit 返回错误: %v", err)
	}
}

// TestRunIssueEditClearAssignee 验证传空字符串可清空指派人（区别于不修改）。
func TestRunIssueEditClearAssignee(t *testing.T) {
	out := &bytes.Buffer{}
	env := issueEditEnv{
		git: &fakeGitRunner{remoteURL: "https://gitee.com/owner/repo.git"},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		editIssue: func(ctx context.Context, host, token, owner, repo, number string, input *api.EditIssueInput) (*api.Issue, error) {
			if input.Assignee == nil {
				t.Errorf("Assignee 应为非 nil 指针（清空意图）")
			}
			if input.Assignee != nil && *input.Assignee != "" {
				t.Errorf("Assignee = %q, 期望空字符串", *input.Assignee)
			}
			return &api.Issue{Number: "I1", Title: "t", HTMLURL: "u", State: "open"}, nil
		},
		out: out,
	}
	opts := issueEditOptions{number: "I1", assigneeChanged: true, assignee: ""}
	err := runIssueEdit(context.Background(), opts, env)
	if err != nil {
		t.Fatalf("runIssueEdit 返回错误: %v", err)
	}
}

// TestRunIssueEditAPIError 验证 API 错误向上传播。
func TestRunIssueEditAPIError(t *testing.T) {
	env := issueEditEnv{
		git: &fakeGitRunner{remoteURL: "https://gitee.com/owner/repo.git"},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		editIssue: func(ctx context.Context, host, token, owner, repo, number string, input *api.EditIssueInput) (*api.Issue, error) {
			return nil, errors.New("404 Not Found")
		},
		out: &bytes.Buffer{},
	}
	opts := issueEditOptions{number: "I1", titleChanged: true, title: "t"}
	err := runIssueEdit(context.Background(), opts, env)
	if err == nil {
		t.Fatal("期望错误但没有返回")
	}
	if !strings.Contains(err.Error(), "编辑 Issue 失败") {
		t.Errorf("期望错误消息包含 '编辑 Issue 失败'，实际: %v", err)
	}
}

// TestRunIssueEditGetRepoError 验证获取仓库信息失败时的错误处理。
func TestRunIssueEditGetRepoError(t *testing.T) {
	env := issueEditEnv{
		git: &fakeGitRunner{remoteURL: ""}, // 触发获取仓库错误
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		out: &bytes.Buffer{},
	}
	opts := issueEditOptions{number: "I1", titleChanged: true, title: "t"}
	err := runIssueEdit(context.Background(), opts, env)
	if err == nil || !strings.Contains(err.Error(), "获取仓库信息失败") {
		t.Errorf("期望仓库信息错误，实际: %v", err)
	}
}
